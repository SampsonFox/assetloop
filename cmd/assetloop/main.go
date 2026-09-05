package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/blob"
	aliyunblob "github.com/SampsonFox/assetloop/internal/blob/aliyun"
	localblob "github.com/SampsonFox/assetloop/internal/blob/local"
	"github.com/SampsonFox/assetloop/internal/config"
	"github.com/SampsonFox/assetloop/internal/store"
	postgresstore "github.com/SampsonFox/assetloop/internal/store/postgres"
	sqlitestore "github.com/SampsonFox/assetloop/internal/store/sqlite"
	webtransport "github.com/SampsonFox/assetloop/internal/web"
)

func main() {
	args := os.Args[1:]
	doubleClicked := len(args) == 0 && ownsConsole()
	err := launch(args, doubleClicked)
	if err != nil {
		slog.Error("assetloop stopped", "error", err)
		if doubleClicked {
			fmt.Fprintln(os.Stderr, "Press Enter to close / 按回车键关闭")
			_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		}
		os.Exit(1)
	}
}

func launch(args []string, doubleClicked bool) error {
	if doubleClicked {
		path, err := os.Executable()
		if err != nil {
			return err
		}
		if err := os.Chdir(filepath.Dir(path)); err != nil {
			return err
		}
	}
	return run(args)
}

func run(args []string) error {
	if len(args) == 0 {
		args = []string{"serve"}
	}
	if len(args) != 1 || (args[0] != "serve" && args[0] != "migrate") {
		return errors.New("usage: assetloop [serve|migrate] (default: serve)")
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
			application.ModelMediaStore
		}
		if cfg.Database.Driver == "sqlite" {
			appStore = sqlitestore.New(db)
		} else {
			appStore = postgresstore.New(db)
		}
		auth := application.NewAuthService(appStore)
		catalog := application.NewCatalogService(appStore)
		lifecycle := application.NewLifecycleService(appStore)
		localStore, err := localblob.New(cfg.Blob.LocalRoot)
		if err != nil {
			return fmt.Errorf("initialize local blob store: %w", err)
		}
		blobStores := blob.Registry{"local": localStore}
		if cfg.Blob.OSS.Region != "" || cfg.Blob.DefaultStore == "aliyun" {
			ossStore, err := aliyunblob.New(aliyunblob.Config{Endpoint: cfg.Blob.OSS.Endpoint, Region: cfg.Blob.OSS.Region, Bucket: cfg.Blob.OSS.Bucket, AccessKeyID: cfg.Blob.OSS.AccessKeyID, AccessKeySecret: cfg.Blob.OSS.AccessKeySecret, PathPrefix: cfg.Blob.OSS.PathPrefix})
			if err != nil {
				return fmt.Errorf("initialize Aliyun OSS blob store: %w", err)
			}
			blobStores["aliyun"] = ossStore
		}
		modelMedia := application.NewModelMediaService(appStore, blobStores, blob.ObjectKeyMapper{}, cfg.Blob.DefaultStore)
		options := webtransport.Options{AuthMode: cfg.AuthMode, SecureCookies: cfg.Environment != "local", ModelMedia: modelMedia}
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
