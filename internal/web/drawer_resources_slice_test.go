package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/blob"
	localblob "github.com/SampsonFox/assetloop/internal/blob/local"
	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
)

// Mount the slice independently so its tests do not require concurrent server.go
// integration to land first. Ordinary routes still use the complete server.
func resourceSliceHandler(t *testing.T) http.Handler {
	t.Helper()
	cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "slice.db")}
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err = basestore.Migrate(context.Background(), db, cfg); err != nil {
		t.Fatal(err)
	}
	adapter := sqlite.New(db)
	local, err := localblob.New(filepath.Join(t.TempDir(), "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	media := application.NewModelMediaService(adapter, blob.Registry{"local": local}, blob.ObjectKeyMapper{}, "local")
	s, err := New(application.NewAuthService(adapter), application.NewCatalogService(adapter), application.NewLifecycleService(adapter), db, Options{AuthMode: "local", ModelMedia: media, Specifications: application.NewSpecificationService(adapter)})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := s.templates["appearance"].Clone()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = parsed.ParseFS(assets, "templates/resource_binding.html")
	if err != nil {
		t.Fatal(err)
	}
	s.templates["resource_binding"] = parsed
	mux := http.NewServeMux()
	mount := func(pattern, target string, handle http.HandlerFunc) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Assetloop-Drawer") != "" {
				w = &drawerResponse{w, target}
			}
			handle(w, r)
		})
	}
	mount("GET /admin/3d/binding/{kind}/{id}", "binding-editor", s.resourceBindingPage)
	mount("POST /admin/3d/binding/{kind}/{id}", "binding-editor", s.saveResourceBinding)
	mount("GET /admin/catalog/models/{id}/appearance", "appearance-editor", s.appearancePage)
	mount("POST /admin/catalog/models/{id}/appearance/upload", "appearance-editor", s.uploadAppearance)
	mux.Handle("/", s.Handler())
	return mux
}

func TestResourceSliceBindingIndependentSave(t *testing.T) {
	h := resourceSliceHandler(t)
	cookies, csrf := resourceSession(t, h)
	post := func(path string, form url.Values) *httptest.ResponseRecorder {
		form.Set("csrf_token", csrf)
		return request(t, h, "POST", path, form, cookies)
	}
	post("/admin/catalog/categories", url.Values{"name": {"Devices"}, "icon_key": {"package"}})
	category := optionID(t, request(t, h, "GET", "/admin/catalog", nil, cookies).Body.String(), "Devices")
	created := post("/admin/catalog/models", url.Values{"category_id": {category}, "name": {"Slice model"}})
	location, _ := url.Parse(created.Header().Get("Location"))
	model := location.Query().Get("edit_model_id")
	if model == "" {
		t.Fatal("model not created", created.Code, created.Header())
	}
	upload := uploadWebResource(t, h, url.Values{"csrf_token": {csrf}, "name": {"Choice"}}, webTestGLB(), cookies)
	id := strings.TrimPrefix(upload.Header().Get("Location"), "/admin/3d/")
	path := resourceBindingURL("model", model, "", 1)
	send := func(method, target, drawer string, form url.Values) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("X-Assetloop-Drawer", drawer)
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	page := send("GET", path, "binding-editor", nil)
	for _, want := range []string{`id="binding-editor"`, `form="binding-form"`, `id="binding-form"`, `data-drawer-query-results="candidates"`, `data-drawer-target="resource-upload"`, `data-drawer-field="resource_id"`, `data-drawer-query-select="resource_id"`} {
		if !strings.Contains(page.Body.String(), want) {
			t.Fatalf("binding integration missing %s", want)
		}
	}
	heading := regexp.MustCompile(`(?s)<header.*?</header>`).FindString(page.Body.String())
	if !strings.Contains(heading, `form="binding-form"`) || strings.Contains(strings.TrimPrefix(page.Body.String()[strings.Index(page.Body.String(), "</header>"):], "</header>"), `form="binding-form"`) {
		t.Fatal("save must occur only in fixed heading")
	}
	uploadPage := send("GET", "/admin/3d?dialog=resource-upload", "resource-upload", nil)
	if uploadPage.Code != 200 || !strings.Contains(uploadPage.Body.String(), `id="resource-upload"`) || strings.Contains(uploadPage.Body.String(), `name="kind"`) {
		t.Fatal("resource child must upload without binding the parent", uploadPage.Code)
	}
	if page.Code != 200 || !strings.Contains(page.Body.String(), `name="resource_id"`) || !strings.Contains(page.Body.String(), "Choice") || strings.Contains(page.Body.String(), "resource-library-table") || strings.Contains(page.Body.String(), "<!DOCTYPE") {
		t.Fatal("binding must offer an inline choice in a pure detail fragment", page.Code, page.Body.String())
	}
	if got := send("POST", path, "binding-editor", url.Values{"resource_id": {id}}); got.Code != 403 {
		t.Fatal("CSRF bypass", got.Code)
	}
	bad := send("POST", path, "binding-editor", url.Values{"csrf_token": {csrf}, "resource_id": {"missing"}})
	if bad.Code != 422 || !strings.Contains(bad.Body.String(), "data-error-summary") {
		t.Fatal("missing local validation", bad.Code)
	}
	saved := send("POST", path, "binding-editor", url.Values{"csrf_token": {csrf}, "resource_id": {id}})
	var result struct{ Kind, ID, Name string }
	if saved.Code != 200 || json.Unmarshal(saved.Body.Bytes(), &result) != nil || result.Kind != "binding" || result.ID != model {
		t.Fatal("missing save receipt", saved.Code, saved.Body.String())
	}
	// Reopening after the child closes proves the binding is already persisted.
	page = send("GET", path, "binding-editor", nil)
	if !strings.Contains(page.Body.String(), `value="`+id+`" selected`) {
		t.Fatal("independent save not persisted")
	}
	reference := send("GET", "/admin/3d/"+id, "resource-editor", nil).Body.String()
	if !strings.Contains(reference, `data-drawer-target="model-drawer"`) || strings.Contains(reference, "kind=model") {
		t.Fatal("resource reference must target model detail")
	}
	appearance := send("GET", "/admin/catalog/models/"+model+"/appearance", "appearance-editor", nil)
	if appearance.Code != 200 || !strings.Contains(appearance.Body.String(), `id="appearance-editor"`) || strings.Contains(appearance.Body.String(), "<!DOCTYPE") {
		t.Fatal("missing appearance detail fragment", appearance.Code)
	}
	if !strings.Contains(appearance.Body.String(), `data-drawer-query-results="candidates"`) || !strings.Contains(appearance.Body.String(), `data-appearance-upload`) {
		t.Fatal("appearance search integration markers missing")
	}
	saved = send("POST", path, "binding-editor", url.Values{"csrf_token": {csrf}, "resource_id": {""}})
	if saved.Code != 200 {
		t.Fatal("unbind", saved.Code)
	}
	page = send("GET", path, "binding-editor", nil)
	if strings.Contains(page.Body.String(), `value="`+id+`" selected`) {
		t.Fatal("unbind not persisted")
	}
	if got := send("GET", resourceBindingURL("model", "missing", "", 1), "binding-editor", nil); got.Code != 404 {
		t.Fatal("missing target accepted", got.Code)
	}
	post("/admin/tags/types", url.Values{"name": {"Finish"}, "enabled": {"1"}, "appearance": {"1"}})
	typeID := optionID(t, request(t, h, "GET", "/admin/tags", nil, cookies).Body.String(), "Finish")
	post("/admin/tags/values", url.Values{"type_id": {typeID}, "name": {"Matte"}, "enabled": {"1"}})
	tagPage := request(t, h, "GET", "/admin/tags", nil, cookies).Body.String()
	tag := regexp.MustCompile(`/admin/tags\?edit=([a-f0-9-]+)`).FindStringSubmatch(tagPage)[1]
	post("/admin/catalog/models/"+model+"/tags", url.Values{"tag_ids": {tag}})
	tagSave := send("POST", "/admin/3d/"+id+"/tags", "resource-editor", url.Values{"csrf_token": {csrf}, "tag_ids": {tag}})
	if tagSave.Code != 200 || json.Unmarshal(tagSave.Body.Bytes(), &result) != nil || result.Kind != "resource" || result.Name != "Choice" {
		t.Fatal("tag save receipt", tagSave.Code, tagSave.Body.String())
	}
	resourceBody := send("GET", "/admin/3d/"+id, "resource-editor", nil).Body.String()
	if !strings.Contains(resourceBody, "view=types&amp;edit="+typeID) || !strings.Contains(resourceBody, "/admin/tags?edit="+tag) {
		t.Fatal("concrete type/tag detail missing")
	}
	uploadDrawer := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("X-Assetloop-Drawer", "appearance-editor")
		h.ServeHTTP(w, r)
	})
	appearanceUpload := uploadWebResourceAt(t, uploadDrawer, "/admin/catalog/models/"+model+"/appearance/upload", url.Values{"csrf_token": {csrf}, "name": {"Matte appearance"}, "tag_ids": {tag}}, webTestGLB(), cookies)
	if appearanceUpload.Code != 200 || json.Unmarshal(appearanceUpload.Body.Bytes(), &result) != nil || result.Kind != "resource" {
		t.Fatal("appearance upload receipt", appearanceUpload.Code, appearanceUpload.Body.String())
	}
	resourceBody = send("GET", "/admin/3d/"+result.ID, "resource-editor", nil).Body.String()
	if !strings.Contains(resourceBody, `/appearance?rule_id=`) || !strings.Contains(resourceBody, `data-drawer-target="appearance-editor"`) {
		t.Fatal("appearance reference must identify the rule")
	}
	post("/admin/members", url.Values{"username": {"reader"}, "password": {"reader secure password"}, "role": {"viewer"}})
	login := post("/login", url.Values{"username": {"reader"}, "password": {"reader secure password"}})
	cookies = []*http.Cookie{responseCookie(t, login, sessionCookie), cookies[1]}
	viewerPage := send("GET", path, "binding-editor", nil)
	if viewerPage.Code != 403 || strings.Contains(viewerPage.Body.String(), `name="resource_id"`) {
		t.Fatal("viewer sees binding mutation", viewerPage.Code)
	}
	if denied := send("POST", path, "binding-editor", url.Values{"csrf_token": {csrf}, "resource_id": {id}}); denied.Code != 403 {
		t.Fatal("viewer binding write allowed", denied.Code)
	}
}

func TestResourceSliceDetailLinks(t *testing.T) {
	data, err := assets.ReadFile("templates/resource.html")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{`data-drawer-target="tag-editor"`, `/admin/tags?view=types&amp;edit={{.ID}}`, `/admin/tags?edit={{.ID}}`, `data-drawer-target="appearance-editor"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing detail link %s", want)
		}
	}
	if strings.Contains(body, `href="/admin/tags"`) {
		t.Fatal("generic tag library navigation remains")
	}
}

func TestResourceSliceFragmentHasNoScriptsAndViewerIsReadOnly(t *testing.T) {
	h := newTestHandlerWithBlob(t, nil)
	cookies, csrf := resourceSession(t, h)
	upload := uploadWebResource(t, h, url.Values{"csrf_token": {csrf}, "name": {"Preview"}, "model_3d_source_url": {"https://example.com/model"}}, webTestGLB(), cookies)
	if upload.Code != http.StatusSeeOther {
		t.Fatal("upload failed", upload.Code)
	}
	path := strings.Replace(upload.Header().Get("Location"), "/admin/3d/", "/resources/", 1)
	fragment := func() string {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("X-Assetloop-Drawer", "resource-editor")
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatal("fragment failed", w.Code)
		}
		body := w.Body.String()
		if strings.Contains(strings.ToLower(body), "<script") {
			t.Error("resource fragment must not emit scripts")
		}
		if !strings.Contains(body, `data-model-viewer`) || strings.Contains(body, `resource-library-table`) {
			t.Error("expected only resource detail")
		}
		return body
	}
	fragment()
	full := request(t, h, http.MethodGet, path, nil, cookies).Body.String()
	if !strings.Contains(full, `<script type="module" src="/static/asset-model-viewer.js"></script>`) {
		t.Error("full page still needs viewer initialization")
	}
	created := request(t, h, http.MethodPost, "/admin/members", url.Values{"csrf_token": {csrf}, "username": {"preview-reader"}, "password": {"reader secure password"}, "role": {"viewer"}}, cookies)
	if created.Code != http.StatusSeeOther {
		t.Fatal("viewer creation failed", created.Code)
	}
	login := request(t, h, http.MethodPost, "/login", url.Values{"csrf_token": {csrf}, "username": {"preview-reader"}, "password": {"reader secure password"}}, cookies)
	cookies = []*http.Cookie{responseCookie(t, login, sessionCookie), cookies[1]}
	body := fragment()
	if strings.Contains(body, `type="submit"`) || strings.Contains(body, `name="model_3d_source_url"`) || strings.Contains(body, `data-guard-dirty`) {
		t.Error("viewer must not receive resource edit/submit controls")
	}
	link := regexp.MustCompile(`<a\b[^>]*href="https://example.com/model"[^>]*>`).FindString(body)
	for _, want := range []string{`target="_blank"`, `rel="noopener noreferrer external"`} {
		if !strings.Contains(link, want) {
			t.Errorf("readonly external source missing %s", want)
		}
	}
}
