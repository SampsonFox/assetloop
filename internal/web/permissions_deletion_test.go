package web

import (
	"context"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
)

func TestRolePagesAndConfirmedDeletion(t *testing.T) {
	ctx := context.Background()
	cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "roles.db")}
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := basestore.Migrate(ctx, db, cfg); err != nil {
		t.Fatal(err)
	}
	st := sqlite.New(db)
	auth := application.NewAuthService(st)
	cat := application.NewCatalogService(st)
	spec := application.NewSpecificationService(st)
	s, err := New(auth, cat, application.NewLifecycleService(st), db, Options{AuthMode: "local", Specifications: spec})
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	adminCookies, csrf := resourceSession(t, h)
	admin, err := auth.Authenticate(ctx, adminCookies[0].Value)
	if err != nil {
		t.Fatal(err)
	}
	category, err := cat.CreateCategory(ctx, admin, application.CreateCategory{Name: "Test type"})
	if err != nil {
		t.Fatal(err)
	}
	model, err := cat.CreateModel(ctx, admin, application.CreateModel{CategoryID: category.ID, Name: "Test model"})
	if err != nil {
		t.Fatal(err)
	}
	asset, err := spec.SaveAsset(ctx, admin, application.SaveSpecificationAsset{ModelID: model.ID, DisplayName: "Delete test only"})
	if err != nil {
		t.Fatal(err)
	}
	path := "/assets/" + asset.ID
	for _, role := range []application.Role{application.RoleViewer, application.RoleEditor, application.RoleOwner} {
		t.Run(string(role), func(t *testing.T) {
			member, err := auth.AddMember(ctx, admin, application.AddMember{Username: "role-" + string(role), Password: "role test secure password", Role: role})
			if err != nil {
				t.Fatal(err)
			}
			login, err := auth.Login(ctx, application.Login{Username: member.Username, Password: "role test secure password"})
			if err != nil {
				t.Fatal(err)
			}
			cookies := []*http.Cookie{{Name: sessionCookie, Value: login.Token}, adminCookies[1]}
			body := request(t, h, "GET", path, nil, cookies).Body.String()
			if strings.Contains(body, textFor(admin.Locale, "asset.delete_warning")) || strings.Contains(body, "<h2>"+textFor(admin.Locale, "asset.delete")+"</h2>") {
				t.Fatal("item detail should show only the delete button; explanation belongs on confirmation page")
			}
			for _, check := range []struct {
				needle string
				want   bool
			}{{`href="/settings"`, role == application.RoleOwner}, {`href="` + path + `/edit"`, role != application.RoleViewer}, {`href="` + path + `/delete"`, role == application.RoleOwner}, {`data-event-type-create`, role == application.RoleOwner}} {
				if strings.Contains(body, check.needle) != check.want {
					t.Errorf("control %s visibility for %s", check.needle, role)
				}
			}
			for _, target := range []string{"/settings", "/admin/catalog", "/admin/tags", "/admin/3d", "/admin/event-types", "/admin/market", "/admin/members", path + "/delete"} {
				page := request(t, h, "GET", target, nil, cookies)
				if role != application.RoleOwner && page.Code != 403 {
					t.Fatalf("%s exposed: %d", target, page.Code)
				}
			}
			edit := request(t, h, "GET", path+"/edit", nil, cookies)
			if role == application.RoleViewer {
				if edit.Code != 403 {
					t.Fatal("viewer edit page")
				}
			} else {
				if edit.Code != 200 {
					t.Fatalf("editor page %d", edit.Code)
				}
				if role == application.RoleEditor && strings.Contains(edit.Body.String(), `href="/admin/`) {
					t.Fatal("worker sees shared management link")
				}
				save := request(t, h, "POST", path, url.Values{"csrf_token": {csrf}, "model_id": {model.ID}, "display_name": {"Edited by " + string(role)}}, cookies)
				if save.Code != 303 {
					t.Fatalf("item edit %d", save.Code)
				}
			}
			if role != application.RoleOwner {
				r := request(t, h, "POST", path+"/delete", url.Values{"csrf_token": {csrf}, "confirm_delete": {"yes"}}, cookies)
				if r.Code != 403 {
					t.Fatal("unauthorized delete")
				}
			}
			if role == application.RoleEditor {
				if err := auth.ChangeMemberRole(ctx, admin, member.UserID, application.RoleViewer); err != nil {
					t.Fatal(err)
				}
				if r := request(t, h, "GET", path+"/edit", nil, cookies); r.Code != 403 {
					t.Fatal("old session ignores role change")
				}
			}
		})
	}
	confirmation := request(t, h, "GET", path+"/delete", nil, adminCookies)
	if confirmation.Code != 200 || !strings.Contains(confirmation.Body.String(), `name="confirm_delete"`) {
		t.Fatal("missing confirmation")
	}
	if r := request(t, h, "POST", path+"/delete", url.Values{"csrf_token": {csrf}}, adminCookies); r.Code != 400 {
		t.Fatal("unconfirmed deletion accepted")
	}
	if r := request(t, h, "POST", path+"/delete", url.Values{"confirm_delete": {"yes"}}, adminCookies); r.Code != 403 {
		t.Fatal("deletion without CSRF accepted")
	}
	if _, err := spec.Asset(ctx, admin, asset.ID); err != nil {
		t.Fatal("GET or rejected POST deleted item")
	}
	for i := 0; i < 2; i++ {
		r := request(t, h, "POST", path+"/delete", url.Values{"csrf_token": {csrf}, "confirm_delete": {"yes"}}, adminCookies)
		if r.Code != 303 || r.Header().Get("Location") != "/" {
			t.Fatalf("delete %d %s", r.Code, r.Body.String())
		}
	}
	if r := request(t, h, "GET", path, nil, adminCookies); r.Code != 404 {
		t.Fatal("deleted item still shown")
	}
}
