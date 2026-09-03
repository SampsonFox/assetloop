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
	if !strings.Contains(dashboard.Body.String(), `<a class="brand" href="/overview">`) || strings.Contains(dashboard.Body.String(), `>概览</a>`) {
		t.Fatalf("brand must be the sole overview entry: %s", dashboard.Body.String())
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
	if setupPage.Code != http.StatusOK || !strings.Contains(setupPage.Body.String(), `<html lang="en" data-theme="system">`) || !strings.Contains(setupPage.Body.String(), "Create your AssetLoop workspace") {
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
	for _, want := range []string{`class="account-avatar"`, `class="menu-icon"`, `data-auto-submit`, `class="segmented language-segmented"`, `role="radiogroup"`, `type="radio" name="locale"`, `type="radio" name="theme"`} {
		if !strings.Contains(home.Body.String(), want) {
			t.Fatalf("compact account control missing %q: %s", want, home.Body.String())
		}
	}
	if strings.Contains(home.Body.String(), `<select name="theme"`) || strings.Contains(home.Body.String(), `>保存偏好</button>`) {
		t.Fatalf("account preferences should switch directly without select boxes or a save button: %s", home.Body.String())
	}

	updated := request(t, handler, http.MethodPost, "/preferences", url.Values{
		"csrf_token": {csrf.Value}, "locale": {"en"}, "theme": {"dark"}, "return_to": {"/?view=grid"},
	}, []*http.Cookie{session, csrf})
	if updated.Code != http.StatusSeeOther || updated.Header().Get("Location") != "/?view=grid" {
		t.Fatalf("preference update: status=%d location=%q body=%s", updated.Code, updated.Header().Get("Location"), updated.Body.String())
	}
	zhCookie := &http.Cookie{Name: localeCookie, Value: "zh-CN"}
	english := request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{session, csrf, zhCookie})
	for _, want := range []string{`<html lang="en" data-theme="dark">`, "My assets", "Asset type settings", "Log out"} {
		if !strings.Contains(english.Body.String(), want) {
			t.Fatalf("stored preference missing %q: %s", want, english.Body.String())
		}
	}

	invalidReturn := request(t, handler, http.MethodPost, "/preferences", url.Values{
		"csrf_token": {csrf.Value}, "locale": {"zh-CN"}, "theme": {"light"}, "return_to": {"https://example.com"},
	}, []*http.Cookie{session, csrf})
	if invalidReturn.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidReturn.Body.String(), "Invalid return address") {
		t.Fatalf("external return target: status=%d body=%s", invalidReturn.Code, invalidReturn.Body.String())
	}
	unchanged := request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{session, csrf})
	if !strings.Contains(unchanged.Body.String(), `lang="en" data-theme="dark"`) {
		t.Fatalf("invalid return target mutated preferences: %s", unchanged.Body.String())
	}
	light := request(t, handler, http.MethodPost, "/preferences", url.Values{
		"csrf_token": {csrf.Value}, "locale": {"zh-CN"}, "theme": {"light"}, "return_to": {"/"},
	}, []*http.Cookie{session, csrf})
	if light.Code != http.StatusSeeOther {
		t.Fatalf("light theme update: status=%d body=%s", light.Code, light.Body.String())
	}
	lightPage := request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{session, csrf})
	if !strings.Contains(lightPage.Body.String(), `lang="zh-CN" data-theme="light"`) {
		t.Fatalf("light theme did not render server-side: %s", lightPage.Body.String())
	}

	withoutCSRF := request(t, handler, http.MethodPost, "/preferences", url.Values{"locale": {"zh-CN"}, "theme": {"light"}}, []*http.Cookie{session})
	if withoutCSRF.Code != http.StatusForbidden {
		t.Fatalf("preferences without CSRF: got %d", withoutCSRF.Code)
	}
}

func TestThemeStylesUseSemanticSurfaces(t *testing.T) {
	handler := newTestHandler(t)
	stylesheet := request(t, handler, http.MethodGet, "/static/app.css", nil, nil)
	for _, want := range []string{`:root[data-theme="dark"]`, `:root[data-theme="system"]`, `prefers-color-scheme:dark`, `background:var(--card)`, `background:var(--field)`, `.account-menu-panel`} {
		if stylesheet.Code != http.StatusOK || !strings.Contains(stylesheet.Body.String(), want) {
			t.Fatalf("theme stylesheet missing %q: status=%d body=%s", want, stylesheet.Code, stylesheet.Body.String())
		}
	}
	script := request(t, handler, http.MethodGet, "/static/app.js", nil, nil)
	for _, want := range []string{`[data-auto-submit]`, `document.documentElement.dataset.theme`, `form.requestSubmit()`} {
		if script.Code != http.StatusOK || !strings.Contains(script.Body.String(), want) {
			t.Fatalf("preference interaction missing %q: status=%d body=%s", want, script.Code, script.Body.String())
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
	for _, want := range []string{`.timeline-heading {`, `.event-drawer-panel { display:flex; flex-direction:column;`, `.event-form-grid { display:grid; grid-template-columns:repeat(2,minmax(0,1fr));`, `.drawer-footer {`} {
		if stylesheet.Code != http.StatusOK || !strings.Contains(stylesheet.Body.String(), want) {
			t.Fatalf("lifecycle drawer style missing %q: status=%d body=%s", want, stylesheet.Code, stylesheet.Body.String())
		}
	}
	script := request(t, handler, http.MethodGet, "/static/app.js", nil, nil)
	for _, want := range []string{`window.location.hash === "#add-event"`, `document.querySelector("#event-drawer .error")`, `document.querySelector('[data-dialog-open="event-drawer"]')`} {
		if script.Code != http.StatusOK || !strings.Contains(script.Body.String(), want) {
			t.Fatalf("lifecycle drawer interaction missing %q: status=%d body=%s", want, script.Code, script.Body.String())
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
	for _, want := range []string{"我的物品", "还没有物品", "录入第一个物品", `data-title="录入第一个物品"`, "当前还没有物品类型", `id="asset-form"`} {
		if home.Code != http.StatusOK || !strings.Contains(home.Body.String(), want) {
			t.Fatalf("asset-first empty state missing %q: status=%d body=%s", want, home.Code, home.Body.String())
		}
	}
	if strings.Contains(home.Body.String(), `>前往物品配置</a>`) {
		t.Fatalf("asset-list empty state must not replace the primary create action with catalog configuration: %s", home.Body.String())
	}
	if strings.Count(home.Body.String(), `id="asset-form"`) != 1 {
		t.Fatalf("create/edit must share one asset form: %s", home.Body.String())
	}

	grid := request(t, handler, http.MethodGet, "/?view=grid", nil, []*http.Cookie{session, csrf})
	viewCookie := responseCookie(t, grid, assetViewCookie)
	if viewCookie.Value != "grid" || !strings.Contains(grid.Body.String(), `class="is-active">卡片`) {
		t.Fatalf("grid preference was not selected: cookie=%q body=%s", viewCookie.Value, grid.Body.String())
	}
	persisted := request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{session, csrf, viewCookie})
	if !strings.Contains(persisted.Body.String(), `class="is-active">卡片`) {
		t.Fatalf("grid preference was not persisted: %s", persisted.Body.String())
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

func TestAssetDrawerCreatesMissingTypeWithoutLeavingAssetList(t *testing.T) {
	handler := newTestHandler(t)
	setupPage := request(t, handler, http.MethodGet, "/setup", nil, nil)
	csrf := responseCookie(t, setupPage, csrfCookie)
	setup := request(t, handler, http.MethodPost, "/setup", url.Values{
		"csrf_token": {csrf.Value}, "tenant_name": {"Inline Type Tenant"}, "base_currency": {"CNY"},
		"username": {"owner"}, "password": {"owner secure password"},
	}, []*http.Cookie{csrf})
	session := responseCookie(t, setup, sessionCookie)

	home := request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{session, csrf})
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
		if location.Path != "/" || location.Query().Get("dialog") != wantDialog || location.Query().Get(idKey) == "" {
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
	variantID := redirect(variant, "asset-drawer", "variant_id")

	reopened := request(t, handler, http.MethodGet, variant.Header().Get("Location"), nil, []*http.Cookie{session, csrf})
	if reopened.Code != http.StatusOK || !strings.Contains(reopened.Body.String(), `<option value="`+variantID+`">手机 / iPhone 17 Pro / 256GB</option>`) {
		t.Fatalf("new type was not available to the reopened asset form: status=%d body=%s", reopened.Code, reopened.Body.String())
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
	for _, want := range []string{"物品类型配置", `class="catalog-table"`, `class="catalog-category"`, `class="variant-tags"`, `class="variant-tag">256GB`, `class="variant-tag">512GB`, `data-model-variants`, "新增型号", "新增类别", "新增规格"} {
		if catalog.Code != http.StatusOK || !strings.Contains(catalog.Body.String(), want) {
			t.Fatalf("model-first type configuration missing %q: status=%d body=%s", want, catalog.Code, catalog.Body.String())
		}
	}
	if strings.Contains(catalog.Body.String(), "价格规格") {
		t.Fatalf("type configuration must use the simpler specification label: %s", catalog.Body.String())
	}
	headingActions := regexp.MustCompile(`(?s)<div class="heading-actions">(.*?)</div>`).FindStringSubmatch(catalog.Body.String())
	if len(headingActions) != 2 || strings.Contains(headingActions[1], "新增类别") {
		t.Fatalf("category creation must not be a top-level action: %s", catalog.Body.String())
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
	catalog = request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{ownerSession, csrf})
	variantID := optionID(t, catalog.Body.String(), "手机 / iPhone 17 Pro / 256GB")

	created = request(t, handler, http.MethodPost, "/assets", url.Values{
		"csrf_token": {csrf.Value}, "variant_id": {variantID}, "display_name": {"我的主力手机"},
		"serial_number": {"WEB-SERIAL-001"}, "color": {"黑色"}, "purchase_channel": {"官方商城"},
		"notes": {"Web 全要素目录记录"},
	}, []*http.Cookie{ownerSession, csrf})
	if created.Code != http.StatusSeeOther {
		t.Fatalf("create asset: status=%d body=%s", created.Code, created.Body.String())
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
	for _, want := range []string{"我的主力手机", "iPhone 17 Pro", "256GB", "WEB-SERIAL-001", "官方商城", "Web 全要素目录记录", `class="timeline-heading"`, `id="add-event"`, `data-dialog-open="event-drawer"`, `id="event-drawer"`, `id="event-form"`, `data-currency-select`, `data-base-currency="CNY"`, `data-fx-field hidden`, `data-fx-required`} {
		if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), want) {
			t.Fatalf("asset detail missing %q: status=%d body=%s", want, detail.Code, detail.Body.String())
		}
	}
	visibleDetail := strings.Split(detail.Body.String(), `<dialog class="drawer" id="event-drawer"`)[0]
	if regexp.MustCompile(`<form[^>]+action="/assets/` + regexp.QuoteMeta(match[1]) + `/events"`).MatchString(visibleDetail) {
		t.Fatalf("lifecycle create form must not be rendered in the page flow: %s", visibleDetail)
	}
	if strings.Count(detail.Body.String(), `data-fx-field hidden`) != 4 {
		t.Fatalf("same-currency event form should initially hide all FX controls: %s", detail.Body.String())
	}

	unconfirmedFX := request(t, handler, http.MethodPost, "/assets/"+match[1]+"/events", url.Values{
		"csrf_token": {csrf.Value}, "event_type": {"purchase"}, "amount": {"1000.00"}, "currency": {"USD"},
		"fx_rate": {"7.12"}, "fx_rate_date": {"2026-08-01"}, "fx_rate_source": {"web-fixture"},
		"occurred_at": {"2026-08-01T10:00"}, "source": {"manual"},
	}, []*http.Cookie{ownerSession, csrf})
	if unconfirmedFX.Code != http.StatusUnprocessableEntity || !strings.Contains(unconfirmedFX.Body.String(), "必须确认汇率换算") {
		t.Fatalf("unconfirmed FX should be rejected: status=%d body=%s", unconfirmedFX.Code, unconfirmedFX.Body.String())
	}
	purchase := request(t, handler, http.MethodPost, "/assets/"+match[1]+"/events", url.Values{
		"csrf_token": {csrf.Value}, "event_type": {"purchase"}, "amount": {"1000.00"}, "currency": {"USD"},
		"fx_rate": {"7.12"}, "fx_rate_date": {"2026-08-01"}, "fx_rate_source": {"web-fixture"}, "fx_confirmed": {"on"},
		"occurred_at": {"2026-08-01T10:00"}, "source": {"manual"}, "external_reference": {"ORDER-WEB-001"}, "notes": {"美元买入"},
	}, []*http.Cookie{ownerSession, csrf})
	if purchase.Code != http.StatusSeeOther {
		t.Fatalf("record purchase: status=%d body=%s", purchase.Code, purchase.Body.String())
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
	if len(correctionLinks) != 2 {
		t.Fatalf("expected purchase and repair correction links: %s", detail.Body.String())
	}
	corrected := request(t, handler, http.MethodPost, "/events/"+correctionLinks[1][1]+"/correct", url.Values{
		"csrf_token": {csrf.Value}, "amount": {"150.00"}, "currency": {"CNY"},
		"occurred_at": {"2026-08-10T10:00"}, "source": {"manual-correction"}, "notes": {"正确维修金额"},
	}, []*http.Cookie{ownerSession, csrf})
	if corrected.Code != http.StatusSeeOther {
		t.Fatalf("correct repair: status=%d body=%s", corrected.Code, corrected.Body.String())
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
	for _, want := range []string{`class="timeline-list"`, `class="timeline-item`, "7270.00 CNY", "8000.00 CNY", "730.00 CNY", "已卖出", "1000.00 USD", "2026-08-01", "web-fixture", "正确维修金额", "已作废"} {
		if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), want) {
			t.Fatalf("lifecycle detail missing %q: status=%d body=%s", want, detail.Code, detail.Body.String())
		}
	}
	assetList := request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{ownerSession, csrf})
	if assetList.Code != http.StatusOK || !strings.Contains(assetList.Body.String(), "7270.00 CNY") {
		t.Fatalf("asset list should show base-currency lifecycle cost: status=%d body=%s", assetList.Code, assetList.Body.String())
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
	lifecycle := application.NewLifecycleService(adapter)
	server, err := New(auth, catalog, lifecycle, db, Options{AuthMode: "local"})
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
