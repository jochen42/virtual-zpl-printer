package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"

	zpl "github.com/StirlingMarketingGroup/go-zpl"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/jochen42/virtual-zpl-printer/internal/api"
	"github.com/jochen42/virtual-zpl-printer/internal/memory"
	"github.com/jochen42/virtual-zpl-printer/internal/printer"
	"github.com/jochen42/virtual-zpl-printer/internal/store"
)

//go:embed all:frontend
var frontendFS embed.FS

// version is set at release build time via -ldflags "-X main.version=…".
var version = "dev"

func main() {
	addr := flag.String("addr", ":9100", "printer listen address")
	dpi := flag.Int("dpi", 203, "printer resolution used for rendering (203, 300 or 600)")
	dataDir := flag.String("data", defaultDataDir(), "directory for prints and stored printer objects (fonts)")
	headless := flag.Bool("headless", false, "run without a window and serve the UI in the browser")
	uiAddr := flag.String("ui-addr", "127.0.0.1:9180", "UI address in headless mode")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	switch zpl.DPI(*dpi) {
	case zpl.DPI203, zpl.DPI300, zpl.DPI600:
	default:
		log.Fatalf("unsupported -dpi %d, use 203, 300 or 600", *dpi)
	}

	st, err := store.Open(filepath.Join(*dataDir, "prints"))
	if err != nil {
		log.Fatalf("opening data dir: %v", err)
	}
	mem, err := memory.Open(filepath.Join(*dataDir, "memory"))
	if err != nil {
		log.Fatalf("opening printer memory: %v", err)
	}

	handler := api.New(st)
	var (
		mu        sync.Mutex
		listenErr error
		wailsCtx  context.Context
	)
	handler.Status = func() api.Status {
		mu.Lock()
		defer mu.Unlock()
		s := api.Status{PrinterAddr: *addr, DPI: *dpi, DataDir: *dataDir, Stored: mem.Names()}
		if listenErr != nil {
			s.Error = listenErr.Error()
		}
		return s
	}

	handler.OpenFolder = openFolder
	if !*headless {
		handler.SaveFile = func(name string, data []byte) (bool, error) {
			mu.Lock()
			c := wailsCtx
			mu.Unlock()
			if c == nil {
				return false, errors.New("window not ready")
			}
			path, err := runtime.SaveFileDialog(c, runtime.SaveDialogOptions{
				DefaultFilename: name,
				Filters:         []runtime.FileFilter{{DisplayName: "PDF", Pattern: "*.pdf"}},
			})
			if err != nil || path == "" {
				return false, err
			}
			return true, os.WriteFile(path, data, 0o644)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	srv := &printer.Server{
		Addr:   *addr,
		DPI:    zpl.DPI(*dpi),
		Store:  st,
		Memory: mem,
		OnPrint: func(store.Print) {
			handler.Notify()
			mu.Lock()
			c := wailsCtx
			mu.Unlock()
			if c != nil {
				runtime.EventsEmit(c, "prints:changed")
			}
		},
	}
	go func() {
		if err := srv.ListenAndServe(ctx); err != nil {
			log.Printf("printer: %v", err)
			mu.Lock()
			listenErr = err
			mu.Unlock()
		}
	}()

	assets, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		log.Fatal(err)
	}

	if *headless {
		runHeadless(ctx, *uiAddr, assets, handler)
		return
	}

	err = wails.Run(&options.App{
		Title:     "Virtual ZPL Printer " + version,
		Width:     1100,
		Height:    720,
		MinWidth:  640,
		MinHeight: 400,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: handler,
		},
		OnStartup: func(c context.Context) {
			mu.Lock()
			wailsCtx = c
			mu.Unlock()
		},
		OnShutdown: func(context.Context) { stop() },
	})
	if err != nil {
		log.Fatal(err)
	}
}

func runHeadless(ctx context.Context, addr string, assets fs.FS, handler http.Handler) {
	mux := http.NewServeMux()
	mux.Handle("/api/", handler)
	mux.Handle("/", http.FileServerFS(assets))
	server := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		server.Close()
	}()
	log.Printf("UI on http://%s", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func defaultDataDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "virtual-zpl-printer")
}
