package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/blob"
	localblob "github.com/SampsonFox/assetloop/internal/blob/local"
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
	for _, want := range []string{`list="setup-currencies"`, `id="setup-currencies"`, `<option value="AED"></option>`, `<option value="BHD"></option>`, `<option value="ZWG"></option>`} {
		if setupPage.Code != http.StatusOK || !strings.Contains(setupPage.Body.String(), want) {
			t.Fatalf("setup currency catalog missing %q: status=%d body=%s", want, setupPage.Code, setupPage.Body.String())
		}
	}
	if strings.Contains(setupPage.Body.String(), `<option value="BGN"></option>`) || strings.Contains(setupPage.Body.String(), `<option value="XAU"></option>`) {
		t.Fatalf("setup must not offer retired or non-currency ISO codes: %s", setupPage.Body.String())
	}
	invalidCurrency := request(t, handler, http.MethodPost, "/setup", url.Values{
		"csrf_token": {csrf.Value}, "tenant_name": {"My Assets"}, "base_currency": {"BGN"},
		"username": {"owner"}, "password": {"owner secure password"},
	}, []*http.Cookie{csrf})
	if invalidCurrency.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidCurrency.Body.String(), "请选择当前支持的 ISO 4217 货币代码") {
		t.Fatalf("retired base currency should be rejected: status=%d body=%s", invalidCurrency.Code, invalidCurrency.Body.String())
	}
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
	if !strings.Contains(dashboard.Body.String(), `<a class="brand" href="/overview" translate="no">`) || strings.Contains(dashboard.Body.String(), `>概览</a>`) {
		t.Fatalf("brand must be the sole overview entry: %s", dashboard.Body.String())
	}
	if !strings.Contains(dashboard.Body.String(), `href="/" aria-current="page"`) {
		t.Fatalf("asset navigation must expose the current page: %s", dashboard.Body.String())
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
	if members.Code != http.StatusOK || !strings.Contains(members.Body.String(), "editor") || !strings.Contains(members.Body.String(), `name="q"`) || !strings.Contains(members.Body.String(), `name="role"`) || !strings.Contains(members.Body.String(), `aria-sort="ascending"`) {
		t.Fatalf("members page: status=%d body=%s", members.Code, members.Body.String())
	}
	filteredMembers := request(t, handler, http.MethodGet, "/admin/members?q=editor&role=editor&sort=created&direction=desc", nil, []*http.Cookie{session, csrf})
	if filteredMembers.Code != http.StatusOK || !strings.Contains(filteredMembers.Body.String(), "editor") || strings.Contains(filteredMembers.Body.String(), `data-label="用户名">owner`) {
		t.Fatalf("server-filtered member page: status=%d body=%s", filteredMembers.Code, filteredMembers.Body.String())
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
	editorHome := request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{editorSession, csrf})
	if !strings.Contains(editorHome.Body.String(), `href="/admin/catalog"`) || strings.Contains(editorHome.Body.String(), `href="/imports"`) || strings.Contains(editorHome.Body.String(), `href="/admin/members"`) {
		t.Fatalf("editor account menu has incorrect entries: %s", editorHome.Body.String())
	}
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

func TestAnonymousLocaleAndAccountPreferences(t *testing.T) {
	handler := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	setupPage := httptest.NewRecorder()
	handler.ServeHTTP(setupPage, req)
	if setupPage.Code != http.StatusOK || !strings.Contains(setupPage.Body.String(), `<html lang="en" data-theme="system" data-accent="emerald">`) || !strings.Contains(setupPage.Body.String(), "Create your AssetLoop workspace") {
		t.Fatalf("anonymous browser locale was not applied: status=%d body=%s", setupPage.Code, setupPage.Body.String())
	}
	csrf := responseCookie(t, setupPage, csrfCookie)
	localeChange := request(t, handler, http.MethodPost, "/locale", url.Values{"csrf_token": {csrf.Value}, "locale": {"zh-CN"}, "return_to": {"/setup"}}, []*http.Cookie{csrf})
	if localeChange.Code != http.StatusSeeOther || responseCookie(t, localeChange, localeCookie).Value != "zh-CN" {
		t.Fatalf("anonymous locale update: status=%d body=%s", localeChange.Code, localeChange.Body.String())
	}
	localeCookieValue := responseCookie(t, localeChange, localeCookie)
	req = httptest.NewRequest(http.MethodGet, "/setup", nil)
	req.Header.Set("Accept-Language", "en-US")
	req.AddCookie(localeCookieValue)
	cookieOverride := httptest.NewRecorder()
	handler.ServeHTTP(cookieOverride, req)
	if !strings.Contains(cookieOverride.Body.String(), `<html lang="zh-CN"`) {
		t.Fatalf("locale cookie did not override browser language: %s", cookieOverride.Body.String())
	}

	setup := request(t, handler, http.MethodPost, "/setup", url.Values{
		"csrf_token": {csrf.Value}, "tenant_name": {"Preference Tenant"}, "base_currency": {"CNY"},
		"username": {"owner"}, "password": {"owner secure password"},
	}, []*http.Cookie{csrf})
	session := responseCookie(t, setup, sessionCookie)
	home := request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{session, csrf})
	for _, want := range []string{`class="account-menu"`, `summary aria-label="用户菜单"`, `href="/admin/catalog"`, `href="/admin/members"`, `action="/preferences"`} {
		if !strings.Contains(home.Body.String(), want) {
			t.Fatalf("owner account menu missing %q: %s", want, home.Body.String())
		}
	}
	if strings.Contains(home.Body.String(), `href="/imports"`) || strings.Contains(home.Body.String(), "待确认导入") {
		t.Fatalf("account menu must not expose a second confirmation queue: %s", home.Body.String())
	}
	for _, want := range []string{`class="account-avatar"`, `class="menu-icon"`, `data-auto-submit`, `class="segmented language-segmented"`, `role="radiogroup"`, `type="radio" name="locale"`, `type="radio" name="theme"`, `type="radio" name="accent"`, `class="accent-picker"`} {
		if !strings.Contains(home.Body.String(), want) {
			t.Fatalf("compact account control missing %q: %s", want, home.Body.String())
		}
	}
	if strings.Contains(home.Body.String(), `<select name="theme"`) || strings.Contains(home.Body.String(), `>保存偏好</button>`) {
		t.Fatalf("account preferences should switch directly without select boxes or a save button: %s", home.Body.String())
	}

	updated := request(t, handler, http.MethodPost, "/preferences", url.Values{
		"csrf_token": {csrf.Value}, "locale": {"en"}, "theme": {"dark"}, "accent": {"violet"}, "return_to": {"/?view=grid"},
	}, []*http.Cookie{session, csrf})
	if updated.Code != http.StatusSeeOther || updated.Header().Get("Location") != "/?view=grid" {
		t.Fatalf("preference update: status=%d location=%q body=%s", updated.Code, updated.Header().Get("Location"), updated.Body.String())
	}
	zhCookie := &http.Cookie{Name: localeCookie, Value: "zh-CN"}
	english := request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{session, csrf, zhCookie})
	for _, want := range []string{`<html lang="en" data-theme="dark" data-accent="violet">`, "My assets", "Asset type settings", "Log out"} {
		if !strings.Contains(english.Body.String(), want) {
			t.Fatalf("stored preference missing %q: %s", want, english.Body.String())
		}
	}

	invalidReturn := request(t, handler, http.MethodPost, "/preferences", url.Values{
		"csrf_token": {csrf.Value}, "locale": {"zh-CN"}, "theme": {"light"}, "accent": {"rose"}, "return_to": {"https://example.com"},
	}, []*http.Cookie{session, csrf})
	if invalidReturn.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidReturn.Body.String(), "Invalid return address") {
		t.Fatalf("external return target: status=%d body=%s", invalidReturn.Code, invalidReturn.Body.String())
	}
	unchanged := request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{session, csrf})
	if !strings.Contains(unchanged.Body.String(), `lang="en" data-theme="dark" data-accent="violet"`) {
		t.Fatalf("invalid return target mutated preferences: %s", unchanged.Body.String())
	}
	light := request(t, handler, http.MethodPost, "/preferences", url.Values{
		"csrf_token": {csrf.Value}, "locale": {"zh-CN"}, "theme": {"light"}, "accent": {"amber"}, "return_to": {"/"},
	}, []*http.Cookie{session, csrf})
	if light.Code != http.StatusSeeOther {
		t.Fatalf("light theme update: status=%d body=%s", light.Code, light.Body.String())
	}
	lightPage := request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{session, csrf})
	if !strings.Contains(lightPage.Body.String(), `lang="zh-CN" data-theme="light" data-accent="amber"`) {
		t.Fatalf("light theme did not render server-side: %s", lightPage.Body.String())
	}

	withoutCSRF := request(t, handler, http.MethodPost, "/preferences", url.Values{"locale": {"zh-CN"}, "theme": {"light"}, "accent": {"emerald"}}, []*http.Cookie{session})
	if withoutCSRF.Code != http.StatusForbidden {
		t.Fatalf("preferences without CSRF: got %d", withoutCSRF.Code)
	}
}

func TestThemeStylesUseSemanticSurfaces(t *testing.T) {
	handler := newTestHandler(t)
	stylesheet := request(t, handler, http.MethodGet, "/static/app.css", nil, nil)
	for _, want := range []string{`:root[data-theme="dark"]`, `:root[data-theme="system"]`, `:root[data-accent="violet"]`, `prefers-color-scheme:dark`, `background:var(--card)`, `background:var(--field)`, `.account-menu-panel`, `.accent-picker`} {
		if stylesheet.Code != http.StatusOK || !strings.Contains(stylesheet.Body.String(), want) {
			t.Fatalf("theme stylesheet missing %q: status=%d body=%s", want, stylesheet.Code, stylesheet.Body.String())
		}
	}
	script := request(t, handler, http.MethodGet, "/static/app.js", nil, nil)
	for _, want := range []string{`[data-auto-submit]`, `document.documentElement.dataset.theme`, `document.documentElement.dataset.accent`, `form.requestSubmit()`} {
		if script.Code != http.StatusOK || !strings.Contains(script.Body.String(), want) {
			t.Fatalf("preference interaction missing %q: status=%d body=%s", want, script.Code, script.Body.String())
		}
	}
}

func TestFrontendQualityGuardrails(t *testing.T) {
	handler := newTestHandler(t)
	setup := request(t, handler, http.MethodGet, "/setup", nil, nil)
	for _, want := range []string{
		`name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover"`,
		`name="theme-color"`,
		`class="skip-link" href="#main-content"`,
		`<main class="shell" id="main-content" tabindex="-1">`,
		`class="brand" href="/overview" translate="no"`,
	} {
		if setup.Code != http.StatusOK || !strings.Contains(setup.Body.String(), want) {
			t.Fatalf("base accessibility markup missing %q: status=%d body=%s", want, setup.Code, setup.Body.String())
		}
	}

	stylesheet := request(t, handler, http.MethodGet, "/static/app.css", nil, nil)
	for _, want := range []string{
		`.skip-link {`,
		`font-variant-numeric:tabular-nums`,
		`overscroll-behavior:contain`,
		`touch-action:manipulation`,
		`@media (prefers-reduced-motion:reduce)`,
		`@media (pointer:coarse)`,
	} {
		if stylesheet.Code != http.StatusOK || !strings.Contains(stylesheet.Body.String(), want) {
			t.Fatalf("frontend quality stylesheet missing %q: status=%d body=%s", want, stylesheet.Code, stylesheet.Body.String())
		}
	}

	script := request(t, handler, http.MethodGet, "/static/app.js", nil, nil)
	for _, want := range []string{
		`[data-error-summary]`,
		`[data-dialog-initial-focus]`,
		`data-guard-dirty`,
		`beforeunload`,
		`dataset.submitting`,
	} {
		if script.Code != http.StatusOK || !strings.Contains(script.Body.String(), want) {
			t.Fatalf("frontend interaction guard missing %q: status=%d body=%s", want, script.Code, script.Body.String())
		}
	}
}

func TestCatalogManagementListUsesTagsAndContainedDrawer(t *testing.T) {
	handler := newTestHandler(t)
	stylesheet := request(t, handler, http.MethodGet, "/static/app.css", nil, nil)
	for _, want := range []string{`.catalog-table-card { padding:0; }`, `.variant-tags { display:flex; flex-wrap:wrap;`, `.variant-manager { padding-top:22px; border-top:1px solid var(--line); }`} {
		if stylesheet.Code != http.StatusOK || !strings.Contains(stylesheet.Body.String(), want) {
			t.Fatalf("catalog management styles missing %q: status=%d body=%s", want, stylesheet.Code, stylesheet.Body.String())
		}
	}
}

func TestAssetPageMobileActionsStayHorizontalAndLeftAligned(t *testing.T) {
	handler := newTestHandler(t)
	stylesheet := request(t, handler, http.MethodGet, "/static/app.css", nil, nil)
	want := `.heading-actions { align-self:stretch; align-items:center; justify-content:flex-start; flex-wrap:wrap; }`
	if stylesheet.Code != http.StatusOK || !strings.Contains(stylesheet.Body.String(), want) {
		t.Fatalf("mobile asset heading actions must stay compact and left aligned: status=%d body=%s", stylesheet.Code, stylesheet.Body.String())
	}
}

func TestFXFieldsOnlyExpandForForeignCurrency(t *testing.T) {
	handler := newTestHandler(t)
	script := request(t, handler, http.MethodGet, "/static/app.js", nil, nil)
	for _, want := range []string{`[data-currency-select]`, `select.value !== select.dataset.baseCurrency`, `[data-fx-field]`, `field.hidden = !foreign`, `[data-fx-required]`} {
		if script.Code != http.StatusOK || !strings.Contains(script.Body.String(), want) {
			t.Fatalf("conditional FX interaction missing %q: status=%d body=%s", want, script.Code, script.Body.String())
		}
	}
}

func TestLifecycleFormUsesResponsiveDrawerInteraction(t *testing.T) {
	handler := newTestHandler(t)
	stylesheet := request(t, handler, http.MethodGet, "/static/app.css", nil, nil)
	for _, want := range []string{`.timeline-heading {`, `.timeline-filters .asset-search { flex:0 1 360px;`, `.timeline-filters .asset-filter-disclosure summary { justify-content:center; gap:0;`, `.timeline-filters.asset-search-shell { flex-direction:row; align-items:end; padding:0;`, `.timeline-filters.asset-search-shell .asset-search { flex:1 1 130px;`, `.event-drawer-panel { display:flex; flex-direction:column;`, `.event-form-grid { display:grid; grid-template-columns:repeat(2,minmax(0,1fr));`, `.drawer-footer {`} {
		if stylesheet.Code != http.StatusOK || !strings.Contains(stylesheet.Body.String(), want) {
			t.Fatalf("lifecycle drawer style missing %q: status=%d body=%s", want, stylesheet.Code, stylesheet.Body.String())
		}
	}
	script := request(t, handler, http.MethodGet, "/static/app.js", nil, nil)
	for _, want := range []string{`window.location.hash === "#add-event"`, `document.querySelector("#event-drawer .error")`, `document.querySelector('[data-dialog-open="event-drawer"]')`, `event.target.matches("dialog.drawer")`, `event.target.close()`} {
		if script.Code != http.StatusOK || !strings.Contains(script.Body.String(), want) {
			t.Fatalf("lifecycle drawer interaction missing %q: status=%d body=%s", want, script.Code, script.Body.String())
		}
	}
}

func TestAssetDetailsUseResponsiveGrid(t *testing.T) {
	handler := newTestHandler(t)
	stylesheet := request(t, handler, http.MethodGet, "/static/app.css", nil, nil)
	for _, want := range []string{`.asset-profile { display:grid; grid-template-columns:minmax(300px,.9fr) minmax(0,1.1fr);`, `.asset-product-image {`, `.asset-details-grid { display:grid; grid-template-columns:repeat(auto-fit,minmax(160px,1fr));`, `.asset-notes {`, `.timeline-summary { display:grid; grid-template-columns:repeat(3,minmax(0,1fr));`, `.timeline-summary dd {`} {
		if stylesheet.Code != http.StatusOK || !strings.Contains(stylesheet.Body.String(), want) {
			t.Fatalf("responsive asset details style missing %q: status=%d body=%s", want, stylesheet.Code, stylesheet.Body.String())
		}
	}
}

func TestAssetListIsPrimaryAndViewPreferencePersists(t *testing.T) {
	handler := newTestHandler(t)
	setupPage := request(t, handler, http.MethodGet, "/setup", nil, nil)
	csrf := responseCookie(t, setupPage, csrfCookie)
	setup := request(t, handler, http.MethodPost, "/setup", url.Values{
		"csrf_token": {csrf.Value}, "tenant_name": {"List Tenant"}, "base_currency": {"CNY"},
		"username": {"owner"}, "password": {"owner secure password"},
	}, []*http.Cookie{csrf})
	session := responseCookie(t, setup, sessionCookie)

	home := request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{session, csrf})
	for _, want := range []string{"我的物品", "还没有物品", "录入第一个物品", `href="/assets/new"`} {
		if home.Code != http.StatusOK || !strings.Contains(home.Body.String(), want) {
			t.Fatalf("asset-first empty state missing %q: status=%d body=%s", want, home.Code, home.Body.String())
		}
	}
	if strings.Contains(home.Body.String(), `>前往物品配置</a>`) {
		t.Fatalf("asset-list empty state must not replace the primary create action with catalog configuration: %s", home.Body.String())
	}
	for _, unwanted := range []string{`id="asset-form"`, `id="asset-drawer"`, `data-dialog-open="asset-drawer"`} {
		if strings.Contains(home.Body.String(), unwanted) {
			t.Fatalf("asset list must not embed the asset editor %q: %s", unwanted, home.Body.String())
		}
	}
	newPage := request(t, handler, http.MethodGet, "/assets/new", nil, []*http.Cookie{session, csrf})
	for _, want := range []string{`class="card asset-profile asset-editor-profile"`, `data-dialog-open="variant-drawer"`, `href="/"`} {
		if newPage.Code != http.StatusOK || !strings.Contains(newPage.Body.String(), want) {
			t.Fatalf("dedicated asset create page missing %q: status=%d body=%s", want, newPage.Code, newPage.Body.String())
		}
	}
	invalidCreate := request(t, handler, http.MethodPost, "/assets", url.Values{
		"csrf_token": {csrf.Value}, "display_name": {"未完成物品"},
	}, []*http.Cookie{session, csrf})
	if invalidCreate.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidCreate.Body.String(), `class="card asset-profile asset-editor-profile"`) || strings.Contains(invalidCreate.Body.String(), `<h1>我的物品</h1>`) {
		t.Fatalf("invalid asset create must stay in the dedicated editor: status=%d body=%s", invalidCreate.Code, invalidCreate.Body.String())
	}

	grid := request(t, handler, http.MethodGet, "/?view=grid", nil, []*http.Cookie{session, csrf})
	viewCookie := responseCookie(t, grid, assetViewCookie)
	if viewCookie.Value != "grid" || !strings.Contains(grid.Body.String(), `class="is-active" aria-current="page">卡片`) {
		t.Fatalf("grid preference was not selected: cookie=%q body=%s", viewCookie.Value, grid.Body.String())
	}
	if !strings.Contains(grid.Body.String(), `href="/?view=list"`) {
		t.Fatalf("grid view must expose an explicit list switch: %s", grid.Body.String())
	}
	persisted := request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{session, csrf, viewCookie})
	if !strings.Contains(persisted.Body.String(), `class="is-active" aria-current="page">卡片`) {
		t.Fatalf("grid preference was not persisted: %s", persisted.Body.String())
	}
	list := request(t, handler, http.MethodGet, "/?view=list", nil, []*http.Cookie{session, csrf, viewCookie})
	listCookie := responseCookie(t, list, assetViewCookie)
	if listCookie.Value != "list" || !strings.Contains(list.Body.String(), `class="is-active" aria-current="page">列表`) {
		t.Fatalf("list switch must override the persisted grid preference: cookie=%q body=%s", listCookie.Value, list.Body.String())
	}
	legacy := request(t, handler, http.MethodGet, "/catalog", nil, []*http.Cookie{session, csrf})
	if legacy.Code != http.StatusPermanentRedirect || legacy.Header().Get("Location") != "/" {
		t.Fatalf("legacy catalog route: status=%d location=%q", legacy.Code, legacy.Header().Get("Location"))
	}
}

func TestPendingImportRoutesDoNotExist(t *testing.T) {
	handler := newTestHandler(t)
	for _, path := range []string{"/imports", "/imports/00000000-0000-0000-0000-000000000000"} {
		response := request(t, handler, http.MethodGet, path, nil, nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("obsolete pending-import route %s: got %d, want %d", path, response.Code, http.StatusNotFound)
		}
	}
}

func TestAssetEditorCreatesMissingTypeWithoutLeavingEditor(t *testing.T) {
	handler := newTestHandler(t)
	setupPage := request(t, handler, http.MethodGet, "/setup", nil, nil)
	csrf := responseCookie(t, setupPage, csrfCookie)
	setup := request(t, handler, http.MethodPost, "/setup", url.Values{
		"csrf_token": {csrf.Value}, "tenant_name": {"Inline Type Tenant"}, "base_currency": {"CNY"},
		"username": {"owner"}, "password": {"owner secure password"},
	}, []*http.Cookie{csrf})
	session := responseCookie(t, setup, sessionCookie)

	home := request(t, handler, http.MethodGet, "/assets/new", nil, []*http.Cookie{session, csrf})
	for _, want := range []string{`data-dialog-open="variant-drawer"`, `data-title="新增物品类型"`, `id="category-form"`, `id="model-form"`, `id="variant-form"`, `name="flow" value="asset"`} {
		if home.Code != http.StatusOK || !strings.Contains(home.Body.String(), want) {
			t.Fatalf("asset drawer shared type component missing %q: status=%d body=%s", want, home.Code, home.Body.String())
		}
	}
	if strings.Contains(home.Body.String(), `href="/admin/catalog">创建规格`) {
		t.Fatalf("asset drawer must create a type in place instead of navigating to management: %s", home.Body.String())
	}

	redirect := func(response *httptest.ResponseRecorder, wantDialog, idKey string) string {
		t.Helper()
		if response.Code != http.StatusSeeOther {
			t.Fatalf("inline type step: status=%d body=%s", response.Code, response.Body.String())
		}
		location, err := url.Parse(response.Header().Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		if location.Path != "/assets/new" || location.Query().Get("dialog") != wantDialog || location.Query().Get(idKey) == "" {
			t.Fatalf("inline type redirect: location=%q", location.String())
		}
		return location.Query().Get(idKey)
	}

	category := request(t, handler, http.MethodPost, "/admin/catalog/categories", url.Values{
		"csrf_token": {csrf.Value}, "flow": {"asset"}, "name": {"手机"}, "icon_key": {"smartphone"},
	}, []*http.Cookie{session, csrf})
	categoryID := redirect(category, "model-drawer", "category_id")

	model := request(t, handler, http.MethodPost, "/admin/catalog/models", url.Values{
		"csrf_token": {csrf.Value}, "flow": {"asset"}, "category_id": {categoryID}, "name": {"iPhone 17 Pro"},
	}, []*http.Cookie{session, csrf})
	modelID := redirect(model, "variant-drawer", "model_id")

	variant := request(t, handler, http.MethodPost, "/admin/catalog/variants", url.Values{
		"csrf_token": {csrf.Value}, "flow": {"asset"}, "model_id": {modelID}, "name": {"256GB"},
	}, []*http.Cookie{session, csrf})
	if variant.Code != http.StatusSeeOther {
		t.Fatalf("inline type final step: status=%d body=%s", variant.Code, variant.Body.String())
	}
	variantLocation, err := url.Parse(variant.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	variantID := variantLocation.Query().Get("variant_id")
	if variantLocation.Path != "/assets/new" || variantLocation.Query().Get("dialog") != "" || variantID == "" {
		t.Fatalf("inline type final redirect: location=%q", variantLocation.String())
	}

	reopened := request(t, handler, http.MethodGet, variant.Header().Get("Location"), nil, []*http.Cookie{session, csrf})
	if reopened.Code != http.StatusOK || !strings.Contains(reopened.Body.String(), `>手机 / iPhone 17 Pro / 256GB</option>`) {
		t.Fatalf("new type was not available to the reopened asset form: status=%d body=%s", reopened.Code, reopened.Body.String())
	}
	if !strings.Contains(reopened.Body.String(), `<option value="`+variantID+`" selected>`) {
		t.Fatalf("new specification was not selected in the asset editor: %s", reopened.Body.String())
	}
}

func TestParseFormDatePreservesCalendarDateAcrossTimezone(t *testing.T) {
	previousLocal := time.Local
	time.Local = time.FixedZone("Asia/Shanghai", 8*60*60)
	t.Cleanup(func() { time.Local = previousLocal })

	got, err := parseFormDate("2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	if formatted := got.Format("2006-01-02"); formatted != "2026-08-01" {
		t.Fatalf("rate date shifted across timezone: got %s", formatted)
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

	created := request(t, handler, http.MethodPost, "/admin/catalog/categories", url.Values{
		"csrf_token": {csrf.Value}, "name": {"手机"}, "icon_key": {"smartphone"},
	}, []*http.Cookie{ownerSession, csrf})
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create category: status=%d body=%s", created.Code, created.Body.String())
	}
	catalog := request(t, handler, http.MethodGet, "/admin/catalog", nil, []*http.Cookie{ownerSession, csrf})
	categoryID := optionID(t, catalog.Body.String(), "手机")

	created = request(t, handler, http.MethodPost, "/admin/catalog/models", url.Values{
		"csrf_token": {csrf.Value}, "category_id": {categoryID}, "name": {"iPhone 17 Pro"},
	}, []*http.Cookie{ownerSession, csrf})
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create model: status=%d body=%s", created.Code, created.Body.String())
	}
	catalog = request(t, handler, http.MethodGet, "/admin/catalog", nil, []*http.Cookie{ownerSession, csrf})
	modelID := optionID(t, catalog.Body.String(), "手机 / iPhone 17 Pro")

	created = request(t, handler, http.MethodPost, "/admin/catalog/variants", url.Values{
		"csrf_token": {csrf.Value}, "model_id": {modelID}, "name": {"256GB"},
	}, []*http.Cookie{ownerSession, csrf})
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create variant: status=%d body=%s", created.Code, created.Body.String())
	}
	created = request(t, handler, http.MethodPost, "/admin/catalog/variants", url.Values{
		"csrf_token": {csrf.Value}, "model_id": {modelID}, "name": {"512GB"},
	}, []*http.Cookie{ownerSession, csrf})
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create second variant: status=%d body=%s", created.Code, created.Body.String())
	}
	catalog = request(t, handler, http.MethodGet, "/admin/catalog", nil, []*http.Cookie{ownerSession, csrf})
	for _, want := range []string{"物品类型配置", `class="catalog-table"`, `class="catalog-category"`, `class="variant-tags"`, `class="variant-tag">256GB`, `class="variant-tag">512GB`, `data-model-variants`, "新增型号", "新增类别", "新增规格", `name="q"`, `name="category"`, `name="sort"`, `aria-sort="ascending"`} {
		if catalog.Code != http.StatusOK || !strings.Contains(catalog.Body.String(), want) {
			t.Fatalf("model-first type configuration missing %q: status=%d body=%s", want, catalog.Code, catalog.Body.String())
		}
	}
	filteredCatalog := request(t, handler, http.MethodGet, "/admin/catalog?q=iPhone&category="+categoryID+"&sort=name&direction=desc", nil, []*http.Cookie{ownerSession, csrf})
	if filteredCatalog.Code != http.StatusOK || !strings.Contains(filteredCatalog.Body.String(), "iPhone 17 Pro") || !strings.Contains(filteredCatalog.Body.String(), `class="variant-tag">256GB`) {
		t.Fatalf("server-filtered catalog page must retain bulk-loaded specifications: status=%d body=%s", filteredCatalog.Code, filteredCatalog.Body.String())
	}
	if strings.Contains(catalog.Body.String(), "价格规格") {
		t.Fatalf("type configuration must use the simpler specification label: %s", catalog.Body.String())
	}
	headingActions := regexp.MustCompile(`(?s)<div class="heading-actions">(.*?)</div>`).FindStringSubmatch(catalog.Body.String())
	if len(headingActions) != 2 || strings.Contains(headingActions[1], "新增类别") || strings.Contains(headingActions[1], `href="/"`) {
		t.Fatalf("catalog heading must expose only model creation: %s", catalog.Body.String())
	}
	modelForm := regexp.MustCompile(`(?s)<form id="model-form".*?>(.*?)</form>`).FindStringSubmatch(catalog.Body.String())
	if len(modelForm) != 2 || !strings.Contains(modelForm[1], "新增类别") {
		t.Fatalf("category creation must remain available inside the model form: %s", catalog.Body.String())
	}
	listMarkup := strings.Split(catalog.Body.String(), `<dialog class="drawer"`)[0]
	if strings.Count(listMarkup, `data-title="编辑型号"`) != 1 || strings.Contains(listMarkup, `data-title="编辑规格"`) || strings.Contains(listMarkup, `data-title="新增规格"`) {
		t.Fatalf("catalog row must expose only model editing; specification actions belong in the drawer: %s", listMarkup)
	}
	variant512Match := regexp.MustCompile(`data-action="/admin/catalog/variants/([0-9a-f-]{36})"[^>]+data-name="512GB"`).FindStringSubmatch(catalog.Body.String())
	if len(variant512Match) != 2 {
		t.Fatalf("512GB edit action not found: %s", catalog.Body.String())
	}
	variant512ID := variant512Match[1]
	deleted := request(t, handler, http.MethodPost, "/admin/catalog/variants/"+variant512ID+"/delete", url.Values{
		"csrf_token": {csrf.Value}, "return_model_id": {modelID},
	}, []*http.Cookie{ownerSession, csrf})
	if deleted.Code != http.StatusSeeOther || deleted.Header().Get("Location") != "/admin/catalog?dialog=model-drawer&edit_model_id="+modelID {
		t.Fatalf("delete unused specification: status=%d location=%q body=%s", deleted.Code, deleted.Header().Get("Location"), deleted.Body.String())
	}
	catalog = request(t, handler, http.MethodGet, "/admin/catalog", nil, []*http.Cookie{ownerSession, csrf})
	if strings.Contains(catalog.Body.String(), "512GB") {
		t.Fatalf("deleted specification remained in catalog: %s", catalog.Body.String())
	}
	catalog = request(t, handler, http.MethodGet, "/assets/new", nil, []*http.Cookie{ownerSession, csrf})
	variantID := optionID(t, catalog.Body.String(), "手机 / iPhone 17 Pro / 256GB")

	created = request(t, handler, http.MethodPost, "/assets", url.Values{
		"csrf_token": {csrf.Value}, "variant_id": {variantID}, "display_name": {"我的主力手机"},
		"serial_number": {"WEB-SERIAL-001"}, "color": {"黑色"}, "purchase_channel": {"官方商城"},
		"notes": {"Web 全要素目录记录"},
	}, []*http.Cookie{ownerSession, csrf})
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create asset: status=%d body=%s", created.Code, created.Body.String())
	}
	if !regexp.MustCompile(`^/assets/[0-9a-f-]{36}$`).MatchString(created.Header().Get("Location")) {
		t.Fatalf("created asset must open its detail page: location=%q", created.Header().Get("Location"))
	}
	blockedDelete := request(t, handler, http.MethodPost, "/admin/catalog/variants/"+variantID+"/delete", url.Values{
		"csrf_token": {csrf.Value}, "return_model_id": {modelID},
	}, []*http.Cookie{ownerSession, csrf})
	if blockedDelete.Code != http.StatusUnprocessableEntity || !strings.Contains(blockedDelete.Body.String(), "该规格已被具体物品使用，不能删除。") {
		t.Fatalf("used specification deletion must be blocked: status=%d body=%s", blockedDelete.Code, blockedDelete.Body.String())
	}
	catalog = request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{ownerSession, csrf})
	if catalog.Code != http.StatusOK || !strings.Contains(catalog.Body.String(), "我的主力手机") || !strings.Contains(catalog.Body.String(), "WEB-SERIAL-001") {
		t.Fatalf("catalog asset list: status=%d body=%s", catalog.Code, catalog.Body.String())
	}
	match := regexp.MustCompile(`/assets/([0-9a-f-]{36})`).FindStringSubmatch(catalog.Body.String())
	if len(match) != 2 {
		t.Fatalf("asset detail link not found: %s", catalog.Body.String())
	}
	for _, want := range []string{"累计成本", "尚未录入", "新增事件", `href="/assets/` + match[1] + `#add-event"`} {
		if !strings.Contains(catalog.Body.String(), want) {
			t.Fatalf("asset list missing lifecycle summary or entry %q: %s", want, catalog.Body.String())
		}
	}
	for _, unwanted := range []string{`href="/assets/` + match[1] + `/edit"`, `data-dialog-open="asset-drawer"`, `id="asset-drawer"`} {
		if strings.Contains(catalog.Body.String(), unwanted) {
			t.Fatalf("asset list must not expose record editing %q: %s", unwanted, catalog.Body.String())
		}
	}
	detailBeforeEdit := request(t, handler, http.MethodGet, "/assets/"+match[1], nil, []*http.Cookie{ownerSession, csrf})
	if detailBeforeEdit.Code != http.StatusOK || !strings.Contains(detailBeforeEdit.Body.String(), `href="/assets/`+match[1]+`/edit"`) {
		t.Fatalf("asset detail must expose the edit entry: status=%d body=%s", detailBeforeEdit.Code, detailBeforeEdit.Body.String())
	}
	detailHeading := strings.Split(detailBeforeEdit.Body.String(), `<section class="card asset-profile">`)[0]
	for _, want := range []string{
		`class="heading-actions asset-detail-actions"`,
		`class="icon-button" href="/" aria-label="返回物品列表" title="返回物品列表"`,
		`class="icon-button" href="/assets/` + match[1] + `/edit" aria-label="编辑物品" title="编辑物品"`,
	} {
		if !strings.Contains(detailHeading, want) {
			t.Fatalf("asset detail heading action missing %q: %s", want, detailHeading)
		}
	}
	for _, unwanted := range []string{`>返回物品列表</a>`, `>编辑物品</a>`} {
		if strings.Contains(detailHeading, unwanted) {
			t.Fatalf("asset detail heading action must be icon-only %q: %s", unwanted, detailHeading)
		}
	}
	editPage := request(t, handler, http.MethodGet, "/assets/"+match[1]+"/edit", nil, []*http.Cookie{ownerSession, csrf})
	for _, want := range []string{`class="card asset-profile asset-editor-profile"`, `id="asset-form"`, `action="/assets/` + match[1] + `"`, `value="我的主力手机"`, `value="WEB-SERIAL-001"`} {
		if editPage.Code != http.StatusOK || !strings.Contains(editPage.Body.String(), want) {
			t.Fatalf("dedicated asset edit page missing %q: status=%d body=%s", want, editPage.Code, editPage.Body.String())
		}
	}
	updates := []struct {
		path string
		form url.Values
		want string
	}{
		{"/admin/catalog/categories/" + categoryID, url.Values{"csrf_token": {csrf.Value}, "name": {"移动设备"}, "icon_key": {"tablet"}}, "移动设备"},
		{"/admin/catalog/models/" + modelID, url.Values{"csrf_token": {csrf.Value}, "category_id": {categoryID}, "name": {"iPhone 17 Pro Max"}}, "iPhone 17 Pro Max"},
		{"/admin/catalog/variants/" + variantID, url.Values{"csrf_token": {csrf.Value}, "model_id": {modelID}, "name": {"512GB"}}, "512GB"},
		{"/assets/" + match[1], url.Values{"csrf_token": {csrf.Value}, "variant_id": {variantID}, "display_name": {"备用手机"}, "serial_number": {"WEB-SERIAL-EDITED"}, "color": {"白色"}}, "备用手机"},
	}
	for _, update := range updates {
		response := request(t, handler, http.MethodPost, update.path, update.form, []*http.Cookie{ownerSession, csrf})
		if response.Code != http.StatusSeeOther {
			t.Fatalf("update %s: status=%d body=%s", update.path, response.Code, response.Body.String())
		}
		if update.path == "/assets/"+match[1] && response.Header().Get("Location") != update.path {
			t.Fatalf("updated asset must return to its detail page: location=%q", response.Header().Get("Location"))
		}
	}
	updatedList := request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{ownerSession, csrf})
	for _, want := range []string{"备用手机", "移动设备", "iPhone 17 Pro Max", "512GB", "WEB-SERIAL-EDITED", `category-icons.svg#tablet`} {
		if !strings.Contains(updatedList.Body.String(), want) {
			t.Fatalf("updated asset list missing %q: %s", want, updatedList.Body.String())
		}
	}
	for _, restore := range []struct {
		path string
		form url.Values
	}{
		{"/admin/catalog/categories/" + categoryID, url.Values{"csrf_token": {csrf.Value}, "name": {"手机"}, "icon_key": {"smartphone"}}},
		{"/admin/catalog/models/" + modelID, url.Values{"csrf_token": {csrf.Value}, "category_id": {categoryID}, "name": {"iPhone 17 Pro"}}},
		{"/admin/catalog/variants/" + variantID, url.Values{"csrf_token": {csrf.Value}, "model_id": {modelID}, "name": {"256GB"}}},
		{"/assets/" + match[1], url.Values{"csrf_token": {csrf.Value}, "variant_id": {variantID}, "display_name": {"我的主力手机"}, "serial_number": {"WEB-SERIAL-001"}, "color": {"黑色"}, "purchase_channel": {"官方商城"}, "notes": {"Web 全要素目录记录"}}},
	} {
		response := request(t, handler, http.MethodPost, restore.path, restore.form, []*http.Cookie{ownerSession, csrf})
		if response.Code != http.StatusSeeOther {
			t.Fatalf("restore %s: status=%d body=%s", restore.path, response.Code, response.Body.String())
		}
	}
	detail := request(t, handler, http.MethodGet, "/assets/"+match[1], nil, []*http.Cookie{ownerSession, csrf})
	for _, want := range []string{"我的主力手机", "iPhone 17 Pro", "256GB", "WEB-SERIAL-001", "官方商城", "Web 全要素目录记录", `class="card asset-profile"`, `class="asset-product-visual"`, `class="asset-product-image"`, `/static/product-demo-iphone-17-pro-deep-blue.jpg`, `width="1728" height="912"`, `decoding="async" fetchpriority="high"`, `型号示意图；具体颜色以所选规格为准。`, `class="asset-profile-content"`, `class="asset-details-grid"`, `class="asset-notes"`, `data-cost-dashboard`, `日均持有成本`, `class="cost-metrics"`, `class="compact-timeline"`, `class="timeline-heading"`, `class="icon-button" id="add-event"`, `aria-label="新增生命周期记录" title="新增生命周期记录"`, `<path d="M12 5v14M5 12h14"/>`, `data-dialog-open="event-drawer"`, `id="event-drawer"`, `id="event-form"`, `data-dialog-open="event-type-drawer"`, `id="event-type-drawer"`, `action="/admin/event-types"`, `data-event-type-select`, `data-cashflow="expense"`, `class="money-input-group"`, `class="currency-suffix"`, `list="event-currencies"`, `aria-label="原始货币"`, `data-positive-pattern=`, `<option value="AED"></option>`, `<option value="BHD"></option>`, `<option value="ZWG"></option>`, `data-currency-select`, `data-base-currency="CNY"`, `data-fx-field hidden`, `data-fx-required`} {
		if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), want) {
			t.Fatalf("asset detail missing %q: status=%d body=%s", want, detail.Code, detail.Body.String())
		}
	}
	if strings.Contains(detail.Body.String(), "金额（正数）") {
		t.Fatalf("the amount label must not repeat its positive-input constraint: %s", detail.Body.String())
	}
	if strings.Contains(detail.Body.String(), `class="timeline-summary"`) || strings.Index(detail.Body.String(), `data-cost-dashboard`) > strings.Index(detail.Body.String(), `id="lifecycle-timeline"`) {
		t.Fatal("cost dashboard must precede the compact timeline without the old summary row")
	}
	if strings.Contains(detail.Body.String(), `class="two-column"`) {
		t.Fatalf("asset notes must remain inside the responsive details card: %s", detail.Body.String())
	}
	visibleDetail := strings.Split(detail.Body.String(), `<dialog class="drawer" id="event-drawer"`)[0]
	if regexp.MustCompile(`<form[^>]+action="/assets/` + regexp.QuoteMeta(match[1]) + `/events"`).MatchString(visibleDetail) {
		t.Fatalf("lifecycle create form must not be rendered in the page flow: %s", visibleDetail)
	}
	if strings.Count(detail.Body.String(), `data-fx-field hidden`) != 3 || strings.Contains(detail.Body.String(), `name="fx_confirmed"`) {
		t.Fatalf("same-currency event form should initially hide all FX controls: %s", detail.Body.String())
	}
	createEventType := request(t, handler, http.MethodPost, "/admin/event-types", url.Values{
		"csrf_token": {csrf.Value}, "asset_id": {match[1]}, "name": {"保养"}, "cashflow": {"neutral"},
	}, []*http.Cookie{ownerSession, csrf})
	if createEventType.Code != http.StatusSeeOther || createEventType.Header().Get("Location") != "/assets/"+match[1]+"?dialog=event-drawer&event_type=%E4%BF%9D%E5%85%BB#add-event" {
		t.Fatalf("create custom event type: status=%d location=%q body=%s", createEventType.Code, createEventType.Header().Get("Location"), createEventType.Body.String())
	}
	duplicateEventType := request(t, handler, http.MethodPost, "/admin/event-types", url.Values{
		"csrf_token": {csrf.Value}, "asset_id": {match[1]}, "name": {"保养"}, "cashflow": {"expense"},
	}, []*http.Cookie{ownerSession, csrf})
	if duplicateEventType.Code != http.StatusUnprocessableEntity || !strings.Contains(duplicateEventType.Body.String(), "这个事件类型已经存在") || !strings.Contains(duplicateEventType.Body.String(), `id="event-type-drawer"`) {
		t.Fatalf("duplicate custom event type must reopen its form: status=%d body=%s", duplicateEventType.Code, duplicateEventType.Body.String())
	}
	detail = request(t, handler, http.MethodGet, "/assets/"+match[1]+"?dialog=event-drawer&event_type=%E4%BF%9D%E5%85%BB", nil, []*http.Cookie{ownerSession, csrf})
	for _, want := range []string{`value="保养" data-cashflow="neutral"`, `name="event_type"`, `新增类型`} {
		if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), want) {
			t.Fatalf("custom event type must be selectable %q: status=%d body=%s", want, detail.Code, detail.Body.String())
		}
	}

	missingFX := request(t, handler, http.MethodPost, "/assets/"+match[1]+"/events", url.Values{
		"csrf_token": {csrf.Value}, "event_type": {"purchase"}, "amount": {"1000.00"}, "currency": {"USD"},
		"occurred_at": {"2026-08-01T10:00"}, "source": {"manual"},
	}, []*http.Cookie{ownerSession, csrf})
	if missingFX.Code != http.StatusUnprocessableEntity || !strings.Contains(missingFX.Body.String(), "汇率格式无效") {
		t.Fatalf("foreign currency without FX evidence should be rejected: status=%d body=%s", missingFX.Code, missingFX.Body.String())
	}
	for _, want := range []string{`value="1000.00"`, `name="currency" value="USD"`, `value="2026-08-01T10:00"`} {
		if !strings.Contains(missingFX.Body.String(), want) {
			t.Fatalf("rejected event form must retain %q: %s", want, missingFX.Body.String())
		}
	}
	purchase := request(t, handler, http.MethodPost, "/assets/"+match[1]+"/events", url.Values{
		"csrf_token": {csrf.Value}, "event_type": {"purchase"}, "amount": {"1000.00"}, "currency": {"USD"},
		"fx_rate": {"7.12"}, "fx_rate_date": {"2026-08-01"}, "fx_rate_source": {"web-fixture"},
		"occurred_at": {"2026-08-01T10:00"}, "source": {"manual"}, "external_reference": {"ORDER-WEB-001"}, "notes": {"美元买入"},
	}, []*http.Cookie{ownerSession, csrf})
	if purchase.Code != http.StatusSeeOther {
		t.Fatalf("record purchase without redundant FX confirmation: status=%d body=%s", purchase.Code, purchase.Body.String())
	}
	maintenance := request(t, handler, http.MethodPost, "/assets/"+match[1]+"/events", url.Values{
		"csrf_token": {csrf.Value}, "event_type": {"保养"}, "amount": {"0"}, "currency": {"CNY"},
		"occurred_at": {"2026-08-05T10:00"}, "source": {"manual"}, "notes": {"清洁并检查"},
	}, []*http.Cookie{ownerSession, csrf})
	if maintenance.Code != http.StatusSeeOther {
		t.Fatalf("record custom no-amount event: status=%d body=%s", maintenance.Code, maintenance.Body.String())
	}
	repair := request(t, handler, http.MethodPost, "/assets/"+match[1]+"/events", url.Values{
		"csrf_token": {csrf.Value}, "event_type": {"repair"}, "amount": {"200.00"}, "currency": {"CNY"},
		"occurred_at": {"2026-08-10T10:00"}, "source": {"manual"}, "notes": {"初始维修金额"},
	}, []*http.Cookie{ownerSession, csrf})
	if repair.Code != http.StatusSeeOther {
		t.Fatalf("record repair: status=%d body=%s", repair.Code, repair.Body.String())
	}
	detail = request(t, handler, http.MethodGet, "/assets/"+match[1], nil, []*http.Cookie{ownerSession, csrf})
	correctionLinks := regexp.MustCompile(`/events/([0-9a-f-]{36})/correct`).FindAllStringSubmatch(detail.Body.String(), -1)
	if len(correctionLinks) != 3 {
		t.Fatalf("expected purchase, custom, and repair correction links: %s", detail.Body.String())
	}
	neutralCorrectionForm := request(t, handler, http.MethodGet, "/events/"+correctionLinks[1][1]+"/correct", nil, []*http.Cookie{ownerSession, csrf})
	if neutralCorrectionForm.Code != http.StatusOK || !strings.Contains(neutralCorrectionForm.Body.String(), `type="hidden" name="amount" value="0"`) || strings.Contains(neutralCorrectionForm.Body.String(), `id="correction-amount"`) {
		t.Fatalf("neutral event correction must not ask for a monetary amount: status=%d body=%s", neutralCorrectionForm.Code, neutralCorrectionForm.Body.String())
	}
	correctionForm := request(t, handler, http.MethodGet, "/events/"+correctionLinks[2][1]+"/correct", nil, []*http.Cookie{ownerSession, csrf})
	for _, want := range []string{`class="money-input-group"`, `class="currency-suffix"`, `list="correction-currencies"`, `aria-label="原始货币"`, `name="amount" value="200.00"`, `data-positive-pattern=`, `class="fx-rate-input-group"`, `1 <span data-fx-rate-from>CNY</span> =`, `type="number" name="fx_rate"`, `min="0" step="any"`, `class="fx-conversion-preview"`, `data-base-minor-units="2"`, `data-fx-result-value`} {
		if correctionForm.Code != http.StatusOK || !strings.Contains(correctionForm.Body.String(), want) {
			t.Fatalf("correction money input missing %q: status=%d body=%s", want, correctionForm.Code, correctionForm.Body.String())
		}
	}
	interactionScript := request(t, handler, http.MethodGet, "/static/app.js", nil, nil)
	for _, want := range []string{`unit.textContent = select.value.toUpperCase()`, `amount.removeAttribute("pattern")`, `amount.pattern = amount.dataset.positivePattern`, `const decimalProduct =`, `BigInt(`, `syncFXPreview(form)`} {
		if interactionScript.Code != http.StatusOK || !strings.Contains(interactionScript.Body.String(), want) {
			t.Fatalf("FX preview interaction missing %q: status=%d body=%s", want, interactionScript.Code, interactionScript.Body.String())
		}
	}
	if strings.Contains(correctionForm.Body.String(), `name="fx_confirmed"`) {
		t.Fatalf("correction form must not ask for redundant FX confirmation: %s", correctionForm.Body.String())
	}
	invalidCorrection := request(t, handler, http.MethodPost, "/events/"+correctionLinks[2][1]+"/correct", url.Values{
		"csrf_token": {csrf.Value}, "amount": {"invalid"}, "currency": {"CNY"},
		"occurred_at": {"2026-08-10T10:00"}, "source": {"manual-correction"}, "notes": {"保留这段更正说明"},
	}, []*http.Cookie{ownerSession, csrf})
	for _, want := range []string{`value="invalid"`, `value="2026-08-10T10:00"`, `value="manual-correction"`, `保留这段更正说明`, `data-error-summary`} {
		if invalidCorrection.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidCorrection.Body.String(), want) {
			t.Fatalf("invalid correction must retain %q: status=%d body=%s", want, invalidCorrection.Code, invalidCorrection.Body.String())
		}
	}
	corrected := request(t, handler, http.MethodPost, "/events/"+correctionLinks[2][1]+"/correct", url.Values{
		"csrf_token": {csrf.Value}, "amount": {"20.00"}, "currency": {"USD"},
		"fx_rate": {"7.5"}, "fx_rate_date": {"2026-08-10"}, "fx_rate_source": {"correction-fixture"},
		"occurred_at": {"2026-08-10T10:00"}, "source": {"manual-correction"}, "notes": {"正确维修金额"},
	}, []*http.Cookie{ownerSession, csrf})
	if corrected.Code != http.StatusSeeOther {
		t.Fatalf("correct repair: status=%d body=%s", corrected.Code, corrected.Body.String())
	}
	correctedDetail := request(t, handler, http.MethodGet, "/assets/"+match[1], nil, []*http.Cookie{ownerSession, csrf})
	correctedLinks := regexp.MustCompile(`/events/([0-9a-f-]{36})/correct`).FindAllStringSubmatch(correctedDetail.Body.String(), -1)
	foreignCorrectionForm := request(t, handler, http.MethodGet, "/events/"+correctedLinks[2][1]+"/correct", nil, []*http.Cookie{ownerSession, csrf})
	for _, want := range []string{`name="currency" value="USD"`, `1 <span data-fx-rate-from>USD</span> =`, `name="fx_rate" value="7.5"`, `<span class="fx-rate-addon">CNY</span>`} {
		if foreignCorrectionForm.Code != http.StatusOK || !strings.Contains(foreignCorrectionForm.Body.String(), want) {
			t.Fatalf("foreign correction rate control missing %q: status=%d body=%s", want, foreignCorrectionForm.Code, foreignCorrectionForm.Body.String())
		}
	}
	sale := request(t, handler, http.MethodPost, "/assets/"+match[1]+"/events", url.Values{
		"csrf_token": {csrf.Value}, "asset_id": {match[1]}, "event_type": {"sale"}, "amount": {"8000.00"},
		"currency": {"CNY"}, "occurred_at": {"2026-08-20T10:00"}, "source": {"ai-harness"},
		"external_reference": {"SALE-WEB-001"}, "notes": {"用户已在 Agent 对话中确认"},
	}, []*http.Cookie{ownerSession, csrf})
	if sale.Code != http.StatusSeeOther {
		t.Fatalf("record Agent-confirmed sale: status=%d body=%s", sale.Code, sale.Body.String())
	}
	detail = request(t, handler, http.MethodGet, "/assets/"+match[1], nil, []*http.Cookie{ownerSession, csrf})
	for _, want := range []string{`class="timeline-list"`, `class="timeline-item`, `class="timeline-filters asset-search-shell"`, `placeholder="搜索备注或汇率来源…" aria-label="搜索记录"`, `class="icon-button" type="submit" aria-label="搜索" title="搜索"`, `<circle cx="11" cy="11" r="6"/>`, `class="asset-filter-disclosure "`, `<summary class="icon-button" aria-label="更多筛选" title="更多筛选">`, `name="event_type"`, `name="sort"`, `name="show_voided"`, `data-auto-submit`, "显示已作废记录", "7270.00 CNY", "8000.00 CNY", "730.00 CNY", "已卖出", "1000.00 USD", "2026-08-01", "web-fixture", "正确维修金额", "保养", "清洁并检查"} {
		if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), want) {
			t.Fatalf("lifecycle detail missing %q: status=%d body=%s", want, detail.Code, detail.Body.String())
		}
	}
	if strings.Contains(detail.Body.String(), `<span>搜索记录</span>`) || strings.Contains(detail.Body.String(), `>搜索</button>`) || strings.Contains(detail.Body.String(), `<span>更多筛选</span>`) {
		t.Fatalf("timeline search actions must remain icon-only: %s", detail.Body.String())
	}
	for _, unwanted := range []string{"初始维修金额", "作废并由更正记录替代", `<option value="void"`, `<span class="muted">已作废</span>`} {
		if strings.Contains(detail.Body.String(), unwanted) {
			t.Fatalf("effective lifecycle view must hide %q: body=%s", unwanted, detail.Body.String())
		}
	}
	fullHistory := request(t, handler, http.MethodGet, "/assets/"+match[1]+"?show_voided=1", nil, []*http.Cookie{ownerSession, csrf})
	for _, want := range []string{"初始维修金额", "正确维修金额", `<span class="muted">已作废</span>`, `name="show_voided" value="1" data-auto-submit checked`} {
		if fullHistory.Code != http.StatusOK || !strings.Contains(fullHistory.Body.String(), want) {
			t.Fatalf("full lifecycle history missing %q: status=%d body=%s", want, fullHistory.Code, fullHistory.Body.String())
		}
	}
	if strings.Contains(fullHistory.Body.String(), "作废并由更正记录替代") || strings.Contains(fullHistory.Body.String(), `<option value="void"`) {
		t.Fatalf("technical void event must remain hidden from full lifecycle history: %s", fullHistory.Body.String())
	}
	filteredTimeline := request(t, handler, http.MethodGet, "/assets/"+match[1]+"?event_type=sale&sort=amount&direction=desc", nil, []*http.Cookie{ownerSession, csrf})
	costSection := regexp.MustCompile(`(?s)<section class="cost-dashboard".*?</dl>`)
	for _, suffix := range []string{"?q=not-found", "?event_type=sale&sort=amount&direction=desc", "?show_voided=1"} {
		filtered := request(t, handler, http.MethodGet, "/assets/"+match[1]+suffix, nil, []*http.Cookie{ownerSession, csrf})
		if filtered.Code != http.StatusOK || costSection.FindString(filtered.Body.String()) != costSection.FindString(detail.Body.String()) {
			t.Fatal("timeline filters changed cost metrics")
		}
	}
	if filteredTimeline.Code != http.StatusOK || !strings.Contains(filteredTimeline.Body.String(), "用户已在 Agent 对话中确认") || strings.Contains(filteredTimeline.Body.String(), "正确维修金额") || !strings.Contains(filteredTimeline.Body.String(), "730.00 CNY") || !strings.Contains(filteredTimeline.Body.String(), `class="asset-filter-disclosure has-active"`) {
		t.Fatalf("server-filtered timeline must keep full summary while filtering rows: status=%d body=%s", filteredTimeline.Code, filteredTimeline.Body.String())
	}
	assetList := request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{ownerSession, csrf})
	for _, want := range []string{"7270.00 CNY", "730.00 CNY", "最终结算", "已卖出", `name="q"`, `name="status"`, `name="sort"`, `name="direction"`, `class="asset-filter-disclosure "`, "更多筛选", `class="table-sort"`} {
		if assetList.Code != http.StatusOK || !strings.Contains(assetList.Body.String(), want) {
			t.Fatalf("asset list should show server-filtered lifecycle summary %q: status=%d body=%s", want, assetList.Code, assetList.Body.String())
		}
	}
	filteredAssetList := request(t, handler, http.MethodGet, "/?q=主力&status=sold", nil, []*http.Cookie{ownerSession, csrf})
	if filteredAssetList.Code != http.StatusOK || !strings.Contains(filteredAssetList.Body.String(), "我的主力手机") || !strings.Contains(filteredAssetList.Body.String(), `class="asset-filter-disclosure has-active"`) || !strings.Contains(filteredAssetList.Body.String(), "已启用") {
		t.Fatalf("asset list search should match asset data: status=%d body=%s", filteredAssetList.Code, filteredAssetList.Body.String())
	}
	sortedAssetList := request(t, handler, http.MethodGet, "/?sort=net&direction=asc", nil, []*http.Cookie{ownerSession, csrf})
	if sortedAssetList.Code != http.StatusOK || !strings.Contains(sortedAssetList.Body.String(), `href="/?sort=net&amp;view=list"`) || !strings.Contains(sortedAssetList.Body.String(), `aria-sort="ascending"`) {
		t.Fatalf("asset server sort controls missing: status=%d body=%s", sortedAssetList.Code, sortedAssetList.Body.String())
	}
	emptyAssetList := request(t, handler, http.MethodGet, "/?q=不存在的物品", nil, []*http.Cookie{ownerSession, csrf})
	if emptyAssetList.Code != http.StatusOK || !strings.Contains(emptyAssetList.Body.String(), "没有匹配的物品") {
		t.Fatalf("asset list should distinguish an empty filter result: status=%d body=%s", emptyAssetList.Code, emptyAssetList.Body.String())
	}
	if strings.Contains(detail.Body.String(), "2026-07-31") {
		t.Fatalf("FX rate date shifted to the prior day: %s", detail.Body.String())
	}
	dashboard := request(t, handler, http.MethodGet, "/overview", nil, []*http.Cookie{ownerSession, csrf})
	for _, want := range []string{"具体物品", "7270.00 CNY", "730.00 CNY", "收入 8000.00 CNY"} {
		if dashboard.Code != http.StatusOK || !strings.Contains(dashboard.Body.String(), want) {
			t.Fatalf("dashboard totals missing %q: status=%d body=%s", want, dashboard.Code, dashboard.Body.String())
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
	visible := request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{viewerSession, csrf})
	if visible.Code != http.StatusOK || !strings.Contains(visible.Body.String(), "我的主力手机") {
		t.Fatalf("viewer should read catalog: status=%d body=%s", visible.Code, visible.Body.String())
	}
	if strings.Contains(visible.Body.String(), `id="asset-form"`) || strings.Contains(visible.Body.String(), `href="/admin/catalog"`) {
		t.Fatalf("viewer should not receive catalog management controls: %s", visible.Body.String())
	}
	viewerEditDenied := request(t, handler, http.MethodGet, "/assets/"+match[1]+"/edit", nil, []*http.Cookie{viewerSession, csrf})
	if viewerEditDenied.Code != http.StatusForbidden {
		t.Fatalf("viewer asset edit page: got %d, want %d", viewerEditDenied.Code, http.StatusForbidden)
	}
	viewerCreateDenied := request(t, handler, http.MethodGet, "/assets/new", nil, []*http.Cookie{viewerSession, csrf})
	if viewerCreateDenied.Code != http.StatusForbidden {
		t.Fatalf("viewer asset create page: got %d, want %d", viewerCreateDenied.Code, http.StatusForbidden)
	}
	if strings.Contains(visible.Body.String(), `#add-event`) {
		t.Fatalf("viewer should not receive lifecycle write controls: %s", visible.Body.String())
	}
	if strings.Contains(visible.Body.String(), `href="/imports"`) || strings.Contains(visible.Body.String(), `href="/admin/members"`) {
		t.Fatalf("viewer account menu has incorrect entries: %s", visible.Body.String())
	}
	adminDenied := request(t, handler, http.MethodGet, "/admin/catalog", nil, []*http.Cookie{viewerSession, csrf})
	if adminDenied.Code != http.StatusForbidden {
		t.Fatalf("viewer catalog management page: got %d, want %d", adminDenied.Code, http.StatusForbidden)
	}
	forbidden := request(t, handler, http.MethodPost, "/admin/catalog/categories", url.Values{
		"csrf_token": {csrf.Value}, "name": {"不应创建"}, "icon_key": {"package"},
	}, []*http.Cookie{viewerSession, csrf})
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("viewer catalog write: got %d, want %d", forbidden.Code, http.StatusForbidden)
	}
	forbidden = request(t, handler, http.MethodPost, "/assets/"+match[1]+"/events", url.Values{
		"csrf_token": {csrf.Value}, "event_type": {"repair"}, "amount": {"1.00"}, "currency": {"CNY"}, "occurred_at": {"2026-08-21T10:00"},
	}, []*http.Cookie{viewerSession, csrf})
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("viewer lifecycle write: got %d, want %d", forbidden.Code, http.StatusForbidden)
	}
	forbidden = request(t, handler, http.MethodGet, "/events/"+correctionLinks[0][1]+"/correct", nil, []*http.Cookie{viewerSession, csrf})
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("viewer correction form: got %d, want %d", forbidden.Code, http.StatusForbidden)
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

func TestProductModel3DUploadViewerAndETag(t *testing.T) {
	handler := newTestHandler(t)
	setupPage := request(t, handler, http.MethodGet, "/setup", nil, nil)
	csrf := responseCookie(t, setupPage, csrfCookie)
	setup := request(t, handler, http.MethodPost, "/setup", url.Values{"csrf_token": {csrf.Value}, "tenant_name": {"3D Tenant"}, "base_currency": {"CNY"}, "username": {"owner"}, "password": {"owner secure password"}}, []*http.Cookie{csrf})
	session := responseCookie(t, setup, sessionCookie)
	request(t, handler, http.MethodPost, "/admin/catalog/categories", url.Values{"csrf_token": {csrf.Value}, "name": {"手机"}, "icon_key": {"smartphone"}}, []*http.Cookie{session, csrf})
	page := request(t, handler, http.MethodGet, "/admin/catalog", nil, []*http.Cookie{session, csrf})
	categoryID := optionID(t, page.Body.String(), "手机")
	request(t, handler, http.MethodPost, "/admin/catalog/models", url.Values{"csrf_token": {csrf.Value}, "category_id": {categoryID}, "name": {"Model 3D"}}, []*http.Cookie{session, csrf})
	page = request(t, handler, http.MethodGet, "/admin/catalog", nil, []*http.Cookie{session, csrf})
	modelID := optionID(t, page.Body.String(), "手机 / Model 3D")
	request(t, handler, http.MethodPost, "/admin/catalog/variants", url.Values{"csrf_token": {csrf.Value}, "model_id": {modelID}, "name": {"Standard"}}, []*http.Cookie{session, csrf})
	page = request(t, handler, http.MethodGet, "/assets/new", nil, []*http.Cookie{session, csrf})
	variantID := optionID(t, page.Body.String(), "手机 / Model 3D / Standard")
	request(t, handler, http.MethodPost, "/assets", url.Values{"csrf_token": {csrf.Value}, "variant_id": {variantID}, "display_name": {"3D Device"}}, []*http.Cookie{session, csrf})
	page = request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{session, csrf})
	match := regexp.MustCompile(`/assets/([0-9a-f-]{36})`).FindStringSubmatch(page.Body.String())
	if len(match) != 2 {
		t.Fatal("asset link missing")
	}
	assetID := match[1]
	before := request(t, handler, http.MethodGet, "/assets/"+assetID, nil, []*http.Cookie{session, csrf})
	if strings.Contains(before.Body.String(), "data-model-viewer") {
		t.Fatal("viewer rendered without model")
	}
	glb := webTestGLB()
	upload := multipartRequest(t, handler, "/admin/catalog/models/"+modelID+"/3d", csrf.Value, glb, []*http.Cookie{session, csrf})
	if upload.Code != http.StatusSeeOther {
		t.Fatalf("upload status=%d body=%s", upload.Code, upload.Body.String())
	}
	catalogPage := request(t, handler, http.MethodGet, "/admin/catalog", nil, []*http.Cookie{session, csrf})
	if !strings.Contains(catalogPage.Body.String(), "已有 3D") {
		t.Fatalf("catalog did not show the model binding: %s", catalogPage.Body.String())
	}
	detail := request(t, handler, http.MethodGet, "/assets/"+assetID, nil, []*http.Cookie{session, csrf})
	if !strings.Contains(detail.Body.String(), "data-model-viewer") || !strings.Contains(detail.Body.String(), "asset-model-viewer.js") {
		t.Fatalf("viewer markup missing: %s", detail.Body.String())
	}
	digest := sha256.Sum256(glb)
	if want := "/assets/" + assetID + "/model.glb?v=" + hex.EncodeToString(digest[:]); !strings.Contains(detail.Body.String(), want) {
		t.Fatalf("versioned model URL missing: %s", detail.Body.String())
	}
	model := request(t, handler, http.MethodGet, "/assets/"+assetID+"/model.glb", nil, []*http.Cookie{session, csrf})
	if model.Code != http.StatusOK || model.Header().Get("Content-Type") != "model/gltf-binary" || !bytes.Equal(model.Body.Bytes(), glb) {
		t.Fatalf("model response status=%d headers=%v", model.Code, model.Header())
	}
	if got := model.Header().Get("Cache-Control"); got != "private, max-age=86400" {
		t.Fatalf("cache control=%q", got)
	}
	req := httptest.NewRequest(http.MethodGet, "/assets/"+assetID+"/model.glb", nil)
	req.Header.Set("If-None-Match", model.Header().Get("ETag"))
	req.AddCookie(session)
	req.AddCookie(csrf)
	conditional := httptest.NewRecorder()
	handler.ServeHTTP(conditional, req)
	if conditional.Code != http.StatusNotModified {
		t.Fatalf("conditional status=%d", conditional.Code)
	}
}

func webTestGLB() []byte {
	jsonData := []byte(`{"asset":{"version":"2.0"}}`)
	for len(jsonData)%4 != 0 {
		jsonData = append(jsonData, ' ')
	}
	data := make([]byte, 20+len(jsonData))
	copy(data, "glTF")
	binary.LittleEndian.PutUint32(data[4:], 2)
	binary.LittleEndian.PutUint32(data[8:], uint32(len(data)))
	binary.LittleEndian.PutUint32(data[12:], uint32(len(jsonData)))
	binary.LittleEndian.PutUint32(data[16:], 0x4e4f534a)
	copy(data[20:], jsonData)
	return data
}
func multipartRequest(t *testing.T, handler http.Handler, target, csrf string, file []byte, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("csrf_token", csrf)
	_ = writer.WriteField("model_3d_author", "Tester")
	part, err := writer.CreateFormFile("model_3d", "model.glb")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(file)
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, target, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func newTestHandler(t *testing.T) http.Handler {
	return newTestHandlerWithBlob(t, nil)
}

func newTestHandlerWithBlob(t *testing.T, wrap func(application.BlobStore) application.BlobStore) http.Handler {
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
	lifecycle := application.NewLifecycleService(adapter)
	localStore, err := localblob.New(filepath.Join(t.TempDir(), "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	var testBlob application.BlobStore = localStore
	if wrap != nil {
		testBlob = wrap(testBlob)
	}
	modelMedia := application.NewModelMediaService(adapter, blob.Registry{"local": testBlob}, blob.ObjectKeyMapper{}, "local")
	server, err := New(auth, catalog, lifecycle, db, Options{AuthMode: "local", ModelMedia: modelMedia})
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
