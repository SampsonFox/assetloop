package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
	"github.com/SampsonFox/assetloop/internal/store/storetest"
)

func TestStoreConformanceAndSafeRemigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assetloop.db")
	cfg := config.Database{Driver: "sqlite", DSN: path}
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
		t.Fatal(err)
	}
	storetest.Run(t, sqlite.New(db))
	if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	backups, err := filepath.Glob(path + ".backup-*")
	if err != nil || len(backups) == 0 {
		t.Fatalf("expected verified pre-upgrade backup, files=%v err=%v", backups, err)
	}
}
