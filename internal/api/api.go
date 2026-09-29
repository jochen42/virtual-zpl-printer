// Package api serves the print list, images and raw ZPL over HTTP. The same
// handler backs the desktop webview (via the Wails asset server) and the
// headless browser UI.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/jochen42/virtual-zpl-printer/internal/store"
)

// Status describes the printer listener, shown in the UI header.
type Status struct {
	PrinterAddr string   `json:"printerAddr"`
	DPI         int      `json:"dpi"`
	DataDir     string   `json:"dataDir"`
	Stored      []string `json:"stored"`
	Error       string   `json:"error,omitempty"`
}

type Handler struct {
	// Status is called for GET /api/status.
	Status func() Status
	// SaveFile, if set, lets the user save data to a file of their choice
	// (the desktop app's save dialog). It reports false if they cancelled.
	// Browsers download GET /api/prints/{id}/pdf instead.
	SaveFile func(name string, data []byte) (bool, error)
	// OpenFolder, if set, shows a directory in the OS file manager.
	OpenFolder func(dir string) error

	store *store.Store
	mux   *http.ServeMux

	mu          sync.Mutex
	subscribers map[chan struct{}]struct{}
}

func New(s *store.Store) *Handler {
	h := &Handler{store: s, mux: http.NewServeMux(), subscribers: map[chan struct{}]struct{}{}}
	h.mux.HandleFunc("GET /api/status", h.status)
	h.mux.HandleFunc("GET /api/prints", h.list)
	h.mux.HandleFunc("GET /api/prints/{id}/zpl", h.zpl)
	h.mux.HandleFunc("GET /api/prints/{id}/images/{name}", h.image)
	h.mux.HandleFunc("GET /api/prints/{id}/pdf", h.pdf)
	h.mux.HandleFunc("POST /api/prints/{id}/pdf/save", h.savePDF)
	h.mux.HandleFunc("POST /api/prints/{id}/open-folder", h.openFolder)
	h.mux.HandleFunc("DELETE /api/prints/{id}", h.delete)
	h.mux.HandleFunc("GET /api/events", h.events)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.mux.ServeHTTP(w, r) }

// Notify tells connected event-stream clients that the print list changed.
func (h *Handler) Notify() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (h *Handler) status(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.Status())
}

func (h *Handler) list(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.store.List())
}

func (h *Handler) zpl(w http.ResponseWriter, r *http.Request) {
	data, err := h.store.ZPL(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(data)
}

func (h *Handler) image(w http.ResponseWriter, r *http.Request) {
	path, err := h.store.ImagePath(r.PathValue("id"), r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Cache-Control", "max-age=31536000, immutable")
	http.ServeFile(w, r, path)
}

func (h *Handler) pdf(w http.ResponseWriter, r *http.Request) {
	name, data, err := h.pdfFile(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	_, _ = w.Write(data)
}

func (h *Handler) savePDF(w http.ResponseWriter, r *http.Request) {
	if h.SaveFile == nil {
		http.Error(w, "saving is only available in the desktop app", http.StatusNotImplemented)
		return
	}
	name, data, err := h.pdfFile(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	saved, err := h.SaveFile(name, data)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"saved": saved})
}

func (h *Handler) openFolder(w http.ResponseWriter, r *http.Request) {
	if h.OpenFolder == nil {
		http.Error(w, "opening folders is not supported", http.StatusNotImplemented)
		return
	}
	dir, err := h.store.PrintDir(r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := h.OpenFolder(dir); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// pdfFile returns the print's PDF and a file name for it.
func (h *Handler) pdfFile(id string) (name string, data []byte, err error) {
	data, err = h.store.PDF(id)
	if err != nil {
		return "", nil, err
	}
	return "label-" + strings.ReplaceAll(id, ".", "-") + ".pdf", data, nil
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Delete(r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	h.Notify()
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusNotImplemented)
		return
	}
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.subscribers, ch)
		h.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			_, _ = w.Write([]byte("data: changed\n\n"))
			flusher.Flush()
		}
	}
}

func writeErr(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
