package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
)

func TestSetupLoginMemberPermissionsAndCSRF(t *testing.T) {
	handler := newTestHandler(t)

	response := request(t, handler, http.MethodPost, "/setup", url.Values{}, nil)
	if response.Code != http.StatusForbidden {
		t.Fatalf("setup without CSRF: got %d, want %d", response.Code, http.StatusForbidden)
	}

	setupPage := request(t, handler, http.MethodGet, "/setup", nil, nil)
	csrf := responseCookie(t, setupPage, csrfCookie)
	response = request(t, handler, http.MethodPost, "/setup", url.Values{
		"csrf_token": {csrf.Value}, "tenant_name": {"My Assets"}, "base_currency": {"CNY"},
		"username": {"owner"}, "password": {"owner secure password"},
	}, []*http.Cookie{csrf})
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/" {
		t.Fatalf("setup response: status=%d location=%q body=%s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	session := responseCookie(t, response, sessionCookie)

	dashboard := request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{session, csrf})
	if dashboard.Code != http.StatusOK || !strings.Contains(dashboard.Body.String(), "owner") {
		t.Fatalf("authenticated dashboard: status=%d body=%s", dashboard.Code, dashboard.Body.String())
	}
	if dashboard.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("security headers were not applied")
	}

	response = request(t, handler, http.MethodPost, "/admin/members", url.Values{
		"csrf_token": {csrf.Value}, "username": {"editor"}, "password": {"editor secure password"}, "role": {"editor"},
	}, []*http.Cookie{session, csrf})
	if response.Code != http.StatusSeeOther {
		t.Fatalf("owner create editor: status=%d body=%s", response.Code, response.Body.String())
	}
	members := request(t, handler, http.MethodGet, "/admin/members", nil, []*http.Cookie{session, csrf})
	if members.Code != http.StatusOK || !strings.Contains(members.Body.String(), "editor") {
		t.Fatalf("members page: status=%d body=%s", members.Code, members.Body.String())
	}

	loginPage := request(t, handler, http.MethodGet, "/login", nil, []*http.Cookie{csrf})
	if loginPage.Code != http.StatusOK {
		t.Fatalf("login form: status=%d body=%s", loginPage.Code, loginPage.Body.String())
	}
	response = request(t, handler, http.MethodPost, "/login", url.Values{
		"csrf_token": {csrf.Value}, "username": {"editor"}, "password": {"editor secure password"},
	}, []*http.Cookie{csrf})
	if response.Code != http.StatusSeeOther {
		t.Fatalf("editor login: status=%d body=%s", response.Code, response.Body.String())
	}
	editorSession := responseCookie(t, response, sessionCookie)
	forbidden := request(t, handler, http.MethodGet, "/admin/members", nil, []*http.Cookie{editorSession, csrf})
	if forbidden.Code != http.StatusForbidden || !strings.Contains(forbidden.Body.String(), "只有 Owner") {
		t.Fatalf("editor member access: status=%d body=%s", forbidden.Code, forbidden.Body.String())
	}
}

func TestUnauthenticatedDashboardRedirectsToLogin(t *testing.T) {
	handler := newTestHandler(t)
	response := request(t, handler, http.MethodGet, "/", nil, nil)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
		t.Fatalf("unexpected unauthenticated response: status=%d location=%q", response.Code, response.Header().Get("Location"))
	}
}

func TestCatalogHierarchyAssetDetailAndViewerWriteDenial(t *testing.T) {
	handler := newTestHandler(t)
	setupPage := request(t, handler, http.MethodGet, "/setup", nil, nil)
	csrf := responseCookie(t, setupPage, csrfCookie)
	setup := request(t, handler, http.MethodPost, "/setup", url.Values{
		"csrf_token": {csrf.Value}, "tenant_name": {"Catalog Tenant"}, "base_currency": {"CNY"},
		"username": {"owner"}, "password": {"owner secure password"},
	}, []*http.Cookie{csrf})
	ownerSession := responseCookie(t, setup, sessionCookie)

	created := request(t, handler, http.MethodPost, "/catalog/categories", url.Values{
		"csrf_token": {csrf.Value}, "name": {"手机"},
	}, []*http.Cookie{ownerSession, csrf})
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create category: status=%d body=%s", created.Code, created.Body.String())
	}
	catalog := request(t, handler, http.MethodGet, "/catalog", nil, []*http.Cookie{ownerSession, csrf})
	categoryID := optionID(t, catalog.Body.String(), "手机")

	created = request(t, handler, http.MethodPost, "/catalog/models", url.Values{
		"csrf_token": {csrf.Value}, "category_id": {categoryID}, "name": {"iPhone 17 Pro"},
	}, []*http.Cookie{ownerSession, csrf})
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create model: status=%d body=%s", created.Code, created.Body.String())
	}
	catalog = request(t, handler, http.MethodGet, "/catalog", nil, []*http.Cookie{ownerSession, csrf})
	modelID := optionID(t, catalog.Body.String(), "手机 / iPhone 17 Pro")

	created = request(t, handler, http.MethodPost, "/catalog/variants", url.Values{
		"csrf_token": {csrf.Value}, "model_id": {modelID}, "name": {"256GB"},
	}, []*http.Cookie{ownerSession, csrf})
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create variant: status=%d body=%s", created.Code, created.Body.String())
	}
	catalog = request(t, handler, http.MethodGet, "/catalog", nil, []*http.Cookie{ownerSession, csrf})
	variantID := optionID(t, catalog.Body.String(), "手机 / iPhone 17 Pro / 256GB")

	created = request(t, handler, http.MethodPost, "/catalog/assets", url.Values{
		"csrf_token": {csrf.Value}, "variant_id": {variantID}, "display_name": {"我的主力手机"},
		"serial_number": {"WEB-SERIAL-001"}, "color": {"黑色"}, "purchase_channel": {"官方商城"},
		"notes": {"Web 全要素目录记录"},
	}, []*http.Cookie{ownerSession, csrf})
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create asset: status=%d body=%s", created.Code, created.Body.String())
	}
	catalog = request(t, handler, http.MethodGet, "/catalog", nil, []*http.Cookie{ownerSession, csrf})
	if catalog.Code != http.StatusOK || !strings.Contains(catalog.Body.String(), "我的主力手机") || !strings.Contains(catalog.Body.String(), "WEB-SERIAL-001") {
		t.Fatalf("catalog asset list: status=%d body=%s", catalog.Code, catalog.Body.String())
	}
	match := regexp.MustCompile(`/assets/([0-9a-f-]{36})`).FindStringSubmatch(catalog.Body.String())
	if len(match) != 2 {
		t.Fatalf("asset detail link not found: %s", catalog.Body.String())
	}
	detail := request(t, handler, http.MethodGet, "/assets/"+match[1], nil, []*http.Cookie{ownerSession, csrf})
	for _, want := range []string{"我的主力手机", "iPhone 17 Pro", "256GB", "WEB-SERIAL-001", "官方商城", "Web 全要素目录记录"} {
		if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), want) {
			t.Fatalf("asset detail missing %q: status=%d body=%s", want, detail.Code, detail.Body.String())
		}
	}

	created = request(t, handler, http.MethodPost, "/admin/members", url.Values{
		"csrf_token": {csrf.Value}, "username": {"viewer"}, "password": {"viewer secure password"}, "role": {"viewer"},
	}, []*http.Cookie{ownerSession, csrf})
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create viewer: status=%d body=%s", created.Code, created.Body.String())
	}
	login := request(t, handler, http.MethodPost, "/login", url.Values{
		"csrf_token": {csrf.Value}, "username": {"viewer"}, "password": {"viewer secure password"},
	}, []*http.Cookie{csrf})
	viewerSession := responseCookie(t, login, sessionCookie)
	visible := request(t, handler, http.MethodGet, "/catalog", nil, []*http.Cookie{viewerSession, csrf})
	if visible.Code != http.StatusOK || !strings.Contains(visible.Body.String(), "我的主力手机") {
		t.Fatalf("viewer should read catalog: status=%d body=%s", visible.Code, visible.Body.String())
	}
	forbidden := request(t, handler, http.MethodPost, "/catalog/categories", url.Values{
		"csrf_token": {csrf.Value}, "name": {"不应创建"},
	}, []*http.Cookie{viewerSession, csrf})
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("viewer catalog write: got %d, want %d", forbidden.Code, http.StatusForbidden)
	}
}

func TestLoginLimiter(t *testing.T) {
	limiter := newLoginLimiter(2, time.Minute)
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if !limiter.Allow("client", now) || !limiter.Allow("client", now) || limiter.Allow("client", now) {
		t.Fatal("limiter did not enforce the configured attempt count")
	}
	if !limiter.Allow("client", now.Add(2*time.Minute)) {
		t.Fatal("limiter did not reset after its window")
	}
}

func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "web.db")}
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
		t.Fatal(err)
	}
	adapter := sqlite.New(db)
	auth := application.NewAuthService(adapter)
	catalog := application.NewCatalogService(adapter)
	server, err := New(auth, catalog, db, Options{AuthMode: "local"})
	if err != nil {
		t.Fatal(err)
	}
	return server.Handler()
}

func optionID(t *testing.T, body, label string) string {
	t.Helper()
	pattern := `value="([0-9a-f-]{36})">` + regexp.QuoteMeta(label) + `</option>`
	match := regexp.MustCompile(pattern).FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatalf("option %q not found in body: %s", label, body)
	}
	return match[1]
}

func request(t *testing.T, handler http.Handler, method, target string, form url.Values, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, target, body)
	req.RemoteAddr = "192.0.2.10:1234"
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func responseCookie(t *testing.T, response *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("response did not contain %s cookie", name)
	return nil
}
