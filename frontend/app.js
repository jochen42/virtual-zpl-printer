const $ = (id) => document.getElementById(id);

// Font downloads carry raw binary; don't dump it all into the drawer.
const MAX_ZPL_PREVIEW = 20000;

const state = {
  prints: [],
  selectedId: null,
  knownIds: null,
  actualSize: false,
  zpl: "",
  dpi: 203,
  pdf: null, // PDF.js document of the selected print
};

const dateFmt = new Intl.DateTimeFormat(undefined, {
  year: "numeric", month: "2-digit", day: "2-digit",
  hour: "2-digit", minute: "2-digit", second: "2-digit",
});

async function api(path, options) {
  const res = await fetch(path, options);
  if (!res.ok) throw new Error(`${res.status} ${await res.text()}`);
  return res;
}

function formatBytes(n) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

function imageUrl(p, name) {
  return `/api/prints/${encodeURIComponent(p.id)}/images/${encodeURIComponent(name)}`;
}

async function loadStatus() {
  const s = await (await api("/api/status")).json();
  state.dpi = s.dpi;
  const el = $("status");
  el.classList.toggle("error", !!s.error);
  el.innerHTML = "";
  const dot = document.createElement("span");
  dot.className = "dot";
  const fonts = s.stored?.length ? ` · ${s.stored.length} stored object${s.stored.length === 1 ? "" : "s"}` : "";
  el.append(dot, s.error ? `Printer offline: ${s.error}` : `Listening on ${s.printerAddr} · ${s.dpi} dpi${fonts}`);
  el.title = [`Data in ${s.dataDir}`, ...(s.stored || [])].join("\n");
  const port = s.printerAddr.split(":").pop();
  $("example").textContent = `printf '^XA^FO50,50^A0N,50,50^FDHello^FS^XZ' | nc localhost ${port}`;
}

async function loadPrints() {
  loadStatus();
  const prints = await (await api("/api/prints")).json();
  const fresh = state.knownIds ? new Set(prints.filter((p) => !state.knownIds.has(p.id)).map((p) => p.id)) : new Set();
  state.knownIds = new Set(prints.map((p) => p.id));
  state.prints = prints;
  if (state.selectedId && !state.knownIds.has(state.selectedId)) closeDrawer();
  renderRows(fresh);
}

function renderRows(fresh) {
  const tbody = $("rows");
  tbody.replaceChildren(...state.prints.map((p) => {
    const tr = document.createElement("tr");
    tr.dataset.id = p.id;
    if (p.id === state.selectedId) tr.classList.add("selected");
    if (fresh.has(p.id)) tr.classList.add("new");

    const cells = [
      [dateFmt.format(new Date(p.receivedAt)), "time"],
      [p.remoteAddr, ""],
      [formatBytes(p.bytes), "num"],
      [String(p.images.length), "num"],
    ];
    for (const [text, cls] of cells) {
      const td = document.createElement("td");
      td.textContent = text;
      if (cls) td.className = cls;
      tr.append(td);
    }
    const status = document.createElement("td");
    const badge = document.createElement("span");
    badge.className = `badge ${p.error ? "err" : "ok"}`;
    badge.textContent = statusText(p);
    badge.title = [p.error, ...(p.stored || []).map((n) => `Stored ${n}`)].filter(Boolean).join("\n");
    status.append(badge);
    tr.append(status);

    tr.addEventListener("click", () => select(p.id));
    return tr;
  }));
  $("empty").hidden = state.prints.length > 0;
}

function statusText(p) {
  if (p.error) return p.images.length ? "Partial" : "Error";
  if (!p.images.length && p.stored?.length) return `Stored ${p.stored.join(", ")}`;
  return "Rendered";
}

async function select(id) {
  state.selectedId = id;
  for (const tr of $("rows").children) tr.classList.toggle("selected", tr.dataset.id === id);
  const p = state.prints.find((x) => x.id === id);
  if (!p) return;

  $("d-title").textContent = dateFmt.format(new Date(p.receivedAt));
  $("d-meta").textContent = `${p.remoteAddr} · ${formatBytes(p.bytes)} · ${p.images.length} label${p.images.length === 1 ? "" : "s"}`;
  $("d-error").hidden = !p.error;
  $("d-error").textContent = p.error || "";
  $("d-pdf").disabled = !p.pdf;
  $("d-stored").hidden = !p.stored?.length;
  $("d-stored").textContent = p.stored?.length ? `Stored on printer: ${p.stored.join(", ")}` : "";

  showLabels(p);

  $("drawer").classList.add("open");
  $("drawer").setAttribute("aria-hidden", "false");

  $("d-zpl").textContent = "";
  state.zpl = "";
  const zpl = await (await api(`/api/prints/${encodeURIComponent(id)}/zpl`)).text();
  if (state.selectedId === id) {
    state.zpl = zpl;
    $("d-zpl").textContent = zpl.length > MAX_ZPL_PREVIEW
      ? `${zpl.slice(0, MAX_ZPL_PREVIEW)}\n… ${zpl.length - MAX_ZPL_PREVIEW} more characters`
      : zpl;
  }
}

function labelFigure(i, count, child) {
  const fig = document.createElement("figure");
  if (count > 1) {
    const cap = document.createElement("figcaption");
    cap.textContent = `Label ${i + 1}`;
    fig.append(cap);
  }
  fig.append(child);
  return fig;
}

function showImages(p) {
  $("d-images").replaceChildren(...p.images.map((name, i) => {
    const img = document.createElement("img");
    img.src = imageUrl(p, name);
    img.alt = `Label ${i + 1}`;
    return labelFigure(i, p.images.length, img);
  }));
}

// Shows the print's PDF, which stays sharp at any zoom. Prints from older
// versions have no PDF and show the PNGs.
async function showLabels(p) {
  closePdf();
  if (!p.pdf) {
    showImages(p);
    return;
  }
  $("d-images").replaceChildren();
  try {
    const lib = await loadPdfjs();
    const doc = await lib.getDocument({ url: `/api/prints/${encodeURIComponent(p.id)}/pdf` }).promise;
    if (state.selectedId !== p.id) {
      doc.destroy();
      return;
    }
    state.pdf = doc;
    const canvases = [];
    for (let i = 0; i < doc.numPages; i++) {
      const canvas = document.createElement("canvas");
      canvas.setAttribute("role", "img");
      canvas.setAttribute("aria-label", `Label ${i + 1}`);
      canvases.push(canvas);
    }
    $("d-images").replaceChildren(...canvases.map((c, i) => labelFigure(i, canvases.length, c)));
    await renderPdf();
  } catch (err) {
    console.error("PDF preview failed, showing PNGs", err);
    if (state.selectedId === p.id) {
      closePdf();
      showImages(p);
    }
  }
}

let pdfjs;
async function loadPdfjs() {
  if (!pdfjs) {
    pdfjs = await import("./vendor/pdfjs/pdf.min.mjs");
    pdfjs.GlobalWorkerOptions.workerSrc = new URL("vendor/pdfjs/pdf.worker.min.mjs", document.baseURI).href;
  }
  return pdfjs;
}

function closePdf() {
  state.pdf?.destroy();
  state.pdf = null;
}

// Renders every page at its on-screen size times the device pixel ratio.
// Pages are sized like the PNGs were: one dot per CSS pixel, shrunk to fit
// unless "Actual size" is on.
let renderTasks = [];
async function renderPdf() {
  const doc = state.pdf;
  if (!doc) return;
  for (const t of renderTasks) t.cancel();
  renderTasks = [];
  const box = $("d-images");
  const available = box.clientWidth - parseFloat(getComputedStyle(box).paddingLeft) * 2 - 2;
  const canvases = box.querySelectorAll("canvas");
  const dpr = window.devicePixelRatio || 1;
  const jobs = [];
  for (let i = 0; i < canvases.length; i++) {
    const page = await doc.getPage(i + 1);
    if (state.pdf !== doc) return;
    const pt = page.getViewport({ scale: 1 });
    const natural = (pt.width * state.dpi) / 72;
    const cssWidth = state.actualSize ? natural : Math.min(natural, available);
    const viewport = page.getViewport({ scale: (cssWidth / pt.width) * dpr });
    const canvas = canvases[i];
    canvas.width = Math.round(viewport.width);
    canvas.height = Math.round(viewport.height);
    canvas.style.width = `${cssWidth}px`;
    canvas.style.height = `${viewport.height / dpr}px`;
    const task = page.render({ canvas, viewport });
    renderTasks.push(task);
    jobs.push(task.promise.catch((err) => {
      if (err?.name !== "RenderingCancelledException") throw err;
    }));
  }
  await Promise.all(jobs);
}

let resizeTimer;
new ResizeObserver(() => {
  clearTimeout(resizeTimer);
  resizeTimer = setTimeout(renderPdf, 100);
}).observe($("d-images"));

function closeDrawer() {
  closePdf();
  state.selectedId = null;
  $("drawer").classList.remove("open");
  $("drawer").setAttribute("aria-hidden", "true");
  for (const tr of $("rows").children) tr.classList.remove("selected");
}

function moveSelection(delta) {
  if (!state.prints.length) return;
  const idx = state.prints.findIndex((p) => p.id === state.selectedId);
  const next = idx < 0 ? 0 : Math.min(Math.max(idx + delta, 0), state.prints.length - 1);
  select(state.prints[next].id);
  $("rows").children[next]?.scrollIntoView({ block: "nearest" });
}

$("d-close").addEventListener("click", closeDrawer);

$("d-zoom").addEventListener("click", () => {
  state.actualSize = !state.actualSize;
  $("d-images").classList.toggle("actual", state.actualSize);
  $("d-zoom").textContent = state.actualSize ? "Fit to width" : "Actual size";
  renderPdf();
});

$("d-copy").addEventListener("click", async () => {
  await navigator.clipboard.writeText(state.zpl);
  $("d-copy").textContent = "Copied";
  setTimeout(() => ($("d-copy").textContent = "Copy ZPL"), 1200);
});

$("d-pdf").addEventListener("click", async () => {
  const id = state.selectedId;
  if (!id) return;
  const url = `/api/prints/${encodeURIComponent(id)}/pdf`;
  // The desktop webview can't download files; the app shows a save dialog.
  if (!window.runtime) {
    const a = document.createElement("a");
    a.href = url;
    a.download = "";
    a.click();
    return;
  }
  const { saved } = await (await api(`${url}/save`, { method: "POST" })).json();
  if (saved) {
    $("d-pdf").textContent = "Saved";
    setTimeout(() => ($("d-pdf").textContent = "Save PDF"), 1200);
  }
});

$("d-folder").addEventListener("click", async () => {
  const id = state.selectedId;
  if (id) await api(`/api/prints/${encodeURIComponent(id)}/open-folder`, { method: "POST" });
});

$("d-delete").addEventListener("click", async () => {
  const id = state.selectedId;
  if (!id) return;
  const idx = state.prints.findIndex((p) => p.id === id);
  await api(`/api/prints/${encodeURIComponent(id)}`, { method: "DELETE" });
  await loadPrints();
  const next = state.prints[Math.min(idx, state.prints.length - 1)];
  if (next) select(next.id); else closeDrawer();
});

document.addEventListener("keydown", (e) => {
  if (e.key === "Escape") closeDrawer();
  else if (e.key === "ArrowDown") { e.preventDefault(); moveSelection(1); }
  else if (e.key === "ArrowUp") { e.preventDefault(); moveSelection(-1); }
});

function subscribe() {
  // Desktop: Wails runtime events. Browser (headless mode): server-sent events.
  if (window.runtime?.EventsOn) {
    window.runtime.EventsOn("prints:changed", loadPrints);
    return;
  }
  const es = new EventSource("/api/events");
  es.onmessage = loadPrints;
  es.onopen = loadPrints;
}

loadPrints();
subscribe();
