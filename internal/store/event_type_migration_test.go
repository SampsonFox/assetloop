package store_test

import (
	"context"
	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEventTypeIDUpgrade(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		for _, unknown := range []bool{false, true} {
			name := driver
			if unknown {
				name += "/unknown"
			}
			t.Run(name, func(t *testing.T) {
				cfg := config.Database{Driver: driver, DSN: filepath.Join(t.TempDir(), "types.db")}
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
				applyMigrationsThrough(t, db, driver, 11)
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

				typeID := "10000000-0000-4000-8000-000000000009"
				typeName := "保养 Δ"
				if !unknown {
					if _, err := db.Exec("INSERT INTO asset_event_types(id,tenant_id,name,normalized_name,cashflow_direction,created_by_user_id,created_at) VALUES ("+values(driver, 7)+")", typeID, ids[0], typeName, strings.ToLower(typeName), "neutral", ids[1], createdAt); err != nil {
						t.Fatal(err)
					}
				}
				customID := "10000000-0000-4000-8000-000000000010"
				if _, err := db.Exec("INSERT INTO asset_events(id,tenant_id,asset_id,transaction_id,event_type,base_amount_minor,base_currency,occurred_at,created_by_user_id,created_at) VALUES ("+values(driver, 10)+")", customID, ids[0], ids[5], ids[6], typeName, 0, "CNY", occurredAt, ids[1], createdAt); err != nil {
					t.Fatal(err)
				}
				err = basestore.Migrate(context.Background(), db, cfg)
				if unknown {
					if err == nil || !strings.Contains(err.Error(), customID) {
						t.Fatalf("unknown record should be identified: %v", err)
					}
					var version int
					if err := db.QueryRow("SELECT MAX(version_id) FROM goose_db_version WHERE is_applied=" + map[string]string{"sqlite": "1", "postgres": "TRUE"}[driver]).Scan(&version); err != nil || version != 11 {
						t.Fatalf("failed migration changed version %d %v", version, err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				var id, name, legacy string
				if err := db.QueryRow("SELECT e.event_type_id,t.name,e.event_type FROM asset_events e JOIN asset_event_types t ON t.id=e.event_type_id AND t.tenant_id=e.tenant_id WHERE e.id="+upgradePlaceholder(driver), customID).Scan(&id, &name, &legacy); err != nil {
					t.Fatal(err)
				}
				if id != typeID || name != typeName || legacy != typeName {
					t.Fatalf("legacy mapping %s %s %s", id, name, legacy)
				}
				var count int
				if err := db.QueryRow("SELECT COUNT(*) FROM asset_events").Scan(&count); err != nil || count != 2 {
					t.Fatalf("history changed %d %v", count, err)
				}
				if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
