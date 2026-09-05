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
	variant, err := catalog.CreateVariant(ctx, owner, application.CreateVariant{ModelID: model.ID, Name: "Body"})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := catalog.CreateAsset(ctx, owner, application.CreateCatalogAsset{VariantID: variant.ID, DisplayName: "Camera"})
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := application.NewLifecycleService(adapter)
	server, err := New(auth, catalog, lifecycle, db, Options{AuthMode: "local"})
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
