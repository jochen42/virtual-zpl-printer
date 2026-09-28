# virtual-zpl-printer

A virtual Zebra label printer. It listens on TCP port 9100 (raw / JetDirect), renders every received ZPL job to PNG with [go-zpl](https://github.com/StirlingMarketingGroup/go-zpl), stores it, and shows all prints in a desktop app (macOS, Windows, Linux via [Wails](https://wails.io)).

## Install

Download the archive for your platform from [Releases](https://github.com/jochen42/virtual-zpl-printer/releases).

- **macOS**: unzip, move `Virtual ZPL Printer.app` to Applications. The app is not notarized; on first start run `xattr -dr com.apple.quarantine "/Applications/Virtual ZPL Printer.app"` (or right-click → Open).
- **Windows**: unzip and start `virtual-zpl-printer.exe`. SmartScreen warns about the unsigned binary: "More info" → "Run anyway". Needs the WebView2 runtime (preinstalled on Windows 10/11).
- **Linux**: extract and run `virtual-zpl-printer`. Needs `libgtk-3` and `libwebkit2gtk-4.1`.

The OS firewall may ask to allow incoming connections; allow it if other machines should print to it.

Send a test label:

```sh
printf '^XA^FO50,50^A0N,50,50^FDHello^FS^XZ' | nc localhost 9100
```

## Add it as a printer

Apps that send ZPL themselves (QZ Tray, label software in "raw" mode) need a printer queue that passes data through unchanged. Use `127.0.0.1` on the same machine, otherwise the IP of the machine running the app.

### macOS

```sh
lpadmin -p Virtual_ZPL -E -v socket://127.0.0.1:9100 -m raw -D "Virtual ZPL Printer"
lp -d Virtual_ZPL -o raw label.zpl   # test
```

CUPS warns that raw queues are deprecated; they still work. In System Settings → Printers & Scanners, the GUI route (Add Printer → IP → protocol "HP Jetdirect – Socket") only offers drivers that convert to PostScript, which this printer cannot render.

### Windows

PowerShell as administrator:

```powershell
Add-PrinterPort -Name "VirtualZPL_9100" -PrinterHostAddress "127.0.0.1" -PortNumber 9100
Add-Printer -Name "Virtual ZPL" -DriverName "Generic / Text Only" -PortName "VirtualZPL_9100"
```

Or via Settings → Printers & scanners → Add device → "Add manually" → "Add a printer using an IP address or hostname" → TCP/IP device, host `127.0.0.1`, untick "Query the printer" → driver "Generic" → "Generic / Text Only".

"Generic / Text Only" passes ZPL through as is. To print ordinary documents (PDF, Word) as labels, install Zebra's ZDesigner driver instead and pick it as the driver; it converts pages to ZPL.

Send a file directly, without a queue:

```powershell
$c = [Net.Sockets.TcpClient]::new("127.0.0.1", 9100); $b = [IO.File]::ReadAllBytes("label.zpl"); $c.GetStream().Write($b, 0, $b.Length); $c.Close()
```

## Flags

| Flag | Default | |
|---|---|---|
| `-addr` | `:9100` | printer listen address |
| `-dpi` | `203` | render resolution (203, 300, 600) |
| `-data` | `<user config dir>/virtual-zpl-printer` | prints and stored printer objects |
| `-headless` | `false` | serve the UI in the browser instead of a window |
| `-ui-addr` | `127.0.0.1:9180` | UI address in headless mode |
| `-version` | | print the version |

## Downloaded fonts

`~DY` (binary, hex, `:B64:`, `:Z64:`) and `~DU` uploads are saved under `<data>/memory/<drive>/<name>`, like the printer's `E:`/`R:` memory, and survive restarts. TrueType/OpenType files are used for `^A@…,E:NAME.TTF` and for letters assigned with `^CW`. An unknown font file falls back to font 0, as on a printer. Characters missing from a downloaded font render as the font's placeholder box, except space characters (e.g. U+202F narrow no-break space from number formatting), which render as a plain space.

go-zpl is vendored in `third_party/go-zpl` for this; see its `PATCHES.md`.

## Storage

Prints live in `<data>/prints`, one directory per job, named by UTC receive time: `job.zpl` (raw bytes), `label-N.png` (one per `^XA…^XZ` block, 1-bit like thermal print output) and `meta.json`. Jobs that fail to render are still saved with the error.

A job ends when the sender closes the connection, or 750 ms after the last complete `^XA…^XZ` block if the connection stays open.

## Development

Requires Go and a C toolchain (CGO) on macOS and Linux; Linux also needs `libgtk-3-dev` and `libwebkit2gtk-4.1-dev`. Windows builds without CGO.

```sh
make run          # build and start the desktop app
make headless     # no window; UI at http://127.0.0.1:9180
make test
make dist         # release archive for this platform in dist/
```

CI runs vet and tests on every push and pull request. Pushing a `v*` tag builds macOS, Windows and Linux for amd64 and arm64 and publishes a GitHub release:

```sh
git tag v0.1.0 && git push origin v0.1.0
```
