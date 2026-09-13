package integration_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"io/fs"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/blob"
	localblob "github.com/SampsonFox/assetloop/internal/blob/local"
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

type imageDownloadFunc func(context.Context, string) ([]byte, error)

func (f imageDownloadFunc) DownloadImage(c context.Context, u string) ([]byte, error) { return f(c, u) }

func testModelImagePNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// testModelImageImport runs the receipt-backed image import against both Store
// adapters through the existing catalog transaction suite. Network access is a
// deterministic fixture; the downloader is never called before authorization,
// the receipt fingerprint or a persisted active binding.
func testModelImageImport(t *testing.T, first, second application.ManagementStore, owner application.Principal, category string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	local, err := localblob.New(root)
	if err != nil {
		t.Fatal(err)
	}
	registry := blob.Registry{"local": local}
	model, err := application.NewManagementService(first, registry).CreateModel(ctx, owner, "image-import-model", application.CreateModel{CategoryID: category, Name: "Imaged model"})
	if err != nil {
		t.Fatal(err)
	}
	build := func(store application.ManagementStore, d application.ImageDownloader) *application.ModelImageImportService {
		images := application.NewModelImageService(store, registry, blob.ObjectKeyMapper{}, "local")
		return application.NewModelImageImportService(application.NewManagementService(store, registry), images, d)
	}
	files := func() int {
		t.Helper()
		n := 0
		if err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				n++
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	var calls atomic.Int32
	data := testModelImagePNG(t)
	simple := imageDownloadFunc(func(context.Context, string) ([]byte, error) { calls.Add(1); return data, nil })
	cmd := application.ImportModelImage{ModelID: model.ID, URL: "https://images.example/model.png", SourceURL: "https://example.com/product"}
	service := build(first, simple)
	viewer := owner
	viewer.Role = application.RoleViewer
	if _, err := service.Import(ctx, viewer, "image-denied", cmd); err == nil || calls.Load() != 0 {
		t.Fatal("unauthorized image download")
	}
	if _, err := service.Import(ctx, owner, "", cmd); err == nil || calls.Load() != 0 {
		t.Fatal("empty request key downloaded an image")
	}
	// Target and source validation happen before any network access.
	missingModel := cmd
	missingModel.ModelID = uuid.NewString()
	if _, err := service.Import(ctx, owner, "image-missing-model", missingModel); err == nil || calls.Load() != 0 {
		t.Fatal("missing model downloaded an image")
	}
	badSource := cmd
	badSource.SourceURL = "https://user:secret@example.com/"
	if _, err := service.Import(ctx, owner, "image-bad-source", badSource); err == nil || calls.Load() != 0 {
		t.Fatal("credential-bearing source downloaded an image")
	}
	bad := build(first, imageDownloadFunc(func(context.Context, string) ([]byte, error) { return []byte("<svg/>"), nil }))
	if _, err := bad.Import(ctx, owner, "image-bad", cmd); err == nil || files() != 0 {
		t.Fatal("invalid image accepted or leaked a blob")
	}
	firstImage, err := service.Import(ctx, owner, "image-first", cmd)
	if err != nil || firstImage.ID == "" || files() != 1 {
		t.Fatalf("first image import: %+v %v files=%d", firstImage, err, files())
	}
	// A failed receipt must roll the active binding back and clean only the new blob.
	fail := build(failingReceiptStore{first}, imageDownloadFunc(func(context.Context, string) ([]byte, error) { return data, nil }))
	changed := cmd
	changed.URL = "https://images.example/rolled-back.png"
	if _, err := fail.Import(ctx, owner, "image-rollback", changed); err == nil {
		t.Fatal("receipt failure reported success")
	}
	images := application.NewModelImageService(first, registry, blob.ObjectKeyMapper{}, "local")
	if current, err := images.Get(ctx, owner, model.ID); err != nil || current.ID != firstImage.ID || files() != 1 {
		t.Fatalf("failed receipt changed the active image: %+v %v files=%d", current, err, files())
	}
	if _, found, err := first.FindManagementRequest(ctx, owner.TenantID, owner.UserID, "image-rollback"); err != nil || found {
		t.Fatal("failed receipt persisted")
	}
	// Concurrent identical imports leave one active revision and clean the loser.
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	parallel := imageDownloadFunc(func(context.Context, string) ([]byte, error) {
		started <- struct{}{}
		select {
		case <-release:
			return data, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	type outcome struct {
		image application.ModelImage
		err   error
	}
	done := make(chan outcome, 2)
	for _, store := range []application.ManagementStore{first, second} {
		go func(s application.ManagementStore) {
			revision, err := build(s, parallel).Import(ctx, owner, "image-race", cmd)
			done <- outcome{revision, err}
		}(store)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("image download held the database transaction")
		}
	}
	close(release)
	a, b := <-done, <-done
	if a.err != nil || b.err != nil || a.image.ID == "" || a.image.ID != b.image.ID {
		t.Fatalf("concurrent image import: %v %v %+v %+v", a.err, b.err, a.image, b.image)
	}
	if files() != 2 {
		t.Fatalf("concurrent image import leaked a blob: files=%d", files())
	}
	before := calls.Load()
	if replay, err := service.Import(ctx, owner, "image-race", cmd); err != nil || replay.ID != a.image.ID || calls.Load() != before {
		t.Fatalf("image replay downloaded or changed identity: %+v %v", replay, err)
	}
	conflictURL := cmd
	conflictURL.URL = "https://images.example/changed.png"
	if _, err := service.Import(ctx, owner, "image-race", conflictURL); err == nil || calls.Load() != before {
		t.Fatal("conflicting image replay downloaded")
	}
	if _, err := service.Import(ctx, viewer, "image-race", cmd); err == nil {
		t.Fatal("image replay bypassed the current role")
	}
	if files() != 2 {
		t.Fatalf("replay changed stored revisions: files=%d", files())
	}
	// Bounded content upload reuses the same receipt, blob verification and atomic
	// binding without any downloader call.
	beforeUpload := calls.Load()
	content := application.UploadModelImage{ModelID: model.ID, SourceURL: "https://example.com/product", Data: data}
	uploaded, err := service.Upload(ctx, owner, "image-upload", content)
	if err != nil || uploaded.ID == "" || calls.Load() != beforeUpload || files() != 3 {
		t.Fatalf("content upload: %+v %v files=%d", uploaded, err, files())
	}
	if _, err := service.Upload(ctx, owner, "", content); err == nil {
		t.Fatal("empty request key upload accepted")
	}
	if _, err := service.Upload(ctx, viewer, "image-upload-denied", content); err == nil {
		t.Fatal("unauthorized content upload")
	}
	if _, err := service.Upload(ctx, owner, "image-upload-invalid", application.UploadModelImage{ModelID: model.ID, Data: []byte("<svg/>")}); err == nil {
		t.Fatal("invalid content upload accepted")
	}
	if _, err := service.Upload(ctx, owner, "image-upload-bad-source", application.UploadModelImage{ModelID: model.ID, SourceURL: "https://user:secret@example.com/", Data: data}); err == nil {
		t.Fatal("credential-bearing upload source accepted")
	}
	crossTenantUpload := owner
	crossTenantUpload.TenantID = uuid.NewString()
	if _, err := service.Upload(ctx, crossTenantUpload, "image-upload-cross-tenant", content); err == nil {
		t.Fatal("cross-tenant content upload accepted")
	}
	if _, err := service.Upload(ctx, owner, "image-upload-missing", application.UploadModelImage{ModelID: uuid.NewString(), Data: data}); err == nil {
		t.Fatal("missing model content upload accepted")
	}
	if files() != 3 {
		t.Fatalf("rejected content upload leaked a blob: files=%d", files())
	}
	// Two Store connections racing one content key commit one revision and clean
	// the loser, exactly like the URL import race.
	var bigger bytes.Buffer
	if err := png.Encode(&bigger, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	uploadOutcome := make(chan outcome, 2)
	for _, s := range []application.ManagementStore{first, second} {
		go func(s application.ManagementStore) {
			revision, err := build(s, simple).Upload(ctx, owner, "image-upload-race", content)
			uploadOutcome <- outcome{revision, err}
		}(s)
	}
	u1, u2 := <-uploadOutcome, <-uploadOutcome
	if u1.err != nil || u2.err != nil || u1.image.ID == "" || u1.image.ID != u2.image.ID {
		t.Fatalf("concurrent content upload: %v %v %+v %+v", u1.err, u2.err, u1.image, u2.image)
	}
	if files() != 4 {
		t.Fatalf("concurrent content upload leaked a blob: files=%d", files())
	}
	if replay, err := service.Upload(ctx, owner, "image-upload-race", content); err != nil || replay.ID != u1.image.ID {
		t.Fatalf("content upload replay: %+v %v", replay, err)
	}
	// A later upload becomes active; replaying the older key returns its original
	// revision without overwriting the newer active image.
	later, err := service.Upload(ctx, owner, "image-upload-later", application.UploadModelImage{ModelID: model.ID, Data: bigger.Bytes()})
	if err != nil || later.ID == "" || later.ID == u1.image.ID {
		t.Fatalf("later content upload: %+v %v", later, err)
	}
	if replay, err := service.Upload(ctx, owner, "image-upload", content); err != nil || replay.ID != uploaded.ID || replay.SHA256 != uploaded.SHA256 {
		t.Fatalf("original key replay after a later upload: %+v %v", replay, err)
	}
	if current, err := images.Get(ctx, owner, model.ID); err != nil || current.ID != later.ID {
		t.Fatalf("old-key replay replaced a later image: %+v %v", current, err)
	}
	if files() != 5 {
		t.Fatalf("content replay changed stored revisions: files=%d", files())
	}
}
