package integration_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"testing"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/blob"
	"github.com/google/uuid"
)

func runImageScenario(t *testing.T, store application.ModelImageStore, storage application.BlobStore, owner, viewer application.Principal, model string) {
	t.Helper()
	ctx := context.Background()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 3, 3))); err != nil {
		t.Fatal(err)
	}
	registry := blob.Registry{"original": storage}
	images := application.NewModelImageService(store, registry, blob.ObjectKeyMapper{}, "original")
	if _, err := images.Upload(ctx, viewer, model, b.Bytes(), ""); !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("viewer upload: %v", err)
	}
	for _, invalid := range [][]byte{nil, []byte("<svg/>"), b.Bytes()[:20], make([]byte, application.MaxModelImageBytes+1)} {
		if _, err := images.Upload(ctx, owner, model, invalid, ""); err == nil {
			t.Fatal("invalid image accepted")
		}
	}
	if _, err := images.Upload(ctx, owner, model, b.Bytes(), "https://user:secret@example.com/"); err == nil {
		t.Fatal("credential source accepted")
	}
	first, err := images.Upload(ctx, owner, model, b.Bytes(), "https://example.com/product")
	if err != nil {
		t.Fatal(err)
	}
	second, err := images.Upload(ctx, owner, model, b.Bytes(), "")
	if err != nil || second.ID == first.ID {
		t.Fatalf("replace: %v", err)
	}
	current, err := images.Get(ctx, viewer, model)
	if err != nil || current.ID != second.ID {
		t.Fatalf("active revision: %v", err)
	}
	other := owner
	other.TenantID = uuid.NewString()
	if _, err := images.Get(ctx, other, model); !errors.Is(err, application.ErrImageNotFound) {
		t.Fatalf("cross-tenant read: %v", err)
	}
	if _, err := images.Upload(ctx, other, model, b.Bytes(), ""); err == nil {
		t.Fatal("cross-tenant upload accepted")
	}
	if err := images.Clear(ctx, other, model); err == nil {
		t.Fatal("cross-tenant clear accepted")
	}
	// New default store does not strand existing files.
	switched := application.NewModelImageService(store, registry, blob.ObjectKeyMapper{}, "different-default")
	r, _, err := switched.Open(ctx, viewer, model)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil || !bytes.Equal(got, b.Bytes()) {
		t.Fatal("stored image differs")
	}
	if err := images.Clear(ctx, viewer, model); !errors.Is(err, application.ErrForbidden) {
		t.Fatal("viewer clear accepted")
	}
	if err := images.Clear(ctx, owner, model); err != nil {
		t.Fatal(err)
	}
	if _, err := images.Get(ctx, owner, model); !errors.Is(err, application.ErrImageNotFound) {
		t.Fatalf("clear: %v", err)
	}
	if _, err := store.GetImageRevision(ctx, owner.TenantID, first.ID); err != nil {
		t.Fatal("replacement lost history")
	}
	if _, err := store.GetImageRevision(ctx, owner.TenantID, second.ID); err != nil {
		t.Fatal("clear lost history")
	}
}
