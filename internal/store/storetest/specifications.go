package storetest

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/google/uuid"
)

// RunSpecifications uses real application commands and independent connections
// for both supported adapters. It creates no legacy variants.
func RunSpecifications(t *testing.T, first, second Store) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	actor, err := first.FirstPrincipal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	svc := application.NewSpecificationService(first)
	reader := application.NewSpecificationService(second)
	catalog := application.NewCatalogService(first)
	category, err := catalog.CreateCategory(ctx, actor, application.CreateCategory{Name: "Tag conformance"})
	if err != nil {
		t.Fatal(err)
	}
	model, err := catalog.CreateModel(ctx, actor, application.CreateModel{CategoryID: category.ID, Name: "Tagged phone"})
	if err != nil {
		t.Fatal(err)
	}
	typeOf := func(name string, appearance bool) domain.SpecificationTagType {
		t.Helper()
		v, err := svc.SaveType(ctx, actor, application.SaveSpecificationType{Name: name, Enabled: true, AffectsAppearance: appearance})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	tagOf := func(kind domain.SpecificationTagType, name string) domain.SpecificationTag {
		t.Helper()
		v, err := svc.SaveTag(ctx, actor, application.SaveSpecificationTag{TypeID: kind.ID, Name: name, Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	color := typeOf("Test color", true)
	storage := typeOf("Test storage", false)
	memory := typeOf("Test memory", false)
	finish := typeOf("Test finish", true)
	black, white := tagOf(color, "Black"), tagOf(color, "White")
	small, large := tagOf(storage, "128GB"), tagOf(storage, "256GB")
	_ = tagOf(memory, "128GB") // Same label in a different dimension is valid.
	matte := tagOf(finish, "Matte")
	t.Run("dictionary database filtering and pagination", func(t *testing.T) {
		literal := tagOf(memory, "Écran %_ test")
		for _, status := range []string{"", "all", "enabled"} {
			page, err := reader.ListTypes(ctx, actor, application.SpecificationListOptions{Query: " TEST ", Status: status, Page: 2, PageSize: 2})
			if err != nil || page.Total != 4 || len(page.Types) != 2 || page.Types[0].ID != memory.ID || page.Types[1].ID != storage.ID {
				t.Fatalf("type page status %q: %+v %v", status, page, err)
			}
			values, err := reader.ListTags(ctx, actor, application.SpecificationListOptions{Query: " ÉCRAN %_ ", Status: status, TypeID: memory.ID})
			if err != nil || values.Total != 1 || len(values.Tags) != 1 || values.Tags[0].ID != literal.ID {
				t.Fatalf("literal normalized value search: %+v %v", values, err)
			}
		}
		page, err := reader.ListTags(ctx, actor, application.SpecificationListOptions{TypeID: storage.ID, Page: 2, PageSize: 1})
		if err != nil || page.Total != 2 || len(page.Tags) != 1 || page.Tags[0].ID != large.ID {
			t.Fatalf("value page: %+v %v", page, err)
		}
		if _, err := svc.SaveTag(ctx, actor, application.SaveSpecificationTag{ID: literal.ID, TypeID: memory.ID, Name: literal.Name, Enabled: false}); err != nil {
			t.Fatal(err)
		}
		disabled, err := reader.ListTags(ctx, actor, application.SpecificationListOptions{Status: "disabled", TypeID: memory.ID})
		if err != nil || disabled.Total != 1 || len(disabled.Tags) != 1 || disabled.Tags[0].ID != literal.ID {
			t.Fatalf("disabled value page: %+v %v", disabled, err)
		}
		stranger := actor
		stranger.TenantID = uuid.NewString()
		foreign, err := reader.ListTags(ctx, stranger, application.SpecificationListOptions{})
		if err != nil || foreign.Total != 0 || len(foreign.Tags) != 0 {
			t.Fatalf("cross-space dictionary leak: %+v %v", foreign, err)
		}
	})
	if _, err := svc.SaveTag(ctx, actor, application.SaveSpecificationTag{TypeID: color.ID, Name: " BLACK ", Enabled: true}); err == nil {
		t.Fatal("normalized duplicate accepted")
	}
	allowed := []string{black.ID, white.ID, small.ID, large.ID, matte.ID}
	if err := svc.SaveModel(ctx, actor, application.SaveModelSpecification{ModelID: model.ID, TagIDs: allowed}); err != nil {
		t.Fatal(err)
	}
	makeAsset := func(name string, tags ...string) domain.Asset {
		t.Helper()
		a, err := svc.SaveAsset(ctx, actor, application.SaveSpecificationAsset{ModelID: model.ID, DisplayName: name, TagIDs: tags})
		if err != nil {
			t.Fatal(err)
		}
		stored, err := second.GetAsset(ctx, actor.TenantID, a.ID)
		if err != nil || stored.ModelID != model.ID {
			t.Fatalf("direct model read: %+v %v", stored, err)
		}
		return a
	}
	one := makeAsset("128 black", black.ID, small.ID, matte.ID)
	two := makeAsset("256 black", black.ID, large.ID)
	empty := makeAsset("No optional tags")
	// This replaces the retired implicit category/model/variant creation test:
	// reads still preserve every supported field and isolate the owning space.
	loaded, err := reader.Asset(ctx, actor, one.ID)
	// PostgreSQL persists timestamps at microsecond precision; compare the
	// common precision while still checking every other field exactly below.
	if err != nil || !loaded.CreatedAt.Truncate(time.Microsecond).Equal(one.CreatedAt.Truncate(time.Microsecond)) {
		t.Fatalf("tagged asset read time: %+v %v", loaded, err)
	}
	loaded.CreatedAt = one.CreatedAt
	if !reflect.DeepEqual(loaded, one) || one.ModelID != two.ModelID || one.CategoryID != two.CategoryID {
		t.Fatalf("tagged asset round trip/reused identity: %+v != %+v", loaded, one)
	}
	if _, err := second.GetAsset(ctx, uuid.NewString(), one.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("cross-space asset read: %v", err)
	}
	if len(one.Tags) != 3 || len(empty.Tags) != 0 {
		t.Fatal("selection hydration failed")
	}
	if _, err := svc.SaveAsset(ctx, actor, application.SaveSpecificationAsset{ModelID: model.ID, DisplayName: "Invalid", TagIDs: []string{small.ID, large.ID}}); err == nil {
		t.Fatal("two single-choice values accepted")
	}
	if _, err := svc.SaveAsset(ctx, actor, application.SaveSpecificationAsset{DisplayName: "Missing direct model"}); err == nil {
		t.Fatal("asset without a direct model accepted")
	}
	if _, err := svc.SaveAsset(ctx, actor, application.SaveSpecificationAsset{ModelID: model.ID, DisplayName: "Unknown", TagIDs: []string{uuid.NewString()}}); err == nil {
		t.Fatal("unknown tag accepted")
	}
	var refs application.SpecificationInUseError
	err = svc.SaveModel(ctx, actor, application.SaveModelSpecification{ModelID: model.ID, TagIDs: []string{white.ID}})
	if !errors.As(err, &refs) || len(refs.References) == 0 {
		t.Fatalf("missing references on shrink: %v", err)
	}
	if _, err := svc.SaveTag(ctx, actor, application.SaveSpecificationTag{ID: black.ID, TypeID: color.ID, Name: "Black corrected", Enabled: true}); err == nil {
		t.Fatal("unconfirmed shared rename accepted")
	}
	black, err = svc.SaveTag(ctx, actor, application.SaveSpecificationTag{ID: black.ID, TypeID: color.ID, Name: "Black corrected", Enabled: false, ConfirmSharedRename: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveAsset(ctx, actor, application.SaveSpecificationAsset{ID: one.ID, ModelID: model.ID, DisplayName: one.DisplayName, TagIDs: []string{black.ID, small.ID, matte.ID}}); err != nil {
		t.Fatalf("cannot retain disabled tag: %v", err)
	}
	if _, err := svc.SaveAsset(ctx, actor, application.SaveSpecificationAsset{ModelID: model.ID, DisplayName: "New disabled", TagIDs: []string{black.ID}}); err == nil {
		t.Fatal("new disabled selection accepted")
	}
	black, err = svc.SaveTag(ctx, actor, application.SaveSpecificationTag{ID: black.ID, TypeID: color.ID, Name: black.Name, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	resource := func(name string, tags ...string) domain.Model3DResource {
		t.Helper()
		id, now := uuid.NewString(), time.Now().UTC()
		r := domain.Model3DResource{ID: id, TenantID: actor.TenantID, Name: name, Status: "ready", CreatedAt: now, ProductModel3D: domain.ProductModel3D{ResourceID: id, StoreID: "local", ObjectKey: "tenants/" + actor.TenantID + "/model-3d-resources/" + id + "/file.glb", SHA256: strings.Repeat("b", 64), SizeBytes: 42, UpdatedAt: now}}
		if err := first.CreateModel3DResource(ctx, r); err != nil {
			t.Fatal(err)
		}
		if err := svc.SaveResource(ctx, actor, application.SaveResourceSpecification{ResourceID: id, TagIDs: tags, CategoryIDs: []string{category.ID}}); err != nil {
			t.Fatal(err)
		}
		return r
	}
	base := resource("Generic")
	blackGLB := resource("Black GLB", black.ID)
	whiteGLB := resource("White GLB", white.ID)
	matteGLB := resource("Matte GLB", matte.ID)
	if err := first.BindModel3DResource(ctx, actor.TenantID, application.BindModel3DResource{Kind: "model", TargetID: model.ID, ResourceID: base.ID}); err != nil {
		t.Fatal(err)
	}
	blackRule, err := svc.SaveAppearance(ctx, actor, application.SaveAppearanceDefault{ModelID: model.ID, ResourceID: blackGLB.ID, TagIDs: []string{black.ID}})
	if err != nil {
		t.Fatal(err)
	}
	assertEffective := func(asset, resource, source string, conflict bool) {
		t.Helper()
		got, err := reader.EffectiveForAsset(ctx, actor, asset)
		if err != nil || got.Resource == nil || got.Resource.ID != resource || got.Source != source || got.Conflict != conflict {
			t.Fatalf("effective: %+v err=%v want %s/%s conflict=%v", got, err, resource, source, conflict)
		}
	}
	assertEffective(one.ID, blackGLB.ID, "appearance", false)
	assertEffective(two.ID, blackGLB.ID, "appearance", false)
	assertEffective(empty.ID, base.ID, "model", false)
	candidates, err := reader.Candidates(ctx, actor, model.ID, []string{black.ID, small.ID}, application.SpecificationListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates.Candidates) == 0 || candidates.Candidates[0].Resource.ID != blackGLB.ID || !candidates.Candidates[0].DescriptionComplete {
		t.Fatalf("candidate ranking: %+v", candidates)
	}
	for _, c := range candidates.Candidates {
		if c.Resource.ID == whiteGLB.ID {
			t.Fatal("conflicting color recommended")
		}
		if c.Resource.ID == base.ID && c.DescriptionComplete {
			t.Fatal("undescribed model marked complete")
		}
	}
	_, err = svc.SaveAppearance(ctx, actor, application.SaveAppearanceDefault{ModelID: model.ID, ResourceID: matteGLB.ID, TagIDs: []string{matte.ID}})
	if err != nil {
		t.Fatal(err)
	}
	assertEffective(one.ID, base.ID, "model", true)
	specific, err := svc.SaveAppearance(ctx, actor, application.SaveAppearanceDefault{ModelID: model.ID, ResourceID: blackGLB.ID, TagIDs: []string{black.ID, matte.ID}})
	if err != nil {
		t.Fatal(err)
	}
	assertEffective(one.ID, blackGLB.ID, "appearance", false)
	if err := first.BindModel3DResource(ctx, actor.TenantID, application.BindModel3DResource{Kind: "asset", TargetID: one.ID, ResourceID: whiteGLB.ID}); err != nil {
		t.Fatal(err)
	}
	assertEffective(one.ID, whiteGLB.ID, "asset", false)
	if err := svc.SaveResource(ctx, actor, application.SaveResourceSpecification{ResourceID: blackGLB.ID, CategoryIDs: []string{category.ID}, TagIDs: []string{white.ID}}); err != nil {
		t.Fatal(err)
	}
	assertEffective(two.ID, blackGLB.ID, "appearance", false) // Descriptions never rewrite confirmed bindings.
	if err := first.BindModel3DResource(ctx, actor.TenantID, application.BindModel3DResource{Kind: "asset", TargetID: one.ID}); err != nil {
		t.Fatal(err)
	}
	assertEffective(one.ID, blackGLB.ID, "appearance", false)
	if err := svc.DeleteAppearance(ctx, actor, specific.ID); err != nil {
		t.Fatal(err)
	}
	assertEffective(one.ID, base.ID, "model", true)
	if err := svc.DeleteAppearance(ctx, actor, blackRule.ID); err != nil {
		t.Fatal(err)
	}
	assertEffective(two.ID, base.ID, "model", false)
	viewer := actor
	viewer.Role = "viewer"
	if _, err := reader.Snapshot(ctx, viewer); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveType(ctx, viewer, application.SaveSpecificationType{Name: "Forbidden", Enabled: true}); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("viewer write: %v", err)
	}
	foreign := actor
	foreign.TenantID = uuid.NewString()
	state, err := reader.Snapshot(ctx, foreign)
	if err != nil || len(state.Tags) != 0 || len(state.Links) != 0 {
		t.Fatalf("cross-space snapshot: %+v %v", state, err)
	}
	if _, err := reader.EffectiveForAsset(ctx, foreign, one.ID); err == nil {
		t.Fatal("cross-space model readable")
	}
	if err := svc.SaveModel(ctx, foreign, application.SaveModelSpecification{ModelID: model.ID, TagIDs: allowed}); err == nil {
		t.Fatal("cross-space model editable")
	}
	t.Run("concurrent allowance removal and selection", func(t *testing.T) {
		start := make(chan struct{})
		var wait sync.WaitGroup
		var selectionErr, removalErr error
		wait.Add(2)
		go func() {
			defer wait.Done()
			<-start
			_, selectionErr = svc.SaveAsset(ctx, actor, application.SaveSpecificationAsset{ModelID: model.ID, DisplayName: "Concurrent white", TagIDs: []string{white.ID}})
		}()
		go func() {
			defer wait.Done()
			<-start
			removalErr = reader.SaveModel(ctx, actor, application.SaveModelSpecification{ModelID: model.ID, TagIDs: []string{black.ID, small.ID, large.ID, matte.ID}})
		}()
		close(start)
		wait.Wait()
		if (selectionErr == nil) == (removalErr == nil) {
			t.Fatalf("one operation must win: selection=%v removal=%v", selectionErr, removalErr)
		}
		state, err := reader.Snapshot(ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		if err := state.ValidateExisting(); err != nil {
			t.Fatalf("race broke a persisted invariant: %v", err)
		}
	})
	t.Run("concurrent appearance binding and resource deletion", func(t *testing.T) {
		candidate := resource("Race resource")
		start := make(chan struct{})
		var wait sync.WaitGroup
		var bindingErr, deletionErr error
		wait.Add(2)
		go func() {
			defer wait.Done()
			<-start
			_, bindingErr = svc.SaveAppearance(ctx, actor, application.SaveAppearanceDefault{ModelID: model.ID, ResourceID: candidate.ID, TagIDs: []string{black.ID}})
		}()
		go func() {
			defer wait.Done()
			<-start
			deletionErr = second.MarkModel3DResourcePendingDelete(ctx, actor.TenantID, candidate.ID)
		}()
		close(start)
		wait.Wait()
		if (bindingErr == nil) == (deletionErr == nil) {
			t.Fatalf("one operation must win: binding=%v deletion=%v", bindingErr, deletionErr)
		}
		persisted, err := second.GetModel3DResource(ctx, actor.TenantID, candidate.ID)
		if err != nil {
			t.Fatal(err)
		}
		if bindingErr == nil && persisted.Status != "ready" {
			t.Fatal("bound resource became pending deletion")
		}
		if deletionErr == nil && persisted.Status != "pending-delete" {
			t.Fatalf("resource status=%s", persisted.Status)
		}
	})
}
