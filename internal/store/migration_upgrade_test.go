package store_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"testing/fstest"
	"time"

	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/migrations"
	"github.com/pressly/goose/v3"
)

func TestUpgradeFromPreviousSchemaPreservesData(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "upgrade.db")
		cfg := config.Database{Driver: "sqlite", DSN: path}
		runUpgradeTest(t, cfg)
		backups, err := filepath.Glob(path + ".backup-*")
		if err != nil || len(backups) == 0 {
			t.Fatalf("expected verified SQLite upgrade backup: files=%v err=%v", backups, err)
		}
	})

	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("TEST_POSTGRES_DSN")
		if dsn == "" {
			if os.Getenv("REQUIRE_POSTGRES_TEST") == "true" {
				t.Fatal("TEST_POSTGRES_DSN is required for PostgreSQL migration upgrade coverage")
			}
			t.Skip("TEST_POSTGRES_DSN is not set")
		}
		cfg, cleanup := postgresUpgradeSchema(t, dsn)
		defer cleanup()
		runUpgradeTest(t, cfg)
	})
}

func runUpgradeTest(t *testing.T, cfg config.Database) {
	t.Helper()
	ctx := context.Background()
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	applyVersionOne(t, db, cfg.Driver)
	insertVersionOneAsset(t, db, cfg.Driver)
	if err := basestore.Migrate(ctx, db, cfg); err != nil {
		t.Fatalf("upgrade from schema v1: %v", err)
	}
	var displayName string
	if err := db.QueryRow("SELECT display_name FROM assets WHERE id = "+upgradePlaceholder(cfg.Driver), "11111111-1111-4111-8111-111111111111").Scan(&displayName); err != nil {
		t.Fatalf("read preserved asset: %v", err)
	}
	if displayName != "Preserved Asset" {
		t.Fatalf("asset changed during upgrade: %q", displayName)
	}
	var users int
	if err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&users); err != nil {
		t.Fatalf("new auth schema is unavailable after upgrade: %v", err)
	}
}

func applyVersionOne(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	data, err := migrations.FS.ReadFile(driver + "/00001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	dialect := goose.DialectPostgres
	if driver == "sqlite" {
		dialect = goose.DialectSQLite3
	}
	provider, err := goose.NewProvider(dialect, db, fstest.MapFS{"00001_initial.sql": &fstest.MapFile{Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func insertVersionOneAsset(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	createdAt := any("2026-09-01T00:00:00Z")
	if driver == "postgres" {
		createdAt = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	}
	statements := []struct {
		query string
		args  []any
	}{
		{"INSERT INTO tenants (id, name, base_currency, created_at) VALUES (" + values(driver, 4) + ")", []any{"22222222-2222-4222-8222-222222222222", "Existing", "CNY", createdAt}},
		{"INSERT INTO item_categories (id, tenant_id, name, created_at) VALUES (" + values(driver, 4) + ")", []any{"33333333-3333-4333-8333-333333333333", "22222222-2222-4222-8222-222222222222", "Phone", createdAt}},
		{"INSERT INTO product_models (id, tenant_id, category_id, name, created_at) VALUES (" + values(driver, 5) + ")", []any{"44444444-4444-4444-8444-444444444444", "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333", "Existing Phone", createdAt}},
		{"INSERT INTO product_variants (id, tenant_id, model_id, name, created_at) VALUES (" + values(driver, 5) + ")", []any{"55555555-5555-4555-8555-555555555555", "22222222-2222-4222-8222-222222222222", "44444444-4444-4444-8444-444444444444", "256GB", createdAt}},
		{"INSERT INTO assets (id, tenant_id, variant_id, display_name, created_at) VALUES (" + values(driver, 5) + ")", []any{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "55555555-5555-4555-8555-555555555555", "Preserved Asset", createdAt}},
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed schema v1: %v", err)
		}
	}
}

func values(driver string, count int) string {
	result := ""
	for i := 1; i <= count; i++ {
		if i > 1 {
			result += ", "
		}
		if driver == "postgres" {
			result += "$" + strconv.Itoa(i)
		} else {
			result += "?"
		}
	}
	return result
}

func upgradePlaceholder(driver string) string {
	if driver == "postgres" {
		return "$1"
	}
	return "?"
}

func postgresUpgradeSchema(t *testing.T, rawDSN string) (config.Database, func()) {
	t.Helper()
	parsed, err := url.Parse(rawDSN)
	if err != nil {
		t.Fatal(err)
	}
	schema := "assetloop_upgrade_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	admin, err := sql.Open("pgx", rawDSN)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(`CREATE SCHEMA "` + schema + `"`); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return config.Database{Driver: "postgres", DSN: parsed.String()}, func() {
		_, _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, schema))
		_ = admin.Close()
	}
}
