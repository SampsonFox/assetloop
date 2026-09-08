package web

import (
	"context"
	"errors"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type webQuoteFixture struct{ err error }

func (p *webQuoteFixture) FetchQuote(context.Context, application.MarketQuery) (application.MarketQuote, error) {
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
	market := application.NewMarketService(st, p, nil, application.MarketOptions{UnitsConfirmed: true})
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
	form := url.Values{"csrf_token": {csrf.Value}, "name": {"Phone"}, "keyword": {"Phone"}, "filter_criteria": {"256GB"}}
	preview := request(t, h, "POST", "/admin/market/preview", form, cookies)
	if preview.Code != 200 || !strings.Contains(preview.Body.String(), "Phone 256GB") || !strings.Contains(preview.Body.String(), "6458.00") {
		t.Fatal(preview.Code, preview.Body.String())
	}
	form.Set("confirmed_model", "Phone 256GB")
	created := request(t, h, "POST", "/admin/market", form, cookies)
	if created.Code != 303 {
		t.Fatal(created.Code, created.Body.String())
	}
	items, e := market.List(ctx, cred.Principal)
	if e != nil || len(items) != 1 {
		t.Fatal(items, e)
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
	for _, target := range []string{"/admin/market", "/admin/market/preview", "/admin/market/" + items[0].ID, "/admin/market/" + items[0].ID + "/refresh"} {
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
