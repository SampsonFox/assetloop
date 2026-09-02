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
	for _, want := range []string{"我的物品", "还没有物品", "录入第一个物品", `data-title="录入第一件物品"`, "当前还没有规格", `id="asset-form"`} {
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
	catalog = request(t, handler, http.MethodGet, "/admin/catalog", nil, []*http.Cookie{ownerSession, csrf})
	for _, want := range []string{"物品类型配置", `class="model-list"`, `class="model-card"`, `class="model-category"`, `class="variant-list"`, "新增型号", "新增类别", "新增规格", "256GB"} {
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
	modelCard := regexp.MustCompile(`(?s)<article class="model-card"[^>]*>.*?手机.*?iPhone 17 Pro.*?<div class="variant-list">.*?256GB.*?</article>`)
	if !modelCard.MatchString(catalog.Body.String()) {
		t.Fatalf("specification must be nested under its model card: %s", catalog.Body.String())
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
	catalog = request(t, handler, http.MethodGet, "/", nil, []*http.Cookie{ownerSession, csrf})
	if catalog.Code != http.StatusOK || !strings.Contains(catalog.Body.String(), "我的主力手机") || !strings.Contains(catalog.Body.String(), "WEB-SERIAL-001") {
		t.Fatalf("catalog asset list: status=%d body=%s", catalog.Code, catalog.Body.String())
	}
	match := regexp.MustCompile(`/assets/([0-9a-f-]{36})`).FindStringSubmatch(catalog.Body.String())
	if len(match) != 2 {
		t.Fatalf("asset detail link not found: %s", catalog.Body.String())
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
	for _, want := range []string{"我的主力手机", "iPhone 17 Pro", "256GB", "WEB-SERIAL-001", "官方商城", "Web 全要素目录记录"} {
		if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), want) {
			t.Fatalf("asset detail missing %q: status=%d body=%s", want, detail.Code, detail.Body.String())
		}
	}

	unconfirmedFX := request(t, handler, http.MethodPost, "/assets/"+match[1]+"/events", url.Values{
		"csrf_token": {csrf.Value}, "event_type": {"purchase"}, "amount": {"1000.00"}, "currency": {"USD"},
		"fx_rate": {"7.12"}, "fx_rate_date": {"2026-08-01"}, "fx_rate_source": {"web-fixture"},
		"occurred_at": {"2026-08-01T10:00"}, "source": {"manual"},
	}, []*http.Cookie{ownerSession, csrf})
	if unconfirmedFX.Code != http.StatusUnprocessableEntity || !strings.Contains(unconfirmedFX.Body.String(), "FX conversion must be confirmed") {
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
	draft := request(t, handler, http.MethodPost, "/imports", url.Values{
		"csrf_token": {csrf.Value}, "asset_id": {match[1]}, "event_type": {"sale"}, "amount": {"8000.00"},
		"currency": {"CNY"}, "occurred_at": {"2026-08-20T10:00"}, "source": {"ai-harness"},
		"external_reference": {"SALE-WEB-001"}, "notes": {"卖出"}, "raw_text": {"识别到卖出金额 8000 CNY"},
	}, []*http.Cookie{ownerSession, csrf})
	if draft.Code != http.StatusSeeOther {
		t.Fatalf("create sale draft: status=%d body=%s", draft.Code, draft.Body.String())
	}
	imports := request(t, handler, http.MethodGet, "/imports", nil, []*http.Cookie{ownerSession, csrf})
	draftMatch := regexp.MustCompile(`/imports/([0-9a-f-]{36})`).FindStringSubmatch(imports.Body.String())
	if len(draftMatch) != 2 || !strings.Contains(imports.Body.String(), "8000.00 CNY") {
		t.Fatalf("pending import not shown: %s", imports.Body.String())
	}
	confirmed := request(t, handler, http.MethodPost, "/imports/"+draftMatch[1]+"/confirm", url.Values{
		"csrf_token": {csrf.Value},
	}, []*http.Cookie{ownerSession, csrf})
	if confirmed.Code != http.StatusSeeOther {
		t.Fatalf("confirm sale draft: status=%d body=%s", confirmed.Code, confirmed.Body.String())
	}
	detail = request(t, handler, http.MethodGet, "/assets/"+match[1], nil, []*http.Cookie{ownerSession, csrf})
	for _, want := range []string{"7270.00 CNY", "8000.00 CNY", "730.00 CNY", "已卖出", "1000.00 USD", "2026-08-01", "web-fixture", "正确维修金额", "已作废"} {
		if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), want) {
			t.Fatalf("lifecycle detail missing %q: status=%d body=%s", want, detail.Code, detail.Body.String())
		}
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
