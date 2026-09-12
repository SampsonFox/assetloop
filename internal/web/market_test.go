package web

import (
	"context"
	"errors"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/config"
	"github.com/SampsonFox/assetloop/internal/domain"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

type webQuoteFixture struct {
	err         error
	calls       int
	detailCalls int
}

func (p *webQuoteFixture) FetchQuote(context.Context, application.MarketQuery) (application.MarketQuote, error) {
	p.calls++
	return application.MarketQuote{ModelDesc: "Phone 256GB", Provider: "zhuanzhuan", ProviderVersion: "fixture", Currency: "CNY", MaxMinor: 645800, ObservedAt: time.Date(2026, 9, 8, 2, 0, 0, 0, time.UTC), Evidence: "{}"}, p.err
}
func TestMarketWebCreateBindDisplayAndPermissions(t *testing.T) {
	ctx := context.Background()
	cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "market.db")}
	db, e := basestore.Open(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e = basestore.Migrate(ctx, db, cfg); e != nil {
		t.Fatal(e)
	}
	st := sqlite.New(db)
	auth := application.NewAuthService(st)
	cred, e := auth.Setup(ctx, application.SetupAuth{TenantName: "Market", BaseCurrency: "CNY", Username: "owner", Password: "owner secure password"})
	if e != nil {
		t.Fatal(e)
	}
	p := &webQuoteFixture{}
	market := application.NewMarketService(st, p, nil, application.MarketOptions{})
	catalog := application.NewCatalogService(st)
	server, e := New(auth, catalog, application.NewLifecycleService(st), db, Options{AuthMode: "local", Specifications: application.NewSpecificationService(st), Market: market})
	if e != nil {
		t.Fatal(e)
	}
	h := server.Handler()
	session := &http.Cookie{Name: sessionCookie, Value: cred.Token}
	page := request(t, h, "GET", "/admin/market", nil, []*http.Cookie{session})
	csrf := responseCookie(t, page, csrfCookie)
	cookies := []*http.Cookie{session, csrf}
	if page.Code != 200 || !strings.Contains(page.Body.String(), "暂无二手物品") {
		t.Fatal(page.Code, page.Body.String())
	}
	if strings.Contains(page.Body.String(), `name="q"`) {
		t.Fatal("empty collection must not show a list filter")
	}
	form := url.Values{"csrf_token": {csrf.Value}, "name": {"Phone"}, "keyword": {"Phone"}, "filter_criteria": {"256GB"}}
	draftID := func(body string) string {
		t.Helper()
		m := regexp.MustCompile(`name="draft_id" value="([^"]+)"`).FindStringSubmatch(body)
		if len(m) != 2 {
			t.Fatal("missing discovery draft")
		}
		return m[1]
	}
	opened := request(t, h, "GET", "/admin/market?dialog=market-editor&keyword=Phone&filter_criteria=256GB", nil, cookies)
	if !strings.Contains(opened.Body.String(), "搜索转转商品") || !strings.Contains(opened.Body.String(), "直接按型号查询") {
		t.Fatal("missing default discovery or direct path")
	}
	form.Set("draft_id", draftID(opened.Body.String()))
	form.Set("market_action", "find")
	found := request(t, h, "POST", "/admin/market/discover", form, cookies)
	if found.Code != 200 || !strings.Contains(found.Body.String(), "Second candidate") || !strings.Contains(found.Body.String(), "在售价") || p.detailCalls != 0 {
		t.Fatal("search candidates", found.Code)
	}
	form.Set("market_action", "detail")
	form.Set("candidate", "1")
	selected := request(t, h, "POST", "/admin/market/discover", form, cookies)
	if selected.Code != 200 || !strings.Contains(selected.Body.String(), "蓝色") || !strings.Contains(selected.Body.String(), "运行内存") || !strings.Contains(selected.Body.String(), "未提供") {
		t.Fatal("missing selected specs", selected.Code)
	}
	if strings.Contains(selected.Body.String(), "fixture-second-metric") {
		t.Fatal("provider tuple exposed in HTML")
	}
	form.Set("market_action", "use")
	used := request(t, h, "POST", "/admin/market/discover", form, cookies)
	if used.Code != 200 || !strings.Contains(used.Body.String(), "颜色:蓝色") {
		t.Fatal("missing prefilled conditions", used.Code)
	}
	form.Del("market_action")
	form.Del("candidate")
	preview := request(t, h, "POST", "/admin/market/preview", form, cookies)
	if preview.Code != 200 || !strings.Contains(preview.Body.String(), "Phone 256GB") || !strings.Contains(preview.Body.String(), "6458.00") {
		t.Fatal(preview.Code, preview.Body.String())
	}
	if !strings.Contains(preview.Body.String(), "所选商品规格") || !strings.Contains(preview.Body.String(), "成交行情范围") {
		t.Fatal("product and quote scopes are not distinct")
	}
	rejected := request(t, h, "POST", "/admin/market", form, cookies)
	if rejected.Code != 422 {
		t.Fatal("scope acceptance was optional")
	}
	form.Set("confirmed_model", "Phone 256GB")
	draftMatch := regexp.MustCompile(`name="draft_id" value="([^"]+)"`).FindStringSubmatch(preview.Body.String())
	if len(draftMatch) != 2 {
		t.Fatal("missing preview draft")
	}
	form.Set("draft_id", draftMatch[1])
	form.Set("accept_scope", "1")
	created := request(t, h, "POST", "/admin/market", form, cookies)
	if created.Code != 303 {
		t.Fatal(created.Code, created.Body.String())
	}
	items, e := market.List(ctx, cred.Principal)
	if e != nil || len(items) != 1 {
		t.Fatal(items, e)
	}
	calls := p.calls
	for _, tc := range []struct {
		query string
		found bool
	}{{"phone", true}, {"256gb", true}, {"missing", false}} {
		page = request(t, h, "GET", "/admin/market?q="+tc.query, nil, cookies)
		body := page.Body.String()
		if page.Code != 200 || !strings.Contains(body, "筛选已添加的行情") || !strings.Contains(body, `name="q"`) {
			t.Fatal("missing saved quote filter", page.Code)
		}
		if strings.Contains(body, `class="market-actions"`) != tc.found {
			t.Fatal("incorrect saved quote results", tc.query)
		}
		if !tc.found && (!strings.Contains(body, "没有匹配的行情") || strings.Contains(body, "暂无二手物品")) {
			t.Fatal("filtered empty state must differ from first use")
		}
	}
	if p.calls != calls {
		t.Fatal("list filtering must not call the provider")
	}
	if items[0].SelectionJSON == "" || p.detailCalls != 2 {
		t.Fatal("selection not rechecked or saved")
	}
	direct := url.Values{"csrf_token": {csrf.Value}, "keyword": {"Phone"}, "filter_criteria": {"256GB"}, "name": {"Direct"}}
	directPage := request(t, h, "POST", "/admin/market/preview", direct, cookies)
	if directPage.Code != 200 || strings.Contains(directPage.Body.String(), "<h3>所选商品规格</h3>") {
		t.Fatal("direct query path")
	}
	direct.Set("draft_id", draftID(directPage.Body.String()))
	direct.Set("query_again", "1")
	edited := request(t, h, "POST", "/admin/market/preview", direct, cookies)
	if edited.Code != 200 || strings.Contains(edited.Body.String(), `name="accept_scope"`) {
		t.Fatal("old preview not invalidated")
	}
	direct.Set("accept_scope", "1")
	if stale := request(t, h, "POST", "/admin/market", direct, cookies); stale.Code != 422 {
		t.Fatal("stale preview accepted")
	}
	cat, e := catalog.CreateCategory(ctx, cred.Principal, application.CreateCategory{Name: "Devices"})
	if e != nil {
		t.Fatal(e)
	}
	model, e := catalog.CreateModel(ctx, cred.Principal, application.CreateModel{CategoryID: cat.ID, Name: "Phone"})
	if e != nil {
		t.Fatal(e)
	}
	saved := request(t, h, "POST", "/assets", url.Values{"csrf_token": {csrf.Value}, "model_id": {model.ID}, "display_name": {"One"}, "market_item_id": {items[0].ID}}, cookies)
	if saved.Code != 303 {
		t.Fatal(saved.Code, saved.Body.String())
	}
	detail := request(t, h, "GET", saved.Header().Get("Location"), nil, cookies)
	for _, want := range []string{"/static/zhuanzhuan.ico", "6458.00", "2026-09-08T02:00:00Z", "最新一期成交最高价"} {
		if detail.Code != 200 || !strings.Contains(detail.Body.String(), want) {
			t.Fatalf("missing %s: %d %s", want, detail.Code, detail.Body.String())
		}
	}
	p.err = application.ErrMarketAuth
	failed := request(t, h, "POST", "/admin/market/"+items[0].ID+"/refresh", url.Values{"csrf_token": {csrf.Value}}, cookies)
	if failed.Code != 422 || !strings.Contains(failed.Body.String(), "无效或已过期") {
		t.Fatal(failed.Code, failed.Body.String())
	}
	detail = request(t, h, "GET", saved.Header().Get("Location"), nil, cookies)
	if !strings.Contains(detail.Body.String(), "6458.00") {
		t.Fatal("lost old quote")
	}
	if _, e = auth.AddMember(ctx, cred.Principal, application.AddMember{Username: "viewer", Password: "viewer secure password", Role: application.RoleViewer}); e != nil {
		t.Fatal(e)
	}
	viewer, e := auth.Login(ctx, application.Login{Username: "viewer", Password: "viewer secure password"})
	if e != nil {
		t.Fatal(e)
	}
	viewerCookies := []*http.Cookie{{Name: sessionCookie, Value: viewer.Token}, csrf}
	page = request(t, h, "GET", "/admin/market", nil, viewerCookies)
	if page.Code != 200 || strings.Contains(page.Body.String(), "确认型号并保存") {
		t.Fatal("viewer mutation UI")
	}
	for _, target := range []string{"/admin/market", "/admin/market/preview", "/admin/market/discover", "/admin/market/" + items[0].ID, "/admin/market/" + items[0].ID + "/refresh"} {
		r := request(t, h, "POST", target, form, viewerCookies)
		if r.Code != 403 {
			t.Fatal(target, r.Code)
		}
	}
	noCSRF := request(t, h, "POST", "/admin/market/preview", url.Values{}, cookies)
	if noCSRF.Code != 403 {
		t.Fatal(noCSRF.Code)
	}
	if !errors.Is(p.err, application.ErrMarketAuth) {
		t.Fatal("fixture changed")
	}
}

func (p *webQuoteFixture) SearchProducts(context.Context, application.ProductSearchQuery) (application.ProductSearchResult, error) {
	n := int64(10000)
	return application.ProductSearchResult{Items: []application.MarketProduct{{Reference: application.ProductReference{ID: "1", Metric: "first"}, Provider: "zhuanzhuan", Title: "First candidate", Currency: "CNY", PriceMinor: &n}, {Reference: application.ProductReference{ID: "2", Metric: "fixture-second-metric"}, Provider: "zhuanzhuan", Title: "Second candidate", Currency: "CNY", PriceMinor: &n}}}, nil
}
func (p *webQuoteFixture) GetProductDetail(ctx context.Context, ref application.ProductReference) (application.MarketProductDetail, error) {
	p.detailCalls++
	if ref.ID != "2" || ref.Metric != "fixture-second-metric" {
		return application.MarketProductDetail{}, application.ErrMarketInvalid
	}
	return application.MarketProductDetail{Selection: domain.MarketSelection{Provider: "zhuanzhuan", ProductID: ref.ID, Title: "Phone 256GB", ObservedAt: time.Now(), Specifications: []domain.MarketSpecification{{Name: "颜色", Value: "蓝色"}, {Name: "存储容量", Value: "256GB"}}}, Currency: "CNY"}, nil
}
