package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestSettingsNavigationAndPermissions(t *testing.T) {
	h := newTestHandler(t)
	anonymous := request(t, h, "GET", "/settings", nil, nil)
	if anonymous.Code != 303 || anonymous.Header().Get("Location") != "/login" {
		t.Fatal("settings must require authentication")
	}
	setup := request(t, h, "GET", "/setup", nil, nil)
	csrf := responseCookie(t, setup, csrfCookie)
	created := request(t, h, "POST", "/setup", url.Values{
		"csrf_token": {csrf.Value}, "tenant_name": {"Settings"}, "base_currency": {"CNY"},
		"username": {"owner"}, "password": {"owner secure password"},
	}, []*http.Cookie{csrf})
	cookies := []*http.Cookie{csrf, responseCookie(t, created, sessionCookie)}
	start := request(t, h, "GET", "/settings", nil, cookies)
	if start.Code != 303 || start.Header().Get("Location") != "/admin/catalog" {
		t.Fatal("owner landing")
	}
	for _, path := range []string{"/admin/catalog", "/admin/tags?view=types", "/admin/3d", "/admin/event-types"} {
		page := request(t, h, "GET", path, nil, cookies)
		if page.Code != 200 {
			t.Fatalf("%s: %d", path, page.Code)
		}
		body := page.Body.String()
		for _, want := range []string{`href="/settings"`, `id="settings-content"`, `class="settings-tabs"`, `src="/static/settings.js"`, `data-settings-tab="catalog"`, `data-settings-tab="tags"`, `data-settings-tab="3d"`, `data-settings-tab="event-types"`} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %s", path, want)
			}
		}
	}
	members := request(t, h, "GET", "/admin/members", nil, cookies)
	if strings.Contains(members.Body.String(), `id="settings-content"`) {
		t.Fatal("members must stay independent")
	}
	add := request(t, h, "POST", "/admin/members", url.Values{"csrf_token": {csrf.Value}, "username": {"reader"}, "password": {"reader secure password"}, "role": {"viewer"}}, cookies)
	if add.Code != 303 {
		t.Fatalf("add viewer: %d", add.Code)
	}
	login := request(t, h, "POST", "/login", url.Values{"csrf_token": {csrf.Value}, "username": {"reader"}, "password": {"reader secure password"}}, []*http.Cookie{csrf})
	viewer := []*http.Cookie{csrf, responseCookie(t, login, sessionCookie)}
	start = request(t, h, "GET", "/settings", nil, viewer)
	if start.Code != 303 || start.Header().Get("Location") != "/admin/tags" {
		t.Fatal("viewer landing")
	}
	page := request(t, h, "GET", "/admin/tags", nil, viewer)
	if strings.Contains(page.Body.String(), `data-settings-tab="catalog"`) || strings.Contains(page.Body.String(), `href="/admin/members"`) {
		t.Fatal("viewer navigation elevated permissions")
	}
	denied := request(t, h, "GET", "/admin/catalog", nil, viewer)
	if denied.Code != 403 {
		t.Fatal("catalog remains protected")
	}
}

func TestSettingsDeepSections(t *testing.T) {
	for path, want := range map[string]string{"/admin/catalog/models/id/appearance": "catalog", "/admin/3d/id": "3d", "/admin/tags": "tags", "/admin/members": "", "/admin/3d-other": "", "/assets/id/edit": ""} {
		if got := settingsSection(path); got != want {
			t.Fatalf("%s: %s", path, got)
		}
	}
}
