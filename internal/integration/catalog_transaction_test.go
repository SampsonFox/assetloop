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
		})
	}
}
