package web

import (
	"context"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/config"
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
	category, err := catalog.CreateCategory(ctx, owner, application.CreateCategory{Name: "Tagged phones"})
	if err != nil {
		t.Fatal(err)
	}
	model, err := catalog.CreateModel(ctx, owner, application.CreateModel{CategoryID: category.ID, Name: "Tagged phone"})
	if err != nil {
		t.Fatal(err)
	}
	allowed := url.Values{"csrf_token": {csrf.Value}, "tag_ids": {tag.ID}, "appearance_" + kind.ID: {"no"}}
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
	response = request(t, handler, "POST", "/assets", assetForm, cookies)
	if response.Code != 303 {
		t.Fatalf("create tagged item: %d %s", response.Code, response.Body.String())
	}
	assetURL := response.Header().Get("Location")
	assetID := strings.TrimPrefix(assetURL, "/assets/")
	created, err := spec.Asset(ctx, owner, assetID)
	if err != nil || created.ModelID != model.ID || created.VariantID != "" || len(created.Tags) != 1 || created.Notes != "Preserve notes" {
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
	allowed.Del("tag_ids")
	allowed.Del("appearance_" + kind.ID)
	response = request(t, handler, "POST", "/admin/catalog/models/"+model.ID+"/tags", allowed, cookies)
	if response.Code != 422 || !strings.Contains(response.Body.String(), "请先处理引用") {
		t.Fatal("used allowance removal accepted")
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
