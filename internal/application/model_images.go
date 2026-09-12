package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/url"
	"time"

	"github.com/SampsonFox/assetloop/internal/domain"
	_ "golang.org/x/image/webp"
)

const MaxModelImageBytes = 8 << 20

var ErrImageNotFound = errors.New("model image not found")

// Image bytes are immutable; clearing/replacing only detaches the old revision.
type ModelImage struct {
	ID, TenantID, ModelID, StoreID, ObjectKey, SHA256, ContentType, SourceURL string
	SizeBytes                                                                 int64
}
type ModelImageStore interface {
	GetProductModel(context.Context, string, string) (domain.ProductModel, error)
	GetModelImage(context.Context, string, string) (ModelImage, error)
	GetImageRevision(context.Context, string, string) (ModelImage, error)
	InsertModelImage(context.Context, ModelImage) error
	ClearModelImage(context.Context, string, string) error
	WithImageWrite(context.Context, string, func(ModelImageStore) error) error
}
type ModelImageKeys interface {
	ModelImage(string, string, string) (string, error)
}
type ModelImageService struct {
	store        ModelImageStore
	blobs        BlobStores
	keys         ModelImageKeys
	defaultStore string
	slots        chan struct{}
}

func NewModelImageService(s ModelImageStore, b BlobStores, k ModelImageKeys, d string) *ModelImageService {
	return &ModelImageService{s, b, k, d, make(chan struct{}, 2)}
}

type ImageDownloader interface {
	DownloadImage(context.Context, string) ([]byte, error)
}

func (s *ModelImageService) Import(ctx context.Context, a Principal, model, raw, source string, d ImageDownloader) (ModelImage, error) {
	if err := a.Require(CapabilityManageCatalog); err != nil {
		return ModelImage{}, err
	}
	if _, err := s.Model(ctx, a, model); err != nil {
		return ModelImage{}, err
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return ModelImage{}, NewInputError("image.unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	data, err := d.DownloadImage(ctx, raw)
	if err != nil {
		return ModelImage{}, NewInputError("image.download_failed")
	}
	return s.Upload(ctx, a, model, data, source)
}
func (s *ModelImageService) Get(ctx context.Context, a Principal, model string) (ModelImage, error) {
	if err := a.Require(CapabilityView); err != nil {
		return ModelImage{}, err
	}
	if err := validID("model ID", model); err != nil {
		return ModelImage{}, err
	}
	return s.store.GetModelImage(ctx, a.TenantID, model)
}

func (s *ModelImageService) Model(ctx context.Context, a Principal, model string) (domain.ProductModel, error) {
	if err := a.Require(CapabilityView); err != nil {
		return domain.ProductModel{}, err
	}
	if err := validID("model ID", model); err != nil {
		return domain.ProductModel{}, err
	}
	return s.store.GetProductModel(ctx, a.TenantID, model)
}
func (s *ModelImageService) Open(ctx context.Context, a Principal, model string) (io.ReadCloser, ModelImage, error) {
	m, err := s.Get(ctx, a, model)
	if err != nil {
		return nil, m, err
	}
	b, ok := s.blobs.Get(m.StoreID)
	if !ok {
		return nil, m, ErrModel3DUnavailable
	}
	r, _, err := b.Open(ctx, m.ObjectKey)
	return r, m, err
}
func validateModelImage(data []byte) (string, error) {
	if len(data) == 0 || len(data) > MaxModelImageBytes {
		return "", NewInputError("image.invalid")
	}
	c, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png" && format != "webp") || c.Width < 1 || c.Height < 1 || int64(c.Width)*int64(c.Height) > 16000000 {
		return "", NewInputError("image.invalid")
	}
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		return "", NewInputError("image.invalid")
	}
	return "image/" + format, nil
}
func (s *ModelImageService) Upload(ctx context.Context, a Principal, model string, data []byte, source string) (ModelImage, error) {
	if err := a.Require(CapabilityManageCatalog); err != nil {
		return ModelImage{}, err
	}
	if _, err := s.store.GetProductModel(ctx, a.TenantID, model); err != nil {
		return ModelImage{}, err
	}
	mime, err := validateModelImage(data)
	if err != nil {
		return ModelImage{}, err
	}
	source, err = modelMediaURL(source)
	if err != nil {
		return ModelImage{}, NewInputError("image.invalid_source")
	}
	if u, _ := url.Parse(source); u != nil && u.User != nil {
		return ModelImage{}, NewInputError("image.invalid_source")
	}
	hash := sha256.Sum256(data)
	m := ModelImage{ID: newID(), TenantID: a.TenantID, ModelID: model, StoreID: s.defaultStore, SHA256: hex.EncodeToString(hash[:]), ContentType: mime, SizeBytes: int64(len(data)), SourceURL: source}
	m.ObjectKey, err = s.keys.ModelImage(a.TenantID, m.ID, m.SHA256)
	if err != nil {
		return ModelImage{}, err
	}
	b, ok := s.blobs.Get(m.StoreID)
	if !ok {
		return ModelImage{}, ErrModel3DUnavailable
	}
	if err = b.Put(ctx, m.ObjectKey, bytes.NewReader(data), BlobMetadata{ContentType: mime}); err != nil {
		_ = cleanupModelBlob(ctx, b, m.ObjectKey)
		return ModelImage{}, err
	}
	r, info, err := b.Open(ctx, m.ObjectKey)
	if err != nil {
		_ = cleanupModelBlob(ctx, b, m.ObjectKey)
		return ModelImage{}, err
	}
	h := sha256.New()
	n, readErr := io.Copy(h, io.LimitReader(r, m.SizeBytes+1))
	closeErr := r.Close()
	if readErr != nil || closeErr != nil || info.Size != m.SizeBytes || n != m.SizeBytes || hex.EncodeToString(h.Sum(nil)) != m.SHA256 {
		_ = cleanupModelBlob(ctx, b, m.ObjectKey)
		return ModelImage{}, ErrModel3DUnavailable
	}
	err = s.store.WithImageWrite(ctx, a.TenantID, func(tx ModelImageStore) error {
		if _, e := tx.GetProductModel(ctx, a.TenantID, model); e != nil {
			return e
		}
		if e := tx.ClearModelImage(ctx, a.TenantID, model); e != nil {
			return e
		}
		return tx.InsertModelImage(ctx, m)
	})
	if err != nil {
		probe, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, e := s.store.GetImageRevision(probe, a.TenantID, m.ID); errors.Is(e, ErrImageNotFound) {
			_ = cleanupModelBlob(ctx, b, m.ObjectKey)
		}
		return ModelImage{}, err
	}
	return m, nil
}
func (s *ModelImageService) Clear(ctx context.Context, a Principal, model string) error {
	if err := a.Require(CapabilityManageCatalog); err != nil {
		return err
	}
	return s.store.WithImageWrite(ctx, a.TenantID, func(tx ModelImageStore) error {
		if _, err := tx.GetProductModel(ctx, a.TenantID, model); err != nil {
			return err
		}
		return tx.ClearModelImage(ctx, a.TenantID, model)
	})
}
