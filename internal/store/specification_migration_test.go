package store_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/postgres"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
)

func TestSpecificationMigration(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			cfg := config.Database{Driver: driver, DSN: filepath.Join(t.TempDir(), "specifications.db")}
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
			applyMigrationsThrough(t, db, driver, 12)
			if driver == "sqlite" {
				mustSpecificationExec(t, db, "PRAGMA foreign_keys=ON")
			}
			const secondResource = "13000000-0000-4000-8000-000000000001"
			const secondVariant = "13000000-0000-4000-8000-000000000002"
			mustSpecificationExec(t, db, "INSERT INTO model_3d_resources(id,tenant_id,name,status,store_id,object_key,sha256,size_bytes,source_url,author,license,created_at,updated_at) SELECT '"+secondResource+"',tenant_id,'Other body',status,store_id,object_key || '.second',sha256,size_bytes,source_url,author,license,created_at,updated_at FROM model_3d_resources")
			mustSpecificationExec(t, db, "UPDATE product_variants SET name='Δ 256',model_3d_resource_id='"+resourceUpgradeModel+"'")
			mustSpecificationExec(t, db, "INSERT INTO product_variants(id,tenant_id,model_id,name,color,created_at,model_3d_resource_id) SELECT '"+secondVariant+"',tenant_id,model_id,'512GB',color,created_at,'"+secondResource+"' FROM product_variants WHERE color='Black'")
			mustSpecificationExec(t, db, "UPDATE assets SET model_3d_resource_id='"+secondResource+"' WHERE id='"+resourceUpgradeAsset+"'")
			// A schema collision proves that the rebuild and version write roll back.
			mustSpecificationExec(t, db, "CREATE TABLE specification_tags (collision INTEGER)")
			if err := basestore.Migrate(context.Background(), db, cfg); err == nil {
				t.Fatal("expected schema collision")
			}
			assertSpecificationCount(t, db, "SELECT MAX(version_id) FROM goose_db_version", 12)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM assets", 5)
			mustSpecificationExec(t, db, "DROP TABLE specification_tags")
			idType := "TEXT"
			if driver == "postgres" {
				idType = "UUID"
			}
			// An unknown remaining dependency must abort contract migration 14,
			// not erase it or leave the rebuilt database partially committed.
			mustSpecificationExec(t, db, "CREATE TABLE retirement_guard (variant_id "+idType+" REFERENCES product_variants(id))")
			mustSpecificationExec(t, db, "INSERT INTO retirement_guard SELECT id FROM product_variants ORDER BY id LIMIT 1")
			if err := basestore.Migrate(context.Background(), db, cfg); err == nil {
				t.Fatal("retirement silently discarded an unknown dependency")
			}
			assertSpecificationCount(t, db, "SELECT MAX(version_id) FROM goose_db_version", 13)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM assets", 5)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM legacy_variant_media", 3)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM retirement_guard", 1)
			mustSpecificationExec(t, db, "DROP TABLE retirement_guard")
			if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
				t.Fatal(err)
			}
			assertSpecificationCount(t, db, "SELECT MAX(version_id) FROM goose_db_version", 19)
			if driver == "sqlite" {
				assertSpecificationCount(t, db, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('product_variants','legacy_variant_tags','legacy_variant_media')", 0)
				assertSpecificationCount(t, db, "SELECT COUNT(*) FROM pragma_table_info('assets') WHERE name IN ('variant_id','color')", 0)
			} else {
				assertSpecificationCount(t, db, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name IN ('product_variants','legacy_variant_tags','legacy_variant_media')", 0)
				assertSpecificationCount(t, db, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='assets' AND column_name IN ('variant_id','color')", 0)
			}
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM assets WHERE model_id='"+resourceUpgradeModel+"'", 5)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM specification_tag_types", 2)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM specification_tags", 4)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM specification_tags WHERE name='Δ 256' AND normalized_name='δ 256'", 1)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM specification_tag_types WHERE name='储存'", 0)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM model_appearance_defaults", 1)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM assets WHERE id='"+resourceUpgradeAsset+"' AND model_3d_resource_id='"+secondResource+"'", 1)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM assets WHERE id<>'"+resourceUpgradeAsset+"' AND model_3d_resource_id='"+resourceUpgradeModel+"'", 3)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM resource_categories", 2)
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM asset_events WHERE asset_id='"+resourceUpgradeAsset+"' AND base_amount_minor=-12345", 1)
			var key string
			if err := db.QueryRow("SELECT object_key FROM model_3d_resources WHERE id='" + resourceUpgradeModel + "'").Scan(&key); err != nil || key != resourceUpgradeKey {
				t.Fatalf("legacy blob moved: %q %v", key, err)
			}
			if _, err := db.Exec("UPDATE model_3d_resources SET status='pending-delete' WHERE id='" + secondResource + "'"); err == nil {
				t.Fatal("referenced resource became pending")
			}
			if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
				t.Fatal(err)
			}
			assertSpecificationCount(t, db, "SELECT COUNT(*) FROM specification_tags", 4)
			// Only live model, appearance and item bindings protect resources now.
			var media application.ModelMediaStore = sqlite.New(db)
			if driver == "postgres" {
				media = postgres.New(db)
			}
			refs, err := media.Model3DReferences(context.Background(), resourceUpgradeTenant, secondResource)
			if err != nil || len(refs) != 1 {
				t.Fatalf("migrated references: %+v %v", refs, err)
			}
			seen := map[string]bool{}
			for _, ref := range refs {
				seen[ref.Kind] = true
			}
			if seen["legacy"] || !seen["asset"] || seen["variant"] {
				t.Fatalf("wrong active reference kinds: %+v", refs)
			}
			page, err := media.ListModel3DResources(context.Background(), resourceUpgradeTenant, application.Model3DResourceListOptions{Query: "Other body", Page: 1, PageSize: 10})
			if err != nil || len(page.Resources) != 1 || page.Resources[0].ReferenceCount != 1 {
				t.Fatalf("live reference count: %+v %v", page, err)
			}
			mustSpecificationExec(t, db, "UPDATE assets SET model_3d_resource_id=NULL WHERE model_3d_resource_id='"+secondResource+"'")
			if err := media.MarkModel3DResourcePendingDelete(context.Background(), resourceUpgradeTenant, secondResource); err != nil {
				t.Fatalf("obsolete references blocked deletion: %v", err)
			}
			if err := media.FinishModel3DResourceDelete(context.Background(), resourceUpgradeTenant, secondResource); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func mustSpecificationExec(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	if _, err := db.Exec(query); err != nil {
		t.Fatalf("%s: %v", strings.Split(query, " ")[0], err)
	}
}

func assertSpecificationCount(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil || got != want {
		t.Fatalf("%s: got %d want %d (%v)", query, got, want, err)
	}
}
