package web

import (
	"context"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/config"
	"github.com/SampsonFox/assetloop/internal/domain"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEventTypeManagementHTTP(t *testing.T) {
	ctx := context.Background()
	cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "retry.db")}
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := basestore.Migrate(ctx, db, cfg); err != nil {
		t.Fatal(err)
	}
	adapter := sqlite.New(db)
	auth := application.NewAuthService(adapter)
	credential, err := auth.Setup(ctx, application.SetupAuth{TenantName: "Retry", BaseCurrency: "CNY", Username: "owner", Password: "owner secure password"})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.Authenticate(ctx, credential.Token)
	if err != nil {
		t.Fatal(err)
	}
	catalog := application.NewCatalogService(adapter)
	category, err := catalog.CreateCategory(ctx, owner, application.CreateCategory{Name: "Camera"})
	if err != nil {
		t.Fatal(err)
	}
	model, err := catalog.CreateModel(ctx, owner, application.CreateModel{CategoryID: category.ID, Name: "Camera model"})
	if err != nil {
		t.Fatal(err)
	}
	variant, err := catalog.CreateVariant(ctx, owner, application.CreateVariant{ModelID: model.ID, Name: "Body"})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := catalog.CreateAsset(ctx, owner, application.CreateCatalogAsset{VariantID: variant.ID, DisplayName: "Camera"})
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := application.NewLifecycleService(adapter)
	server, err := New(auth, catalog, lifecycle, db, Options{AuthMode: "local"})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	session := &http.Cookie{Name: sessionCookie, Value: credential.Token}
	page := request(t, handler, http.MethodGet, "/assets/"+asset.ID, nil, []*http.Cookie{session})
	csrf := responseCookie(t, page, csrfCookie)
	cookies := []*http.Cookie{session, csrf}

	manage := request(t, handler, "GET", "/admin/event-types", nil, cookies)
	if manage.Code != 200 || !strings.Contains(manage.Body.String(), "生命周期类型管理") || strings.Contains(manage.Body.String(), "asset.event_type_cashflow") {
		t.Fatalf("management page: %d %s", manage.Code, manage.Body.String())
	}
	created := request(t, handler, "POST", "/admin/event-types", url.Values{"csrf_token": {csrf.Value}, "name": {"HTTP custom"}, "cashflow": {"expense"}}, cookies)
	if created.Code != 303 {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	types, err := lifecycle.EventTypes(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	id := ""
	for _, item := range types {
		if item.Name == "HTTP custom" {
			id = item.ID
		}
	}
	if id == "" {
		t.Fatal("type not created")
	}
	target := "/admin/event-types/" + id
	denied := request(t, handler, "POST", target, url.Values{"name": {"bad"}, "cashflow": {"expense"}}, cookies)
	if denied.Code != 403 {
		t.Fatal("missing CSRF accepted")
	}
	event, err := lifecycle.Record(ctx, owner, application.RecordEvent{AssetID: asset.ID, TypeID: id, AmountMinor: 100, Currency: "CNY", OccurredAt: time.Now().Add(-time.Hour), Source: "HTTP test"})
	if err != nil {
		t.Fatal(err)
	}
	updated := request(t, handler, "POST", target, url.Values{"csrf_token": {csrf.Value}, "name": {"Renamed via HTTP"}, "cashflow": {"expense"}}, cookies)
	if updated.Code != 303 {
		t.Fatalf("update: %d %s", updated.Code, updated.Body.String())
	}
	detail := request(t, handler, "GET", "/assets/"+asset.ID+"?event_type="+id, nil, cookies)
	if detail.Code != 200 || !strings.Contains(detail.Body.String(), "Renamed via HTTP") {
		t.Fatal("history did not resolve current name")
	}
	locked := request(t, handler, "POST", target, url.Values{"csrf_token": {csrf.Value}, "name": {"Wrong"}, "cashflow": {"income"}}, cookies)
	if locked.Code != 422 || !strings.Contains(locked.Body.String(), "不能修改收支方向") {
		t.Fatal("used direction not rejected")
	}
	edit := request(t, handler, "GET", "/admin/event-types?edit_type_id="+id+"&dialog=event-type-manage", nil, cookies)
	if edit.Code != 200 || !strings.Contains(edit.Body.String(), "已有记录，收支方向不可修改") {
		t.Fatalf("edit drawer: %d %s", edit.Code, edit.Body.String())
	}
	for _, state := range []string{"0", "1"} {
		response := request(t, handler, "POST", target+"/status", url.Values{"csrf_token": {csrf.Value}, "enabled": {state}}, cookies)
		if response.Code != 303 {
			t.Fatalf("status: %d %s", response.Code, response.Body.String())
		}
		detail = request(t, handler, "GET", "/assets/"+asset.ID, nil, cookies)
		options := strings.Split(strings.Split(detail.Body.String(), "<select id=\"event-type-select\"")[1], "</select>")[0]
		if strings.Contains(options, id) != (state == "1") {
			t.Fatal("disabled type still available for creation")
		}
		correction := request(t, handler, "GET", "/events/"+event.ID+"/correct", nil, cookies)
		if correction.Code != 200 {
			t.Fatal("historical correction unavailable")
		}
	}
	viewer, err := auth.AddMember(ctx, owner, application.AddMember{Username: "type-viewer", Password: "viewer secure password", Role: application.RoleViewer})
	_ = viewer
	if err != nil {
		t.Fatal(err)
	}
	viewerCredential, err := auth.Login(ctx, application.Login{Username: "type-viewer", Password: "viewer secure password"})
	if err != nil {
		t.Fatal(err)
	}
	vc := []*http.Cookie{{Name: sessionCookie, Value: viewerCredential.Token}, csrf}
	readonly := request(t, handler, "GET", "/admin/event-types", nil, vc)
	if readonly.Code != 200 || strings.Contains(readonly.Body.String(), "data-dialog-open=\"event-type-manage\"") {
		t.Fatal("viewer management page is not read-only")
	}
	forbidden := request(t, handler, "POST", target, url.Values{"csrf_token": {csrf.Value}, "name": {"Denied"}, "cashflow": {string(domain.AssetEventExpense)}}, vc)
	if forbidden.Code != 403 {
		t.Fatal("viewer mutation accepted")
	}
}
