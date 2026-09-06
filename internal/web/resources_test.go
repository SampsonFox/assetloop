package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
)

func resourceSession(t *testing.T, handler http.Handler) ([]*http.Cookie, string) {
	t.Helper()
	page := request(t, handler, http.MethodGet, "/setup", nil, nil)
	csrf := responseCookie(t, page, csrfCookie)
	setup := request(t, handler, http.MethodPost, "/setup", url.Values{"csrf_token": {csrf.Value}, "tenant_name": {"Resource tests"}, "base_currency": {"CNY"}, "username": {"owner"}, "password": {"owner secure password"}}, []*http.Cookie{csrf})
	return []*http.Cookie{responseCookie(t, setup, sessionCookie), csrf}, csrf.Value
}

func uploadWebResource(t *testing.T, h http.Handler, fields url.Values, data []byte, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	return uploadWebResourceAt(t, h, "/admin/3d", fields, data, cookies)
}

func uploadWebResourceAt(t *testing.T, h http.Handler, path string, fields url.Values, data []byte, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for key, values := range fields {
		for _, value := range values {
			if err := w.WriteField(key, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	file, err := w.CreateFormFile("model_3d", "resource.glb")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.Write(data); err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, path, &body)
	r.Header.Set("Content-Type", w.FormDataContentType())
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, r)
	return response
}

func TestResourceLibraryBindingPrecedenceColorAndReferences(t *testing.T) {
	h := newTestHandlerWithBlob(t, nil)
	cookies, csrf := resourceSession(t, h)
	post := func(path string, form url.Values) *httptest.ResponseRecorder {
		t.Helper()
		form.Set("csrf_token", csrf)
		return request(t, h, http.MethodPost, path, form, cookies)
	}
	get := func(path string) *httptest.ResponseRecorder { return request(t, h, http.MethodGet, path, nil, cookies) }
	assertStatus := func(r *httptest.ResponseRecorder, status int) {
		t.Helper()
		if r.Code != status {
			t.Fatalf("status=%d want=%d body=%s", r.Code, status, r.Body.String())
		}
	}
	assertStatus(post("/admin/catalog/categories", url.Values{"name": {"Devices"}, "icon_key": {"smartphone"}}), 303)
	category := optionID(t, get("/admin/catalog").Body.String(), "Devices")
	assertStatus(post("/admin/catalog/models", url.Values{"category_id": {category}, "name": {"Phone"}}), 303)
	model := regexp.MustCompile(`data-edit-model-id="([a-f0-9-]+)"`).FindStringSubmatch(get("/admin/catalog").Body.String())[1]
	assertStatus(post("/admin/tags/types", url.Values{"name": {"Color"}, "enabled": {"1"}, "appearance": {"1"}, "entity": {"type"}}), 303)
	typeID := regexp.MustCompile(`value="([a-f0-9-]+)"[^>]*>Color</option>`).FindStringSubmatch(get("/admin/tags").Body.String())[1]
	assertStatus(post("/admin/tags/values", url.Values{"type_id": {typeID}, "name": {"Blue"}, "enabled": {"1"}, "entity": {"value"}}), 303)
	blue := regexp.MustCompile(`/admin/tags\?edit=([a-f0-9-]+)`).FindStringSubmatch(get("/admin/tags").Body.String())[1]
	assertStatus(post("/admin/catalog/models/"+model+"/tags", url.Values{"tag_ids": {blue}}), 303)
	newPage := get("/assets/new?model_id=" + model).Body.String()
	assetForm := regexp.MustCompile(`(?s)<form id="asset-form".*?</form>`).FindString(newPage)
	if strings.Contains(assetForm, `name="color"`) || strings.Contains(newPage, "data-model-viewer") || strings.Contains(newPage, "kind=asset") {
		t.Fatal("unsaved asset must choose a color specification and keep static preview")
	}
	created := post("/assets", url.Values{"model_id": {model}, "tag_ids": {blue}, "display_name": {"My phone"}, "color": {"Injected independent color"}})
	assertStatus(created, 303)
	assetPath := created.Header().Get("Location")
	asset := strings.TrimPrefix(assetPath, "/assets/")
	if body := get(assetPath).Body.String(); strings.Contains(body, "kind=asset") || !strings.Contains(body, assetPath+"/edit") {
		t.Fatal("asset detail must keep editing entry but reserve model binding for the edit page")
	}
	if body := get(assetPath).Body.String(); !strings.Contains(body, "Blue") || strings.Contains(body, "Injected independent color") {
		t.Fatal("asset color must come from specification")
	}
	if body := get(assetPath + "/edit").Body.String(); !strings.Contains(body, "kind=asset") {
		t.Fatal("saved asset binding entry missing")
	}
	ids := []string{}
	digests := []string{}
	appearanceRule := ""
	for i, kind := range []string{"model", "appearance", "asset"} {
		data := append([]byte(nil), webTestGLB()...)
		// Distinct valid documents make the resolved URL observable.
		data = bytes.Replace(data, []byte("2.0"), []byte(fmt.Sprintf("2.%d", i)), 1)
		upload := uploadWebResource(t, h, url.Values{"csrf_token": {csrf}, "name": {kind + " resource"}, "model_3d_author": {"Original author"}}, data, cookies)
		assertStatus(upload, 303)
		id := strings.TrimPrefix(upload.Header().Get("Location"), "/admin/3d/")
		ids = append(ids, id)
		digest := sha256.Sum256(data)
		digests = append(digests, hex.EncodeToString(digest[:]))
		if kind == "appearance" {
			bound := post("/admin/catalog/models/"+model+"/appearance", url.Values{"resource_id": {id}, "tag_ids": {blue}})
			assertStatus(bound, 303)
			location, err := url.Parse(bound.Header().Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			appearanceRule = location.Query().Get("appearance_rule_id")
		} else {
			target := model
			if kind == "asset" {
				target = asset
			}
			assertStatus(post("/admin/3d/bind/"+kind+"/"+target, url.Values{"resource_id": {id}}), 303)
		}
		body := get(assetPath).Body.String()
		if !strings.Contains(body, "model.glb?v="+digests[i]) {
			t.Fatalf("%s override not resolved in asset detail", kind)
		}
		served := get(assetPath + "/model.glb")
		assertStatus(served, 200)
		if !bytes.Equal(served.Body.Bytes(), data) {
			t.Fatalf("%s override bytes incorrect", kind)
		}
		ref := get("/admin/3d/" + id)
		assertStatus(ref, 200)
		if !strings.Contains(ref.Body.String(), "disabled") {
			t.Fatal("referenced resource delete must be disabled")
		}
		assertStatus(post("/admin/3d/"+id+"/delete", url.Values{}), http.StatusConflict)
	}
	descriptionPath := "/admin/3d/" + ids[1] + "/tags"
	assertStatus(post(descriptionPath, url.Values{"tag_ids": {"", blue}, "category_ids": {category}}), 303)
	description := get("/admin/3d/" + ids[1]).Body.String()
	for _, want := range []string{`value="` + blue + `" selected`, `value="` + category + `" checked`, "模型标签与分类", "外观默认", "edit_model_id=" + model} {
		if !strings.Contains(description, want) {
			t.Fatalf("resource description missing %s", want)
		}
	}
	if strings.Contains(description, "kind=appearance") {
		t.Fatal("appearance reference must link to the model editor, not a nonexistent binding picker")
	}
	appearancePath := "/admin/catalog/models/" + model + "/appearance"
	candidates := get(appearancePath + "?search=1&tag_ids=" + blue)
	assertStatus(candidates, 200)
	for _, want := range []string{"外观描述齐全", "appearance resource", `value="` + blue + `" checked`, `name="return_appearance"`, "已确认绑定"} {
		if !strings.Contains(candidates.Body.String(), want) {
			t.Fatalf("appearance candidate UI missing %s", want)
		}
	}
	if strings.Contains(candidates.Body.String(), ">asset resource</td>") {
		t.Fatal("uncategorized resource recommended")
	}
	manual := get(appearancePath + "?search=1&manual=1&tag_ids=" + blue)
	assertStatus(manual, 200)
	if !strings.Contains(manual.Body.String(), ">asset resource</td>") || !strings.Contains(manual.Body.String(), "未按标签匹配") {
		t.Fatal("manual resource selection unavailable")
	}
	updatedRule := post(appearancePath, url.Values{"return_appearance": {"1"}, "rule_id": {appearanceRule}, "resource_id": {ids[1]}, "tag_ids": {blue}})
	assertStatus(updatedRule, 303)
	if updatedRule.Header().Get("Location") != appearancePath+"?rule_id="+appearanceRule {
		t.Fatal("appearance save lost editor context")
	}
	assertStatus(get(appearancePath+"?rule_id=00000000-0000-0000-0000-000000000001"), 404)
	assertStatus(request(t, h, "POST", descriptionPath, url.Values{"tag_ids": {blue}}, cookies), 403)
	invalid := post(descriptionPath, url.Values{"tag_ids": {blue}, "category_ids": {"00000000-0000-0000-0000-000000000001"}})
	assertStatus(invalid, 422)
	if !strings.Contains(invalid.Body.String(), `value="`+blue+`" selected`) {
		t.Fatal("resource tag validation lost submitted choices")
	}
	// Clearing descriptive tags must not change an explicitly confirmed rule.
	assertStatus(post(descriptionPath, url.Values{}), 303)
	if !strings.Contains(get(assetPath).Body.String(), "model.glb?v="+digests[2]) {
		t.Fatal("resource descriptions changed an item override")
	}
	assertStatus(post("/admin/3d/"+ids[0], url.Values{"name": {"Renamed resource"}, "model_3d_author": {"Shared author"}, "model_3d_license": {"CC0"}, "model_3d_source_url": {"https://example.com/model"}}), 303)
	filtered := get("/admin/3d?q=Shared+author")
	assertStatus(filtered, 200)
	if !strings.Contains(filtered.Body.String(), "Renamed resource") || strings.Contains(filtered.Body.String(), ">asset resource</a>") {
		t.Fatal("resource search must be server filtered")
	}
	for i, kind := range []string{"asset", "appearance", "model"} {
		if kind == "appearance" {
			assertStatus(post("/admin/catalog/appearance/"+appearanceRule+"/delete", url.Values{}), 303)
		} else {
			target := asset
			if kind == "model" {
				target = model
			}
			assertStatus(post("/admin/3d/bind/"+kind+"/"+target, url.Values{}), 303)
		}
		body := get(assetPath).Body.String()
		if i < 2 && !strings.Contains(body, "model.glb?v="+digests[1-i]) {
			t.Fatalf("%s unbinding failed to restore inheritance", kind)
		}
		if i == 2 && strings.Contains(body, "data-model-viewer") {
			t.Fatal("all bindings absent must use static fallback")
		}
	}
	for _, id := range ids {
		assertStatus(post("/admin/3d/"+id+"/delete", url.Values{}), 303)
		assertStatus(get("/admin/3d/"+id), 404)
	}
	uploadBound := uploadWebResource(t, h, url.Values{"csrf_token": {csrf}, "name": {"Atomic dedicated resource"}, "kind": {"asset"}, "target": {asset}}, webTestGLB(), cookies)
	assertStatus(uploadBound, 303)
	boundID := strings.TrimPrefix(uploadBound.Header().Get("Location"), "/admin/3d/")
	picker := get("/admin/3d?kind=asset&target=" + asset)
	assertStatus(picker, 200)
	for _, want := range []string{"当前层级绑定", "Atomic dedicated resource", "My phone"} {
		if !strings.Contains(picker.Body.String(), want) {
			t.Fatalf("binding picker missing %s", want)
		}
	}
	if !strings.Contains(get(assetPath).Body.String(), "具体物品专用") {
		t.Fatal("asset must identify its dedicated resource")
	}
	assertStatus(post("/admin/3d/bind/asset/"+asset, url.Values{}), 303)
	assertStatus(post("/admin/3d/"+boundID+"/delete", url.Values{}), 303)
	failed := uploadWebResource(t, h, url.Values{"csrf_token": {csrf}, "name": {"Must not persist"}, "kind": {"asset"}, "target": {"00000000-0000-0000-0000-000000000001"}}, webTestGLB(), cookies)
	if failed.Code == 303 {
		t.Fatal("missing target upload binding should fail")
	}
	if strings.Contains(get("/admin/3d").Body.String(), "Must not persist") {
		t.Fatal("failed atomic upload binding must not create a library resource")
	}
	// Appearance upload is the same verified-blob/atomic-metadata workflow.
	badAppearance := uploadWebResourceAt(t, h, appearancePath+"/upload", url.Values{"csrf_token": {csrf}, "name": {"Rejected appearance"}}, webTestGLB(), cookies)
	assertStatus(badAppearance, 422)
	if strings.Contains(get("/admin/3d").Body.String(), "Rejected appearance") {
		t.Fatal("invalid appearance upload left a resource row")
	}
	assertStatus(uploadWebResourceAt(t, h, appearancePath+"/upload", url.Values{"name": {"Missing CSRF"}, "tag_ids": {blue}}, webTestGLB(), cookies), 403)
	boundAppearance := uploadWebResourceAt(t, h, appearancePath+"/upload", url.Values{"csrf_token": {csrf}, "name": {"Uploaded appearance"}, "tag_ids": {blue}}, webTestGLB(), cookies)
	assertStatus(boundAppearance, 303)
	if !bytes.Equal(get(assetPath+"/model.glb").Body.Bytes(), webTestGLB()) {
		t.Fatal("uploaded appearance not inherited")
	}
	duplicate := uploadWebResourceAt(t, h, appearancePath+"/upload", url.Values{"csrf_token": {csrf}, "name": {"Duplicate appearance"}, "tag_ids": {blue}}, webTestGLB(), cookies)
	assertStatus(duplicate, 422)
	if strings.Contains(get("/admin/3d").Body.String(), "Duplicate appearance") {
		t.Fatal("duplicate conditions retained new resource")
	}
	if !strings.Contains(duplicate.Body.String(), `value="`+blue+`" checked`) || !strings.Contains(duplicate.Body.String(), `value="Duplicate appearance"`) {
		t.Fatal("failed upload lost draft")
	}
}

func TestResourcePagingUploadValidationAndCSRF(t *testing.T) {
	h := newTestHandler(t)
	cookies, csrf := resourceSession(t, h)
	for i := 0; i < 26; i++ {
		r := uploadWebResource(t, h, url.Values{"csrf_token": {csrf}, "name": {fmt.Sprintf("Paged resource %02d", i)}}, webTestGLB(), cookies)
		if r.Code != 303 {
			t.Fatalf("upload: %d %s", r.Code, r.Body.String())
		}
	}
	page := request(t, h, http.MethodGet, "/admin/3d?q=Paged", nil, cookies)
	if page.Code != 200 || strings.Count(page.Body.String(), `data-label="资源名称"`) != 25 || !strings.Contains(page.Body.String(), "page=2") {
		t.Fatal("first resource page must contain 25 rows and a next link")
	}
	page = request(t, h, http.MethodGet, "/admin/3d?q=Paged&page=2", nil, cookies)
	if strings.Count(page.Body.String(), `data-label="资源名称"`) != 1 {
		t.Fatal("second resource page must contain remaining row")
	}
	bad := uploadWebResource(t, h, url.Values{"csrf_token": {csrf}, "name": {"Preserve draft"}, "model_3d_author": {"Draft author"}}, []byte("invalid GLB"), cookies)
	if bad.Code != 422 || !strings.Contains(bad.Body.String(), `value="Preserve draft"`) || !strings.Contains(bad.Body.String(), `value="Draft author"`) {
		t.Fatal("invalid upload should preserve metadata and offer retry")
	}
	bad = uploadWebResource(t, h, url.Values{"name": {"CSRF forbidden"}}, webTestGLB(), cookies)
	if bad.Code != 403 {
		t.Fatalf("upload missing CSRF=%d", bad.Code)
	}
	for _, path := range []string{"/admin/3d/bind/model/00000000-0000-0000-0000-000000000001", "/admin/3d/00000000-0000-0000-0000-000000000001", "/admin/3d/00000000-0000-0000-0000-000000000001/delete"} {
		if r := request(t, h, http.MethodPost, path, url.Values{}, cookies); r.Code != 403 {
			t.Fatalf("mutation without CSRF %s =%d", path, r.Code)
		}
	}
}

type retryDeleteBlob struct {
	application.BlobStore
	fail bool
}

func (b *retryDeleteBlob) Delete(ctx context.Context, key string) error {
	if b.fail {
		return fmt.Errorf("temporary test storage failure")
	}
	return b.BlobStore.Delete(ctx, key)
}

func TestResourceDeleteFailureCanRetry(t *testing.T) {
	var storage *retryDeleteBlob
	h := newTestHandlerWithBlob(t, func(original application.BlobStore) application.BlobStore {
		storage = &retryDeleteBlob{BlobStore: original, fail: true}
		return storage
	})
	cookies, csrf := resourceSession(t, h)
	upload := uploadWebResource(t, h, url.Values{"csrf_token": {csrf}, "name": {"Retry resource"}}, webTestGLB(), cookies)
	if upload.Code != 303 {
		t.Fatalf("upload=%d", upload.Code)
	}
	path := upload.Header().Get("Location")
	deleted := request(t, h, http.MethodPost, path+"/delete", url.Values{"csrf_token": {csrf}}, cookies)
	if deleted.Code != 409 || !strings.Contains(deleted.Body.String(), "重试删除") || strings.Contains(deleted.Body.String(), "data-model-viewer") {
		t.Fatalf("pending delete must show retry and no preview: %d %s", deleted.Code, deleted.Body.String())
	}
	storage.fail = false
	deleted = request(t, h, http.MethodPost, path+"/delete", url.Values{"csrf_token": {csrf}}, cookies)
	if deleted.Code != 303 {
		t.Fatalf("retry=%d body=%s", deleted.Code, deleted.Body.String())
	}
	if page := request(t, h, http.MethodGet, path, nil, cookies); page.Code != 404 {
		t.Fatalf("deleted resource=%d", page.Code)
	}
}

func TestResourceViewerReadAndWriteDenialLocalized(t *testing.T) {
	h := newTestHandlerWithBlob(t, nil)
	owner, csrf := resourceSession(t, h)
	request(t, h, "POST", "/admin/catalog/categories", url.Values{"csrf_token": {csrf}, "name": {"Viewer catalog"}, "icon_key": {"smartphone"}}, owner)
	catalogPage := request(t, h, "GET", "/admin/catalog", nil, owner)
	categoryID := optionID(t, catalogPage.Body.String(), "Viewer catalog")
	createdModel := request(t, h, "POST", "/admin/catalog/models", url.Values{"csrf_token": {csrf}, "category_id": {categoryID}, "name": {"Viewer phone"}, "flow": {"asset"}}, owner)
	if createdModel.Code != 303 || !strings.HasPrefix(createdModel.Header().Get("Location"), "/assets/new?model_id=") {
		t.Fatal("inline model creation still requires a legacy specification")
	}
	modelID := strings.TrimPrefix(createdModel.Header().Get("Location"), "/assets/new?model_id=")
	upload := uploadWebResource(t, h, url.Values{"csrf_token": {csrf}, "name": {"Viewer resource"}, "model_3d_author": {"Visible author"}}, webTestGLB(), owner)
	if upload.Code != 303 {
		t.Fatal(upload.Body.String())
	}
	path := upload.Header().Get("Location")
	member := request(t, h, http.MethodPost, "/admin/members", url.Values{"csrf_token": {csrf}, "username": {"reader"}, "password": {"reader secure password"}, "role": {"viewer"}}, owner)
	if member.Code != 303 {
		t.Fatal(member.Body.String())
	}
	login := request(t, h, http.MethodPost, "/login", url.Values{"csrf_token": {csrf}, "username": {"reader"}, "password": {"reader secure password"}}, owner[1:])
	viewer := []*http.Cookie{responseCookie(t, login, sessionCookie), owner[1]}
	preferences := request(t, h, http.MethodPost, "/preferences", url.Values{"csrf_token": {csrf}, "locale": {"en"}, "theme": {"light"}, "accent": {"emerald"}, "return_to": {"/admin/3d"}}, viewer)
	if preferences.Code != 303 {
		t.Fatal(preferences.Body.String())
	}
	appearancePath := "/admin/catalog/models/" + modelID + "/appearance"
	appearancePage := request(t, h, "GET", appearancePath, nil, viewer)
	if appearancePage.Code != 200 || !strings.Contains(appearancePage.Body.String(), "Appearance defaults") || strings.Contains(appearancePage.Body.String(), `method="post" action="`+appearancePath) {
		t.Fatal("viewer appearance page is not localized and read-only")
	}
	for _, target := range []string{appearancePath, appearancePath + "/upload"} {
		if response := request(t, h, "POST", target, url.Values{"csrf_token": {csrf}}, viewer); response.Code != 403 {
			t.Fatalf("viewer appearance mutation %s: %d", target, response.Code)
		}
	}
	for _, target := range []string{"/admin/3d", path, path + "/model.glb"} {
		page := request(t, h, http.MethodGet, target, nil, viewer)
		if page.Code != 200 {
			t.Fatalf("viewer read %s=%d", target, page.Code)
		}
		if strings.HasSuffix(target, ".glb") {
			continue
		}
		for _, forbidden := range []string{`data-dialog-open="resource-upload"`, `action="` + path + `/delete"`, `name="model_3d_author"`, `name="resource_id"`, `name="tag_ids"`, `name="category_ids"`} {
			if strings.Contains(page.Body.String(), forbidden) {
				t.Fatalf("viewer exposed mutation %s", forbidden)
			}
		}
		if !strings.Contains(page.Body.String(), `lang="en"`) || !strings.Contains(page.Body.String(), "3D resource library") {
			t.Fatal("resource English localization missing")
		}
	}
	for _, target := range []string{path, path + "/delete", path + "/tags", "/admin/3d/bind/model/00000000-0000-0000-0000-000000000001"} {
		r := request(t, h, http.MethodPost, target, url.Values{"csrf_token": {csrf}}, viewer)
		if r.Code != 403 {
			t.Fatalf("viewer mutation %s=%d", target, r.Code)
		}
	}
	if r := uploadWebResource(t, h, url.Values{"csrf_token": {csrf}, "name": {"Forbidden"}}, webTestGLB(), viewer); r.Code != 403 {
		t.Fatalf("viewer upload=%d", r.Code)
	}
}
