package application

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/google/uuid"
)

type AssetService struct {
	store Store
	now   func() time.Time
}

type CreateAsset struct {
	TenantID    string
	Category    string
	Model       string
	Variant     string
	DisplayName string
}

func NewAssetService(store Store) *AssetService {
	return &AssetService{store: store, now: time.Now}
}

func (s *AssetService) Create(ctx context.Context, cmd CreateAsset) (domain.Asset, error) {
	if strings.TrimSpace(cmd.TenantID) == "" {
		return domain.Asset{}, errors.New("tenant ID is required")
	}
	if _, err := uuid.Parse(cmd.TenantID); err != nil {
		return domain.Asset{}, errors.New("tenant ID must be a UUID")
	}
	if strings.TrimSpace(cmd.Category) == "" || strings.TrimSpace(cmd.Model) == "" || strings.TrimSpace(cmd.Variant) == "" {
		return domain.Asset{}, errors.New("category, model, and variant are required")
	}
	asset := domain.Asset{
		ID:          newID(),
		TenantID:    cmd.TenantID,
		CategoryID:  newID(),
		Category:    strings.TrimSpace(cmd.Category),
		ModelID:     newID(),
		Model:       strings.TrimSpace(cmd.Model),
		VariantID:   newID(),
		Variant:     strings.TrimSpace(cmd.Variant),
		DisplayName: strings.TrimSpace(cmd.DisplayName),
		CreatedAt:   s.now().UTC(),
	}
	if asset.DisplayName == "" {
		asset.DisplayName = asset.Model + " " + asset.Variant
	}
	asset, err := s.store.CreateAsset(ctx, asset)
	if err != nil {
		return domain.Asset{}, fmt.Errorf("create asset: %w", err)
	}
	return asset, nil
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
