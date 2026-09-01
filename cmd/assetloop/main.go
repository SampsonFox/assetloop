package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SampsonFox/assetloop/internal/config"
	"github.com/SampsonFox/assetloop/internal/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("assetloop stopped", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: assetloop <serve|migrate>")
	}

	cfg, err := config.Load(".env")
	if err != nil {
		return err
	}

	db, err := store.Open(cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()

	switch args[0] {
	case "migrate":
		return store.Migrate(context.Background(), db, cfg.Database)
	case "serve":
		if cfg.Database.Driver == "sqlite" {
			if err := store.Migrate(context.Background(), db, cfg.Database); err != nil {
				return err
			}
		}
		return serve(cfg.HTTPAddr, db)
	default:
		return fmt.Errorf("unknown command %q; usage: assetloop <serve|migrate>", args[0])
	}
}

type pinger interface {
	PingContext(context.Context) error
}

func serve(addr string, db pinger) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.PingContext(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	slog.Info("AssetLoop listening", "address", addr)
	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
