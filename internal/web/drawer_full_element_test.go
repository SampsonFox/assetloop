package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
	"github.com/SampsonFox/assetloop/internal/store/sqlite/sqlitedb"
	"github.com/google/uuid"
)

// TestDrawerFullElementSQLite exercises the production mux and negotiation,
// not a hand-wired test mux. Browser draft state is represented by an unsent form.
func TestDrawerFullElementSQLite(t *testing.T) {
	ctx := context.Background()
	cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "drawer-full-element.db")}
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := basestore.Migrate(ctx, db, cfg); err != nil {
		t.Fatal(err)
	}
	adapter := sqlite.New(db)
	auth := application.NewAuthService(adapter)
	spec := application.NewSpecificationService(adapter)
	lifecycle := application.NewLifecycleService(adapter)
	s, err := New(auth, application.NewCatalogService(adapter), lifecycle, db, Options{AuthMode: "local", Specifications: spec})
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	cookies, csrf := resourceSession(t, h)
	owner, err := auth.Authenticate(ctx, cookies[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	ownerCookies := cookies
	send := func(method, path, target string, form url.Values) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if target != "" {
			r.Header.Set("X-Assetloop-Drawer", target)
		}
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	status := func(w *httptest.ResponseRecorder, want int) {
		t.Helper()
		if w.Code != want {
			t.Fatalf("HTTP %d, want %d: %s", w.Code, want, w.Body.String())
		}
	}
	save := func(path, target, kind string, form url.Values) string {
		t.Helper()
		form.Set("csrf_token", csrf)
		w := send("POST", path, target, form)
		status(w, http.StatusOK)
		var result struct{ Kind, ID string }
		if !strings.Contains(w.Header().Get("Content-Type"), "application/json") || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Kind != kind || result.ID == "" {
			t.Fatalf("missing semantic saved result: %s", w.Body.String())
		}
		return result.ID
	}
	fragment := func(path, target string, wants ...string) {
		t.Helper()
		w := send("GET", path, target, nil)
		status(w, http.StatusOK)
		if strings.Contains(w.Body.String(), "<!doctype") || !strings.Contains(w.Body.String(), `id="`+target+`"`) || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("not a negotiated fragment: %s", w.Body.String())
		}
		for _, want := range wants {
			if !strings.Contains(w.Body.String(), want) {
				t.Fatalf("fragment missing %q: %s", want, w.Body.String())
			}
		}
	}

	kind := save("/admin/tags/types", "tag-editor", "tag-type", url.Values{"name": {"Drawer storage"}, "enabled": {"1"}})
	originalTag := save("/admin/tags/values", "tag-editor", "tag", url.Values{"type_id": {kind}, "name": {"128GB"}, "enabled": {"1"}})
	draftTag := save("/admin/tags/values", "tag-editor", "tag", url.Values{"type_id": {kind}, "name": {"256GB"}, "enabled": {"1"}})
	category := save("/admin/catalog/categories", "category-drawer", "category", url.Values{"name": {"Drawer phones"}, "icon_key": {"smartphone"}})
	model := save("/admin/catalog/models", "model-drawer", "model", url.Values{"category_id": {category}, "name": {"Drawer phone"}})
	modelForm := url.Values{"model_configuration": {"1"}, "category_id": {category}, "name": {"Drawer phone"}, "tag_ids": {originalTag, draftTag}, "appearance_" + kind: {"no"}}
	save("/admin/catalog/models/"+model, "model-drawer", "model", modelForm)
	original := save("/assets", "asset-editor", "asset", url.Values{"model_id": {model}, "tag_ids": {originalTag}, "display_name": {"Original phone"}, "serial_number": {"original-serial"}, "notes": {"Original notes"}})
	status(send("POST", "/assets/"+original+"/events", "", url.Values{"csrf_token": {csrf}, "request_key": {uuid.NewString()}, "event_type": {"purchase"}, "amount": {"123.45"}, "currency": {"CNY"}, "occurred_at": {time.Now().Add(-24 * time.Hour).Format("2006-01-02T15:04")}, "source": {"drawer full element"}}), http.StatusSeeOther)
	originalAsset, err := spec.Asset(ctx, owner, original)
	if err != nil {
		t.Fatal(err)
	}
	originalEvents, originalCost, err := lifecycle.Timeline(ctx, owner, original)
	if err != nil || len(originalEvents) != 1 || originalCost.ExpenseMinor != 12345 {
		t.Fatalf("missing nonzero cost baseline: %+v %v", originalCost, err)
	}
	unchanged := func() {
		t.Helper()
		asset, err := spec.Asset(ctx, owner, original)
		if err != nil || !reflect.DeepEqual(asset, originalAsset) {
			t.Fatalf("original asset changed: %+v %v", asset, err)
		}
		events, cost, err := lifecycle.Timeline(ctx, owner, original)
		if err != nil || !reflect.DeepEqual(events, originalEvents) || cost != originalCost {
			t.Fatalf("original lifecycle/cost changed: %+v %v", cost, err)
		}
	}

	parentURL := "/assets/new?model_id=" + model + "&tag_ids=" + draftTag
	fragment(parentURL, "asset-editor", `data-tag-name="256GB"`, `form="asset-form"`)
	fragment("/admin/catalog?dialog=model-drawer&edit_model_id="+model, "model-drawer", `value="Drawer phone"`, draftTag)
	fragment("/admin/tags?view=types&edit="+kind, "tag-editor", `value="Drawer storage"`)
	fragment("/admin/tags?edit="+draftTag, "tag-editor", `value="256GB"`)
	fragment("/assets/"+original, "asset-detail", "Original phone", "original-serial", "Original notes")
	fullDetail := send("GET", "/assets/"+original, "", nil)
	status(fullDetail, http.StatusOK)
	if !strings.Contains(fullDetail.Body.String(), "123.45") || !strings.Contains(fullDetail.Body.String(), `data-cost-dashboard`) {
		t.Fatal("ordinary asset page lost its recorded cost dashboard")
	}
	// These controls stay client-side while the child request commits independently.
	parentDraft := url.Values{"model_id": {model}, "tag_ids": {draftTag}, "display_name": {"Parent unsaved phone"}, "serial_number": {"parent-serial"}, "purchase_channel": {"parent shop"}, "notes": {"Parent draft must survive child save"}}
	rename := url.Values{"type_id": {kind}, "name": {"512GB"}, "enabled": {"1"}, "csrf_token": {csrf}}
	status(send("POST", "/admin/tags/values/"+draftTag, "tag-editor", rename), http.StatusUnprocessableEntity)
	rename.Set("confirm_rename", "1")
	if save("/admin/tags/values/"+draftTag, "tag-editor", "tag", rename) != draftTag {
		t.Fatal("rename replaced the tag identity")
	}
	unchanged()
	fragment(parentURL, "asset-editor", `data-tag-name="512GB"`)
	parent := save("/assets", "asset-editor", "asset", parentDraft)
	if parent == original {
		t.Fatal("parent draft overwrote original asset")
	}
	saved, err := spec.Asset(ctx, owner, parent)
	if err != nil || saved.DisplayName != parentDraft.Get("display_name") || saved.SerialNumber != "parent-serial" || saved.PurchaseChannel != "parent shop" || saved.Notes != parentDraft.Get("notes") || len(saved.Tags) != 1 || saved.Tags[0].ID != draftTag || saved.Tags[0].Name != "512GB" {
		t.Fatalf("parent draft lost fields or child identity: %+v %v", saved, err)
	}
	unchanged()

	t.Run("CSRF and viewer cannot mutate", func(t *testing.T) {
		noCSRF := url.Values{"model_id": {model}, "display_name": {"Rejected"}}
		status(send("POST", "/assets/"+original, "asset-editor", noCSRF), http.StatusForbidden)
		status(send("POST", "/admin/tags/values/"+draftTag, "tag-editor", url.Values{"name": {"Rejected"}}), http.StatusForbidden)
		status(send("POST", "/admin/catalog/models/"+model, "model-drawer", url.Values{"name": {"Rejected"}}), http.StatusForbidden)
		status(request(t, h, "POST", "/admin/members", url.Values{"csrf_token": {csrf}, "username": {"drawer-scenario-viewer"}, "password": {"scenario viewer password"}, "role": {"viewer"}}, cookies), http.StatusSeeOther)
		login := request(t, h, "POST", "/login", url.Values{"csrf_token": {csrf}, "username": {"drawer-scenario-viewer"}, "password": {"scenario viewer password"}}, cookies)
		status(login, http.StatusSeeOther)
		cookies = []*http.Cookie{responseCookie(t, login, sessionCookie), ownerCookies[1]}
		defer func() { cookies = ownerCookies }()
		fragment("/assets/"+original, "asset-detail", "Original phone")
		status(send("GET", "/assets/"+original+"/edit", "asset-editor", nil), http.StatusForbidden)
		status(send("POST", "/assets/"+original, "asset-editor", parentDraft), http.StatusForbidden)
		status(send("POST", "/admin/tags/values/"+draftTag, "tag-editor", rename), http.StatusForbidden)
		status(send("POST", "/admin/catalog/models/"+model, "model-drawer", modelForm), http.StatusForbidden)
		unchanged()
	})

	t.Run("cross tenant cannot read or write guessed identifiers", func(t *testing.T) {
		// No tenant-creation HTTP use case exists. Seed the second tenant and
		// its bootstrap membership; member creation/login and tested requests are real.
		other := owner
		other.TenantID = uuid.NewString()
		if err := sqlitedb.New(db).CreateTenant(ctx, sqlitedb.CreateTenantParams{ID: other.TenantID, Name: "Other tenant", BaseCurrency: "CNY", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
			t.Fatal(err)
		}
		if err := sqlitedb.New(db).CreateMembership(ctx, sqlitedb.CreateMembershipParams{TenantID: other.TenantID, UserID: other.UserID, Role: "owner", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
			t.Fatal(err)
		}
		if _, err := auth.AddMember(ctx, other, application.AddMember{Username: "drawer-other-owner", Password: "other tenant password", Role: application.RoleOwner}); err != nil {
			t.Fatal(err)
		}
		login := request(t, h, "POST", "/login", url.Values{"csrf_token": {csrf}, "username": {"drawer-other-owner"}, "password": {"other tenant password"}}, ownerCookies)
		status(login, http.StatusSeeOther)
		cookies = []*http.Cookie{responseCookie(t, login, sessionCookie), ownerCookies[1]}
		defer func() { cookies = ownerCookies }()
		for _, item := range []struct{ path, target string }{
			{"/assets/" + original + "/edit", "asset-editor"},
			{"/assets/" + original, "asset-detail"},
			{"/admin/catalog?edit_model_id=" + model, "model-drawer"},
			{"/admin/tags?edit=" + draftTag, "tag-editor"},
		} {
			w := send("GET", item.path, item.target, nil)
			status(w, http.StatusNotFound)
			if strings.Contains(w.Body.String(), "Original phone") || strings.Contains(w.Body.String(), "512GB") {
				t.Fatal("cross-tenant fragment leaked data")
			}
		}
		for _, item := range []struct {
			path, target string
			form         url.Values
		}{
			{"/assets/" + original, "asset-editor", parentDraft},
			{"/assets", "asset-editor", parentDraft},
			{"/admin/catalog/models/" + model, "model-drawer", modelForm},
			{"/admin/tags/values/" + draftTag, "tag-editor", rename},
		} {
			w := send("POST", item.path, item.target, item.form)
			if w.Code != http.StatusNotFound && w.Code != http.StatusUnprocessableEntity && w.Code != http.StatusForbidden {
				t.Fatalf("cross-tenant write accepted: %s: %d", item.path, w.Code)
			}
		}
		unchanged()
	})
}
