package web

import (
	"bytes"
	"context"
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

func TestCatalogModelFragmentPrefill(t *testing.T) {
	s, err := New(nil, nil, nil, nil, Options{Specifications: application.NewSpecificationService(nil)})
	if err != nil {
		t.Fatal(err)
	}
	data := pageData{
		Strings:             stringsFor(application.LocaleEn),
		Categories:          []domain.ItemCategory{{ID: "category-a", Name: "Camera"}, {ID: "category-b", Name: "Phone"}},
		CatalogEditingModel: &domain.ProductModel{ID: "model-a", Name: "Saved model", CategoryID: "category-b"},
		ModelTagEditors: []modelTagEditor{
			{ModelID: "model-a", Dimensions: []tagDimension{{ID: "type-a", Name: "Color", Choices: []tagChoice{{ID: "tag-a", Name: "Red", Selected: true, Enabled: true}}}}},
			{ModelID: "model-b"},
		},
	}
	var out bytes.Buffer
	if err := s.templates["catalog"].ExecuteTemplate(&out, "fragment-model-drawer", data); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, want := range []string{`action="/admin/catalog/models/model-a"`, `value="Saved model"`, `value="category-b" selected`, `data-model-tag-group="model-a">`, `data-model-tag-group="model-b" hidden disabled`, `data-drawer-target="category-drawer"`, `data-drawer-field="category_id"`, `href="/admin/catalog/categories/category-b"`, `href="/admin/catalog/models/model-a/binding"`, `data-drawer-target="binding-editor"`, `data-drawer-target="appearance-editor"`, `view=types&amp;edit=type-a`, `edit=tag-a`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s", want)
		}
	}
	for _, unwanted := range []string{`href="/admin/tags"`, `href="/admin/3d"`, `data-dialog-initial-open`, `<script`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("unexpected %s", unwanted)
		}
	}
	data.CatalogEditingModel = nil
	out.Reset()
	if err := s.templates["catalog"].ExecuteTemplate(&out, "fragment-model-drawer", data); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `action="/admin/catalog/models"`) || !strings.Contains(out.String(), `data-model-tag-group="model-a" hidden disabled`) {
		t.Fatal("create must not submit another model's configuration")
	}
}

func TestCatalogCategoryDetail(t *testing.T) {
	ctx := context.Background()
	cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "category.db")}
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
	credential, err := auth.Setup(ctx, application.SetupAuth{TenantName: "Categories", BaseCurrency: "CNY", Username: "owner", Password: "category test secure password"})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := auth.Authenticate(ctx, credential.Token)
	if err != nil {
		t.Fatal(err)
	}
	catalog := application.NewCatalogService(adapter)
	category, err := catalog.CreateCategory(ctx, actor, application.CreateCategory{Name: "Saved category", IconKey: "camera"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(auth, catalog, application.NewLifecycleService(adapter), db, Options{AuthMode: "local", Specifications: application.NewSpecificationService(adapter)})
	if err != nil {
		t.Fatal(err)
	}
	send := func(id string, authenticated, fragment bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/admin/catalog/categories/"+id, nil)
		r.SetPathValue("id", id)
		if authenticated {
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: credential.Token})
		}
		if fragment {
			r.Header.Set("X-Assetloop-Drawer", "category-drawer")
		}
		w := httptest.NewRecorder()
		drawerTransport(http.HandlerFunc(s.categoryDetail)).ServeHTTP(w, r)
		return w
	}
	page := send(category.ID, true, true)
	if page.Code != http.StatusOK {
		t.Fatalf("detail: %d %s", page.Code, page.Body.String())
	}
	for _, want := range []string{`action="/admin/catalog/categories/` + category.ID + `"`, `value="Saved category"`, `value="camera" selected`, `name="csrf_token"`} {
		if !strings.Contains(page.Body.String(), want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(page.Body.String(), `<script`) || strings.Contains(page.Body.String(), `<table`) {
		t.Fatal("detail must be a fragment")
	}
	if missing := send("00000000-0000-0000-0000-000000000099", true, true); missing.Code != http.StatusNotFound {
		t.Fatal("unknown category accepted", missing.Code)
	}
	if anonymous := send(category.ID, false, true); anonymous.Code == http.StatusOK {
		t.Fatal("anonymous editor allowed")
	}
	created := send("new", true, true)
	if created.Code != http.StatusOK || !strings.Contains(created.Body.String(), `action="/admin/catalog/categories"`) {
		t.Fatal("create fragment action is invalid")
	}
	native := send(category.ID, true, false)
	if native.Code != http.StatusOK || !strings.Contains(native.Body.String(), `data-dialog-initial-open data-dialog-open="category-drawer"`) {
		t.Fatal("native opener missing")
	}
	for _, id := range []string{category.ID, ""} {
		form := url.Values{"name": {"Unsaved draft"}, "icon_key": {"watch"}}
		r := httptest.NewRequest(http.MethodPost, "/admin/catalog/categories/"+id, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()
		s.renderCategoryDetail(&drawerResponse{w, "category-drawer"}, r, actor, http.StatusUnprocessableEntity, "Rejected")
		for _, want := range []string{`value="Unsaved draft"`, `value="watch" selected`, `data-error-summary`} {
			if !strings.Contains(w.Body.String(), want) {
				t.Errorf("draft missing %s", want)
			}
		}
	}
	retained, err := catalog.Categories(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	for _, current := range retained {
		if current.ID == category.ID && current.Name != "Saved category" {
			t.Fatal("GET/error rendering mutated category")
		}
	}
}
