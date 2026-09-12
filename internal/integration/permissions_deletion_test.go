package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/config"
	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/SampsonFox/assetloop/internal/store/postgres"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
	"github.com/google/uuid"
)

func TestPermissionsAndPermanentDeletion(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			cfg := config.Database{Driver: driver, DSN: filepath.Join(t.TempDir(), "permissions.db")}
			if driver == "postgres" {
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					if os.Getenv("REQUIRE_POSTGRES_TEST") == "true" {
						t.Fatal("PostgreSQL required")
					}
					t.Skip("TEST_POSTGRES_DSN not set")
				}
				var cleanup func()
				cfg, cleanup = isolatedPostgres(t, dsn)
				defer cleanup()
			}
			db := openAndMigrate(t, cfg)
			var st scenarioStore = sqlite.New(db)
			if driver == "postgres" {
				st = postgres.New(db)
			}
			credential, err := application.NewAuthService(st).Setup(context.Background(), application.SetupAuth{TenantName: "Permissions", BaseCurrency: "CNY", Username: "admin", Password: "admin test password"})
			if err != nil {
				t.Fatal(err)
			}
			runPermissionsDeletion(t, db, st, credential.Principal, driver)
		})
	}
}

func runPermissionsDeletion(t *testing.T, db *sql.DB, st scenarioStore, admin application.Principal, driver string) {
	t.Helper()
	ctx := context.Background()
	auth := application.NewAuthService(st)
	catalog := application.NewCatalogService(st)
	spec := application.NewSpecificationService(st)
	life := application.NewLifecycleService(st)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	worker, err := auth.AddMember(ctx, admin, application.AddMember{Username: "boundary-worker", Password: "worker test password", Role: application.RoleEditor})
	must(err)
	session, err := auth.Login(ctx, application.Login{Username: worker.Username, Password: "worker test password"})
	must(err)
	actor := session.Principal
	category, err := catalog.CreateCategory(ctx, admin, application.CreateCategory{Name: "Boundary category", IconKey: "camera"})
	must(err)
	model, err := catalog.CreateModel(ctx, admin, application.CreateModel{CategoryID: category.ID, Name: "Boundary model"})
	must(err)
	if _, err := catalog.CreateCategory(ctx, actor, application.CreateCategory{Name: "Forbidden", IconKey: "camera"}); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("worker catalog write: %v", err)
	}
	if _, err := life.CreateEventType(ctx, actor, application.CreateAssetEventType{Name: "Forbidden", Cashflow: domain.AssetEventExpense}); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("worker event type write: %v", err)
	}
	management := application.NewManagementService(st.(application.ManagementStore), nil)
	cmd := application.SaveSpecificationAsset{ModelID: model.ID, DisplayName: "Purge this test item"}
	asset, err := management.SaveAsset(ctx, actor, "create-purge-item", cmd)
	must(err)
	survivor, err := spec.SaveAsset(ctx, actor, application.SaveSpecificationAsset{ModelID: model.ID, DisplayName: "Retained item"})
	must(err)
	record := application.RecordEvent{AssetID: asset.ID, Type: domain.AssetEventPurchase, AmountMinor: 10000, Currency: "CNY", OccurredAt: time.Now().UTC(), RequestKey: "purchase-purge-item"}
	purchase, err := life.Record(ctx, actor, record)
	must(err)
	repair := record
	repair.Type = domain.AssetEventRepair
	repair.AmountMinor = 300
	repair.RequestKey = "repair-purge-item"
	event, err := life.Record(ctx, actor, repair)
	must(err)
	correction := repair
	correction.AmountMinor = 400
	correction.RequestKey = "correct-purge-item"
	_, err = life.Correct(ctx, actor, event.ID, correction)
	must(err)
	sale := record
	sale.Type = domain.AssetEventSale
	sale.AmountMinor = 7000
	sale.RequestKey = "sale-purge-item"
	_, err = life.Record(ctx, actor, sale)
	must(err)
	for _, role := range []application.Role{application.RoleViewer, application.RoleEditor} {
		p := actor
		p.Role = role
		if err := spec.DeleteAsset(ctx, p, asset.ID); !errors.Is(err, application.ErrForbidden) {
			t.Fatalf("%s deleted item: %v", role, err)
		}
	}
	foreign := admin
	foreign.TenantID = uuid.NewString()
	if err := spec.DeleteAsset(ctx, foreign, asset.ID); err == nil {
		t.Fatal("cross-tenant deletion")
	}
	// A shared transaction must survive with the other item's event.
	sharedID := uuid.NewString()
	query := `INSERT INTO asset_events (id,tenant_id,asset_id,transaction_id,event_type,base_amount_minor,base_currency,notes,occurred_at,created_by_user_id,created_at,event_type_id) SELECT ` + placeholder(driver) + `,tenant_id,`
	if driver == "postgres" {
		query += `$2`
	} else {
		query += `?`
	}
	query += `,transaction_id,event_type,base_amount_minor,base_currency,notes,occurred_at,created_by_user_id,created_at,event_type_id FROM asset_events WHERE id=`
	if driver == "postgres" {
		query += `$3`
	} else {
		query += `?`
	}
	_, err = db.ExecContext(ctx, query, sharedID, survivor.ID, purchase.ID)
	must(err)
	// Failure after deleting child rows must roll the entire transaction back.
	if driver == "sqlite" {
		_, err = db.Exec(`CREATE TRIGGER test_block_purge BEFORE DELETE ON assets BEGIN SELECT RAISE(ABORT,'test rollback'); END`)
		must(err)
	} else {
		_, err = db.Exec(`CREATE FUNCTION test_block_purge() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'test rollback'; END; $$ LANGUAGE plpgsql; CREATE TRIGGER test_block_purge BEFORE DELETE ON assets FOR EACH ROW EXECUTE FUNCTION test_block_purge()`)
		must(err)
	}
	if err := spec.DeleteAsset(ctx, admin, asset.ID); err == nil {
		t.Fatal("expected rollback")
	}
	if driver == "sqlite" {
		_, err = db.Exec(`DROP TRIGGER test_block_purge`)
	} else {
		_, err = db.Exec(`DROP TRIGGER test_block_purge ON assets; DROP FUNCTION test_block_purge()`)
	}
	must(err)
	timeline, _, err := life.Timeline(ctx, actor, asset.ID)
	must(err)
	if len(timeline) != 5 {
		t.Fatalf("partial deletion: %d events", len(timeline))
	}
	must(spec.DeleteAsset(ctx, admin, asset.ID))
	must(spec.DeleteAsset(ctx, admin, asset.ID))
	if _, err := spec.Asset(ctx, admin, asset.ID); err == nil {
		t.Fatal("deleted item still readable")
	}
	_, err = spec.Asset(ctx, actor, survivor.ID)
	must(err)
	_, err = st.GetProductModel(ctx, admin.TenantID, model.ID)
	must(err)
	var count int
	must(db.QueryRow(`SELECT COUNT(*) FROM asset_events WHERE asset_id=`+placeholder(driver), asset.ID).Scan(&count))
	if count != 0 {
		t.Fatal("events remain")
	}
	must(db.QueryRow(`SELECT COUNT(*) FROM asset_transactions WHERE id=`+placeholder(driver), purchase.TransactionID).Scan(&count))
	if count != 1 {
		t.Fatal("shared transaction removed")
	}
	if _, err := management.SaveAsset(ctx, actor, "create-purge-item", cmd); err == nil {
		t.Fatal("deleted creation receipt replayed")
	}
	record.AssetID = survivor.ID
	if _, err := life.Record(ctx, actor, record); err == nil {
		t.Fatal("deleted request key reused")
	}
	must(auth.ChangeMemberRole(ctx, admin, worker.UserID, application.RoleViewer))
	current, err := auth.Authenticate(ctx, session.Token)
	must(err)
	if current.Role != application.RoleViewer {
		t.Fatal("session retained stale role")
	}
	if _, err := spec.SaveAsset(ctx, current, cmd); !errors.Is(err, application.ErrForbidden) {
		t.Fatal("demoted session can write")
	}
	if err := auth.ChangeMemberRole(ctx, admin, admin.UserID, application.RoleViewer); err == nil {
		t.Fatal("last administrator demoted")
	}
	must(auth.ChangeMemberRole(ctx, admin, worker.UserID, application.RoleOwner))
	other, err := auth.Authenticate(ctx, session.Token)
	must(err)
	// Two administrators racing to demote themselves cannot remove both admins.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, p := range []application.Principal{admin, other} {
		wg.Add(1)
		go func(p application.Principal) {
			defer wg.Done()
			results <- auth.ChangeMemberRole(ctx, p, p.UserID, application.RoleEditor)
		}(p)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent demotions: %d succeeded", success)
	}
	must(db.QueryRow(`SELECT COUNT(*) FROM tenant_memberships WHERE role='owner' AND tenant_id=`+placeholder(driver), admin.TenantID).Scan(&count))
	if count != 1 {
		t.Fatal("administrator invariant lost")
	}
}
