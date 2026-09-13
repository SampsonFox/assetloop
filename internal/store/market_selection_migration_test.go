package store_test

import (
	"context"
	"fmt"
	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/migrations"
	"os"
	"path/filepath"
	"testing"
)

func TestMarketSelectionUpgradeFromV15(t *testing.T) { testLegacyMarketUpgrade(t, 15) }
func TestMarketSelectionUpgradeFromV16(t *testing.T) { testLegacyMarketUpgrade(t, 16) }
func testLegacyMarketUpgrade(t *testing.T, version int) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			cfg := config.Database{Driver: driver, DSN: filepath.Join(t.TempDir(), "selection.db")}
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
			applyMigrationsThrough(t, db, driver, 14)
			ddl, err := migrations.FS.ReadFile(driver + "/00018_market_quotes.sql")
			if err != nil {
				t.Fatal(err)
			}
			mustSpecificationExec(t, db, string(ddl))
			mustSpecificationExec(t, db, fmt.Sprintf("INSERT INTO goose_db_version(version_id,is_applied) VALUES(15,%s)", map[string]string{"sqlite": "1", "postgres": "true"}[driver]))
			if version == 16 {
				ddl, err := migrations.FS.ReadFile(driver + "/00019_market_selection.sql")
				if err != nil {
					t.Fatal(err)
				}
				mustSpecificationExec(t, db, string(ddl))
				mustSpecificationExec(t, db, fmt.Sprintf("INSERT INTO goose_db_version(version_id,is_applied) VALUES(16,%s)", map[string]string{"sqlite": "1", "postgres": "true"}[driver]))
			}
			if driver == "sqlite" {
				mustSpecificationExec(t, db, "PRAGMA foreign_keys=ON")
			}
			mustSpecificationExec(t, db, "INSERT INTO market_items(id,tenant_id,name,provider,keyword,filter_criteria,model_desc,region,enabled,created_at,last_success) VALUES('legacy-market','"+resourceUpgradeTenant+"','Old phone','zhuanzhuan','phone','256GB','phone 256GB','CN',1,1,1)")
			mustSpecificationExec(t, db, "INSERT INTO market_prices(tenant_id,market_item_id,observation_date,observed_at,max_minor,currency,base_currency,base_minor,rate_scaled,rate_date,rate_source,provider,provider_version,provenance,evidence) VALUES('"+resourceUpgradeTenant+"','legacy-market','2026-09-08',1,645800,'CNY','CNY',645800,100000000,'2026-09-08','identity','zhuanzhuan','1','latest maximum','{}')")
			mustSpecificationExec(t, db, "INSERT INTO asset_market_bindings(tenant_id,asset_id,market_item_id) VALUES('"+resourceUpgradeTenant+"','"+resourceUpgradeAsset+"','legacy-market')")
			if version == 16 {
				mustSpecificationExec(t, db, `UPDATE market_items SET selection_json='{"provider":"zhuanzhuan","productId":"preserved-selection"}'`)
				// The forward bridge must roll back OAuth/receipt DDL if any part fails.
				mustSpecificationExec(t, db, "CREATE TABLE oauth_credentials(collision TEXT)")
				if err := basestore.Migrate(context.Background(), db, cfg); err == nil {
					t.Fatal("expected bridge collision")
				}
				assertSpecificationCount(t, db, "SELECT MAX(version_id) FROM goose_db_version", 17)
				mustSpecificationExec(t, db, "DROP TABLE oauth_credentials")
			}
			if e := basestore.Migrate(context.Background(), db, cfg); e != nil {
				t.Fatal(e)
			}
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM market_items WHERE id='legacy-market' AND keyword='phone'", 1)
			if version == 15 {
				assertSpecificationCount(t, db, "SELECT COUNT(*) FROM market_items WHERE selection_json IS NULL", 1)
			} else {
				var selection string
				if err := db.QueryRow("SELECT selection_json FROM market_items WHERE id='legacy-market'").Scan(&selection); err != nil || selection != `{"provider":"zhuanzhuan","productId":"preserved-selection"}` {
					t.Fatal("selection changed", err)
				}
			}
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM oauth_grants", 0)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM oauth_credentials", 0)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM management_requests", 0)
			assertSpecificationCount(t, db, "SELECT MAX(version_id) FROM goose_db_version", 20)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM market_prices WHERE max_minor=645800", 1)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM asset_market_bindings WHERE market_item_id='legacy-market'", 1)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM asset_events WHERE base_amount_minor=-12345", 1)
			if e := basestore.Migrate(context.Background(), db, cfg); e != nil {
				t.Fatal(e)
			}
		})
	}
}
