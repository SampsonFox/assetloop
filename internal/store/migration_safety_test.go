package store_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
)

func TestMigrationSafety(t *testing.T) {
	t.Run("foreign key failure keeps subsequent startup closed", func(t *testing.T) {
		cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "foreign-key.db")}
		db, err := basestore.Open(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		applyMigrationsThrough(t, db, "sqlite", 7)
		if _, err := db.Exec("PRAGMA foreign_keys=OFF; INSERT INTO item_categories (id,tenant_id,name,created_at) VALUES ('bad-category','missing-tenant','Corrupt','2026-09-01T00:00:00Z'); PRAGMA foreign_keys=ON;"); err != nil {
			t.Fatal(err)
		}
		for range 2 {
			if err := basestore.Migrate(context.Background(), db, cfg); err == nil {
				t.Fatal("foreign key violation accepted at startup")
			}
			var version, foreignKeys, resources int
			if err := db.QueryRow("SELECT MAX(version_id) FROM goose_db_version").Scan(&version); err != nil || version != 10 {
				t.Fatalf("failed resource upgrade must roll back its version: %d %v", version, err)
			}
			if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil || foreignKeys != 1 {
				t.Fatalf("foreign keys not restored after failed upgrade: %d %v", foreignKeys, err)
			}
			if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name='model_3d_resources'").Scan(&resources); err != nil || resources != 0 {
				t.Fatalf("failed resource upgrade left partial schema: %d %v", resources, err)
			}
		}
		backups, err := filepath.Glob(cfg.DSN + ".backup-*")
		// Migration 11 rolls back; each startup therefore makes a new upgrade attempt
		// and must retain its own pre-upgrade recovery backup (v7, then v10).
		if err != nil || len(backups) != 2 {
			t.Fatalf("missing per-attempt upgrade backup: %v %v", backups, err)
		}
		for i, path := range backups {
			backup, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			var version, preserved int
			versionErr := backup.QueryRow("SELECT MAX(version_id) FROM goose_db_version").Scan(&version)
			dataErr := backup.QueryRow("SELECT COUNT(*) FROM item_categories WHERE id='bad-category' AND tenant_id='missing-tenant'").Scan(&preserved)
			_ = backup.Close()
			if versionErr != nil || dataErr != nil || version != []int{7, 10}[i] || preserved != 1 {
				t.Fatalf("recovery backup changed: version=%d preserved=%d errors=%v/%v", version, preserved, versionErr, dataErr)
			}
		}
	})
	t.Run("failed upgrade preserves recoverable backup", func(t *testing.T) {
		cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "failed.db")}
		db, err := basestore.Open(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		applyMigrationsThrough(t, db, "sqlite", 6)
		// Force the v7 rename to fail after migration execution has begun.
		if _, err := db.Exec("CREATE TABLE asset_events_v6 (sentinel TEXT); INSERT INTO asset_events_v6 VALUES ('preserve me')"); err != nil {
			t.Fatal(err)
		}
		if err := basestore.Migrate(context.Background(), db, cfg); err == nil {
			t.Fatal("failed upgrade reported success")
		}
		backups, err := filepath.Glob(cfg.DSN + ".backup-*")
		if err != nil || len(backups) != 1 {
			t.Fatalf("missing recovery backup: %v %v", backups, err)
		}
		restored, err := sql.Open("sqlite", backups[0])
		if err != nil {
			t.Fatal(err)
		}
		defer restored.Close()
		var sentinel string
		if err := restored.QueryRow("SELECT sentinel FROM asset_events_v6").Scan(&sentinel); err != nil || sentinel != "preserve me" {
			t.Fatalf("backup data: %q %v", sentinel, err)
		}
		var triggers int
		if err := restored.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='trigger' AND name IN ('asset_events_no_update','asset_events_no_delete')").Scan(&triggers); err != nil || triggers != 2 {
			t.Fatalf("backup not pre-upgrade: %d %v", triggers, err)
		}
	})
	t.Run("current schema does not create another backup", func(t *testing.T) {
		cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "current.db")}
		db, err := basestore.Open(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
			t.Fatal(err)
		}
		before, _ := filepath.Glob(cfg.DSN + ".backup-*")
		if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
			t.Fatal(err)
		}
		after, _ := filepath.Glob(cfg.DSN + ".backup-*")
		if len(after) != len(before) {
			t.Fatalf("unchanged schema created backups: before=%d after=%d", len(before), len(after))
		}
	})
	t.Run("newer database is rejected", func(t *testing.T) {
		cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "future.db")}
		db, err := basestore.Open(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO goose_db_version (version_id, is_applied) VALUES (99999, 1)"); err != nil {
			t.Fatal(err)
		}
		if err := basestore.Migrate(context.Background(), db, cfg); err == nil {
			t.Fatal("accepted a database newer than this binary")
		}
	})
}

func TestSchemaChecksAndConcurrentUpgrade(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "safety.db")}
			if dialect == "postgres" {
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					if os.Getenv("REQUIRE_POSTGRES_TEST") == "true" {
						t.Fatal("PostgreSQL required")
					}
					t.Skip("TEST_POSTGRES_DSN not set")
				}
				var cleanup func()
				cfg, cleanup = postgresUpgradeSchema(t, dsn)
				defer cleanup()
			}
			db, err := basestore.Open(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			ctx := context.Background()
			if err := basestore.CheckSchema(ctx, db, dialect); err == nil {
				t.Fatal("empty schema accepted")
			}
			var count int
			query := "SELECT count(*) FROM sqlite_master WHERE name='goose_db_version'"
			if dialect == "postgres" {
				query = "SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='goose_db_version'"
			}
			if err := db.QueryRow(query).Scan(&count); err != nil || count != 0 {
				t.Fatalf("version check mutated schema: %d %v", count, err)
			}
			applyMigrationsThrough(t, db, dialect, 6)
			if err := basestore.CheckSchema(ctx, db, dialect); err == nil {
				t.Fatal("old schema accepted")
			}
			var wg sync.WaitGroup
			errs := make(chan error, 4)
			for range 4 {
				other, err := basestore.Open(cfg)
				if err != nil {
					t.Fatal(err)
				}
				wg.Go(func() { defer other.Close(); errs <- basestore.Migrate(ctx, other, cfg) })
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := basestore.CheckSchema(ctx, db, dialect); err != nil {
				t.Fatal(err)
			}
			if dialect == "sqlite" {
				backups, err := filepath.Glob(cfg.DSN + ".backup-*")
				if err != nil || len(backups) != 1 {
					t.Fatalf("expected one backup across concurrent upgrades: %v %v", backups, err)
				}
				restored, err := sql.Open("sqlite", backups[0])
				if err != nil {
					t.Fatal(err)
				}
				defer restored.Close()
				var version int
				if err := restored.QueryRow("SELECT MAX(version_id) FROM goose_db_version").Scan(&version); err != nil || version != 6 {
					t.Fatalf("backup lost original schema: %d %v", version, err)
				}
			}
			value := "1"
			if dialect == "postgres" {
				value = "true"
			}
			if _, err := db.Exec("INSERT INTO goose_db_version (version_id,is_applied) VALUES (99999," + value + ")"); err != nil {
				t.Fatal(err)
			}
			if err := basestore.CheckSchema(ctx, db, dialect); err == nil {
				t.Fatal("future schema accepted at startup")
			}
			if err := basestore.Migrate(ctx, db, cfg); err == nil {
				t.Fatal("future schema accepted at migration")
			}
		})
	}
}
