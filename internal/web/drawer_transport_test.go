package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestDrawerTagTransport(t *testing.T) {
	h := newTestHandlerWithBlob(t, nil)
	cookies, csrf := resourceSession(t, h)
	send := func(method, path string, form url.Values, withCSRF bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("X-Assetloop-Drawer", "tag-editor")
		for _, c := range cookies {
			r.AddCookie(c)
		}
		if withCSRF {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	page := send("GET", "/admin/tags?view=types&dialog=tag-editor", nil, false)
	if page.Code != 200 || !strings.Contains(page.Body.String(), "<dialog") || strings.Contains(page.Body.String(), "<script") || strings.Contains(page.Body.String(), "<table") {
		t.Fatal("not a pure drawer fragment", page.Code, page.Body.String())
	}
	bad := send("POST", "/admin/tags/types", url.Values{"name": {"Nested"}}, false)
	if bad.Code != http.StatusForbidden {
		t.Fatal("fragment bypassed CSRF")
	}
	created := send("POST", "/admin/tags/types", url.Values{"name": {"Nested"}, "enabled": {"1"}, "csrf_token": {csrf}}, false)
	var result struct {
		Kind, ID, Name string
		Enabled        bool
	}
	if created.Code != 200 || json.Unmarshal(created.Body.Bytes(), &result) != nil || result.Kind != "tag-type" || result.ID == "" {
		t.Fatal("missing semantic save result", created.Code, created.Body.String())
	}
	failed := send("POST", "/admin/tags/types/"+result.ID, url.Values{"name": {""}, "csrf_token": {csrf}}, false)
	if failed.Code != 422 || !strings.Contains(failed.Body.String(), "data-error-summary") || strings.Contains(failed.Body.String(), "<table") {
		t.Fatal("validation is not local", failed.Code)
	}
}
