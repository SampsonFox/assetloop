package integration_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
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

type scenarioStore interface {
	application.Store
	application.AuthStore
	application.CatalogStore
}

func TestFullElementScenario(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "full-element.db")}
		db := openAndMigrate(t, cfg)
		runFullElementScenario(t, db, sqlite.New(db), cfg.Driver)
	})

	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("TEST_POSTGRES_DSN")
		if dsn == "" {
			if os.Getenv("REQUIRE_POSTGRES_TEST") == "true" {
				t.Fatal("TEST_POSTGRES_DSN is required for the UAT full-element scenario")
			}
			t.Skip("TEST_POSTGRES_DSN is not set")
		}
		cfg, cleanup := isolatedPostgres(t, dsn)
		defer cleanup()
		db := openAndMigrate(t, cfg)
		runFullElementScenario(t, db, postgres.New(db), cfg.Driver)
	})
}

func runFullElementScenario(t *testing.T, db *sql.DB, store scenarioStore, driver string) {
	t.Helper()
	ctx := context.Background()
	auth := application.NewAuthService(store)
	ownerSession, err := auth.Setup(ctx, application.SetupAuth{
		TenantName: "Full Element", BaseCurrency: "CNY", Username: "owner", Password: "owner secure password",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	owner, err := auth.Authenticate(ctx, ownerSession.Token)
	if err != nil {
		t.Fatalf("authenticate owner: %v", err)
	}
	if _, err := auth.AddMember(ctx, owner, application.AddMember{Username: "editor", Password: "editor secure password", Role: application.RoleEditor}); err != nil {
		t.Fatalf("add editor: %v", err)
	}
	if _, err := auth.AddMember(ctx, owner, application.AddMember{Username: "viewer", Password: "viewer secure password", Role: application.RoleViewer}); err != nil {
		t.Fatalf("add viewer: %v", err)
	}
	viewerSession, err := auth.Login(ctx, application.Login{Username: "viewer", Password: "viewer secure password"})
	if err != nil {
		t.Fatalf("login viewer: %v", err)
	}
	if _, err := auth.ListMembers(ctx, viewerSession.Principal); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("viewer should not list members, got %v", err)
	}

	catalog := application.NewCatalogService(store)
	category, err := catalog.CreateCategory(ctx, owner, "手机")
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	model, err := catalog.CreateModel(ctx, owner, application.CreateModel{CategoryID: category.ID, Name: "示例手机 Pro"})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}
	variant256, err := catalog.CreateVariant(ctx, owner, application.CreateVariant{ModelID: model.ID, Name: "256GB"})
	if err != nil {
		t.Fatalf("create 256GB variant: %v", err)
	}
	if _, err := catalog.CreateVariant(ctx, owner, application.CreateVariant{ModelID: model.ID, Name: "512GB"}); err != nil {
		t.Fatalf("create 512GB variant: %v", err)
	}
	asset, err := catalog.CreateAsset(ctx, owner, application.CreateCatalogAsset{
		VariantID: variant256.ID, DisplayName: "全要素测试手机", SerialNumber: "FULL-ELEMENT-001",
		Color: "钛金属", PurchaseChannel: "官方商城", Notes: "全要素目录记录",
	})
	if err != nil {
		t.Fatalf("create catalog asset: %v", err)
	}
	got, err := catalog.GetAsset(ctx, owner, asset.ID)
	if err != nil || got.DisplayName != "全要素测试手机" || got.SerialNumber != "FULL-ELEMENT-001" || got.Variant != "256GB" {
		t.Fatalf("get catalog asset: got=%+v err=%v", got, err)
	}
	snapshot, err := catalog.Snapshot(ctx, viewerSession.Principal)
	if err != nil || len(snapshot.Categories) != 1 || len(snapshot.Models) != 1 || len(snapshot.Variants) != 2 || len(snapshot.Assets) != 1 {
		t.Fatalf("viewer catalog snapshot: %+v err=%v", snapshot, err)
	}
	if _, err := catalog.CreateCategory(ctx, viewerSession.Principal, "禁止写入"); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("viewer should not mutate catalog, got %v", err)
	}
	var auditCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM security_audit_events WHERE tenant_id = "+placeholder(driver), owner.TenantID).Scan(&auditCount); err != nil {
		t.Fatalf("count security audit events: %v", err)
	}
	if auditCount < 4 {
		t.Fatalf("expected setup, two membership and login audit events, got %d", auditCount)
	}
}

func openAndMigrate(t *testing.T, cfg config.Database) *sql.DB {
	t.Helper()
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
		t.Fatal(err)
	}
	return db
}

func isolatedPostgres(t *testing.T, rawDSN string) (config.Database, func()) {
	t.Helper()
	parsed, err := url.Parse(rawDSN)
	if err != nil {
		t.Fatal(err)
	}
	schema := "assetloop_full_" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "_"))
	if len(schema) > 55 {
		schema = schema[:55]
	}
	admin, err := sql.Open("pgx", rawDSN)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(`CREATE SCHEMA "` + schema + `"`); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	cleanup := func() {
		_, _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, schema))
		_ = admin.Close()
	}
	return config.Database{Driver: "postgres", DSN: parsed.String()}, cleanup
}

func placeholder(driver string) string {
	if driver == "postgres" {
		return "$1"
	}
	return "?"
}
