package web

import (
	"context"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
)

func TestLifecycleFormsRetryWithoutDuplicateEvents(t *testing.T) {
	ctx := context.Background()
	cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "retry.db")}
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := basestore.Migrate(ctx, db, cfg); err != nil {
		t.Fatal(err)
	}
	adapter := sqlite.New(db)
	auth := application.NewAuthService(adapter)
	credential, err := auth.Setup(ctx, application.SetupAuth{TenantName: "Retry", BaseCurrency: "CNY", Username: "owner", Password: "owner secure password"})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.Authenticate(ctx, credential.Token)
	if err != nil {
		t.Fatal(err)
	}
	catalog := application.NewCatalogService(adapter)
	category, err := catalog.CreateCategory(ctx, owner, application.CreateCategory{Name: "Camera"})
	if err != nil {
		t.Fatal(err)
	}
	model, err := catalog.CreateModel(ctx, owner, application.CreateModel{CategoryID: category.ID, Name: "Camera model"})
	if err != nil {
		t.Fatal(err)
	}
	spec := application.NewSpecificationService(adapter)
	asset, err := spec.SaveAsset(ctx, owner, application.SaveSpecificationAsset{ModelID: model.ID, DisplayName: "Camera"})
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := application.NewLifecycleService(adapter)
	server, err := New(auth, catalog, lifecycle, db, Options{AuthMode: "local", Specifications: spec})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	session := &http.Cookie{Name: sessionCookie, Value: credential.Token}
	page := request(t, handler, http.MethodGet, "/assets/"+asset.ID, nil, []*http.Cookie{session})
	csrf := responseCookie(t, page, csrfCookie)
	cookies := []*http.Cookie{session, csrf}
	keyFrom := func(body string) string {
		t.Helper()
		match := regexp.MustCompile(`name="request_key" value="([^"]+)"`).FindStringSubmatch(body)
		if len(match) != 2 {
			t.Fatal("missing request key")
		}
		return match[1]
	}
	key := keyFrom(page.Body.String())
	form := url.Values{"csrf_token": {csrf.Value}, "request_key": {key}, "event_type": {"purchase"}, "currency": {"CNY"}, "amount": {"invalid"}, "occurred_at": {"2026-08-26T12:00"}, "source": {"manual"}}
	failed := request(t, handler, http.MethodPost, "/assets/"+asset.ID+"/events", form, cookies)
	if failed.Code != http.StatusUnprocessableEntity || keyFrom(failed.Body.String()) != key {
		t.Fatal("validation error lost retry key")
	}
	form.Set("amount", "100.00")
	for range 2 {
		response := request(t, handler, http.MethodPost, "/assets/"+asset.ID+"/events", form, cookies)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("purchase retry: %d %s", response.Code, response.Body.String())
		}
	}
	events, summary, err := lifecycle.Timeline(ctx, owner, asset.ID)
	if err != nil || len(events) != 1 || summary.ExpenseMinor != 10000 {
		t.Fatalf("duplicate purchase: %d %+v %v", len(events), summary, err)
	}
	form.Set("amount", "200.00")
	conflict := request(t, handler, http.MethodPost, "/assets/"+asset.ID+"/events", form, cookies)
	if conflict.Code != http.StatusUnprocessableEntity || !strings.Contains(conflict.Body.String(), "该请求已提交过其他内容") {
		t.Fatalf("request conflict not explained: %d", conflict.Code)
	}
	correctionPath := "/events/" + events[0].ID + "/correct"
	page = request(t, handler, http.MethodGet, correctionPath, nil, cookies)
	correctionKey := keyFrom(page.Body.String())
	if correctionKey == key {
		t.Fatal("different commands share a key")
	}
	form.Set("request_key", correctionKey)
	for range 2 {
		response := request(t, handler, http.MethodPost, correctionPath, form, cookies)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("correction retry: %d %s", response.Code, response.Body.String())
		}
	}
	events, summary, err = lifecycle.Timeline(ctx, owner, asset.ID)
	if err != nil || len(events) != 3 || summary.ExpenseMinor != 20000 {
		t.Fatalf("duplicate correction: %d %+v %v", len(events), summary, err)
	}
}

// A second built-in purchase is rejected, a purchase keeps its positive amount
// requirement, and a user-confirmed custom neutral type records the zero gift
// through the same no-JavaScript form.
func TestLifecycleFormsEnforceUniquePurchaseAndNeutralGift(t *testing.T) {
	ctx := context.Background()
	cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "gift.db")}
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := basestore.Migrate(ctx, db, cfg); err != nil {
		t.Fatal(err)
	}
	adapter := sqlite.New(db)
	auth := application.NewAuthService(adapter)
	credential, err := auth.Setup(ctx, application.SetupAuth{TenantName: "Gift", BaseCurrency: "CNY", Username: "owner", Password: "owner secure password"})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := auth.Authenticate(ctx, credential.Token)
	if err != nil {
		t.Fatal(err)
	}
	catalog := application.NewCatalogService(adapter)
	category, err := catalog.CreateCategory(ctx, owner, application.CreateCategory{Name: "Phone"})
	if err != nil {
		t.Fatal(err)
	}
	model, err := catalog.CreateModel(ctx, owner, application.CreateModel{CategoryID: category.ID, Name: "Gift model"})
	if err != nil {
		t.Fatal(err)
	}
	spec := application.NewSpecificationService(adapter)
	asset, err := spec.SaveAsset(ctx, owner, application.SaveSpecificationAsset{ModelID: model.ID, DisplayName: "Gifted phone"})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := spec.SaveAsset(ctx, owner, application.SaveSpecificationAsset{ModelID: model.ID, DisplayName: "Ungifted phone"})
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := application.NewLifecycleService(adapter)
	server, err := New(auth, catalog, lifecycle, db, Options{AuthMode: "local", Specifications: spec})
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	session := &http.Cookie{Name: sessionCookie, Value: credential.Token}
	page := request(t, handler, http.MethodGet, "/assets/"+asset.ID, nil, []*http.Cookie{session})
	csrf := responseCookie(t, page, csrfCookie)
	cookies := []*http.Cookie{session, csrf}
	keyFrom := func(body string) string {
		t.Helper()
		match := regexp.MustCompile(`name="request_key" value="([^"]+)"`).FindStringSubmatch(body)
		if len(match) != 2 {
			t.Fatal("missing request key")
		}
		return match[1]
	}
	// The no-JavaScript purchase amount control requires a positive magnitude and
	// states the same translated rule the server reports, punctuation included.
	amountTitle := `title="` + textFor(application.LocaleZhCN, "validation.amount_positive") + `"`
	for _, want := range []string{`data-positive-pattern=`, amountTitle, `pattern="(?:0*[1-9][0-9]*(?:[.][0-9]*)?|0*[.][0-9]*[1-9][0-9]*)"`} {
		if !strings.Contains(page.Body.String(), want) {
			t.Fatalf("purchase amount control missing %q: %s", want, page.Body.String())
		}
	}
	if strings.Contains(page.Body.String(), "data-system-code=") || strings.Contains(page.Body.String(), "data-nonnegative-pattern=") {
		t.Fatalf("purchase must not advertise a zero-amount policy: %s", page.Body.String())
	}
	purchase := url.Values{"request_key": {"web-purchase"}, "event_type": {"purchase"}, "amount": {"100.00"}, "currency": {"CNY"}, "occurred_at": {"2026-08-26T12:00"}, "source": {"manual"}, "notes": {"device"}}
	purchase.Set("csrf_token", csrf.Value)
	if response := request(t, handler, http.MethodPost, "/assets/"+asset.ID+"/events", purchase, cookies); response.Code != http.StatusSeeOther {
		t.Fatalf("first purchase: %d %s", response.Code, response.Body.String())
	}
	// The built-in purchase means acquiring the item, so it stays unique.
	second := request(t, handler, http.MethodGet, "/assets/"+asset.ID, nil, cookies)
	secondPurchase := url.Values{"csrf_token": {csrf.Value}, "request_key": {keyFrom(second.Body.String())}, "event_type": {"purchase"}, "amount": {"19.99"}, "currency": {"CNY"}, "occurred_at": {"2026-08-26T12:30"}, "source": {"manual"}}
	if response := request(t, handler, http.MethodPost, "/assets/"+asset.ID+"/events", secondPurchase, cookies); response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "物品已经存在有效买入记录") {
		t.Fatalf("second purchase: %d %s", response.Code, response.Body.String())
	}
	zeroPurchase := url.Values{"csrf_token": {csrf.Value}, "request_key": {"web-zero-purchase"}, "event_type": {"purchase"}, "amount": {"0"}, "currency": {"CNY"}, "occurred_at": {"2026-08-26T12:00"}, "source": {"manual"}}
	if response := request(t, handler, http.MethodPost, "/assets/"+empty.ID+"/events", zeroPurchase, cookies); response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), "金额必须大于 0") {
		t.Fatalf("zero purchase on a fresh item: %d %s", response.Code, response.Body.String())
	}
	// Purchased services and a free gift are reusable custom cost categories.
	created := url.Values{"csrf_token": {csrf.Value}, "asset_id": {asset.ID}, "name": {"赠品"}, "cashflow": {"neutral"}}
	createdType := request(t, handler, http.MethodPost, "/admin/event-types", created, cookies)
	location, err := url.Parse(createdType.Header().Get("Location"))
	if err != nil || createdType.Code != http.StatusSeeOther || location.Query().Get("event_type") == "" {
		t.Fatalf("create neutral type: %d %s", createdType.Code, createdType.Body.String())
	}
	gift := url.Values{"csrf_token": {csrf.Value}, "request_key": {"web-gift"}, "event_type": {location.Query().Get("event_type")}, "amount": {"0"}, "currency": {"CNY"}, "occurred_at": {"2026-08-27T12:00"}, "source": {"manual"}, "notes": {"free gift"}}
	if response := request(t, handler, http.MethodPost, "/assets/"+asset.ID+"/events", gift, cookies); response.Code != http.StatusSeeOther {
		t.Fatalf("neutral gift: %d %s", response.Code, response.Body.String())
	}
	events, summary, err := lifecycle.Timeline(ctx, owner, asset.ID)
	if err != nil || len(events) != 2 || summary.ExpenseMinor != 10000 || summary.Status != "active" || events[1].BaseAmountMinor != 0 {
		t.Fatalf("gift lifecycle mismatch: %d %+v %v", len(events), summary, err)
	}
}
