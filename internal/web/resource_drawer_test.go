package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestResourceDrawerUnifiedSave(t *testing.T) {
	h := newTestHandlerWithBlob(t, nil)
	cookies, csrf := resourceSession(t, h)
	uploaded := uploadWebResource(t, h, url.Values{"csrf_token": {csrf}, "name": {"Original"}}, webTestGLB(), cookies)
	path := uploaded.Header().Get("Location")
	if uploaded.Code != 303 {
		t.Fatal(uploaded.Code)
	}
	get := func() string { return request(t, h, "GET", path, nil, cookies).Body.String() }
	body := get()
	if !strings.Contains(body, `data-resource-editor`) || !strings.Contains(body, `form="resource-form"`) || !strings.Contains(body, `resource-library-table`) {
		t.Fatal("missing list-backed unified drawer")
	}
	values := url.Values{"resource_configuration": {"1"}, "name": {"Updated"}, "model_3d_author": {"New author"}}
	if r := request(t, h, "POST", path, values, cookies); r.Code != http.StatusForbidden {
		t.Fatal("missing csrf accepted")
	}
	values.Set("csrf_token", csrf)
	values.Set("tag_ids", "00000000-0000-0000-0000-000000000099")
	if r := request(t, h, "POST", path, values, cookies); r.Code != 422 || !strings.Contains(r.Body.String(), `value="Updated"`) || strings.Contains(r.Body.String(), `id="resource-upload"`) {
		t.Fatal("invalid tag should preserve draft in only the resource drawer", r.Code)
	}
	if strings.Contains(get(), `value="Updated"`) {
		t.Fatal("failed save persisted metadata")
	}
	values.Del("tag_ids")
	if r := request(t, h, "POST", path, values, cookies); r.Code != 303 || r.Header().Get("Location") != "/admin/3d" {
		t.Fatal("unified save did not return to library", r.Code)
	}
	if !strings.Contains(get(), `value="Updated"`) || !strings.Contains(get(), `value="New author"`) {
		t.Fatal("saved metadata missing")
	}
}
