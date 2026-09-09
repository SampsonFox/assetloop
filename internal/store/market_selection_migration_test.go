package store_test

import (
	"context"
	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func TestMarketSelectionUpgradeFromV15(t *testing.T) {
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
			applyMigrationsThrough(t, db, driver, 15)
			if driver == "sqlite" {
				mustSpecificationExec(t, db, "PRAGMA foreign_keys=ON")
			}
			mustSpecificationExec(t, db, "INSERT INTO market_items(id,tenant_id,name,provider,keyword,filter_criteria,model_desc,region,enabled,created_at,last_success) VALUES('legacy-market','"+resourceUpgradeTenant+"','Old phone','zhuanzhuan','phone','256GB','phone 256GB','CN',1,1,1)")
			mustSpecificationExec(t, db, "INSERT INTO market_prices(tenant_id,market_item_id,observation_date,observed_at,max_minor,currency,base_currency,base_minor,rate_scaled,rate_date,rate_source,provider,provider_version,provenance,evidence) VALUES('"+resourceUpgradeTenant+"','legacy-market','2026-09-08',1,645800,'CNY','CNY',645800,100000000,'2026-09-08','identity','zhuanzhuan','1','latest maximum','{}')")
			mustSpecificationExec(t, db, "INSERT INTO asset_market_bindings(tenant_id,asset_id,market_item_id) VALUES('"+resourceUpgradeTenant+"','"+resourceUpgradeAsset+"','legacy-market')")
			if e := basestore.Migrate(context.Background(), db, cfg); e != nil {
				t.Fatal(e)
			}
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM market_items WHERE id='legacy-market' AND selection_json IS NULL AND keyword='phone'", 1)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM market_prices WHERE max_minor=645800", 1)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM asset_market_bindings WHERE market_item_id='legacy-market'", 1)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM asset_events WHERE base_amount_minor=-12345", 1)
			if e := basestore.Migrate(context.Background(), db, cfg); e != nil {
				t.Fatal(e)
			}
		})
	}
}
