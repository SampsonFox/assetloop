package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/config"
	"github.com/SampsonFox/assetloop/internal/domain"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
)

func TestDrawerAssetAndLifecycleFragments(t *testing.T) {
	s, err := New(nil, nil, nil, nil, Options{Specifications: application.NewSpecificationService(nil)})
	if err != nil {
		t.Fatal(err)
	}
	data := pageData{Strings: stringsFor(application.LocaleEn), CanManageCatalog: true, CanManageLifecycle: true, BaseCurrency: "CNY",
		Asset:           &domain.Asset{ID: "asset-a", ModelID: "model-a", CategoryID: "category-a", DisplayName: "My camera"},
		Models:          []domain.ProductModel{{ID: "model-a", CategoryID: "category-a", CategoryName: "Cameras", Name: "Camera"}},
		AssetFormAction: "/assets/asset-a", AssetFormEditing: true, CSRFToken: "test-csrf",
		EditingEventType: &domain.AssetEventTypeDefinition{ID: "type-a", Name: "Repair", Cashflow: domain.AssetEventExpense, ReferenceCount: 2},
		EventTypeForm:    eventTypeFormData{Name: "Repair", Cashflow: "expense"},
	}
	render := func(page, target string) string {
		t.Helper()
		var out bytes.Buffer
		if err := s.templates[page].ExecuteTemplate(&out, target, data); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	form := render("asset_form", "fragment-asset-editor")
	for _, want := range []string{`id="asset-editor"`, `action="/assets/asset-a"`, `name="csrf_token" value="test-csrf"`, `data-category-name="Cameras" selected`, `edit_model_id=model-a`, `data-drawer-target="model-drawer"`, `data-guard-dirty`} {
		if !strings.Contains(form, want) {
			t.Errorf("asset fragment missing %s", want)
		}
	}
	if strings.Contains(form, `<script`) || strings.Contains(form, `id="model-drawer"`) {
		t.Fatal("asset fragment includes unrelated drawers/scripts")
	}
	if strings.Contains(form, `<h1>`) || strings.Contains(form, `class="editor-actions"`) || !strings.Contains(form, `form="asset-form"`) {
		t.Fatal("fragment must keep one fixed heading with its associated save button")
	}
	normal := render("asset_form", "content")
	if !strings.Contains(normal, `id="asset-form"`) || !strings.Contains(normal, `/static/asset-tags.js`) {
		t.Fatal("normal page lost its form or enhancement")
	}
	managed := render("event_types", "fragment-event-type-manage")
	if !strings.Contains(managed, `action="/admin/event-types/type-a"`) || !strings.Contains(managed, `type="hidden" name="cashflow" value="expense"`) {
		t.Fatal("used type loses identity/direction lock")
	}
	data.Model3D = &domain.ProductModel3D{SHA256: "test-digest"}
	compact := render("asset", "fragment-asset-detail")
	for _, unwanted := range []string{`<script`, `href="/"`, `lifecycle-timeline`, `data-cost-dashboard`, `id="event-drawer"`, `id="event-type-drawer"`} {
		if strings.Contains(compact, unwanted) {
			t.Errorf("compact detail contains %s", unwanted)
		}
	}
	for _, want := range []string{`data-model-viewer`, `data-model-url="/assets/asset-a/model.glb?v=test-digest"`, `data-drawer-target="asset-editor"`, `data-drawer-target="model-drawer"`} {
		if !strings.Contains(compact, want) {
			t.Errorf("compact detail missing %s", want)
		}
	}
	full := render("asset", "content")
	for _, want := range []string{`lifecycle-timeline`, `data-cost-dashboard`, `/static/asset-model-viewer.js`, `id="event-drawer"`} {
		if !strings.Contains(full, want) {
			t.Errorf("ordinary detail missing %s", want)
		}
	}
	data.EditingEventType.BuiltIn = true
	if !strings.Contains(render("event_types", "fragment-event-type-manage"), "Built-in types cannot be edited or disabled.") {
		t.Fatal("built-in type detail must explain why editing is unavailable")
	}
	if strings.Contains(render("event_types", "fragment-event-type-manage"), `<form`) {
		t.Fatal("built-in type exposed mutation form")
	}
	data.CanManageCatalog, data.CanManageLifecycle = false, false
	data.EditingEventType.BuiltIn = false
	if strings.Contains(render("event_types", "fragment-event-type-manage"), `<form`) {
		t.Fatal("viewer exposed lifecycle mutation form")
	}
	if strings.Contains(render("asset_form", "fragment-asset-editor"), `<form`) {
		t.Fatal("viewer exposed asset mutation form")
	}
	detail := render("asset", "fragment-asset-detail")
	if !strings.Contains(detail, `id="asset-detail"`) || strings.Contains(detail, `data-drawer-target="asset-editor"`) || strings.Contains(detail, `id="event-form"`) {
		t.Fatal("viewer detail not read-only")
	}
}

func TestDrawerAssetAndLifecycleHTTP(t *testing.T) {
	ctx := context.Background()
	cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "drawers.db")}
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := basestore.Migrate(ctx, db, cfg); err != nil {
		t.Fatal(err)
	}
	adapter := sqlite.New(db)
	auth := application.NewAuthService(adapter)
	spec := application.NewSpecificationService(adapter)
	s, err := New(auth, application.NewCatalogService(adapter), application.NewLifecycleService(adapter), db, Options{AuthMode: "local", Specifications: spec})
	if err != nil {
		t.Fatal(err)
	}
	cookies, csrf := resourceSession(t, s.Handler())
	actor, err := auth.Authenticate(ctx, cookies[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	category, err := s.catalog.CreateCategory(ctx, actor, application.CreateCategory{Name: "Camera"})
	if err != nil {
		t.Fatal(err)
	}
	model, err := s.catalog.CreateModel(ctx, actor, application.CreateModel{CategoryID: category.ID, Name: "Nested camera"})
	if err != nil {
		t.Fatal(err)
	}
	// Wire only the owned handlers here; production route/allow-list wiring belongs
	// to the integrating agent, and must use these same handlers.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /assets/new", s.newAssetForm)
	mux.HandleFunc("GET /assets/{id}/edit", s.editAssetForm)
	mux.HandleFunc("GET /assets/{id}", s.assetDetail)
	mux.HandleFunc("POST /assets", s.saveDrawerAsset)
	mux.HandleFunc("POST /assets/{id}", s.saveDrawerAsset)
	mux.HandleFunc("GET /admin/event-types", s.eventTypesPage)
	mux.HandleFunc("POST /admin/event-types", s.createAssetEventType)
	mux.HandleFunc("POST /admin/event-types/{id}", s.updateAssetEventType)
	mux.HandleFunc("POST /admin/event-types/{id}/status", s.setAssetEventTypeStatus)
	send := func(method, path, target string, form url.Values) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		if target == "" {
			mux.ServeHTTP(w, r)
		} else {
			mux.ServeHTTP(&drawerResponse{w, target}, r)
		}
		return w
	}
	check := func(w *httptest.ResponseRecorder, status int, contains string) {
		t.Helper()
		if w.Code != status || !strings.Contains(w.Body.String(), contains) {
			t.Fatalf("status=%d want=%d body=%s", w.Code, status, w.Body.String())
		}
	}
	check(send("GET", "/assets/new?model_id="+model.ID, "asset-editor", nil), 200, `id="asset-editor"`)
	form := url.Values{"csrf_token": {csrf}, "model_id": {model.ID}, "display_name": {"Nested asset"}, "serial_number": {"serial"}, "purchase_channel": {"shop"}, "notes": {"draft notes"}, "tag_ids": {""}}
	check(send("POST", "/assets", "asset-editor", url.Values{"model_id": {model.ID}}), 403, "")
	created := send("POST", "/assets", "asset-editor", form)
	check(created, 200, `"kind":"asset"`)
	var saved struct {
		ID, Name string
		Enabled  bool
	}
	if err := json.Unmarshal(created.Body.Bytes(), &saved); err != nil || saved.ID == "" {
		t.Fatal("missing saved asset ID")
	}
	asset, err := spec.Asset(ctx, actor, saved.ID)
	if err != nil || asset.SerialNumber != "serial" || asset.Notes != "draft notes" || asset.PurchaseChannel != "shop" {
		t.Fatal("form fields were not persisted", err)
	}
	check(send("GET", "/assets/"+saved.ID, "asset-detail", nil), 200, `data-drawer-target="asset-editor"`)
	form.Set("display_name", "Updated asset")
	check(send("POST", "/assets/"+saved.ID, "asset-editor", form), 200, `"name":"Updated asset"`)
	form.Set("display_name", "")
	check(send("POST", "/assets/"+saved.ID, "asset-editor", form), 200, `"name":"Nested camera"`)
	check(send("GET", "/assets/"+saved.ID, "asset-detail", nil), 200, `>Nested camera</h2>`)
	check(send("GET", "/assets/"+saved.ID, "", nil), 200, `>Nested camera</h1>`)
	form.Set("display_name", strings.Repeat("x", 201))
	check(send("POST", "/assets/"+saved.ID, "asset-editor", form), 422, `data-error-summary`)
	form.Set("display_name", "Still present")
	form.Set("variant_id", "retired")
	check(send("POST", "/assets/"+saved.ID, "asset-editor", form), 422, `data-error-summary`)
	form.Del("variant_id")
	check(send("POST", "/assets/"+saved.ID, "", form), 303, "")
	if _, err := auth.AddMember(ctx, actor, application.AddMember{Username: "drawer-viewer", Password: "viewer secure password", Role: application.RoleViewer}); err != nil {
		t.Fatal(err)
	}
	viewer, err := auth.Login(ctx, application.Login{Username: "drawer-viewer", Password: "viewer secure password"})
	if err != nil {
		t.Fatal(err)
	}
	ownerCookies := cookies
	cookies = []*http.Cookie{{Name: sessionCookie, Value: viewer.Token}, ownerCookies[1]}
	check(send("GET", "/assets/"+saved.ID+"/edit", "asset-editor", nil), 403, "")
	check(send("POST", "/assets/"+saved.ID, "asset-editor", form), 403, "")
	form.Set("variant_id", "retired")
	check(send("POST", "/assets/"+saved.ID, "asset-editor", form), 403, "")
	form.Del("variant_id")
	check(send("GET", "/assets/"+saved.ID, "asset-detail", nil), 200, `id="asset-detail"`)
	cookies = ownerCookies
	typeForm := url.Values{"csrf_token": {csrf}, "name": {"Nested type"}, "cashflow": {"expense"}}
	created = send("POST", "/admin/event-types", "event-type-manage", typeForm)
	check(created, 200, `"kind":"event-type"`)
	check(created, 200, `"cashflow":"expense"`)
	if err := json.Unmarshal(created.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	typeURL := "/admin/event-types/" + saved.ID
	check(send("GET", "/admin/event-types?edit_type_id="+saved.ID, "event-type-manage", nil), 200, `value="Nested type"`)
	typeForm.Set("name", "Renamed type")
	typeForm.Set("cashflow", "income")
	updated := send("POST", typeURL, "event-type-manage", typeForm)
	check(updated, 200, `"name":"Renamed type"`)
	check(updated, 200, `"cashflow":"income"`)
	disabled := send("POST", typeURL+"/status", "event-type-manage", url.Values{"csrf_token": {csrf}, "enabled": {"0"}})
	check(disabled, 200, `"enabled":false`)
	check(disabled, 200, `"cashflow":"income"`)
	typeForm.Set("name", "")
	check(send("POST", typeURL, "event-type-manage", typeForm), 422, `data-error-summary`)
	cookies = []*http.Cookie{{Name: sessionCookie, Value: viewer.Token}, ownerCookies[1]}
	readOnly := send("GET", "/admin/event-types?edit_type_id="+saved.ID, "event-type-manage", nil)
	check(readOnly, 200, "Renamed type")
	if strings.Contains(readOnly.Body.String(), `<form`) {
		t.Fatal("viewer receives mutation form")
	}
	typeForm.Set("name", "Viewer change")
	check(send("POST", typeURL, "event-type-manage", typeForm), 403, "")
}
