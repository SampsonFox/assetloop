package web

import (
	"context"
	"net/http"
	"net/http/httptest"
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

// A free gift is a zero built-in purchase; every other cash-flow type keeps its
// positive amount requirement, and the form works without JavaScript.
func TestLifecycleFormsAcceptZeroPurchaseOnlyForGifts(t *testing.T) {
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
	// The purchase option carries its immutable system code, and the no-JavaScript
	// amount control already accepts zero for it.
	for _, want := range []string{`data-system-code="purchase"`, `pattern="(?:0*[0-9]+(?:[.][0-9]*)?|0*[.][0-9]+)"`, `data-nonnegative-pattern=`, `data-nonnegative-title=`, `data-positive-pattern=`} {
		if !strings.Contains(page.Body.String(), want) {
			t.Fatalf("purchase amount control missing %q: %s", want, page.Body.String())
		}
	}
	post := func(values url.Values) *httptest.ResponseRecorder {
		t.Helper()
		values.Set("csrf_token", csrf.Value)
		return request(t, handler, http.MethodPost, "/assets/"+asset.ID+"/events", values, cookies)
	}
	base := url.Values{"event_type": {"purchase"}, "amount": {"0"}, "currency": {"CNY"}, "occurred_at": {"2026-08-26T12:00"}, "source": {"manual"}, "notes": {"free gift"}}
	base.Set("request_key", keyFrom(page.Body.String()))
	if gift := post(base); gift.Code != http.StatusSeeOther {
		t.Fatalf("zero purchase: %d %s", gift.Code, gift.Body.String())
	}
	repair := url.Values{"request_key": {"web-zero-repair"}, "event_type": {"repair"}, "amount": {"0"}, "currency": {"CNY"}, "occurred_at": {"2026-08-26T12:00"}, "source": {"manual"}}
	if rejected := post(repair); rejected.Code != http.StatusUnprocessableEntity || !strings.Contains(rejected.Body.String(), "金额必须大于 0") {
		t.Fatalf("zero repair: %d %s", rejected.Code, rejected.Body.String())
	}
	// A second distinct purchase on the same item stays valid.
	next := request(t, handler, http.MethodGet, "/assets/"+asset.ID, nil, cookies)
	second := url.Values{"request_key": {keyFrom(next.Body.String())}, "event_type": {"purchase"}, "amount": {"19.99"}, "currency": {"CNY"}, "occurred_at": {"2026-08-26T12:30"}, "source": {"manual"}, "notes": {"screen protection service"}}
	if response := post(second); response.Code != http.StatusSeeOther {
		t.Fatalf("second purchase: %d %s", response.Code, response.Body.String())
	}
	events, summary, err := lifecycle.Timeline(ctx, owner, asset.ID)
	if err != nil || len(events) != 2 || summary.ExpenseMinor != 1999 || summary.Status != "active" {
		t.Fatalf("gift lifecycle mismatch: %d %+v %v", len(events), summary, err)
	}
}
