package integration_test

import (
	"context"
	"errors"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/blob"
	localblob "github.com/SampsonFox/assetloop/internal/blob/local"
	"io/fs"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type downloadFunc func(context.Context, string) ([]byte, error)

func (f downloadFunc) Download(c context.Context, u string) ([]byte, error) { return f(c, u) }

type failingReceiptStore struct{ application.ManagementStore }

func (s failingReceiptStore) WithManagementWrite(ctx context.Context, tenant string, fn func(application.ManagementStore) error) error {
	return s.ManagementStore.WithManagementWrite(ctx, tenant, func(tx application.ManagementStore) error { return fn(failingReceiptStore{tx}) })
}
func (failingReceiptStore) SaveManagementRequest(context.Context, application.ManagementRequest) error {
	return errors.New("injected receipt failure")
}

func testModelImport(t *testing.T, first, second application.ManagementStore, owner application.Principal) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	local, err := localblob.New(root)
	if err != nil {
		t.Fatal(err)
	}
	registry := blob.Registry{"local": local}
	build := func(store application.ManagementStore, d application.ModelDownloader) *application.ModelImportService {
		return application.NewModelImportService(application.NewManagementService(store, registry), application.NewModelMediaService(store, registry, blob.ObjectKeyMapper{}, "local"), d)
	}
	var calls atomic.Int32
	simple := downloadFunc(func(context.Context, string) ([]byte, error) { calls.Add(1); return fullElementGLB(), nil })
	cmd := application.ImportModel3D{URL: "https://example.com/model.glb", Name: "Imported fixture", License: "test fixture"}
	service := build(first, simple)
	viewer := owner
	viewer.Role = application.RoleViewer
	if _, err := service.Import(ctx, viewer, "denied", cmd); err == nil || calls.Load() != 0 {
		t.Fatal("unauthorized download")
	}
	if _, err := service.Import(ctx, owner, "", cmd); err == nil || calls.Load() != 0 {
		t.Fatal("bad key downloaded")
	}
	bad := build(first, downloadFunc(func(context.Context, string) ([]byte, error) { return []byte("not GLB"), nil }))
	if _, err := bad.Import(ctx, owner, "import-bad", cmd); err == nil {
		t.Fatal("invalid GLB accepted")
	}
	fail := build(failingReceiptStore{first}, simple)
	if _, err := fail.Import(ctx, owner, "import-rollback", cmd); err == nil {
		t.Fatal("receipt failure ignored")
	}
	files := func() int {
		n := 0
		err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				n++
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if files() != 0 {
		t.Fatal("rollback leaked blob")
	}
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	parallel := downloadFunc(func(context.Context, string) ([]byte, error) {
		started <- struct{}{}
		select {
		case <-release:
			return fullElementGLB(), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	type outcome struct {
		id  string
		err error
	}
	done := make(chan outcome, 2)
	for _, store := range []application.ManagementStore{first, second} {
		go func(s application.ManagementStore) {
			r, e := build(s, parallel).Import(ctx, owner, "import-race", cmd)
			done <- outcome{r.ID, e}
		}(store)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("download held database lock")
		}
	}
	close(release)
	a, b := <-done, <-done
	if a.err != nil || b.err != nil || a.id == "" || a.id != b.id {
		t.Fatalf("race failed: %v %v", a.err, b.err)
	}
	if files() != 1 {
		t.Fatal("concurrent replay leaked blob")
	}
	before := calls.Load()
	r, err := service.Import(ctx, owner, "import-race", cmd)
	if err != nil || r.ID != a.id || calls.Load() != before {
		t.Fatal("replay downloaded or changed identity")
	}
	cmd.URL = "https://example.com/changed.glb"
	if _, err := service.Import(ctx, owner, "import-race", cmd); err == nil || calls.Load() != before {
		t.Fatal("conflict downloaded")
	}
	if _, err := service.Import(ctx, viewer, "import-race", cmd); err == nil {
		t.Fatal("replay bypassed current role")
	}
	// The process-level importer rejects excess work instead of queuing an
	// unbounded number of 25 MiB buffers. Failed downloads leave no receipt/blob.
	entered := make(chan struct{}, 2)
	finish := make(chan struct{})
	limited := build(first, downloadFunc(func(context.Context, string) ([]byte, error) {
		entered <- struct{}{}
		select {
		case <-finish:
			return nil, application.ErrModel3DUnavailable
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}))
	for _, key := range []string{"import-limit-a", "import-limit-b"} {
		go func(k string) { _, e := limited.Import(ctx, owner, k, cmd); done <- outcome{err: e} }(key)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("imports did not enter")
		}
	}
	if _, e := limited.Import(ctx, owner, "import-limit-c", cmd); !errors.Is(e, application.ErrModel3DUnavailable) {
		t.Fatal("concurrency limit not enforced")
	}
	close(finish)
	for i := 0; i < 2; i++ {
		if r := <-done; r.err == nil {
			t.Fatal("failed download succeeded")
		}
	}
	if files() != 1 {
		t.Fatal("failed download leaked blob")
	}
}
