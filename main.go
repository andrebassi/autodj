//go:build desktop

// Command autodj-desktop wraps the exact same AutoDJ backend + web UI in a native
// window (Wails v2). It reuses cmd/serverkit.Build — no duplicated wiring, no second
// backend — serving that handler on a loopback listener, and lets the Wails window
// reverse-proxy every request (static UI + /api + /ws) to it, so the app runs on a
// single same-origin surface. The whole Wails surface (this file) sits behind the
// `desktop` build tag: the default build, `go test ./...` and the coverage gate
// never compile it, so no native webkit dependency leaks into the unit/CI path.
//
// This main lives at the module root because `wails build` compiles the root
// package. Build a native macOS .app bundle with:
//
//	wails build -tags desktop        # → build/bin/autodj.app
//	wails dev -tags desktop          # hot reload
//
// or a raw binary (must still be launched inside the .app on macOS to show a window):
//
//	cd web && pnpm build
//	go build -tags desktop -o bin/autodj-desktop .
//
// Passing --browser starts the same backend headless (no native window) on a
// localhost port and opens the default browser at it — a "stealth" mode where the
// browser is the UI and the process just serves in the background until Ctrl-C:
//
//	./build/bin/autodj.app/Contents/MacOS/autodj-desktop --browser
//	task desktop:browser
package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"autodj/cmd/serverkit"
)

// webOut is the exported Next.js UI, embedded so the .app bundle is self-contained:
// the bundle's working directory is not the project root, so serverkit could not
// find web/out on disk. Only the desktop build embeds it (this file is behind the
// `desktop` tag) — the default/server build keeps serving web/out from disk.
//
//go:embed all:web/out
var webOut embed.FS

func main() {
	// --browser: run headless (no Wails window), serve on localhost, open the
	// default browser at it. --port sets the localhost port (0 = OS-chosen).
	browser := flag.Bool("browser", false, "run headless and open the UI in the default browser instead of a native window")
	port := flag.String("port", envOr("AUTODJ_PORT", "8095"), "localhost port for --browser mode (0 = OS-chosen)")
	flag.Parse()

	// ctx cancels on shutdown, stopping serverkit's background loops.
	ctx, cancel := context.WithCancel(context.Background())

	// Serve the embedded UI instead of reading web/out from disk.
	uiFS, err := fs.Sub(webOut, "web/out")
	if err != nil {
		log.Fatalf("embed web/out: %v", err)
	}
	serverkit.StaticFS = uiFS

	handler, cleanup, err := serverkit.Build(ctx)
	if err != nil {
		log.Fatalf("build: %v", err)
	}

	if *browser {
		runBrowser(handler, *port, cancel, cleanup)
		return
	}
	runWindow(handler, cancel, cleanup)
}

// runWindow opens the native Wails window. The backend runs on an OS-chosen
// loopback port and the window reverse-proxies every request to it (static UI,
// /api and /ws) — one same-origin surface, same wiring as the web server.
func runWindow(handler http.Handler, cancel context.CancelFunc, cleanup func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: handler}
	go serve(srv, ln)

	target := &url.URL{Scheme: "http", Host: ln.Addr().String()}
	proxy := httputil.NewSingleHostReverseProxy(target)

	err = wails.Run(&options.App{
		Title:  "AutoDJ",
		Width:  1600,
		Height: 1000,
		AssetServer: &assetserver.Options{
			Handler: proxy,
		},
		OnShutdown: func(context.Context) {
			cancel()
			_ = srv.Close()
			cleanup()
		},
	})
	if err != nil {
		log.Fatalf("wails: %v", err)
	}
}

// runBrowser is the headless/stealth mode: no native window. It binds the backend
// to 127.0.0.1:<port> (falling back to an OS-chosen port if that one is taken),
// opens the default browser at the URL, and serves quietly until interrupted.
func runBrowser(handler http.Handler, port string, cancel context.CancelFunc, cleanup func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		// Preferred port busy — let the OS pick a free one so the app still opens.
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			log.Fatalf("listen: %v", err)
		}
	}
	srv := &http.Server{Handler: handler}
	go serve(srv, ln)

	uiURL := "http://" + ln.Addr().String()
	log.Printf("AutoDJ headless em %s (Ctrl-C encerra)", uiURL)
	if err := openBrowser(uiURL); err != nil {
		log.Printf("abra manualmente: %s (%v)", uiURL, err)
	}

	// Block until the process is asked to stop, then tear the backend down.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	cancel()
	_ = srv.Close()
	cleanup()
}

func serve(srv *http.Server, ln net.Listener) {
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Printf("serve: %v", err)
	}
}

// openBrowser launches the default browser at url without blocking.
func openBrowser(u string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", u).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	default:
		return exec.Command("xdg-open", u).Start()
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
