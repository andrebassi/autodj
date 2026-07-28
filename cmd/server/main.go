// Command server is the web backend entrypoint. It's thin: the whole hexagonal
// wiring (repo/engine/services/seeds + REST + WebSocket + static UI) lives in
// cmd/serverkit, shared with the desktop app (cmd/desktop). Here we just serve the
// assembled handler on PORT and shut it down cleanly on a signal.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"autodj/cmd/serverkit"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8095"
	}

	handler, cleanup, err := serverkit.Build(ctx)
	if err != nil {
		log.Fatalf("build: %v", err)
	}
	defer cleanup()

	srv := &http.Server{Addr: ":" + port, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("autodj server on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()

	<-ctx.Done()
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
	log.Println("shutdown")
}
