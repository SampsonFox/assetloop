package application

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/google/uuid"
)

type failingMediaStore struct {
	*mediaTestStore
	createErr, bindErr, finishErr     error
	commitThenError, probeUnavailable bool
}

func (s *failingMediaStore) CreateModel3DResource(ctx context.Context, r domain.Model3DResource) error {
	if s.createErr != nil {
		return s.createErr
	}
	return s.mediaTestStore.CreateModel3DResource(ctx, r)
}
func (s *failingMediaStore) CreateAndBindModel3DResource(ctx context.Context, r domain.Model3DResource, b BindModel3DResource) error {
	if s.commitThenError {
		if err := s.mediaTestStore.CreateAndBindModel3DResource(ctx, r, b); err != nil {
			return err
		}
		return errors.New("commit acknowledgement lost")
	}
	if s.bindErr != nil {
		return s.bindErr
	}
	return s.mediaTestStore.CreateAndBindModel3DResource(ctx, r, b)
}
func (s *failingMediaStore) GetModel3DResource(ctx context.Context, tenant, id string) (domain.Model3DResource, error) {
	if s.probeUnavailable {
		return domain.Model3DResource{}, errors.New("database offline")
	}
	return s.mediaTestStore.GetModel3DResource(ctx, tenant, id)
}
func (s *failingMediaStore) MarkModel3DResourcePendingDelete(ctx context.Context, tenant, id string) error {
	r, err := s.GetModel3DResource(ctx, tenant, id)
	if err != nil {
		return err
	}
	if s.model.Model3D != nil && s.model.Model3D.ResourceID == id {
		return ErrModel3DReferenced
	}
	r.Status = "pending-delete"
	s.resources[id] = r
	return nil
}
func (s *failingMediaStore) FinishModel3DResourceDelete(_ context.Context, tenant, id string) error {
	if s.finishErr != nil {
		return s.finishErr
	}
	delete(s.resources, id)
	return nil
}
func (s *failingMediaStore) UpdateModel3DResource(_ context.Context, r domain.Model3DResource) error {
	s.resources[r.ID] = r
	return nil
}

type failingMediaBlob struct {
	*memoryBlob
	failure            string
	deleteErr          error
	cleanupUncancelled bool
}

func (b *failingMediaBlob) Put(ctx context.Context, key string, r io.Reader, m BlobMetadata) error {
	if err := b.memoryBlob.Put(ctx, key, r, m); err != nil {
		return err
	}
	if b.failure == "put" {
		return errors.New("interrupted put")
	}
	return nil
}
func (b *failingMediaBlob) Stat(ctx context.Context, key string) (BlobInfo, error) {
	info, err := b.memoryBlob.Stat(ctx, key)
	if b.failure == "stat" {
		return BlobInfo{}, errors.New("stat failed")
	}
	if b.failure == "size" {
		info.Size++
	}
	return info, err
}
func (b *failingMediaBlob) Open(ctx context.Context, key string) (io.ReadCloser, BlobInfo, error) {
	if b.failure == "open" {
		return nil, BlobInfo{}, errors.New("open failed")
	}
	if b.failure == "checksum" {
		return io.NopCloser(strings.NewReader("corrupt")), BlobInfo{Size: 7}, nil
	}
	return b.memoryBlob.Open(ctx, key)
}
func (b *failingMediaBlob) Delete(ctx context.Context, key string) error {
	if ctx.Err() == nil {
		if _, ok := ctx.Deadline(); ok {
			b.cleanupUncancelled = true
		}
	}
	if b.deleteErr != nil {
		return b.deleteErr
	}
	return b.memoryBlob.Delete(ctx, key)
}

func TestModelResourceAmbiguousCommitRetainsBytes(t *testing.T) {
	for _, mode := range []string{"committed", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			actor, store, blob, svc := mediaFailureSetup()
			ctx := context.Background()
			if mode == "committed" {
				store.commitThenError = true
			} else {
				store.bindErr = errors.New("unknown commit")
				store.probeUnavailable = true
			}
			_, err := svc.UploadAndBind(ctx, actor, UploadModel3DResource{Name: "Ambiguous", File: testGLB(`{"asset":{"version":"2.0"}}`)}, BindModel3DResource{Kind: "model", TargetID: store.model.ID})
			if err == nil {
				t.Fatal("ambiguous commit not reported")
			}
			if len(blob.data) != 2 || len(blob.deleted) != 0 {
				t.Fatalf("ambiguous commit destroyed bytes: %+v", blob.data)
			}
			if mode == "committed" {
				if store.model.Model3D == nil {
					t.Fatal("fake did not commit binding")
				}
				if _, ok := blob.data[store.model.Model3D.ObjectKey]; !ok {
					t.Fatal("persisted binding has no bytes")
				}
			}
		})
	}
}

func TestModelResourceCanceledUploadCleanupIsBoundedAndUncancelled(t *testing.T) {
	actor, _, blob, svc := mediaFailureSetup()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	blob.failure = "put"
	if _, err := svc.Upload(ctx, actor, UploadModel3DResource{Name: "Canceled", File: testGLB(`{"asset":{"version":"2.0"}}`)}); err == nil {
		t.Fatal("put failure accepted")
	}
	if !blob.cleanupUncancelled || len(blob.data) != 1 {
		t.Fatal("canceled upload was not cleaned with bounded uncancelled context")
	}
}

func mediaFailureSetup() (Principal, *failingMediaStore, *failingMediaBlob, *ModelMediaService) {
	actor := Principal{TenantID: uuid.NewString(), UserID: uuid.NewString(), Role: RoleEditor}
	store := &failingMediaStore{mediaTestStore: &mediaTestStore{model: domain.ProductModel{ID: uuid.NewString(), TenantID: actor.TenantID, Name: "Existing"}}}
	blob := &failingMediaBlob{memoryBlob: &memoryBlob{data: map[string][]byte{"legacy-key.glb": []byte("keep")}}}
	svc := NewModelMediaService(store, mediaRegistry{"local": blob}, mediaKeys{}, "local")
	return actor, store, blob, svc
}

func TestModelResourceUploadFailureCleansOnlyNewObject(t *testing.T) {
	for _, failure := range []string{"put", "stat", "size", "open", "checksum", "create", "bind"} {
		t.Run(failure, func(t *testing.T) {
			actor, store, blob, svc := mediaFailureSetup()
			blob.failure = failure
			if failure == "create" {
				store.createErr = errors.New("transaction failed")
			}
			if failure == "bind" {
				store.bindErr = errors.New("target concurrently removed")
			}
			cmd := UploadModel3DResource{Name: "New", File: testGLB(`{"asset":{"version":"2.0"}}`)}
			var err error
			if failure == "bind" {
				_, err = svc.UploadAndBind(context.Background(), actor, cmd, BindModel3DResource{Kind: "model", TargetID: store.model.ID})
			} else {
				_, err = svc.Upload(context.Background(), actor, cmd)
			}
			if err == nil {
				t.Fatal("failure accepted")
			}
			if len(blob.data) != 1 || string(blob.data["legacy-key.glb"]) != "keep" {
				t.Fatalf("cleanup touched legacy or left new object: %+v", blob.data)
			}
			if len(store.resources) != 0 || len(store.updates) != 0 {
				t.Fatalf("failed upload persisted: %+v", store)
			}
		})
	}
}

func TestModelResourceDeleteFailuresRemainRetryable(t *testing.T) {
	for _, failure := range []string{"blob", "finish", "unconfigured"} {
		t.Run(failure, func(t *testing.T) {
			actor, store, blob, svc := mediaFailureSetup()
			ctx := context.Background()
			r, err := svc.Upload(ctx, actor, UploadModel3DResource{Name: "Retry", File: testGLB(`{"asset":{"version":"2.0"}}`)})
			if err != nil {
				t.Fatal(err)
			}
			switch failure {
			case "blob":
				blob.deleteErr = errors.New("offline")
			case "finish":
				store.finishErr = errors.New("metadata offline")
			case "unconfigured":
				svc.stores = mediaRegistry{}
			}
			if err := svc.DeleteResource(ctx, actor, r.ID); err == nil {
				t.Fatal("delete failure accepted")
			}
			got, err := svc.GetResource(ctx, actor, r.ID)
			if err != nil || got.Status != "pending-delete" {
				t.Fatalf("pending state missing: %+v %v", got, err)
			}
			if err := svc.Bind(ctx, actor, BindModel3DResource{Kind: "model", TargetID: store.model.ID, ResourceID: r.ID}); !errors.Is(err, ErrModel3DUnavailable) {
				t.Fatalf("pending bind=%v", err)
			}
			if _, err := svc.OpenResource(ctx, actor, r.ID); !errors.Is(err, ErrModel3DUnavailable) {
				t.Fatalf("pending open=%v", err)
			}
			if _, err := svc.UpdateResource(ctx, actor, UpdateModel3DResource{ID: r.ID, Name: "Changed"}); !errors.Is(err, ErrModel3DUnavailable) {
				t.Fatalf("pending edit=%v", err)
			}
			blob.deleteErr = nil
			store.finishErr = nil
			svc.stores = mediaRegistry{"local": blob}
			if err := svc.DeleteResource(ctx, actor, r.ID); err != nil {
				t.Fatal("retry failed:", err)
			}
			if _, err := svc.GetResource(ctx, actor, r.ID); !errors.Is(err, ErrModel3DNotFound) {
				t.Fatalf("resource retained: %v", err)
			}
			if _, ok := blob.data[r.ObjectKey]; ok {
				t.Fatal("bytes retained")
			}
			if string(blob.data["legacy-key.glb"]) != "keep" {
				t.Fatal("deleted legacy bytes")
			}
		})
	}
}

func TestModelResourceIdenticalUploadsAttributionAndPermissions(t *testing.T) {
	actor, store, blob, svc := mediaFailureSetup()
	ctx := context.Background()
	file := testGLB(`{"asset":{"version":"2.0"}}`)
	cmd := UploadModel3DResource{Name: "Shared", File: file, Author: "Maker", License: "CC0"}
	one, err := svc.UploadAndBind(ctx, actor, cmd, BindModel3DResource{Kind: "model", TargetID: store.model.ID})
	if err != nil {
		t.Fatal(err)
	}
	two, err := svc.Upload(ctx, actor, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if one.ID == two.ID || one.ObjectKey == two.ObjectKey || one.SHA256 != two.SHA256 {
		t.Fatalf("identity/key collision: %+v %+v", one, two)
	}
	if err := svc.DeleteResource(ctx, actor, one.ID); !errors.Is(err, ErrModel3DReferenced) {
		t.Fatalf("referenced delete=%v", err)
	}
	if !bytes.Equal(blob.data[one.ObjectKey], file) {
		t.Fatal("rejected deletion touched bytes")
	}
	updated, err := svc.Update(ctx, actor, UpdateProductModel3D{ModelID: store.model.ID, SourceURL: "https://example.com/source", Author: "Updated", License: "CC-BY"})
	if err != nil || updated.ResourceID != one.ID || updated.ObjectKey != one.ObjectKey {
		t.Fatalf("compatibility attribution=%+v %v", updated, err)
	}
	if err := svc.DeleteResource(ctx, actor, two.ID); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(blob.data[one.ObjectKey], file) {
		t.Fatal("deleting identical upload touched shared bytes")
	}
	viewer := actor
	viewer.Role = RoleViewer
	if _, err := svc.Upload(ctx, viewer, cmd); !errors.Is(err, ErrForbidden) {
		t.Fatal("viewer upload", err)
	}
	if _, err := svc.UploadAndBind(ctx, viewer, cmd, BindModel3DResource{Kind: "model", TargetID: store.model.ID}); !errors.Is(err, ErrForbidden) {
		t.Fatal("viewer upload+bind", err)
	}
	if _, err := svc.UpdateResource(ctx, viewer, UpdateModel3DResource{ID: one.ID, Name: "x"}); !errors.Is(err, ErrForbidden) {
		t.Fatal("viewer edit", err)
	}
	if err := svc.Bind(ctx, viewer, BindModel3DResource{Kind: "model", TargetID: store.model.ID}); !errors.Is(err, ErrForbidden) {
		t.Fatal("viewer unbind", err)
	}
	if err := svc.DeleteResource(ctx, viewer, one.ID); !errors.Is(err, ErrForbidden) {
		t.Fatal("viewer delete", err)
	}
	outsider := actor
	outsider.TenantID = uuid.NewString()
	if _, err := svc.GetResource(ctx, outsider, one.ID); !errors.Is(err, ErrModel3DNotFound) {
		t.Fatal("cross-tenant read", err)
	}
	// Read routing must still use the original registry entry after changing defaults.
	svc.defaultStore = "different"
	opened, err := svc.OpenResource(ctx, viewer, one.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(opened.Reader)
	opened.Reader.Close()
	if !bytes.Equal(data, file) {
		t.Fatal("default change hid existing bytes")
	}
}
