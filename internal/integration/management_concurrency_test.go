package integration_test

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/blob"
	localblob "github.com/SampsonFox/assetloop/internal/blob/local"
)

func testManagementConcurrentReplay(t *testing.T, first, second application.ManagementStore, owner application.Principal) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	local, err := localblob.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry := blob.Registry{"local": local}
	services := []*application.ManagementService{application.NewManagementService(first, registry), application.NewManagementService(second, registry)}
	type outcome struct {
		id  string
		err error
	}
	start := make(chan struct{})
	done := make(chan outcome, 8)
	cmd := application.CreateCategory{Name: "Concurrent management", IconKey: "camera"}
	for i := 0; i < 8; i++ {
		go func(i int) {
			<-start
			category, err := services[i%2].CreateCategory(ctx, owner, "concurrent-category", cmd)
			done <- outcome{category.ID, err}
		}(i)
	}
	close(start)
	var id string
	for i := 0; i < 8; i++ {
		result := <-done
		if result.err != nil {
			t.Fatal(result.err)
		}
		if id == "" {
			id = result.id
		}
		if result.id != id {
			t.Fatal("concurrent retries created different IDs")
		}
	}
	categories, err := second.ListCategories(ctx, owner.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, category := range categories {
		if category.Name == cmd.Name {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("concurrent category count = %d", count)
	}

	// Different commands racing for one key must produce one success and one conflict.
	start = make(chan struct{})
	for i, name := range []string{"Race first", "Race second"} {
		go func(i int, name string) {
			<-start
			category, err := services[i].CreateCategory(ctx, owner, "conflicting-category", application.CreateCategory{Name: name, IconKey: "camera"})
			done <- outcome{category.ID, err}
		}(i, name)
	}
	close(start)
	successes, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		result := <-done
		var input application.InputError
		if result.err == nil {
			successes++
		} else if errors.As(result.err, &input) && input.Error() == "validation.request_conflict" {
			conflicts++
		} else {
			t.Fatalf("unexpected race error: %v", result.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("race: successes=%d conflicts=%d", successes, conflicts)
	}

	media := application.NewModelMediaService(first, registry, blob.ObjectKeyMapper{}, "local")
	r, err := media.Upload(ctx, owner, application.UploadModel3DResource{Name: "Concurrent deletion", File: fullElementGLB()})
	if err != nil {
		t.Fatal(err)
	}
	start = make(chan struct{})
	for i := 0; i < 8; i++ {
		go func(i int) {
			<-start
			done <- outcome{err: services[i%2].DeleteResource(ctx, owner, "concurrent-delete", r.ID)}
		}(i)
	}
	close(start)
	for i := 0; i < 8; i++ {
		if result := <-done; result.err != nil {
			t.Fatalf("concurrent deletion: %v", result.err)
		}
	}
	if _, err := local.Stat(ctx, r.ObjectKey); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("blob remains: %v", err)
	}
	if _, err := media.GetResource(ctx, owner, r.ID); !errors.Is(err, application.ErrModel3DNotFound) {
		t.Fatalf("metadata remains: %v", err)
	}
	if err := application.NewManagementService(second, registry).DeleteResource(ctx, owner, "concurrent-delete", r.ID); err != nil {
		t.Fatalf("reconstructed service lost receipt: %v", err)
	}
}
