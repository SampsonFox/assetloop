package store_test

import (
	"context"
	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func TestAssetDeletionUpgradeFromV19PreservesData(t *testing.T) { testMarketUpgradeFromBaseline(t, 19) }
func TestMarketUpgradeFromV14PreservesData(t *testing.T)        { testMarketUpgradeFromBaseline(t, 14) }
func TestMarketUpgradeFromMCPUAT17PreservesData(t *testing.T)   { testMarketUpgradeFromBaseline(t, 17) }
func testMarketUpgradeFromBaseline(t *testing.T, version int) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			cfg := config.Database{Driver: driver, DSN: filepath.Join(t.TempDir(), "market-upgrade.db")}
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
			db := resourceUpgradeV10(t, cfg)
			if driver == "sqlite" {
				mustSpecificationExec(t, db, "PRAGMA foreign_keys=OFF")
			}
			applyMigrationsThrough(t, db, driver, version)
			if driver == "sqlite" {
				mustSpecificationExec(t, db, "PRAGMA foreign_keys=ON")
			}
			if version >= 17 {
				mustSpecificationExec(t, db, "INSERT INTO oauth_grants(id,tenant_id,user_id,client_id,scope,resource,created_at,expires_at) VALUES('12345678-1234-4234-8234-123456789012','"+resourceUpgradeTenant+"','88888888-8888-4888-8888-888888888888','existing-client','assetloop:read','http://127.0.0.1/mcp','2026-09-01T00:00:00Z','2030-09-01T00:00:00Z')")
				mustSpecificationExec(t, db, "INSERT INTO oauth_credentials(hash,tenant_id,grant_id,kind,expires_at) VALUES('synthetic-hash','"+resourceUpgradeTenant+"','12345678-1234-4234-8234-123456789012','access','2030-09-01T00:00:00Z')")
				mustSpecificationExec(t, db, `INSERT INTO management_requests(tenant_id,user_id,request_key,request_hash,result_json) VALUES('`+resourceUpgradeTenant+`','88888888-8888-4888-8888-888888888888','preserved-request','synthetic-hash','{"id":"original-result"}')`)
			}
			tables := []string{"assets", "asset_events", "asset_transactions", "specification_tags", "asset_specification_tags", "model_3d_resources"}
			counts := map[string]int{}
			var countsValue int
			for _, table := range tables {
				if e := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&countsValue); e != nil {
					t.Fatal(e)
				}
				counts[table] = countsValue
			}
			if e := basestore.Migrate(context.Background(), db, cfg); e != nil {
				t.Fatal(e)
			}
			for table, n := range counts {
				assertSpecificationCount(t, db, "SELECT COUNT(*) FROM "+table, n)
			}
			assertSpecificationCount(t, db, "SELECT MAX(version_id) FROM goose_db_version", 20)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM market_items", 0)
			if version >= 17 {
				assertSpecificationCount(t, db, "SELECT COUNT(*) FROM oauth_grants WHERE client_id='existing-client'", 1)
				assertSpecificationCount(t, db, "SELECT COUNT(*) FROM oauth_credentials WHERE hash='synthetic-hash'", 1)
				var receipt string
				if err := db.QueryRow("SELECT result_json FROM management_requests WHERE request_key='preserved-request'").Scan(&receipt); err != nil || receipt != `{"id":"original-result"}` {
					t.Fatal("MCP receipt changed", err)
				}
			}
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM asset_events WHERE base_amount_minor=-12345", 1)
			var key string
			if e := db.QueryRow("SELECT object_key FROM model_3d_resources WHERE id='" + resourceUpgradeModel + "'").Scan(&key); e != nil || key != resourceUpgradeKey {
				t.Fatal("blob key changed", e)
			}
			if e := basestore.Migrate(context.Background(), db, cfg); e != nil {
				t.Fatal(e)
			}
		})
	}
}
