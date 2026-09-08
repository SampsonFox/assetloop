package integration_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/config"
	"github.com/SampsonFox/assetloop/internal/store/postgres"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
)

func TestCatalogTransactionRollback(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			cfg := config.Database{Driver: driver, DSN: filepath.Join(t.TempDir(), "catalog-tx.db")}
			if driver == "postgres" {
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					if os.Getenv("REQUIRE_POSTGRES_TEST") == "true" {
						t.Fatal("PostgreSQL required")
					}
					t.Skip("TEST_POSTGRES_DSN is not set")
				}
				var cleanup func()
				cfg, cleanup = isolatedPostgres(t, dsn)
				defer cleanup()
			}
			db := openAndMigrate(t, cfg)
			var store scenarioStore = sqlite.New(db)
			if driver == "postgres" {
				store = postgres.New(db)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			account, err := application.NewAuthService(store).Setup(ctx, application.SetupAuth{TenantName: "Catalog transaction", BaseCurrency: "CNY", Username: "owner", Password: "test owner password"})
			if err != nil {
				t.Fatal(err)
			}
			catalog := application.NewCatalogService(store)
			original, err := catalog.CreateCategory(ctx, account.Principal, application.CreateCategory{Name: "Original", IconKey: "camera"})
			if err != nil {
				t.Fatal(err)
			}
			rollback := errors.New("rollback entire command")
			err = store.WithSpecificationWrite(ctx, account.Principal.TenantID, func(scoped application.SpecificationStore) error {
				service := application.NewCatalogService(scoped.(application.CatalogStore))
				if _, err := service.UpdateCategory(ctx, account.Principal, application.UpdateCategory{ID: original.ID, Name: "Changed", IconKey: "camera"}); err != nil {
					return err
				}
				category, err := service.CreateCategory(ctx, account.Principal, application.CreateCategory{Name: "Temporary", IconKey: "camera"})
				if err != nil {
					return err
				}
				if _, err := service.CreateModel(ctx, account.Principal, application.CreateModel{CategoryID: category.ID, Name: "Temporary model"}); err != nil {
					return err
				}
				categories, err := service.Categories(ctx, account.Principal)
				if err != nil {
					return err
				}
				if len(categories) != 2 {
					t.Errorf("transaction cannot read its writes")
				}
				return rollback
			})
			if !errors.Is(err, rollback) {
				t.Fatalf("transaction did not reach deliberate rollback: %v", err)
			}
			categories, err := catalog.Categories(ctx, account.Principal)
			if err != nil || len(categories) != 1 || categories[0].Name != "Original" {
				t.Fatalf("category changes escaped transaction: %v / %v", categories, err)
			}
			models, err := store.ListModels(ctx, account.Principal.TenantID)
			if err != nil || len(models) != 0 {
				t.Fatalf("model escaped transaction: %v / %v", models, err)
			}
			testManagementReplay(t, store.(application.ManagementStore), account.Principal)
		})
	}
}

type failedReceiptStore struct{ application.ManagementStore }

func (s failedReceiptStore) WithManagementWrite(ctx context.Context, tenant string, fn func(application.ManagementStore) error) error {
	return s.ManagementStore.WithManagementWrite(ctx, tenant, func(tx application.ManagementStore) error { return fn(failedReceiptStore{tx}) })
}
func (s failedReceiptStore) SaveManagementRequest(context.Context, application.ManagementRequest) error {
	return errors.New("injected receipt failure")
}

func testManagementReplay(t *testing.T, store application.ManagementStore, actor application.Principal) {
	t.Helper()
	ctx := context.Background()
	manager := application.NewManagementService(store)
	cmd := application.CreateCategory{Name: "Idempotent", IconKey: "camera"}
	first, err := manager.CreateCategory(ctx, actor, "category-key", cmd)
	if err != nil {
		t.Fatal(err)
	}
	again, err := manager.CreateCategory(ctx, actor, "category-key", cmd)
	if err != nil || first.ID != again.ID {
		t.Fatalf("replay changed identity: %v", err)
	}
	cmd.Name = "Different"
	if _, err := manager.CreateCategory(ctx, actor, "category-key", cmd); err == nil {
		t.Fatal("conflicting payload accepted")
	}
	if _, err := manager.CreateCategory(ctx, actor, "", cmd); err == nil {
		t.Fatal("empty key accepted")
	}
	before, err := store.ListCategories(ctx, actor.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.NewManagementService(failedReceiptStore{store}).CreateCategory(ctx, actor, "rollback-key", cmd); err == nil {
		t.Fatal("receipt failure ignored")
	}
	after, err := store.ListCategories(ctx, actor.TenantID)
	if err != nil || len(after) != len(before) {
		t.Fatal("business mutation survived receipt failure")
	}
	if _, found, err := store.FindManagementRequest(ctx, actor.TenantID, actor.UserID, "rollback-key"); err != nil || found {
		t.Fatal("failed receipt persisted")
	}
	if _, err := manager.CreateCategory(ctx, actor, "rollback-key", cmd); err != nil {
		t.Fatal("retry after rollback failed")
	}
	viewer := actor
	testSpecificationReplay(t, manager, store, actor, first.ID)
	viewer.Role = application.RoleViewer
	if _, err := manager.CreateCategory(ctx, viewer, "category-key", application.CreateCategory{Name: "Idempotent", IconKey: "camera"}); !errors.Is(err, application.ErrForbidden) {
		t.Fatal("replay bypassed current permission")
	}
}

func testSpecificationReplay(t *testing.T, manager *application.ManagementService, store application.ManagementStore, actor application.Principal, category string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	kind, err := manager.SaveType(ctx, actor, "type-key", application.SaveSpecificationType{Name: "Capacity", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	tag, err := manager.SaveTag(ctx, actor, "tag-key", application.SaveSpecificationTag{TypeID: kind.ID, Name: "256GB", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if retry, err := manager.SaveTag(ctx, actor, "tag-key", application.SaveSpecificationTag{TypeID: kind.ID, Name: "256GB", Enabled: true}); err != nil || retry.ID != tag.ID {
		t.Fatal("tag replay failed")
	}
	model, err := manager.CreateModel(ctx, actor, "spec-model-key", application.CreateModel{CategoryID: category, Name: "Tagged model"})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SaveModel(ctx, actor, "model-config-key", application.SaveModelSpecification{ModelID: model.ID, TagIDs: []string{tag.ID}}); err != nil {
		t.Fatal(err)
	}
	cmd := application.SaveSpecificationAsset{ModelID: model.ID, DisplayName: "Tagged item", TagIDs: []string{tag.ID}}
	asset, err := manager.SaveAsset(ctx, actor, "asset-key", cmd)
	if err != nil {
		t.Fatal(err)
	}
	if retry, err := manager.SaveAsset(ctx, actor, "asset-key", cmd); err != nil || retry.ID != asset.ID {
		t.Fatal("asset replay failed")
	}
	cmd.DisplayName = "Must roll back"
	if _, err := application.NewManagementService(failedReceiptStore{store}).SaveAsset(ctx, actor, "asset-rollback", cmd); err == nil {
		t.Fatal("asset receipt failure ignored")
	}
	assets, err := store.ListAssets(ctx, actor.TenantID)
	if err != nil || len(assets) != 1 {
		t.Fatalf("nested asset escaped rollback: %d / %v", len(assets), err)
	}
	if err := store.WithManagementWrite(ctx, actor.TenantID, func(tx application.ManagementStore) error {
		return tx.WithSpecificationWrite(ctx, "different-tenant", func(application.SpecificationStore) error {
			t.Error("cross-tenant nested transaction invoked")
			return nil
		})
	}); !errors.Is(err, application.ErrForbidden) {
		t.Fatal("nested tenant restriction missing")
	}
}
