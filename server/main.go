// Riffle Server: a companion for the Riffle iOS player. It indexes video folders on
// the NAS, streams original files over HTTP with byte-range support, and pre-renders
// trickplay thumbnail sheets so scrubbing previews never wait on the network.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const version = "0.1.0"

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("riffle ")

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	for _, r := range cfg.Roots {
		log.Printf("media root %q -> %s", r.Name, r.Path)
	}
	log.Printf("data dir %s, trickplay hwaccel=%s interval=%gs width=%dpx workers=%d accurate=%v",
		cfg.DataDir, cfg.HWAccel, cfg.Interval, cfg.TileWidth, cfg.Workers, cfg.Accurate)
	if cfg.Token == "" {
		log.Printf("WARNING: RIFFLE_TOKEN is empty, the API is open to anyone on your network")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	lib := NewLibrary(cfg)
	trick := NewTrickplay(cfg, lib)
	lib.onNew = func(it *Item) { trick.Enqueue(it.ID, false) }
	trick.Start(ctx)

	go func() {
		lib.Scan(ctx)
		if cfg.ScanEvery <= 0 {
			return
		}
		t := time.NewTicker(cfg.ScanEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				lib.Scan(ctx)
			}
		}
	}()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           NewServer(cfg, lib, trick).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// No WriteTimeout: a paused player may hold a range response open for a long time.
	}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutCtx)
	}()
	log.Printf("listening on %s", cfg.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
