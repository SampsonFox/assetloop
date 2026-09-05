package application

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/google/uuid"
)

func testGLB(jsonText string) []byte {
	jsonData := []byte(jsonText)
	for len(jsonData)%4 != 0 {
		jsonData = append(jsonData, ' ')
	}
	data := make([]byte, 20+len(jsonData))
	copy(data, "glTF")
	binary.LittleEndian.PutUint32(data[4:], 2)
	binary.LittleEndian.PutUint32(data[8:], uint32(len(data)))
	binary.LittleEndian.PutUint32(data[12:], uint32(len(jsonData)))
	binary.LittleEndian.PutUint32(data[16:], 0x4e4f534a)
	copy(data[20:], jsonData)
	return data
}

type mediaTestStore struct {
	model   domain.ProductModel
	asset   domain.Asset
	updates []domain.ProductModel3D
}

func (s *mediaTestStore) GetAsset(_ context.Context, tenant, id string) (domain.Asset, error) {
	if s.asset.TenantID != tenant || s.asset.ID != id {
		return domain.Asset{}, errors.New("not found")
	}
	return s.asset, nil
}
func (s *mediaTestStore) GetProductModel(_ context.Context, tenant, id string) (domain.ProductModel, error) {
	if s.model.TenantID != tenant || s.model.ID != id {
		return domain.ProductModel{}, errors.New("not found")
	}
	return s.model, nil
}
func (s *mediaTestStore) UpdateProductModel3D(_ context.Context, tenant, id string, media domain.ProductModel3D) error {
	if s.model.TenantID != tenant || s.model.ID != id {
		return errors.New("not found")
	}
	s.updates = append(s.updates, media)
	s.model.Model3D = &media
	return nil
}

type memoryBlob struct {
	data    map[string][]byte
	deleted []string
}

func (b *memoryBlob) Put(_ context.Context, key string, r io.Reader, _ BlobMetadata) error {
	value, err := io.ReadAll(r)
	if b.data == nil {
		b.data = map[string][]byte{}
	}
	b.data[key] = value
	return err
}
func (b *memoryBlob) Open(_ context.Context, key string) (io.ReadCloser, BlobInfo, error) {
	value, ok := b.data[key]
	if !ok {
		return nil, BlobInfo{}, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(value)), BlobInfo{Size: int64(len(value))}, nil
}
func (b *memoryBlob) Stat(_ context.Context, key string) (BlobInfo, error) {
	value, ok := b.data[key]
	if !ok {
		return BlobInfo{}, errors.New("not found")
	}
	return BlobInfo{Size: int64(len(value))}, nil
}
func (b *memoryBlob) Delete(_ context.Context, key string) error {
	delete(b.data, key)
	b.deleted = append(b.deleted, key)
	return nil
}

type mediaRegistry map[string]BlobStore

func (r mediaRegistry) Get(id string) (BlobStore, bool) { s, ok := r[id]; return s, ok }

type mediaKeys struct{}

func (mediaKeys) ProductModel3D(tenant, model, sha string) (string, error) {
	return "tenants/" + tenant + "/models/" + model + "/" + sha + ".glb", nil
}

func TestModelMediaUploadReplaceAndRead(t *testing.T) {
	tenant, modelID, assetID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	old := &domain.ProductModel3D{StoreID: "local", ObjectKey: "old.glb", SHA256: "old", SizeBytes: 3}
	store := &mediaTestStore{model: domain.ProductModel{ID: modelID, TenantID: tenant, Model3D: old}, asset: domain.Asset{ID: assetID, TenantID: tenant, ModelID: modelID}}
	blobs := &memoryBlob{data: map[string][]byte{"old.glb": []byte("old")}}
	service := NewModelMediaService(store, mediaRegistry{"local": blobs}, mediaKeys{}, "local")
	service.now = func() time.Time { return time.Unix(100, 0) }
	actor := Principal{TenantID: tenant, UserID: uuid.NewString(), Role: RoleEditor}
	media, err := service.Update(context.Background(), actor, UpdateProductModel3D{ModelID: modelID, File: testGLB(`{"asset":{"version":"2.0"}}`), Author: "Maker"})
	if err != nil {
		t.Fatal(err)
	}
	if media.StoreID != "local" || media.Author != "Maker" || len(store.updates) != 1 {
		t.Fatalf("unexpected media: %#v", media)
	}
	if len(blobs.deleted) != 1 || blobs.deleted[0] != "old.glb" {
		t.Fatalf("old object not deleted: %#v", blobs.deleted)
	}
	opened, err := service.OpenForAsset(context.Background(), actor, assetID)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Reader.Close()
	got, _ := io.ReadAll(opened.Reader)
	if !bytes.Equal(got, testGLB(`{"asset":{"version":"2.0"}}`)) {
		t.Fatal("opened bytes differ")
	}
}

func TestModelMediaValidationAndPermissions(t *testing.T) {
	tenant, modelID := uuid.NewString(), uuid.NewString()
	store := &mediaTestStore{model: domain.ProductModel{ID: modelID, TenantID: tenant}}
	service := NewModelMediaService(store, mediaRegistry{"local": &memoryBlob{}}, mediaKeys{}, "local")
	viewer := Principal{TenantID: tenant, UserID: uuid.NewString(), Role: RoleViewer}
	if _, err := service.Update(context.Background(), viewer, UpdateProductModel3D{ModelID: modelID, File: testGLB(`{"asset":{"version":"2.0"}}`)}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer error=%v", err)
	}
	editor := Principal{TenantID: tenant, UserID: uuid.NewString(), Role: RoleEditor}
	if _, err := service.Update(context.Background(), editor, UpdateProductModel3D{ModelID: modelID, File: []byte("fake")}); err == nil {
		t.Fatal("accepted fake GLB")
	}
	if _, err := service.Update(context.Background(), editor, UpdateProductModel3D{ModelID: modelID, File: testGLB(`{"asset":{"version":"2.0"},"images":[{"uri":"image.png"}]}`)}); err == nil {
		t.Fatal("accepted external resource")
	}
	other := editor
	other.TenantID = uuid.NewString()
	if _, err := service.GetForModel(context.Background(), other, modelID); err == nil {
		t.Fatal("cross-tenant read succeeded")
	}
}
