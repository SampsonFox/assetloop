package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/SampsonFox/assetloop/internal/domain"
	"io"
	"io/fs"
	"net/url"
	"strings"
	"time"
)

const MaxProductModel3DBytes int64 = 25 << 20

var (
	ErrModel3DNotFound    = errors.New("3D model not found")
	ErrModel3DReferenced  = errors.New("3D resource is referenced")
	ErrModel3DUnavailable = errors.New("3D resource is unavailable")
)

type ModelMediaService struct {
	store        ModelMediaStore
	stores       BlobStores
	keys         ObjectKeyMapper
	defaultStore string
	now          func() time.Time
}
type UpdateProductModel3D struct {
	ModelID   string
	File      []byte
	SourceURL string
	Author    string
	License   string
}
type UploadModel3DResource struct {
	Name      string
	File      []byte
	SourceURL string
	Author    string
	License   string
}
type UpdateModel3DResource struct{ ID, Name, SourceURL, Author, License string }
type BindModel3DResource struct{ Kind, TargetID, ResourceID string }
type Model3DReference struct{ Kind, ID, Name string }
type Model3DBinding struct {
	Name, ResourceID, EffectiveResourceID, Source string
	Effective                                     *domain.ProductModel3D
}
type Model3DResourceListOptions struct {
	Query          string
	Page, PageSize int
}
type Model3DResourceListResult struct {
	Resources []domain.Model3DResource
	Total     int
}
type OpenProductModel3D struct {
	Reader io.ReadCloser
	Info   BlobInfo
	Model  domain.ProductModel3D
}

func NewModelMediaService(store ModelMediaStore, stores BlobStores, keys ObjectKeyMapper, defaultStore string) *ModelMediaService {
	return &ModelMediaService{store: store, stores: stores, keys: keys, defaultStore: defaultStore, now: time.Now}
}

// Update retains the original model-upload API; replacements never destroy an old resource.
func (s *ModelMediaService) Update(ctx context.Context, actor Principal, cmd UpdateProductModel3D) (domain.ProductModel3D, error) {
	if err := actor.Require(CapabilityManageCatalog); err != nil {
		return domain.ProductModel3D{}, err
	}
	if err := validID("model ID", cmd.ModelID); err != nil {
		return domain.ProductModel3D{}, err
	}
	model, err := s.store.GetProductModel(ctx, actor.TenantID, cmd.ModelID)
	if err != nil {
		return domain.ProductModel3D{}, err
	}
	if len(cmd.File) == 0 {
		if model.Model3D == nil {
			return domain.ProductModel3D{}, ErrModel3DNotFound
		}
		r, err := s.GetResource(ctx, actor, model.Model3D.ResourceID)
		if err != nil {
			return domain.ProductModel3D{}, err
		}
		r, err = s.UpdateResource(ctx, actor, UpdateModel3DResource{ID: r.ID, Name: r.Name, SourceURL: cmd.SourceURL, Author: cmd.Author, License: cmd.License})
		return r.ProductModel3D, err
	}
	r, err := s.UploadAndBind(ctx, actor, UploadModel3DResource{Name: model.Name, File: cmd.File, SourceURL: cmd.SourceURL, Author: cmd.Author, License: cmd.License}, BindModel3DResource{Kind: "model", TargetID: cmd.ModelID})
	return r.ProductModel3D, err
}
func (s *ModelMediaService) Upload(ctx context.Context, actor Principal, cmd UploadModel3DResource) (domain.Model3DResource, error) {
	return s.upload(ctx, actor, cmd, BindModel3DResource{})
}
func (s *ModelMediaService) UploadAndBind(ctx context.Context, actor Principal, cmd UploadModel3DResource, binding BindModel3DResource) (domain.Model3DResource, error) {
	if err := actor.Require(CapabilityManageCatalog); err != nil {
		return domain.Model3DResource{}, err
	}
	if _, err := s.Binding(ctx, actor, binding.Kind, binding.TargetID); err != nil {
		return domain.Model3DResource{}, err
	}
	return s.upload(ctx, actor, cmd, binding)
}
func (s *ModelMediaService) upload(ctx context.Context, actor Principal, cmd UploadModel3DResource, binding BindModel3DResource) (domain.Model3DResource, error) {
	if err := actor.Require(CapabilityManageCatalog); err != nil {
		return domain.Model3DResource{}, err
	}
	media, err := validateResourceMetadata(UpdateModel3DResource{Name: cmd.Name, SourceURL: cmd.SourceURL, Author: cmd.Author, License: cmd.License})
	if err != nil {
		return domain.Model3DResource{}, err
	}
	media.ID, media.TenantID, media.Status = newID(), actor.TenantID, "ready"
	media.ResourceID = media.ID
	media.CreatedAt = s.now().UTC()
	media.UpdatedAt = media.CreatedAt
	if int64(len(cmd.File)) > MaxProductModel3DBytes {
		return domain.Model3DResource{}, errors.New("GLB file exceeds 25 MiB")
	}
	if err := validateGLB(cmd.File); err != nil {
		return domain.Model3DResource{}, err
	}
	digest := sha256.Sum256(cmd.File)
	media.SHA256 = hex.EncodeToString(digest[:])
	media.SizeBytes = int64(len(cmd.File))
	media.StoreID = s.defaultStore
	media.ObjectKey, err = s.keys.Model3DResource(actor.TenantID, media.ID, media.SHA256)
	if err != nil {
		return domain.Model3DResource{}, err
	}
	blobStore, ok := s.stores.Get(media.StoreID)
	if !ok {
		return domain.Model3DResource{}, fmt.Errorf("blob store %q is not configured", media.StoreID)
	}
	if err := blobStore.Put(ctx, media.ObjectKey, bytes.NewReader(cmd.File), BlobMetadata{ContentType: "model/gltf-binary"}); err != nil {
		_ = cleanupModelBlob(ctx, blobStore, media.ObjectKey)
		return domain.Model3DResource{}, fmt.Errorf("store GLB: %w", err)
	}
	info, err := blobStore.Stat(ctx, media.ObjectKey)
	if err != nil || info.Size != media.SizeBytes {
		_ = cleanupModelBlob(ctx, blobStore, media.ObjectKey)
		if err != nil {
			return domain.Model3DResource{}, fmt.Errorf("verify GLB: %w", err)
		}
		return domain.Model3DResource{}, errors.New("stored GLB size mismatch")
	}
	stored, _, err := blobStore.Open(ctx, media.ObjectKey)
	if err != nil {
		_ = cleanupModelBlob(ctx, blobStore, media.ObjectKey)
		return domain.Model3DResource{}, fmt.Errorf("verify GLB checksum: %w", err)
	}
	storedHash := sha256.New()
	_, copyErr := io.Copy(storedHash, stored)
	closeErr := stored.Close()
	if copyErr != nil || closeErr != nil || hex.EncodeToString(storedHash.Sum(nil)) != media.SHA256 {
		_ = cleanupModelBlob(ctx, blobStore, media.ObjectKey)
		return domain.Model3DResource{}, errors.New("stored GLB checksum mismatch")
	}

	if binding.Kind == "" {
		err = s.store.CreateModel3DResource(ctx, media)
	} else {
		binding.ResourceID = media.ID
		err = s.store.CreateAndBindModel3DResource(ctx, media, binding)
	}
	if err != nil {
		// Commit errors may be ambiguous. Delete bytes only after positively confirming rollback.
		probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		_, probeErr := s.store.GetModel3DResource(probeCtx, actor.TenantID, media.ID)
		cancel()
		if errors.Is(probeErr, ErrModel3DNotFound) {
			_ = cleanupModelBlob(ctx, blobStore, media.ObjectKey)
		}
		return domain.Model3DResource{}, fmt.Errorf("save 3D resource: %w", err)
	}
	return media, nil
}
func cleanupModelBlob(ctx context.Context, b BlobStore, key string) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return b.Delete(cleanupCtx, key)
}
func (s *ModelMediaService) Binding(ctx context.Context, actor Principal, kind, targetID string) (Model3DBinding, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return Model3DBinding{}, err
	}
	if kind != "model" && kind != "variant" && kind != "asset" {
		return Model3DBinding{}, NewInputError("validation.filter_invalid")
	}
	if err := validID("target ID", targetID); err != nil {
		return Model3DBinding{}, err
	}
	binding, err := s.store.GetModel3DBinding(ctx, actor.TenantID, kind, targetID)
	if err != nil {
		return Model3DBinding{}, err
	}
	if binding.EffectiveResourceID != "" {
		r, err := s.GetResource(ctx, actor, binding.EffectiveResourceID)
		if err != nil {
			return Model3DBinding{}, err
		}
		binding.Effective = &r.ProductModel3D
	}
	return binding, nil
}
func validateResourceMetadata(cmd UpdateModel3DResource) (domain.Model3DResource, error) {
	name, err := catalogText("resource name", cmd.Name, 200, true)
	if err != nil {
		return domain.Model3DResource{}, err
	}
	source, err := modelMediaURL(cmd.SourceURL)
	if err != nil {
		return domain.Model3DResource{}, err
	}
	author, err := catalogText("model author", cmd.Author, 200, false)
	if err != nil {
		return domain.Model3DResource{}, err
	}
	license, err := catalogText("model license", cmd.License, 200, false)
	if err != nil {
		return domain.Model3DResource{}, err
	}
	return domain.Model3DResource{Name: name, ProductModel3D: domain.ProductModel3D{SourceURL: source, Author: author, License: license}}, nil
}
func (s *ModelMediaService) UpdateResource(ctx context.Context, actor Principal, cmd UpdateModel3DResource) (domain.Model3DResource, error) {
	if err := actor.Require(CapabilityManageCatalog); err != nil {
		return domain.Model3DResource{}, err
	}
	r, err := s.GetResource(ctx, actor, cmd.ID)
	if err != nil {
		return domain.Model3DResource{}, err
	}
	if r.Status != "ready" {
		return domain.Model3DResource{}, ErrModel3DUnavailable
	}
	fields, err := validateResourceMetadata(cmd)
	if err != nil {
		return domain.Model3DResource{}, err
	}
	r.Name, r.SourceURL, r.Author, r.License = fields.Name, fields.SourceURL, fields.Author, fields.License
	r.UpdatedAt = s.now().UTC()
	if err := s.store.UpdateModel3DResource(ctx, r); err != nil {
		return domain.Model3DResource{}, err
	}
	return r, nil
}
func (s *ModelMediaService) GetResource(ctx context.Context, actor Principal, id string) (domain.Model3DResource, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return domain.Model3DResource{}, err
	}
	if err := validID("resource ID", id); err != nil {
		return domain.Model3DResource{}, err
	}
	return s.store.GetModel3DResource(ctx, actor.TenantID, id)
}
func (s *ModelMediaService) ListResources(ctx context.Context, actor Principal, opts Model3DResourceListOptions) (Model3DResourceListResult, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return Model3DResourceListResult{}, err
	}
	opts.Page, opts.PageSize = normalizePage(opts.Page, opts.PageSize)
	opts.Query = strings.TrimSpace(opts.Query)
	return s.store.ListModel3DResources(ctx, actor.TenantID, opts)
}
func (s *ModelMediaService) Bind(ctx context.Context, actor Principal, cmd BindModel3DResource) error {
	if err := actor.Require(CapabilityManageCatalog); err != nil {
		return err
	}
	if cmd.Kind != "model" && cmd.Kind != "variant" && cmd.Kind != "asset" {
		return NewInputError("validation.filter_invalid")
	}
	if err := validID("target ID", cmd.TargetID); err != nil {
		return err
	}
	if cmd.ResourceID != "" {
		r, err := s.GetResource(ctx, actor, cmd.ResourceID)
		if err != nil {
			return err
		}
		if r.Status != "ready" {
			return ErrModel3DUnavailable
		}
	}
	return s.store.BindModel3DResource(ctx, actor.TenantID, cmd)
}
func (s *ModelMediaService) References(ctx context.Context, actor Principal, id string) ([]Model3DReference, error) {
	if _, err := s.GetResource(ctx, actor, id); err != nil {
		return nil, err
	}
	return s.store.Model3DReferences(ctx, actor.TenantID, id)
}
func (s *ModelMediaService) DeleteResource(ctx context.Context, actor Principal, id string) error {
	if err := actor.Require(CapabilityManageCatalog); err != nil {
		return err
	}
	r, err := s.GetResource(ctx, actor, id)
	if err != nil {
		return err
	}
	// Atomic guarded transition blocks new bindings before any blob deletion.
	if err := s.store.MarkModel3DResourcePendingDelete(ctx, actor.TenantID, id); err != nil {
		return err
	}
	b, ok := s.stores.Get(r.StoreID)
	if !ok {
		return fmt.Errorf("blob store %q is not configured", r.StoreID)
	}
	if err := b.Delete(ctx, r.ObjectKey); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("delete GLB (retry pending resource): %w", err)
	}
	return s.store.FinishModel3DResourceDelete(ctx, actor.TenantID, id)
}
func (s *ModelMediaService) GetForModel(ctx context.Context, actor Principal, modelID string) (domain.ProductModel3D, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return domain.ProductModel3D{}, err
	}
	if err := validID("model ID", modelID); err != nil {
		return domain.ProductModel3D{}, err
	}
	m, err := s.store.GetProductModel(ctx, actor.TenantID, modelID)
	if err != nil {
		return domain.ProductModel3D{}, err
	}
	if m.Model3D == nil {
		return domain.ProductModel3D{}, ErrModel3DNotFound
	}
	return *m.Model3D, nil
}
func (s *ModelMediaService) ResolveForAsset(ctx context.Context, actor Principal, assetID string) (domain.ProductModel3D, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return domain.ProductModel3D{}, err
	}
	if err := validID("asset ID", assetID); err != nil {
		return domain.ProductModel3D{}, err
	}
	r, err := s.store.ResolveAssetModel3D(ctx, actor.TenantID, assetID)
	return r.ProductModel3D, err
}
func (s *ModelMediaService) OpenForAsset(ctx context.Context, actor Principal, assetID string) (OpenProductModel3D, error) {
	media, err := s.ResolveForAsset(ctx, actor, assetID)
	if err != nil {
		return OpenProductModel3D{}, err
	}
	return s.open(ctx, media)
}
func (s *ModelMediaService) OpenResource(ctx context.Context, actor Principal, id string) (OpenProductModel3D, error) {
	r, err := s.GetResource(ctx, actor, id)
	if err != nil {
		return OpenProductModel3D{}, err
	}
	if r.Status != "ready" {
		return OpenProductModel3D{}, ErrModel3DUnavailable
	}
	return s.open(ctx, r.ProductModel3D)
}
func (s *ModelMediaService) open(ctx context.Context, media domain.ProductModel3D) (OpenProductModel3D, error) {
	b, ok := s.stores.Get(media.StoreID)
	if !ok {
		return OpenProductModel3D{}, fmt.Errorf("blob store %q is not configured", media.StoreID)
	}
	reader, info, err := b.Open(ctx, media.ObjectKey)
	if err != nil {
		return OpenProductModel3D{}, err
	}
	return OpenProductModel3D{Reader: reader, Info: info, Model: media}, nil
}
func validateGLB(data []byte) error {
	if len(data) < 12 || string(data[:4]) != "glTF" {
		return errors.New("file is not a GLB")
	}
	if binary.LittleEndian.Uint32(data[4:8]) != 2 {
		return errors.New("only GLB v2 is supported")
	}
	if uint64(binary.LittleEndian.Uint32(data[8:12])) != uint64(len(data)) {
		return errors.New("GLB declared length does not match file length")
	}
	offset := 12
	if len(data) < offset+8 {
		return errors.New("GLB is missing its JSON chunk")
	}
	jsonLength := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
	if binary.LittleEndian.Uint32(data[offset+4:offset+8]) != 0x4e4f534a || jsonLength < 2 || offset+8+jsonLength > len(data) {
		return errors.New("GLB has an invalid JSON chunk")
	}
	var document struct {
		Buffers []struct {
			URI string `json:"uri"`
		} `json:"buffers"`
		Images []struct {
			URI string `json:"uri"`
		} `json:"images"`
	}
	if err := json.Unmarshal(bytes.TrimRight(data[offset+8:offset+8+jsonLength], " \x00"), &document); err != nil {
		return errors.New("GLB JSON chunk is invalid")
	}
	for _, resource := range append(document.Buffers, document.Images...) {
		if resource.URI != "" && !strings.HasPrefix(resource.URI, "data:") {
			return errors.New("GLB must be self-contained")
		}
	}
	return nil
}

func modelMediaURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len([]rune(value)) > 2000 {
		return "", errors.New("model source URL is too long")
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", errors.New("model source URL must be HTTP or HTTPS")
	}
	return value, nil
}
