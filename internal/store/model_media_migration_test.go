package store_test

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/blob"
	localblob "github.com/SampsonFox/assetloop/internal/blob/local"
	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/postgres"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
)

const resourceUpgradeTenant = "22222222-2222-4222-8222-222222222222"
const resourceUpgradeModel = "44444444-4444-4444-8444-444444444444"
const resourceUpgradeVariant = "55555555-5555-4555-8555-555555555555"
const resourceUpgradeAsset = "11111111-1111-4111-8111-111111111111"
const resourceUpgradeKey = "tenants/" + resourceUpgradeTenant + "/models/" + resourceUpgradeModel + "/legacy.glb"

var resourceUpgradeColors = []string{" Black ", "Black", "White", "", "   "}

func TestModel3DResourceMigrationPreservesV10(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			cfg := config.Database{Driver: driver, DSN: filepath.Join(t.TempDir(), "resource-upgrade.db")}
			if driver == "postgres" {
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					if os.Getenv("REQUIRE_POSTGRES_TEST") == "true" {
						t.Fatal("TEST_POSTGRES_DSN is required for resource migration coverage")
					}
					t.Skip("TEST_POSTGRES_DSN is not set")
				}
				var cleanup func()
				cfg, cleanup = postgresUpgradeSchema(t, dsn)
				defer cleanup()
			}
			db := resourceUpgradeV10(t, cfg)
			local, err := localblob.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			legacyBytes := []byte("preserved legacy GLB bytes")
			if err := local.Put(context.Background(), resourceUpgradeKey, bytes.NewReader(legacyBytes), application.BlobMetadata{ContentType: "model/gltf-binary"}); err != nil {
				t.Fatal(err)
			}
			if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
				t.Fatalf("upgrade resource model: %v", err)
			}
			assertResourceUpgrade(t, db, cfg.Driver)
			// The migrated metadata must still select the original store and object key.
			var store application.ModelMediaStore = sqlite.New(db)
			if cfg.Driver == "postgres" {
				store = postgres.New(db)
			}
			service := application.NewModelMediaService(store, blob.Registry{"legacy-local": local}, blob.ObjectKeyMapper{}, "different-default")
			opened, err := service.OpenForAsset(context.Background(), application.Principal{UserID: "88888888-8888-4888-8888-888888888888", TenantID: resourceUpgradeTenant, Role: application.RoleViewer}, resourceUpgradeAsset)
			if err != nil {
				t.Fatalf("open migrated GLB through original store: %v", err)
			}
			data, readErr := io.ReadAll(opened.Reader)
			closeErr := opened.Reader.Close()
			if readErr != nil || closeErr != nil || !bytes.Equal(data, legacyBytes) {
				t.Fatalf("legacy bytes changed: read=%v close=%v", readErr, closeErr)
			}
			if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
				t.Fatalf("restart upgraded database: %v", err)
			}
			assertResourceUpgrade(t, db, cfg.Driver)
		})
	}
}

func resourceUpgradeV10(t *testing.T, cfg config.Database) *sql.DB {
	t.Helper()
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	applyMigrationsThrough(t, db, cfg.Driver, 10)
	insertVersionOneAsset(t, db, cfg.Driver)
	created := any("2026-09-01T00:00:00Z")
	if cfg.Driver == "postgres" {
		created = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("seed resource migration: %v", err)
		}
	}
	exec("UPDATE assets SET color=' Black ' WHERE id='" + resourceUpgradeAsset + "'")
	for i, color := range resourceUpgradeColors[1:] {
		exec("INSERT INTO assets(id,tenant_id,variant_id,display_name,created_at,color) VALUES ("+values(cfg.Driver, 6)+")", fmt.Sprintf("11000000-0000-4000-8000-%012d", i+2), resourceUpgradeTenant, resourceUpgradeVariant, fmt.Sprintf("Legacy asset %d", i+2), created, color)
	}
	exec("UPDATE product_models SET model_3d_store_id='legacy-local',model_3d_object_key='"+resourceUpgradeKey+"',model_3d_sha256='legacy-sha256',model_3d_size_bytes=25,model_3d_source_url='https://example.com/legacy',model_3d_author='Legacy author',model_3d_license='CC0',model_3d_updated_at="+upgradePlaceholder(cfg.Driver)+" WHERE id='"+resourceUpgradeModel+"'", created)
	userID := "88888888-8888-4888-8888-888888888888"
	transactionID := "99999999-9999-4999-8999-999999999999"
	exec("INSERT INTO users(id,username,username_normalized,password_hash,created_at) VALUES ("+values(cfg.Driver, 5)+")", userID, "resource-owner", "resource-owner", "fixture-hash", created)
	exec("INSERT INTO tenant_memberships(tenant_id,user_id,role,created_at) VALUES ("+values(cfg.Driver, 4)+")", resourceUpgradeTenant, userID, "owner", created)
	exec("INSERT INTO asset_transactions(id,tenant_id,occurred_at,source,created_by_user_id,created_at) VALUES ("+values(cfg.Driver, 6)+")", transactionID, resourceUpgradeTenant, created, "manual", userID, created)
	exec("INSERT INTO asset_events(id,tenant_id,asset_id,transaction_id,event_type,base_amount_minor,base_currency,notes,occurred_at,created_by_user_id,created_at) VALUES ("+values(cfg.Driver, 11)+")", "77777777-7777-4777-8777-777777777777", resourceUpgradeTenant, resourceUpgradeAsset, transactionID, "purchase", -12345, "CNY", "keep lifecycle history", created, userID, created)
	return db
}

func assertResourceUpgrade(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	var count int
	if driver == "sqlite" {
		if err := db.QueryRow("PRAGMA foreign_keys").Scan(&count); err != nil || count != 1 {
			t.Fatalf("foreign keys not restored after successful upgrade: %d %v", count, err)
		}
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM product_variants").Scan(&count); err != nil || count != 3 {
		t.Fatalf("want blank, Black, White variants: count=%d err=%v", count, err)
	}
	var originalColor string
	if err := db.QueryRow("SELECT color FROM product_variants WHERE id='" + resourceUpgradeVariant + "'").Scan(&originalColor); err != nil || originalColor != "" {
		t.Fatalf("original variant must remain the blank choice: %q %v", originalColor, err)
	}
	blackID := ""
	for i, raw := range resourceUpgradeColors {
		id := resourceUpgradeAsset
		if i > 0 {
			id = fmt.Sprintf("11000000-0000-4000-8000-%012d", i+1)
		}
		var variantID, rawColor, color, modelID string
		if err := db.QueryRow("SELECT a.variant_id,a.color,v.color,v.model_id FROM assets a JOIN product_variants v ON v.tenant_id=a.tenant_id AND v.id=a.variant_id WHERE a.id="+upgradePlaceholder(driver), id).Scan(&variantID, &rawColor, &color, &modelID); err != nil {
			t.Fatalf("preserved asset %s: %v", id, err)
		}
		wantColor := []string{"Black", "Black", "White", "", ""}[i]
		if rawColor != raw || color != wantColor || modelID != resourceUpgradeModel {
			t.Fatalf("asset %s changed: raw=%q color=%q model=%s", id, rawColor, color, modelID)
		}
		if color == "" && variantID != resourceUpgradeVariant {
			t.Fatal("blank asset no longer uses the original variant ID")
		}
		if i == 0 {
			blackID = variantID
		} else if i == 1 && variantID != blackID {
			t.Fatal("trim-equivalent colors must share one variant")
		}
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM assets").Scan(&count); err != nil || count != len(resourceUpgradeColors) {
		t.Fatalf("asset count changed: %d %v", count, err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM asset_events WHERE id='77777777-7777-4777-8777-777777777777' AND tenant_id='" + resourceUpgradeTenant + "' AND asset_id='" + resourceUpgradeAsset + "' AND transaction_id='99999999-9999-4999-8999-999999999999' AND event_type='purchase' AND base_amount_minor=-12345 AND base_currency='CNY' AND notes='keep lifecycle history'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("lifecycle identity/economic history changed: %d %v", count, err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM model_3d_resources r JOIN product_models m ON m.tenant_id=r.tenant_id AND m.model_3d_resource_id=r.id WHERE m.id='" + resourceUpgradeModel + "' AND r.status='ready' AND r.store_id=m.model_3d_store_id AND r.object_key=m.model_3d_object_key AND r.sha256=m.model_3d_sha256 AND r.size_bytes=m.model_3d_size_bytes AND r.source_url=m.model_3d_source_url AND r.author=m.model_3d_author AND r.license=m.model_3d_license AND r.updated_at=m.model_3d_updated_at AND r.store_id='legacy-local' AND r.object_key='" + resourceUpgradeKey + "' AND r.sha256='legacy-sha256' AND r.size_bytes=25 AND r.source_url='https://example.com/legacy' AND r.author='Legacy author' AND r.license='CC0'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("legacy GLB metadata not preserved: %d %v", count, err)
	}
}

func TestModel3DResourceMigrationFailureRollsBackAndRetries(t *testing.T) {
	for _, failure := range []string{"statement failure", "foreign key guard"} {
		t.Run(failure, func(t *testing.T) {
			cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "failed-resource-upgrade.db")}
			db := resourceUpgradeV10(t, cfg)
			inject := "CREATE TRIGGER fail_migration11 BEFORE UPDATE ON assets BEGIN SELECT RAISE(ABORT,'injected migration11 failure'); END"
			repair := "DROP TRIGGER fail_migration11"
			if failure == "foreign key guard" {
				inject = "PRAGMA foreign_keys=OFF; INSERT INTO item_categories(id,tenant_id,name,created_at) VALUES('bad-category','missing-tenant','Corrupt','2026-09-01T00:00:00Z'); PRAGMA foreign_keys=ON;"
				repair = "DELETE FROM item_categories WHERE id='bad-category'"
			}
			if _, err := db.Exec(inject); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if err := basestore.Migrate(context.Background(), db, cfg); err == nil {
					t.Fatal("invalid migration succeeded")
				}
				assertResourceMigrationRolledBack(t, db)
			}
			if _, err := db.Exec(repair); err != nil {
				t.Fatal(err)
			}
			if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
				t.Fatalf("retry after removing failure: %v", err)
			}
			assertResourceUpgrade(t, db, cfg.Driver)
		})
	}
}

func assertResourceMigrationRolledBack(t *testing.T, db *sql.DB) {
	t.Helper()
	for query, want := range map[string]int{
		"PRAGMA foreign_keys":                          1,
		"SELECT MAX(version_id) FROM goose_db_version": 10,
		"SELECT COUNT(*) FROM sqlite_master WHERE name IN ('model_3d_resources','product_variants_new')": 0,
		"SELECT COUNT(*) FROM pragma_table_info('product_variants') WHERE name='color'":                  0,
		"SELECT COUNT(*) FROM pragma_table_info('assets') WHERE name='model_3d_resource_id'":             0,
		"SELECT COUNT(*) FROM product_variants WHERE id='" + resourceUpgradeVariant + "'":                1,
		"SELECT COUNT(*) FROM assets WHERE variant_id='" + resourceUpgradeVariant + "'":                  len(resourceUpgradeColors),
	} {
		var got int
		if err := db.QueryRow(query).Scan(&got); err != nil || got != want {
			t.Fatalf("rollback check %s: got=%d want=%d err=%v", query, got, want, err)
		}
	}
}
