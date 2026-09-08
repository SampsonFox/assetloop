package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestLoginPreservesAuthorizationReturn(t *testing.T) {
	h := newTestHandler(t)
	setup := request(t, h, "GET", "/setup", nil, nil)
	csrf := responseCookie(t, setup, csrfCookie)
	created := request(t, h, "POST", "/setup", url.Values{"csrf_token": {csrf.Value}, "tenant_name": {"Test"}, "base_currency": {"CNY"}, "username": {"owner"}, "password": {"owner secure password"}}, []*http.Cookie{csrf})
	session := responseCookie(t, created, sessionCookie)
	target := "/oauth/authorize?client_id=codex-local&state=test-state"
	page := request(t, h, "GET", "/login?return_to="+url.QueryEscape(target), nil, []*http.Cookie{csrf})
	if page.Code != 200 || !strings.Contains(page.Body.String(), `name="return_to"`) || !strings.Contains(page.Body.String(), "test-state") {
		t.Fatal("login form lost authorization return target")
	}
	form := url.Values{"csrf_token": {csrf.Value}, "username": {"owner"}, "password": {"wrong"}, "return_to": {target}}
	failed := request(t, h, "POST", "/login", form, []*http.Cookie{csrf})
	if failed.Code != 401 || !strings.Contains(failed.Body.String(), "test-state") {
		t.Fatal("failed login lost return target")
	}
	form.Set("password", "owner secure password")
	success := request(t, h, "POST", "/login", form, []*http.Cookie{csrf})
	if success.Code != 303 || success.Header().Get("Location") != target {
		t.Fatalf("login destination: %d %q", success.Code, success.Header().Get("Location"))
	}
	already := request(t, h, "GET", "/login?return_to="+url.QueryEscape(target), nil, []*http.Cookie{session, csrf})
	if already.Code != 303 || already.Header().Get("Location") != target {
		t.Fatal("authenticated login lost target")
	}
	for _, bad := range []string{"https://evil.invalid/", "//evil.invalid/", `/\evil.invalid/`, "/%2fevil.invalid/", "/%5cevil.invalid/", "/ok%0d%0aLocation:evil"} {
		form.Set("return_to", bad)
		result := request(t, h, "POST", "/login", form, []*http.Cookie{csrf})
		if result.Code != 400 {
			t.Fatalf("unsafe return accepted: %q status %d", bad, result.Code)
		}
	}
}
