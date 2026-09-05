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

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/config"
	"github.com/SampsonFox/assetloop/internal/store"
	postgresstore "github.com/SampsonFox/assetloop/internal/store/postgres"
	sqlitestore "github.com/SampsonFox/assetloop/internal/store/sqlite"
	webtransport "github.com/SampsonFox/assetloop/internal/web"
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
		if err := store.CheckSchema(context.Background(), db, cfg.Database.Driver); err != nil {
			return err
		}
		var appStore interface {
			application.Store
			application.AuthStore
			application.CatalogStore
			application.LifecycleStore
		}
		if cfg.Database.Driver == "sqlite" {
			appStore = sqlitestore.New(db)
		} else {
			appStore = postgresstore.New(db)
		}
		auth := application.NewAuthService(appStore)
		catalog := application.NewCatalogService(appStore)
		lifecycle := application.NewLifecycleService(appStore)
		options := webtransport.Options{AuthMode: cfg.AuthMode, SecureCookies: cfg.Environment != "local"}
		if cfg.AuthMode == "disabled" {
			_, err := auth.EnsureDisabledPrincipal(context.Background())
			if err != nil {
				return fmt.Errorf("initialize disabled authentication: %w", err)
			}
		}
		webServer, err := webtransport.New(auth, catalog, lifecycle, db, options)
		if err != nil {
			return err
		}
		return serve(cfg.HTTPAddr, webServer.Handler())
	default:
		return fmt.Errorf("unknown command %q; usage: assetloop <serve|migrate>", args[0])
	}
}

func serve(addr string, handler http.Handler) error {
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
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
