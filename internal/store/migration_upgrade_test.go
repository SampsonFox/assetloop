package store_test

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

func TestUpgradeExistingUserGetsSafePreferences(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "preferences-upgrade.db")}
		runUserPreferencesUpgradeTest(t, cfg)
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
		runUserPreferencesUpgradeTest(t, cfg)
	})
}

func TestUpgradeExistingPreferencesGetsSafeAccent(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			cfg := config.Database{Driver: driver, DSN: filepath.Join(t.TempDir(), "accent-upgrade.db")}
			if driver == "postgres" {
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					if os.Getenv("REQUIRE_POSTGRES_TEST") == "true" {
						t.Fatal("TEST_POSTGRES_DSN is required for PostgreSQL migration upgrade coverage")
					}
					t.Skip("TEST_POSTGRES_DSN is not set")
				}
				var cleanup func()
				cfg, cleanup = postgresUpgradeSchema(t, dsn)
				defer cleanup()
			}
			runAccentUpgradeTest(t, cfg)
		})
	}
}

func TestUpgradeCustomEventTypesPreservesLifecycle(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		runCustomEventTypeUpgradeTest(t, config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "event-types-upgrade.db")})
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
		runCustomEventTypeUpgradeTest(t, cfg)
	})
}

func TestUpgradeLifecycleRequestsPreservesV7Data(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			cfg := config.Database{Driver: driver, DSN: filepath.Join(t.TempDir(), "v7.db")}
			if driver == "postgres" {
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
			applyMigrationsThrough(t, db, driver, 7)
			createdAt := any("2026-09-01T00:00:00Z")
			if driver == "postgres" {
				createdAt = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
			}
			if _, err := db.Exec("INSERT INTO tenants (id,name,base_currency,created_at) VALUES ("+values(driver, 4)+")", "99999999-9999-4999-8999-999999999999", "v7 preserved tenant", "CNY", createdAt); err != nil {
				t.Fatal(err)
			}
			if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
				t.Fatal(err)
			}
			var name string
			if err := db.QueryRow("SELECT name FROM tenants").Scan(&name); err != nil || name != "v7 preserved tenant" {
				t.Fatalf("v7 data changed: %q %v", name, err)
			}
			var count int
			if err := db.QueryRow("SELECT count(*) FROM lifecycle_requests").Scan(&count); err != nil || count != 0 {
				t.Fatalf("new receipt table: %d %v", count, err)
			}
		})
	}
}

func runCustomEventTypeUpgradeTest(t *testing.T, cfg config.Database) {
	t.Helper()
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	applyMigrationsThrough(t, db, cfg.Driver, 6)

	createdAt := any("2026-09-01T00:00:00Z")
	occurredAt := any("2026-09-01T01:00:00Z")
	if cfg.Driver == "postgres" {
		createdAt = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
		occurredAt = time.Date(2026, 9, 1, 1, 0, 0, 0, time.UTC)
	}
	ids := []string{
		"10000000-0000-4000-8000-000000000001", "10000000-0000-4000-8000-000000000002",
		"10000000-0000-4000-8000-000000000003", "10000000-0000-4000-8000-000000000004",
		"10000000-0000-4000-8000-000000000005", "10000000-0000-4000-8000-000000000006",
		"10000000-0000-4000-8000-000000000007", "10000000-0000-4000-8000-000000000008",
	}
	statements := []struct {
		query string
		args  []any
	}{
		{"INSERT INTO tenants (id, name, base_currency, created_at) VALUES (" + values(cfg.Driver, 4) + ")", []any{ids[0], "Lifecycle Upgrade", "CNY", createdAt}},
		{"INSERT INTO users (id, username, username_normalized, password_hash, created_at) VALUES (" + values(cfg.Driver, 5) + ")", []any{ids[1], "upgrade-owner", "upgrade-owner", "preserved-hash", createdAt}},
		{"INSERT INTO tenant_memberships (tenant_id, user_id, role, created_at) VALUES (" + values(cfg.Driver, 4) + ")", []any{ids[0], ids[1], "owner", createdAt}},
		{"INSERT INTO item_categories (id, tenant_id, name, created_at) VALUES (" + values(cfg.Driver, 4) + ")", []any{ids[2], ids[0], "Phone", createdAt}},
		{"INSERT INTO product_models (id, tenant_id, category_id, name, created_at) VALUES (" + values(cfg.Driver, 5) + ")", []any{ids[3], ids[0], ids[2], "Upgrade Phone", createdAt}},
		{"INSERT INTO product_variants (id, tenant_id, model_id, name, created_at) VALUES (" + values(cfg.Driver, 5) + ")", []any{ids[4], ids[0], ids[3], "256GB", createdAt}},
		{"INSERT INTO assets (id, tenant_id, variant_id, display_name, created_at) VALUES (" + values(cfg.Driver, 5) + ")", []any{ids[5], ids[0], ids[4], "Preserved Lifecycle Asset", createdAt}},
		{"INSERT INTO asset_transactions (id, tenant_id, occurred_at, source, created_by_user_id, created_at) VALUES (" + values(cfg.Driver, 6) + ")", []any{ids[6], ids[0], occurredAt, "manual", ids[1], createdAt}},
		{"INSERT INTO asset_events (id, tenant_id, asset_id, transaction_id, event_type, base_amount_minor, base_currency, notes, occurred_at, created_by_user_id, created_at) VALUES (" + values(cfg.Driver, 11) + ")", []any{ids[7], ids[0], ids[5], ids[6], "purchase", -10000, "CNY", "preserve me", occurredAt, ids[1], createdAt}},
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed v6 lifecycle: %v", err)
		}
	}
	if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
		t.Fatalf("upgrade custom event types: %v", err)
	}
	var eventType, notes string
	if err := db.QueryRow("SELECT event_type, notes FROM asset_events WHERE id = "+upgradePlaceholder(cfg.Driver), ids[7]).Scan(&eventType, &notes); err != nil {
		t.Fatalf("read preserved event: %v", err)
	}
	if eventType != "purchase" || notes != "preserve me" {
		t.Fatalf("lifecycle event changed during upgrade: type=%q notes=%q", eventType, notes)
	}
	if _, err := db.Exec("INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at) VALUES ("+values(cfg.Driver, 7)+")", "10000000-0000-4000-8000-000000000009", ids[0], "保养", "保养", "neutral", ids[1], createdAt); err != nil {
		t.Fatalf("insert custom event type after upgrade: %v", err)
	}
}

func runUserPreferencesUpgradeTest(t *testing.T, cfg config.Database) {
	t.Helper()
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	applyMigrationsThrough(t, db, cfg.Driver, 5)
	createdAt := any("2026-09-01T00:00:00Z")
	if cfg.Driver == "postgres" {
		createdAt = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	}
	if _, err := db.Exec("INSERT INTO users (id, username, username_normalized, password_hash, created_at) VALUES ("+values(cfg.Driver, 5)+")",
		"77777777-7777-4777-8777-777777777777", "Existing User", "existing-user", "preserved-hash", createdAt); err != nil {
		t.Fatalf("seed existing user: %v", err)
	}
	if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
		t.Fatalf("upgrade user preferences: %v", err)
	}
	var username, passwordHash, locale, theme string
	if err := db.QueryRow("SELECT username, password_hash, locale, theme FROM users WHERE id = "+upgradePlaceholder(cfg.Driver), "77777777-7777-4777-8777-777777777777").Scan(&username, &passwordHash, &locale, &theme); err != nil {
		t.Fatalf("read upgraded user: %v", err)
	}
	if username != "Existing User" || passwordHash != "preserved-hash" || locale != "zh-CN" || theme != "system" {
		t.Fatalf("unexpected upgraded user: username=%q hash=%q locale=%q theme=%q", username, passwordHash, locale, theme)
	}
}

func runAccentUpgradeTest(t *testing.T, cfg config.Database) {
	t.Helper()
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	applyMigrationsThrough(t, db, cfg.Driver, 8)
	createdAt := any("2026-09-01T00:00:00Z")
	if cfg.Driver == "postgres" {
		createdAt = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	}
	if _, err := db.Exec("INSERT INTO users (id, username, username_normalized, password_hash, locale, theme, created_at) VALUES ("+values(cfg.Driver, 7)+")",
		"88888888-8888-4888-8888-888888888888", "Accent User", "accent-user", "preserved-hash", "en", "dark", createdAt); err != nil {
		t.Fatalf("seed existing preferences: %v", err)
	}
	if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
		t.Fatalf("upgrade user accent: %v", err)
	}
	var locale, theme, accent string
	if err := db.QueryRow("SELECT locale, theme, accent FROM users WHERE id = "+upgradePlaceholder(cfg.Driver), "88888888-8888-4888-8888-888888888888").Scan(&locale, &theme, &accent); err != nil {
		t.Fatalf("read upgraded accent: %v", err)
	}
	if locale != "en" || theme != "dark" || accent != "emerald" {
		t.Fatalf("unexpected upgraded preferences: locale=%q theme=%q accent=%q", locale, theme, accent)
	}
}

func applyMigrationsThrough(t *testing.T, db *sql.DB, driver string, version int) {
	t.Helper()
	files := fstest.MapFS{}
	for current := 1; current <= version; current++ {
		name := fmt.Sprintf("%05d_", current)
		entries, err := fs.ReadDir(migrations.FS, driver)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), name) {
				data, err := migrations.FS.ReadFile(driver + "/" + entry.Name())
				if err != nil {
					t.Fatal(err)
				}
				files[entry.Name()] = &fstest.MapFile{Data: data}
			}
		}
	}
	dialect := goose.DialectPostgres
	if driver == "sqlite" {
		dialect = goose.DialectSQLite3
	}
	provider, err := goose.NewProvider(dialect, db, files)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
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
	var displayName, serialNumber, color, purchaseChannel, notes string
	if err := db.QueryRow("SELECT display_name, serial_number, color, purchase_channel, notes FROM assets WHERE id = "+upgradePlaceholder(cfg.Driver), "11111111-1111-4111-8111-111111111111").Scan(&displayName, &serialNumber, &color, &purchaseChannel, &notes); err != nil {
		t.Fatalf("read preserved asset: %v", err)
	}
	if displayName != "Preserved Asset" {
		t.Fatalf("asset changed during upgrade: %q", displayName)
	}
	if serialNumber != "" || color != "" || purchaseChannel != "" || notes != "" {
		t.Fatalf("new catalog details should have safe empty defaults: serial=%q color=%q channel=%q notes=%q", serialNumber, color, purchaseChannel, notes)
	}
	var iconKey string
	if err := db.QueryRow("SELECT icon_key FROM item_categories WHERE id = "+upgradePlaceholder(cfg.Driver), "33333333-3333-4333-8333-333333333333").Scan(&iconKey); err != nil {
		t.Fatalf("read upgraded category icon: %v", err)
	}
	if iconKey != "package" {
		t.Fatalf("existing category should receive safe default icon, got %q", iconKey)
	}
	var modelMediaFields int
	if err := db.QueryRow("SELECT (CASE WHEN model_3d_store_id IS NULL THEN 1 ELSE 0 END) + (CASE WHEN model_3d_object_key IS NULL THEN 1 ELSE 0 END) + (CASE WHEN model_3d_sha256 IS NULL THEN 1 ELSE 0 END) + (CASE WHEN model_3d_size_bytes IS NULL THEN 1 ELSE 0 END) + (CASE WHEN model_3d_source_url IS NULL THEN 1 ELSE 0 END) + (CASE WHEN model_3d_author IS NULL THEN 1 ELSE 0 END) + (CASE WHEN model_3d_license IS NULL THEN 1 ELSE 0 END) + (CASE WHEN model_3d_updated_at IS NULL THEN 1 ELSE 0 END) FROM product_models WHERE id = "+upgradePlaceholder(cfg.Driver), "44444444-4444-4444-8444-444444444444").Scan(&modelMediaFields); err != nil {
		t.Fatalf("read upgraded model media: %v", err)
	}
	if modelMediaFields != 8 {
		t.Fatalf("new model media fields must be null, got %d null fields", modelMediaFields)
	}
	var users int
	if err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&users); err != nil {
		t.Fatalf("new auth schema is unavailable after upgrade: %v", err)
	}
	var events, drafts int
	if err := db.QueryRow("SELECT COUNT(*) FROM asset_events").Scan(&events); err != nil {
		t.Fatalf("new lifecycle schema is unavailable after upgrade: %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM import_drafts").Scan(&drafts); err != nil {
		t.Fatalf("new import draft schema is unavailable after upgrade: %v", err)
	}
	if events != 0 || drafts != 0 {
		t.Fatalf("upgrade should not fabricate lifecycle records: events=%d drafts=%d", events, drafts)
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
