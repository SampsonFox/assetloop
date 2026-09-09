package store_test

import (
	"context"
	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func TestMarketUpgradeFromV14PreservesData(t *testing.T) {
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
			applyMigrationsThrough(t, db, driver, 14)
			if driver == "sqlite" {
				mustSpecificationExec(t, db, "PRAGMA foreign_keys=ON")
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
			assertSpecificationCount(t, db, "SELECT MAX(version_id) FROM goose_db_version", 16)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM market_items", 0)
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
