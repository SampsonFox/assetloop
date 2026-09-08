package web

import (
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

func TestSpecificationManagementHTTP(t *testing.T) {
	ctx := context.Background()
	cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "specifications.db")}
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
	credential, err := auth.Setup(ctx, application.SetupAuth{TenantName: "Tag HTTP", BaseCurrency: "CNY", Username: "owner", Password: "tag test secure password"})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.Authenticate(ctx, credential.Token)
	if err != nil {
		t.Fatal(err)
	}
	spec := application.NewSpecificationService(adapter)
	server, err := New(auth, application.NewCatalogService(adapter), application.NewLifecycleService(adapter), db, Options{AuthMode: "local", Specifications: spec})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	session := &http.Cookie{Name: sessionCookie, Value: credential.Token}
	page := request(t, handler, "GET", "/admin/tags?view=types", nil, []*http.Cookie{session})
	if page.Code != 200 || !strings.Contains(page.Body.String(), "规格标签管理") {
		t.Fatalf("page: %d %s", page.Code, page.Body.String())
	}
	if !strings.Contains(page.Body.String(), `name="return_to" value="/admin/tags?view=types"`) {
		t.Fatal("preferences must return to the current tag page")
	}
	csrf := responseCookie(t, page, csrfCookie)
	cookies := []*http.Cookie{session, csrf}
	denied := request(t, handler, "POST", "/admin/tags/types", url.Values{"name": {"Storage"}, "enabled": {"1"}}, cookies)
	if denied.Code != 403 {
		t.Fatal("missing CSRF accepted")
	}
	create := url.Values{"csrf_token": {csrf.Value}, "name": {"Storage"}, "enabled": {"1"}, "entity": {"type"}}
	response := request(t, handler, "POST", "/admin/tags/types", create, cookies)
	if response.Code != 303 {
		t.Fatalf("create type: %d %s", response.Code, response.Body.String())
	}
	types, err := spec.ListTypes(ctx, owner, application.SpecificationListOptions{Query: "Storage"})
	if err != nil || len(types.Types) != 1 {
		t.Fatalf("types: %+v %v", types, err)
	}
	kind := types.Types[0]
	if kind.Multiple || kind.AffectsAppearance {
		t.Fatal("type defaults must be single/non-appearance")
	}
	create = url.Values{"csrf_token": {csrf.Value}, "name": {"128GB"}, "enabled": {"1"}, "type_id": {kind.ID}, "entity": {"value"}}
	response = request(t, handler, "POST", "/admin/tags/values", create, cookies)
	if response.Code != 303 {
		t.Fatalf("create value: %d %s", response.Code, response.Body.String())
	}
	tags, err := spec.ListTags(ctx, owner, application.SpecificationListOptions{TypeID: kind.ID})
	if err != nil || len(tags.Tags) != 1 {
		t.Fatalf("tags: %+v %v", tags, err)
	}
	tag := tags.Tags[0]
	create.Set("name", " 128gb ")
	response = request(t, handler, "POST", "/admin/tags/values", create, cookies)
	if response.Code != 422 || !strings.Contains(response.Body.String(), "已有该名称") || !strings.Contains(response.Body.String(), `value=" 128gb "`) || !strings.Contains(response.Body.String(), "data-error-summary") {
		t.Fatalf("duplicate error echo: %d %s", response.Code, response.Body.String())
	}
	create.Set("name", "128GB")
	create.Del("enabled")
	response = request(t, handler, "POST", "/admin/tags/values/"+tag.ID, create, cookies)
	if response.Code != 303 {
		t.Fatalf("disable: %d %s", response.Code, response.Body.String())
	}
	page = request(t, handler, "GET", "/admin/tags?status=disabled&q=128", nil, cookies)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "128GB") || !strings.Contains(page.Body.String(), "已停用") {
		t.Fatal("disabled value missing")
	}
	create.Set("enabled", "1")
	response = request(t, handler, "POST", "/admin/tags/values/"+tag.ID, create, cookies)
	if response.Code != 303 {
		t.Fatal("restore failed")
	}
	catalog := application.NewCatalogService(adapter)
	for _, path := range []string{"/admin/catalog/variants", "/admin/catalog/variants/00000000-0000-0000-0000-000000000001", "/admin/catalog/variants/00000000-0000-0000-0000-000000000001/delete"} {
		result := request(t, handler, "POST", path, url.Values{"csrf_token": {csrf.Value}, "name": {"Must not create legacy spec"}}, cookies)
		if result.Code != http.StatusNotFound {
			t.Fatalf("legacy write not retired: %d", result.Code)
		}
	}
	legacySnapshot, err := catalog.Snapshot(ctx, owner)
	if err != nil || len(legacySnapshot.Models) != 0 {
		t.Fatalf("legacy write changed specifications: %+v %v", legacySnapshot, err)
	}
	category, err := catalog.CreateCategory(ctx, owner, application.CreateCategory{Name: "Tagged phones"})
	if err != nil {
		t.Fatal(err)
	}
	model, err := catalog.CreateModel(ctx, owner, application.CreateModel{CategoryID: category.ID, Name: "Tagged phone"})
	if err != nil {
		t.Fatal(err)
	}
	allowed := url.Values{"csrf_token": {csrf.Value}, "tag_ids": {tag.ID}, "appearance_" + kind.ID: {"no"}}
	t.Run("unified model drawer POST preserves draft and saves all fields", func(t *testing.T) {
		m, err := catalog.CreateModel(ctx, owner, application.CreateModel{CategoryID: category.ID, Name: "Drawer HTTP"})
		if err != nil {
			t.Fatal(err)
		}
		path := "/admin/catalog/models/" + m.ID
		form := url.Values{"model_configuration": {"1"}, "category_id": {category.ID}, "name": {"Drawer changed"}, "tag_ids": {tag.ID}, "appearance_" + kind.ID: {"yes"}}
		if got := request(t, handler, "POST", path, form, cookies); got.Code != 403 {
			t.Fatal("unified save skipped CSRF")
		}
		form.Set("csrf_token", csrf.Value)
		form.Set("appearance_"+kind.ID, "invalid")
		got := request(t, handler, "POST", path, form, cookies)
		if got.Code != 422 || !strings.Contains(got.Body.String(), `data-name="Drawer changed"`) || !strings.Contains(got.Body.String(), `data-dialog-initial-open`) {
			t.Fatalf("lost draft: %d %s", got.Code, got.Body.String())
		}
		unchanged, err := spec.Model(ctx, owner, m.ID)
		if err != nil || unchanged.Name != m.Name {
			t.Fatal("partial save on failure")
		}
		form.Set("appearance_"+kind.ID, "yes")
		got = request(t, handler, "POST", path, form, cookies)
		if got.Code != 303 || got.Header().Get("Location") != "/admin/catalog" {
			t.Fatalf("save: %d %s", got.Code, got.Body.String())
		}
		saved, err := spec.Model(ctx, owner, m.ID)
		state, stateErr := spec.Snapshot(ctx, owner)
		definition := state.Model(owner.TenantID, m.ID)
		if err != nil || stateErr != nil || saved.Name != "Drawer changed" || len(definition.AllowedTagIDs) != 1 || definition.AllowedTagIDs[0] != tag.ID || !definition.AppearanceOverrides[kind.ID] {
			t.Fatalf("incomplete save: %+v %+v", saved, definition)
		}
	})
	response = request(t, handler, "POST", "/admin/catalog/models/"+model.ID+"/tags", allowed, cookies)
	if response.Code != 303 {
		t.Fatalf("model allowance: %d %s", response.Code, response.Body.String())
	}
	page = request(t, handler, "GET", "/admin/catalog", nil, cookies)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "data-model-tag-group") || !strings.Contains(page.Body.String(), "允许标签") || strings.Contains(page.Body.String(), `id="variant-drawer"`) {
		t.Fatalf("model tag editor: %d", page.Code)
	}
	page = request(t, handler, "GET", "/assets/new?model_id="+model.ID, nil, cookies)
	if page.Code != 200 || !strings.Contains(page.Body.String(), `name="model_id"`) || !strings.Contains(page.Body.String(), `data-tag-name="128GB"`) || strings.Contains(page.Body.String(), `name="variant_id"`) {
		t.Fatalf("asset tag form: %d %s", page.Code, page.Body.String())
	}
	assetForm := url.Values{"csrf_token": {csrf.Value}, "model_id": {model.ID}, "tag_ids": {tag.ID, ""}, "display_name": {"My tagged phone"}, "notes": {"Preserve notes"}}
	for _, oldValue := range []string{"", "00000000-0000-0000-0000-000000000001"} {
		for _, includeModel := range []bool{false, true} {
			obsolete := url.Values{"variant_id": {oldValue}, "display_name": {"Must not create"}, "csrf_token": {csrf.Value}}
			if includeModel {
				obsolete.Set("model_id", model.ID)
			}
			for _, method := range []string{"GET", "POST"} {
				path := "/assets"
				if method == "GET" {
					path = "/assets/new?" + obsolete.Encode()
				}
				rejected := request(t, handler, method, path, obsolete, cookies)
				if rejected.Code != 422 || !strings.Contains(rejected.Body.String(), "旧规格参数已移除") {
					t.Fatalf("obsolete selection accepted (%s, model=%v): %d", method, includeModel, rejected.Code)
				}
			}
		}
	}
	page = request(t, handler, "GET", "/assets/new?"+url.Values{"model_id": {model.ID}, "tag_ids": {tag.ID}}.Encode(), nil, cookies)
	if page.Code != 200 || !strings.Contains(page.Body.String(), `data-tag-name="128GB" selected`) {
		t.Fatalf("tagged draft selection: %d %s", page.Code, page.Body.String())
	}
	page = request(t, handler, "GET", "/assets/new?"+url.Values{"model_id": {model.ID}, "variant_id": {"00000000-0000-0000-0000-000000000001"}}.Encode(), nil, cookies)
	if page.Code != 422 {
		t.Fatalf("ambiguous legacy/new draft accepted: %d", page.Code)
	}
	response = request(t, handler, "POST", "/assets", assetForm, cookies)
	if response.Code != 303 {
		t.Fatalf("create tagged item: %d %s", response.Code, response.Body.String())
	}
	assetURL := response.Header().Get("Location")
	assetID := strings.TrimPrefix(assetURL, "/assets/")
	created, err := spec.Asset(ctx, owner, assetID)
	if err != nil || created.ModelID != model.ID || len(created.Tags) != 1 || created.Notes != "Preserve notes" {
		t.Fatalf("direct item: %+v %v", created, err)
	}
	page = request(t, handler, "GET", assetURL+"/edit", nil, cookies)
	if page.Code != 200 || !strings.Contains(page.Body.String(), `data-tag-name="128GB" selected`) {
		t.Fatalf("selection not restored: %d %s", page.Code, page.Body.String())
	}
	create.Set("name", "128 GB")
	response = request(t, handler, "POST", "/admin/tags/values/"+tag.ID, create, cookies)
	if response.Code != 422 {
		t.Fatal("shared rename lacked confirmation")
	}
	create.Set("confirm_rename", "1")
	response = request(t, handler, "POST", "/admin/tags/values/"+tag.ID, create, cookies)
	if response.Code != 303 {
		t.Fatal("confirmed label correction failed")
	}
	page = request(t, handler, "GET", assetURL, nil, cookies)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "128 GB") {
		t.Fatal("item description did not resolve current tag label")
	}
	page = request(t, handler, "GET", "/", nil, cookies)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "128 GB") {
		t.Fatal("list description did not resolve current tag label")
	}
	page = request(t, handler, "GET", "/?q=128+GB", nil, cookies)
	if page.Code != 200 || !strings.Contains(page.Body.String(), `href="`+assetURL+`"`) {
		t.Fatal("list search did not use the current tag label")
	}
	allowed.Del("tag_ids")
	allowed.Del("appearance_" + kind.ID)
	response = request(t, handler, "POST", "/admin/catalog/models/"+model.ID+"/tags", allowed, cookies)
	if response.Code != 422 || !strings.Contains(response.Body.String(), "请先处理引用") {
		t.Fatal("used allowance removal accepted")
	}
	if !strings.Contains(response.Body.String(), `data-drawer-target="asset-detail" href="/assets/`+assetID+`"`) || !strings.Contains(response.Body.String(), "data-dialog-initial-open") {
		t.Fatal("allowance error must open the editor and link to its blocking references")
	}
	page = request(t, handler, "GET", "/admin/catalog?"+url.Values{"q": {"no model matches this"}, "dialog": {"model-drawer"}, "edit_model_id": {model.ID}}.Encode(), nil, cookies)
	if page.Code != 200 || !strings.Contains(page.Body.String(), `data-edit-model-id="`+model.ID+`"`) || !strings.Contains(page.Body.String(), `data-model-tag-group="`+model.ID+`"`) || strings.Contains(page.Body.String(), "<tbody>") {
		t.Fatalf("off-page editor must not change list results: %d %s", page.Code, page.Body.String())
	}
	create.Set("name", "128GB")
	response = request(t, handler, "POST", "/admin/tags/values/"+tag.ID, create, cookies)
	if response.Code != 303 {
		t.Fatal("restore label failed")
	}
	_, err = auth.AddMember(ctx, owner, application.AddMember{Username: "tag-viewer", Password: "viewer secure password", Role: application.RoleViewer})
	if err != nil {
		t.Fatal(err)
	}
	viewerCredential, err := auth.Login(ctx, application.Login{Username: "tag-viewer", Password: "viewer secure password"})
	if err != nil {
		t.Fatal(err)
	}
	viewerCookies := []*http.Cookie{{Name: sessionCookie, Value: viewerCredential.Token}, csrf}
	viewerPage := request(t, handler, "GET", "/admin/tags", nil, viewerCookies)
	if viewerPage.Code != 200 || !strings.Contains(viewerPage.Body.String(), "128GB") || strings.Contains(viewerPage.Body.String(), `id="tag-editor"`) {
		t.Fatalf("viewer page: %d %s", viewerPage.Code, viewerPage.Body.String())
	}
	response = request(t, handler, "POST", "/admin/tags/values/"+tag.ID, create, viewerCookies)
	if response.Code != 403 {
		t.Fatalf("viewer write: %d", response.Code)
	}
	_, err = auth.UpdatePreferences(ctx, owner, application.UpdatePreferences{Locale: application.LocaleEn, Theme: application.ThemeDark, Accent: application.AccentViolet})
	if err != nil {
		t.Fatal(err)
	}
	page = request(t, handler, "GET", "/admin/tags?view=types", nil, cookies)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "Specification tags") || !strings.Contains(page.Body.String(), "All dimensions are optional") {
		t.Fatal("English management copy missing")
	}
}

func TestAppearanceConflictNoticeIsEditorOnly(t *testing.T) {
	s, err := New(nil, nil, nil, nil, Options{Specifications: application.NewSpecificationService(nil)})
	if err != nil {
		t.Fatal(err)
	}
	for _, canManage := range []bool{true, false} {
		p := application.Principal{TenantID: "tenant", Role: application.RoleViewer, Locale: application.LocaleZhCN}
		if canManage {
			p.Role = application.RoleEditor
		}
		w := httptest.NewRecorder()
		s.render(w, 200, "asset", pageData{Principal: &p, Asset: &domain.Asset{ID: "item", ModelID: "model", DisplayName: "Phone"}, CanManageCatalog: canManage, Binding: &application.Model3DBinding{Conflict: true}, BaseCurrency: "CNY"})
		if w.Code != 200 {
			t.Fatalf("render: %d %s", w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "多个同等具体的外观默认") != canManage {
			t.Fatal("conflict warning visibility incorrect")
		}
	}
}
