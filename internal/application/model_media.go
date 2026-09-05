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
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/SampsonFox/assetloop/internal/domain"
)

const MaxProductModel3DBytes int64 = 25 << 20

var ErrModel3DNotFound = errors.New("3D model not found")

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

type OpenProductModel3D struct {
	Reader io.ReadCloser
	Info   BlobInfo
	Model  domain.ProductModel3D
}

func NewModelMediaService(store ModelMediaStore, stores BlobStores, keys ObjectKeyMapper, defaultStore string) *ModelMediaService {
	return &ModelMediaService{store: store, stores: stores, keys: keys, defaultStore: defaultStore, now: time.Now}
}

func (s *ModelMediaService) Update(ctx context.Context, actor Principal, cmd UpdateProductModel3D) (domain.ProductModel3D, error) {
	if err := actor.Require(CapabilityManageCatalog); err != nil {
		return domain.ProductModel3D{}, err
	}
	if err := validID("model ID", cmd.ModelID); err != nil {
		return domain.ProductModel3D{}, err
	}
	model, err := s.store.GetProductModel(ctx, actor.TenantID, cmd.ModelID)
	if err != nil {
		return domain.ProductModel3D{}, fmt.Errorf("get product model: %w", err)
	}
	sourceURL, err := modelMediaURL(cmd.SourceURL)
	if err != nil {
		return domain.ProductModel3D{}, err
	}
	author, err := catalogText("model author", cmd.Author, 200, false)
	if err != nil {
		return domain.ProductModel3D{}, err
	}
	license, err := catalogText("model license", cmd.License, 200, false)
	if err != nil {
		return domain.ProductModel3D{}, err
	}

	media := domain.ProductModel3D{SourceURL: sourceURL, Author: author, License: license, UpdatedAt: s.now().UTC()}
	if model.Model3D != nil {
		media.StoreID, media.ObjectKey, media.SHA256, media.SizeBytes = model.Model3D.StoreID, model.Model3D.ObjectKey, model.Model3D.SHA256, model.Model3D.SizeBytes
	}
	if len(cmd.File) == 0 {
		if model.Model3D == nil {
			return domain.ProductModel3D{}, errors.New("select a GLB file")
		}
		if err := s.store.UpdateProductModel3D(ctx, actor.TenantID, cmd.ModelID, media); err != nil {
			return domain.ProductModel3D{}, fmt.Errorf("update model metadata: %w", err)
		}
		return media, nil
	}
	if int64(len(cmd.File)) > MaxProductModel3DBytes {
		return domain.ProductModel3D{}, errors.New("GLB file exceeds 25 MiB")
	}
	if err := validateGLB(cmd.File); err != nil {
		return domain.ProductModel3D{}, err
	}
	digest := sha256.Sum256(cmd.File)
	media.SHA256 = hex.EncodeToString(digest[:])
	media.SizeBytes = int64(len(cmd.File))
	media.StoreID = s.defaultStore
	media.ObjectKey, err = s.keys.ProductModel3D(actor.TenantID, cmd.ModelID, media.SHA256)
	if err != nil {
		return domain.ProductModel3D{}, err
	}
	blobStore, ok := s.stores.Get(media.StoreID)
	if !ok {
		return domain.ProductModel3D{}, fmt.Errorf("blob store %q is not configured", media.StoreID)
	}
	if err := blobStore.Put(ctx, media.ObjectKey, bytes.NewReader(cmd.File), BlobMetadata{ContentType: "model/gltf-binary"}); err != nil {
		return domain.ProductModel3D{}, fmt.Errorf("store GLB: %w", err)
	}
	info, err := blobStore.Stat(ctx, media.ObjectKey)
	if err != nil || info.Size != media.SizeBytes {
		_ = blobStore.Delete(ctx, media.ObjectKey)
		if err != nil {
			return domain.ProductModel3D{}, fmt.Errorf("verify GLB: %w", err)
		}
		return domain.ProductModel3D{}, errors.New("stored GLB size mismatch")
	}
	stored, _, err := blobStore.Open(ctx, media.ObjectKey)
	if err != nil {
		_ = blobStore.Delete(ctx, media.ObjectKey)
		return domain.ProductModel3D{}, fmt.Errorf("verify GLB checksum: %w", err)
	}
	storedHash := sha256.New()
	_, copyErr := io.Copy(storedHash, stored)
	closeErr := stored.Close()
	if copyErr != nil || closeErr != nil || hex.EncodeToString(storedHash.Sum(nil)) != media.SHA256 {
		_ = blobStore.Delete(ctx, media.ObjectKey)
		return domain.ProductModel3D{}, errors.New("stored GLB checksum mismatch")
	}
	if err := s.store.UpdateProductModel3D(ctx, actor.TenantID, cmd.ModelID, media); err != nil {
		_ = blobStore.Delete(ctx, media.ObjectKey)
		return domain.ProductModel3D{}, fmt.Errorf("bind GLB: %w", err)
	}
	if model.Model3D != nil && (model.Model3D.StoreID != media.StoreID || model.Model3D.ObjectKey != media.ObjectKey) {
		if oldStore, ok := s.stores.Get(model.Model3D.StoreID); ok {
			_ = oldStore.Delete(ctx, model.Model3D.ObjectKey)
		}
	}
	return media, nil
}

func (s *ModelMediaService) GetForModel(ctx context.Context, actor Principal, modelID string) (domain.ProductModel3D, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return domain.ProductModel3D{}, err
	}
	if err := validID("model ID", modelID); err != nil {
		return domain.ProductModel3D{}, err
	}
	model, err := s.store.GetProductModel(ctx, actor.TenantID, modelID)
	if err != nil {
		return domain.ProductModel3D{}, err
	}
	if model.Model3D == nil {
		return domain.ProductModel3D{}, ErrModel3DNotFound
	}
	return *model.Model3D, nil
}

func (s *ModelMediaService) OpenForAsset(ctx context.Context, actor Principal, assetID string) (OpenProductModel3D, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return OpenProductModel3D{}, err
	}
	if err := validID("asset ID", assetID); err != nil {
		return OpenProductModel3D{}, err
	}
	asset, err := s.store.GetAsset(ctx, actor.TenantID, assetID)
	if err != nil {
		return OpenProductModel3D{}, err
	}
	model, err := s.store.GetProductModel(ctx, actor.TenantID, asset.ModelID)
	if err != nil {
		return OpenProductModel3D{}, err
	}
	if model.Model3D == nil {
		return OpenProductModel3D{}, ErrModel3DNotFound
	}
	blobStore, ok := s.stores.Get(model.Model3D.StoreID)
	if !ok {
		return OpenProductModel3D{}, fmt.Errorf("blob store %q is not configured", model.Model3D.StoreID)
	}
	reader, info, err := blobStore.Open(ctx, model.Model3D.ObjectKey)
	if err != nil {
		return OpenProductModel3D{}, err
	}
	return OpenProductModel3D{Reader: reader, Info: info, Model: *model.Model3D}, nil
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
