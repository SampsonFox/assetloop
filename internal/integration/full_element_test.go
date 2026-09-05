package integration_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/blob"
	localblob "github.com/SampsonFox/assetloop/internal/blob/local"
	"github.com/SampsonFox/assetloop/internal/config"
	"github.com/SampsonFox/assetloop/internal/domain"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/postgres"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
)

type scenarioStore interface {
	application.Store
	application.AuthStore
	application.CatalogStore
	application.LifecycleStore
	application.ModelMediaStore
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
	owner, err = auth.UpdatePreferences(ctx, owner, application.UpdatePreferences{Locale: application.LocaleEn, Theme: application.ThemeDark, Accent: application.AccentRose})
	if err != nil {
		t.Fatalf("update owner preferences: %v", err)
	}
	reauthenticated, err := auth.Login(ctx, application.Login{Username: "owner", Password: "owner secure password"})
	if err != nil || reauthenticated.Principal.Locale != application.LocaleEn || reauthenticated.Principal.Theme != application.ThemeDark || reauthenticated.Principal.Accent != application.AccentRose {
		t.Fatalf("preferences did not survive reauthentication: principal=%+v err=%v", reauthenticated.Principal, err)
	}
	owner = reauthenticated.Principal
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
	if _, err := auth.ListMembers(ctx, viewerSession.Principal, application.MemberListOptions{}); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("viewer should not list members, got %v", err)
	}

	catalog := application.NewCatalogService(store)
	category, err := catalog.CreateCategory(ctx, owner, application.CreateCategory{Name: "手机", IconKey: "smartphone"})
	if err != nil {
		t.Fatalf("create category: %v", err)
	}
	model, err := catalog.CreateModel(ctx, owner, application.CreateModel{CategoryID: category.ID, Name: "示例手机 Pro"})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}
	variant256, err := catalog.CreateVariant(ctx, owner, application.CreateVariant{ModelID: model.ID, Name: "256GB", Color: "钛金属"})
	if err != nil {
		t.Fatalf("create 256GB variant: %v", err)
	}
	if _, err := catalog.CreateVariant(ctx, owner, application.CreateVariant{ModelID: model.ID, Name: "512GB"}); err != nil {
		t.Fatalf("create 512GB variant: %v", err)
	}
	blackVariant, err := catalog.CreateVariant(ctx, owner, application.CreateVariant{ModelID: model.ID, Name: "256GB", Color: "黑色"})
	if err != nil || blackVariant.ID == variant256.ID {
		t.Fatalf("create distinct color variant: %+v err=%v", blackVariant, err)
	}
	if _, err := catalog.CreateVariant(ctx, owner, application.CreateVariant{ModelID: model.ID, Name: "256GB", Color: "钛金属"}); err == nil {
		t.Fatal("duplicate model/name/color variant was accepted")
	}
	asset, err := catalog.CreateAsset(ctx, owner, application.CreateCatalogAsset{
		VariantID: variant256.ID, DisplayName: "全要素测试手机", SerialNumber: "FULL-ELEMENT-001",
		PurchaseChannel: "官方商城", Notes: "全要素目录记录",
	})
	if err != nil {
		t.Fatalf("create catalog asset: %v", err)
	}
	got, err := catalog.GetAsset(ctx, owner, asset.ID)
	if err != nil || got.DisplayName != "全要素测试手机" || got.SerialNumber != "FULL-ELEMENT-001" || got.Variant != "256GB" || got.Color != "钛金属" {
		t.Fatalf("get catalog asset: got=%+v err=%v", got, err)
	}
	snapshot, err := catalog.Snapshot(ctx, viewerSession.Principal)
	if err != nil || len(snapshot.Categories) != 1 || len(snapshot.Models) != 1 || len(snapshot.Variants) != 3 || len(snapshot.Assets) != 1 {
		t.Fatalf("viewer catalog snapshot: %+v err=%v", snapshot, err)
	}
	if _, err := catalog.CreateCategory(ctx, viewerSession.Principal, application.CreateCategory{Name: "禁止写入"}); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("viewer should not mutate catalog, got %v", err)
	}
	localStore, err := localblob.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	modelMedia := application.NewModelMediaService(store, blob.Registry{"local": localStore}, blob.ObjectKeyMapper{}, "local")
	glb := fullElementGLB()
	media, err := modelMedia.Update(ctx, owner, application.UpdateProductModel3D{ModelID: model.ID, File: glb, SourceURL: "https://example.com/source", License: "CC0"})
	if err != nil {
		t.Fatalf("upload product model GLB: %v", err)
	}
	if media.ResourceID == "" || !strings.HasPrefix(media.ObjectKey, "tenants/"+owner.TenantID+"/") {
		t.Fatalf("unexpected model object key: %q", media.ObjectKey)
	}
	opened, err := modelMedia.OpenForAsset(ctx, viewerSession.Principal, asset.ID)
	if err != nil {
		t.Fatalf("viewer open product model GLB: %v", err)
	}
	modelBytes, readErr := io.ReadAll(opened.Reader)
	closeErr := opened.Reader.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(modelBytes, glb) {
		t.Fatalf("resolved model GLB differs: read=%v close=%v", readErr, closeErr)
	}

	// One resource can serve unrelated models and variants; each asset may override it.
	sharedModel, err := catalog.CreateModel(ctx, owner, application.CreateModel{CategoryID: category.ID, Name: "共享资源手机"})
	if err != nil {
		t.Fatalf("create sharing model: %v", err)
	}
	sharedVariant, err := catalog.CreateVariant(ctx, owner, application.CreateVariant{ModelID: sharedModel.ID, Name: "128GB", Color: "银色"})
	if err != nil {
		t.Fatalf("create sharing variant: %v", err)
	}
	sharedAsset, err := catalog.CreateAsset(ctx, owner, application.CreateCatalogAsset{VariantID: sharedVariant.ID, DisplayName: "共享模型资产"})
	if err != nil {
		t.Fatalf("create sharing asset: %v", err)
	}
	bind := func(kind, targetID, resourceID string) {
		t.Helper()
		if err := modelMedia.Bind(ctx, owner, application.BindModel3DResource{Kind: kind, TargetID: targetID, ResourceID: resourceID}); err != nil {
			t.Fatalf("bind %s resource %q: %v", kind, resourceID, err)
		}
	}
	assertResource := func(assetID, resourceID string) {
		t.Helper()
		// A fresh service must resolve the persisted reference and read its actual bytes.
		service := application.NewModelMediaService(store, blob.Registry{"local": localStore}, blob.ObjectKeyMapper{}, "local")
		resolved, err := service.ResolveForAsset(ctx, viewerSession.Principal, assetID)
		if err != nil || resolved.ResourceID != resourceID {
			t.Fatalf("resolve asset %s: resource=%+v want=%s err=%v", assetID, resolved, resourceID, err)
		}
		opened, err := service.OpenForAsset(ctx, viewerSession.Principal, assetID)
		if err != nil {
			t.Fatalf("open resolved resource: %v", err)
		}
		data, readErr := io.ReadAll(opened.Reader)
		closeErr := opened.Reader.Close()
		if readErr != nil || closeErr != nil || opened.Model.ResourceID != resourceID || !bytes.Equal(data, glb) {
			t.Fatalf("read resolved resource: resource=%s read=%v close=%v", opened.Model.ResourceID, readErr, closeErr)
		}
	}
	assertReferenced := func(resourceID string) {
		t.Helper()
		if err := modelMedia.DeleteResource(ctx, owner, resourceID); !errors.Is(err, application.ErrModel3DReferenced) {
			t.Fatalf("delete referenced resource %s: want reference rejection, got %v", resourceID, err)
		}
		if resource, err := modelMedia.GetResource(ctx, viewerSession.Principal, resourceID); err != nil || resource.Status != "ready" {
			t.Fatalf("rejected deletion damaged resource: status=%q err=%v", resource.Status, err)
		}
	}
	bind("model", sharedModel.ID, media.ResourceID)
	assertResource(sharedAsset.ID, media.ResourceID)
	assertReferenced(media.ResourceID)

	variantResource, err := modelMedia.Upload(ctx, owner, application.UploadModel3DResource{Name: "变体共享资源", File: glb, License: "CC0"})
	if err != nil {
		t.Fatalf("upload variant resource: %v", err)
	}
	bind("variant", variant256.ID, variantResource.ID)
	bind("variant", blackVariant.ID, variantResource.ID)
	assertResource(asset.ID, variantResource.ID)
	assertReferenced(variantResource.ID)
	override, err := modelMedia.Upload(ctx, owner, application.UploadModel3DResource{Name: "资产独立资源", File: glb, License: "CC0"})
	if err != nil {
		t.Fatalf("upload asset override: %v", err)
	}
	bind("asset", asset.ID, override.ID)
	assertResource(asset.ID, override.ID)
	assertResource(sharedAsset.ID, media.ResourceID)
	assertReferenced(override.ID)
	assertResource(asset.ID, override.ID)
	bind("asset", asset.ID, "")
	assertResource(asset.ID, variantResource.ID)
	if err := modelMedia.DeleteResource(ctx, owner, override.ID); err != nil {
		t.Fatalf("delete unbound override: %v", err)
	}
	if _, err := modelMedia.GetResource(ctx, owner, override.ID); err == nil {
		t.Fatal("deleted resource remains readable")
	}
	if reader, _, err := localStore.Open(ctx, override.ObjectKey); err == nil {
		_ = reader.Close()
		t.Fatal("deleted resource bytes remain readable")
	}
	bind("variant", variant256.ID, "")
	assertResource(asset.ID, media.ResourceID)
	assertReferenced(variantResource.ID) // The other variant still references it.
	bind("variant", blackVariant.ID, "")
	if err := modelMedia.DeleteResource(ctx, owner, variantResource.ID); err != nil {
		t.Fatalf("delete fully unbound variant resource: %v", err)
	}
	bind("model", model.ID, "")
	if _, err := modelMedia.OpenForAsset(ctx, viewerSession.Principal, asset.ID); !errors.Is(err, application.ErrModel3DNotFound) {
		t.Fatalf("unbound asset should have no resource: %v", err)
	}
	assertReferenced(media.ResourceID) // The other model still references it.
	assertResource(sharedAsset.ID, media.ResourceID)
	bind("model", model.ID, media.ResourceID)
	assertResource(asset.ID, media.ResourceID)

	lifecycle := application.NewLifecycleService(store)
	purchase, err := lifecycle.Record(ctx, owner, application.RecordEvent{
		RequestKey: "full-element-purchase",
		AssetID:    asset.ID, Type: domain.AssetEventPurchase, AmountMinor: 100_000, Currency: "USD",
		OccurredAt: time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC), Source: "ai-harness",
		ExternalReference: "ORDER-FULL-001", Notes: "user-confirmed foreign purchase",
		FXRateScaled: 712_000_000, FXRateDate: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		FXRateSource: "full-element-fixture", FXConfirmed: true,
	})
	if err != nil {
		t.Fatalf("record Agent-confirmed foreign purchase: %v", err)
	}
	if purchase.BaseAmountMinor != -712_000 || purchase.FX == nil || purchase.FX.OriginalAmountMinor != 100_000 || purchase.FX.OriginalCurrency != "USD" {
		t.Fatalf("purchase money evidence mismatch: %+v", purchase)
	}
	retry, err := application.NewLifecycleService(store).Record(ctx, owner, application.RecordEvent{
		RequestKey: "full-element-purchase", AssetID: asset.ID, Type: domain.AssetEventPurchase, AmountMinor: 100_000, Currency: "USD",
		OccurredAt: time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC), Source: "ai-harness", ExternalReference: "ORDER-FULL-001", Notes: "user-confirmed foreign purchase",
		FXRateScaled: 712_000_000, FXRateDate: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), FXRateSource: "full-element-fixture", FXConfirmed: true,
	})
	if err != nil || retry.ID != purchase.ID {
		t.Fatalf("confirmed purchase retry: %+v %v", retry, err)
	}
	_, locked, err := lifecycle.BaseCurrency(ctx, owner)
	if err != nil || !locked {
		t.Fatalf("base currency should lock after Agent-confirmed purchase: locked=%v err=%v", locked, err)
	}
	repair, err := lifecycle.Record(ctx, owner, application.RecordEvent{
		AssetID: asset.ID, Type: domain.AssetEventRepair, AmountMinor: 20_000, Currency: "CNY",
		OccurredAt: time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC), Source: "manual", Notes: "initial repair amount",
	})
	if err != nil {
		t.Fatalf("record repair: %v", err)
	}
	if _, err := lifecycle.Correct(ctx, owner, repair.ID, application.RecordEvent{
		AmountMinor: 15_000, Currency: "CNY", OccurredAt: time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC),
		Source: "manual-correction", Notes: "corrected repair amount",
	}); err != nil {
		t.Fatalf("correct repair: %v", err)
	}
	if _, err := lifecycle.Record(ctx, owner, application.RecordEvent{
		AssetID: asset.ID, Type: domain.AssetEventSale, AmountMinor: 800_000, Currency: "CNY",
		OccurredAt: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC), Source: "manual", ExternalReference: "SALE-FULL-001", Notes: "sold",
	}); err != nil {
		t.Fatalf("record sale: %v", err)
	}
	events, summary, err := lifecycle.Timeline(ctx, viewerSession.Principal, asset.ID)
	if err != nil {
		t.Fatalf("viewer lifecycle timeline: %v", err)
	}
	if len(events) != 5 || summary.ExpenseMinor != 727_000 || summary.IncomeMinor != 800_000 || summary.NetCashflowMinor != 73_000 || summary.Status != "sold" {
		t.Fatalf("full lifecycle mismatch: events=%d summary=%+v", len(events), summary)
	}
	if _, err := lifecycle.Record(ctx, viewerSession.Principal, application.RecordEvent{}); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("viewer should not mutate lifecycle, got %v", err)
	}
	cost, err := lifecycle.CostDashboard(ctx, viewerSession.Principal, asset.ID)
	if err != nil || cost.NetMinor != -73_000 || !cost.Sold || !cost.HasDuration || len(cost.Points) != 3 || len(cost.Categories) != 2 {
		t.Fatalf("full cost dashboard mismatch: %+v %v", cost, err)
	}
	outsider := viewerSession.Principal
	outsider.TenantID = "00000000-0000-4000-8000-000000000099"
	if _, err := lifecycle.CostDashboard(ctx, outsider, asset.ID); err == nil {
		t.Fatal("cost dashboard leaked across tenants")
	}
	filtered, err := lifecycle.TimelinePage(ctx, viewerSession.Principal, asset.ID, application.EventListOptions{Type: "sale", Page: 1, PageSize: 1})
	if err != nil || len(filtered.Events) != 1 {
		t.Fatalf("filtered timeline: %v", err)
	}
	again, err := lifecycle.CostDashboard(ctx, viewerSession.Principal, asset.ID)
	if err != nil || again.NetMinor != cost.NetMinor || len(again.Points) != len(cost.Points) {
		t.Fatal("cost dashboard depends on pagination")
	}
	var auditCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM security_audit_events WHERE tenant_id = "+placeholder(driver), owner.TenantID).Scan(&auditCount); err != nil {
		t.Fatalf("count security audit events: %v", err)
	}
	if auditCount < 4 {
		t.Fatalf("expected setup, two membership and login audit events, got %d", auditCount)
	}
}

func fullElementGLB() []byte {
	jsonData := []byte(`{"asset":{"version":"2.0"}}`)
	for len(jsonData)%4 != 0 {
		jsonData = append(jsonData, ' ')
	}
	data := make([]byte, 20+len(jsonData))
	copy(data, "glTF")
	binary.LittleEndian.PutUint32(data[4:], 2)
	binary.LittleEndian.PutUint32(data[8:], uint32(len(data)))
	binary.LittleEndian.PutUint32(data[12:], uint32(len(jsonData)))
	binary.LittleEndian.PutUint32(data[16:], 0x4e4f534a)
	copy(data[20:], jsonData)
	return data
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
