package storetest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/google/uuid"
)

// RunModelResources exercises the same invariants over separate connections in both dialects.
func RunModelResources(t *testing.T, first, second Store, db *sql.DB, driver string) {
	t.Helper()
	ctx := context.Background()
	actor, err := first.FirstPrincipal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cat := application.NewCatalogService(first)
	category, err := cat.CreateCategory(ctx, actor, application.CreateCategory{Name: "3D conformance"})
	if err != nil {
		t.Fatal(err)
	}
	model, err := cat.CreateModel(ctx, actor, application.CreateModel{CategoryID: category.ID, Name: "3D model"})
	if err != nil {
		t.Fatal(err)
	}
	specs := application.NewSpecificationService(first)
	color, err := specs.SaveType(ctx, actor, application.SaveSpecificationType{Name: "Body color", Enabled: true, AffectsAppearance: true})
	if err != nil {
		t.Fatal(err)
	}
	red, err := specs.SaveTag(ctx, actor, application.SaveSpecificationTag{TypeID: color.ID, Name: " Red ", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	blue, err := specs.SaveTag(ctx, actor, application.SaveSpecificationTag{TypeID: color.ID, Name: "Blue", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if red.Name != "Red" {
		t.Fatalf("color not normalized: %q", red.Name)
	}
	if _, err := specs.SaveTag(ctx, actor, application.SaveSpecificationTag{TypeID: color.ID, Name: "Red", Enabled: true}); err == nil {
		t.Fatal("duplicate tag identity accepted")
	}
	if err := specs.SaveModel(ctx, actor, application.SaveModelSpecification{ModelID: model.ID, TagIDs: []string{red.ID, blue.ID}}); err != nil {
		t.Fatal(err)
	}
	asset, err := specs.SaveAsset(ctx, actor, application.SaveSpecificationAsset{ModelID: model.ID, TagIDs: []string{red.ID}, DisplayName: "Colored"})
	if err != nil {
		t.Fatal(err)
	}
	if len(asset.Tags) != 1 || asset.Tags[0].Name != "Red" {
		t.Fatalf("asset tags=%+v", asset.Tags)
	}
	asset, err = specs.SaveAsset(ctx, actor, application.SaveSpecificationAsset{ID: asset.ID, ModelID: model.ID, TagIDs: []string{blue.ID}, DisplayName: "Colored"})
	if err != nil || len(asset.Tags) != 1 || asset.Tags[0].Name != "Blue" {
		t.Fatalf("selected color not hydrated: %+v %v", asset, err)
	}
	create := func(name string) domain.Model3DResource {
		t.Helper()
		id := uuid.NewString()
		now := time.Now().UTC().Truncate(time.Microsecond)
		r := domain.Model3DResource{ID: id, TenantID: actor.TenantID, Name: name, Status: "ready", CreatedAt: now, ProductModel3D: domain.ProductModel3D{ResourceID: id, StoreID: "local", ObjectKey: "tenants/" + actor.TenantID + "/model-3d-resources/" + id + "/file.glb", SHA256: strings.Repeat("a", 64), SizeBytes: 42, UpdatedAt: now}}
		if err := first.CreateModel3DResource(ctx, r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	resource := create("Conformance resource")
	for _, b := range []application.BindModel3DResource{{Kind: "model", TargetID: model.ID}, {Kind: "asset", TargetID: asset.ID}} {
		b.ResourceID = resource.ID
		if err := first.BindModel3DResource(ctx, actor.TenantID, b); err != nil {
			t.Fatal(err)
		}
	}
	rule, err := specs.SaveAppearance(ctx, actor, application.SaveAppearanceDefault{ModelID: model.ID, ResourceID: resource.ID, TagIDs: []string{red.ID}})
	if err != nil {
		t.Fatal(err)
	}
	refs, err := first.Model3DReferences(ctx, actor.TenantID, resource.ID)
	if err != nil || len(refs) != 3 {
		t.Fatalf("references=%+v err=%v", refs, err)
	}
	for _, ref := range refs {
		if ref.Kind == "appearance" && (ref.ID != rule.ID || ref.Name != model.Name) {
			t.Fatalf("appearance reference ambiguous: %+v", ref)
		}
	}
	page, err := first.ListModel3DResources(ctx, actor.TenantID, application.Model3DResourceListOptions{Query: "Conformance", Page: 1, PageSize: 1})
	if err != nil || page.Total != 1 || len(page.Resources) != 1 || page.Resources[0].ReferenceCount != 3 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	page, err = first.ListModel3DResources(ctx, actor.TenantID, application.Model3DResourceListOptions{Query: "Conformance", Page: 2, PageSize: 1})
	if err != nil || page.Total != 1 || len(page.Resources) != 0 {
		t.Fatalf("out-of-range page=%+v err=%v", page, err)
	}
	if err := first.MarkModel3DResourcePendingDelete(ctx, actor.TenantID, resource.ID); !errors.Is(err, application.ErrModel3DReferenced) {
		t.Fatalf("referenced deletion: %v", err)
	}
	if err := first.FinishModel3DResourceDelete(ctx, actor.TenantID, resource.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := first.GetModel3DResource(ctx, actor.TenantID, resource.ID); err != nil {
		t.Fatal("ready resource removed")
	}
	otherTenant := uuid.NewString()
	if _, err := second.GetModel3DResource(ctx, otherTenant, resource.ID); err == nil {
		t.Fatal("cross-tenant read")
	}
	if err := second.BindModel3DResource(ctx, otherTenant, application.BindModel3DResource{Kind: "model", TargetID: model.ID, ResourceID: resource.ID}); err == nil {
		t.Fatal("cross-tenant target bind")
	}
	// A different tenant's existing resource must also be rejected, not only a missing target.
	q := "?"
	if driver == "postgres" {
		q = "$1"
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO tenants(id,name,created_at) VALUES ("+q+",'other','2026-09-05T00:00:00Z')", otherTenant); err != nil {
		t.Fatal(err)
	}
	foreign := resource
	foreign.ID = uuid.NewString()
	foreign.TenantID = otherTenant
	foreign.ObjectKey = "tenants/" + otherTenant + "/foreign.glb"
	if err := first.CreateModel3DResource(ctx, foreign); err != nil {
		t.Fatal(err)
	}
	if err := first.BindModel3DResource(ctx, actor.TenantID, application.BindModel3DResource{Kind: "model", TargetID: model.ID, ResourceID: foreign.ID}); err == nil {
		t.Fatal("cross-tenant resource bind")
	}
	if _, err := db.ExecContext(ctx, "UPDATE model_3d_resources SET object_key='changed' WHERE id="+q, resource.ID); err == nil {
		t.Fatal("immutable bytes changed")
	}
	changed := resource
	changed.Name = "Renamed"
	changed.Author = "Shared author"
	changed.SHA256 = "ignored"
	changed.ObjectKey = "ignored"
	if err := first.UpdateModel3DResource(ctx, changed); err != nil {
		t.Fatal(err)
	}
	got, err := second.GetModel3DResource(ctx, actor.TenantID, resource.ID)
	if err != nil || got.ObjectKey != resource.ObjectKey || got.SHA256 != resource.SHA256 || got.Author != "Shared author" {
		t.Fatalf("metadata update=%+v %v", got, err)
	}
	binding, err := specs.EffectiveForAsset(ctx, actor, asset.ID)
	if err != nil || binding.Source != "asset" || binding.Resource == nil || binding.Resource.ID != resource.ID {
		t.Fatalf("binding=%+v %v", binding, err)
	}
	// A failed target bind rolls back the just-created resource.
	candidate := resource
	candidate.ID = uuid.NewString()
	candidate.ResourceID = candidate.ID
	candidate.ObjectKey = "tenants/" + actor.TenantID + "/" + candidate.ID + ".glb"
	if err := first.CreateAndBindModel3DResource(ctx, candidate, application.BindModel3DResource{Kind: "model", TargetID: uuid.NewString()}); err == nil {
		t.Fatal("missing target accepted")
	}
	if _, err := first.GetModel3DResource(ctx, actor.TenantID, candidate.ID); !errors.Is(err, application.ErrModel3DNotFound) {
		t.Fatalf("failed binding persisted resource: %v", err)
	}
	for _, b := range []application.BindModel3DResource{{Kind: "model", TargetID: model.ID}, {Kind: "asset", TargetID: asset.ID}} {
		if err := first.BindModel3DResource(ctx, actor.TenantID, b); err != nil {
			t.Fatal(err)
		}
	}
	if err := specs.DeleteAppearance(ctx, actor, rule.ID); err != nil {
		t.Fatal(err)
	}
	if err := first.MarkModel3DResourcePendingDelete(ctx, actor.TenantID, resource.ID); err != nil {
		t.Fatal(err)
	}
	if err := first.MarkModel3DResourcePendingDelete(ctx, actor.TenantID, resource.ID); err != nil {
		t.Fatal("pending retry:", err)
	}
	if err := first.BindModel3DResource(ctx, actor.TenantID, application.BindModel3DResource{Kind: "model", TargetID: model.ID, ResourceID: resource.ID}); err == nil {
		t.Fatal("bound pending deletion")
	}
	if _, err := db.ExecContext(ctx, "UPDATE model_3d_resources SET status='ready' WHERE id="+q, resource.ID); err == nil {
		t.Fatal("revived pending deletion")
	}
	if err := first.FinishModel3DResourceDelete(ctx, actor.TenantID, resource.ID); err != nil {
		t.Fatal(err)
	}
	if err := second.FinishModel3DResourceDelete(ctx, actor.TenantID, resource.ID); err != nil {
		t.Fatal("finish retry:", err)
	}
	for i := 0; i < 12; i++ {
		r := create(fmt.Sprintf("race %d", i))
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		var bindErr, deleteErr error
		go func() {
			defer wg.Done()
			<-start
			bindErr = first.BindModel3DResource(ctx, actor.TenantID, application.BindModel3DResource{Kind: "model", TargetID: model.ID, ResourceID: r.ID})
		}()
		go func() {
			defer wg.Done()
			<-start
			deleteErr = second.MarkModel3DResourcePendingDelete(ctx, actor.TenantID, r.ID)
		}()
		close(start)
		wg.Wait()
		if (bindErr == nil) == (deleteErr == nil) {
			t.Fatalf("race must have one winner: bind=%v delete=%v", bindErr, deleteErr)
		}
		got, err := first.GetModel3DResource(ctx, actor.TenantID, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		if bindErr == nil {
			if got.Status != "ready" {
				t.Fatal("bound resource pending")
			}
			if err := first.BindModel3DResource(ctx, actor.TenantID, application.BindModel3DResource{Kind: "model", TargetID: model.ID}); err != nil {
				t.Fatal(err)
			}
		} else if got.Status != "pending-delete" {
			t.Fatal("deletion winner still ready")
		}
	}
}
