package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/google/uuid"
)

type CatalogService struct {
	store CatalogStore
	now   func() time.Time
}

type CatalogSnapshot struct {
	Categories []domain.ItemCategory
	Models     []domain.ProductModel
	Variants   []domain.ProductVariant
	Assets     []domain.Asset
}

type CreateModel struct {
	CategoryID string
	Name       string
}

type CreateVariant struct {
	ModelID string
	Name    string
}

type CreateCatalogAsset struct {
	VariantID       string
	DisplayName     string
	SerialNumber    string
	Color           string
	PurchaseChannel string
	Notes           string
}

func NewCatalogService(store CatalogStore) *CatalogService {
	return &CatalogService{store: store, now: time.Now}
}

func (s *CatalogService) CreateCategory(ctx context.Context, actor Principal, name string) (domain.ItemCategory, error) {
	if err := actor.Require(CapabilityManageCatalog); err != nil {
		return domain.ItemCategory{}, err
	}
	name, err := catalogText("category name", name, 120, true)
	if err != nil {
		return domain.ItemCategory{}, err
	}
	category := domain.ItemCategory{ID: newID(), TenantID: actor.TenantID, Name: name, CreatedAt: s.now().UTC()}
	if err := s.store.CreateCategory(ctx, category); err != nil {
		return domain.ItemCategory{}, fmt.Errorf("create category: %w", err)
	}
	return category, nil
}

func (s *CatalogService) CreateModel(ctx context.Context, actor Principal, cmd CreateModel) (domain.ProductModel, error) {
	if err := actor.Require(CapabilityManageCatalog); err != nil {
		return domain.ProductModel{}, err
	}
	if err := validID("category ID", cmd.CategoryID); err != nil {
		return domain.ProductModel{}, err
	}
	name, err := catalogText("model name", cmd.Name, 160, true)
	if err != nil {
		return domain.ProductModel{}, err
	}
	model := domain.ProductModel{ID: newID(), TenantID: actor.TenantID, CategoryID: cmd.CategoryID, Name: name, CreatedAt: s.now().UTC()}
	if err := s.store.CreateModel(ctx, model); err != nil {
		return domain.ProductModel{}, fmt.Errorf("create model: %w", err)
	}
	return model, nil
}

func (s *CatalogService) CreateVariant(ctx context.Context, actor Principal, cmd CreateVariant) (domain.ProductVariant, error) {
	if err := actor.Require(CapabilityManageCatalog); err != nil {
		return domain.ProductVariant{}, err
	}
	if err := validID("model ID", cmd.ModelID); err != nil {
		return domain.ProductVariant{}, err
	}
	name, err := catalogText("variant name", cmd.Name, 160, true)
	if err != nil {
		return domain.ProductVariant{}, err
	}
	variant := domain.ProductVariant{ID: newID(), TenantID: actor.TenantID, ModelID: cmd.ModelID, Name: name, CreatedAt: s.now().UTC()}
	if err := s.store.CreateVariant(ctx, variant); err != nil {
		return domain.ProductVariant{}, fmt.Errorf("create variant: %w", err)
	}
	return variant, nil
}

func (s *CatalogService) CreateAsset(ctx context.Context, actor Principal, cmd CreateCatalogAsset) (domain.Asset, error) {
	if err := actor.Require(CapabilityManageCatalog); err != nil {
		return domain.Asset{}, err
	}
	if err := validID("variant ID", cmd.VariantID); err != nil {
		return domain.Asset{}, err
	}
	displayName, err := catalogText("display name", cmd.DisplayName, 200, true)
	if err != nil {
		return domain.Asset{}, err
	}
	serial, err := catalogText("serial number", cmd.SerialNumber, 200, false)
	if err != nil {
		return domain.Asset{}, err
	}
	color, err := catalogText("color", cmd.Color, 120, false)
	if err != nil {
		return domain.Asset{}, err
	}
	channel, err := catalogText("purchase channel", cmd.PurchaseChannel, 160, false)
	if err != nil {
		return domain.Asset{}, err
	}
	notes, err := catalogText("notes", cmd.Notes, 2000, false)
	if err != nil {
		return domain.Asset{}, err
	}
	asset := domain.Asset{
		ID: newID(), TenantID: actor.TenantID, VariantID: cmd.VariantID,
		DisplayName: displayName, SerialNumber: serial, Color: color,
		PurchaseChannel: channel, Notes: notes, CreatedAt: s.now().UTC(),
	}
	if err := s.store.CreateCatalogAsset(ctx, asset); err != nil {
		return domain.Asset{}, fmt.Errorf("create asset: %w", err)
	}
	return s.GetAsset(ctx, actor, asset.ID)
}

func (s *CatalogService) Snapshot(ctx context.Context, actor Principal) (CatalogSnapshot, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return CatalogSnapshot{}, err
	}
	categories, err := s.store.ListCategories(ctx, actor.TenantID)
	if err != nil {
		return CatalogSnapshot{}, fmt.Errorf("list categories: %w", err)
	}
	models, err := s.store.ListModels(ctx, actor.TenantID)
	if err != nil {
		return CatalogSnapshot{}, fmt.Errorf("list models: %w", err)
	}
	variants, err := s.store.ListVariants(ctx, actor.TenantID)
	if err != nil {
		return CatalogSnapshot{}, fmt.Errorf("list variants: %w", err)
	}
	assets, err := s.store.ListAssets(ctx, actor.TenantID)
	if err != nil {
		return CatalogSnapshot{}, fmt.Errorf("list assets: %w", err)
	}
	return CatalogSnapshot{Categories: categories, Models: models, Variants: variants, Assets: assets}, nil
}

func (s *CatalogService) GetAsset(ctx context.Context, actor Principal, assetID string) (domain.Asset, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return domain.Asset{}, err
	}
	if err := validID("asset ID", assetID); err != nil {
		return domain.Asset{}, err
	}
	asset, err := s.store.GetAsset(ctx, actor.TenantID, assetID)
	if err != nil {
		return domain.Asset{}, fmt.Errorf("get asset: %w", err)
	}
	return asset, nil
}

func validID(label, value string) error {
	if _, err := uuid.Parse(strings.TrimSpace(value)); err != nil {
		return fmt.Errorf("%s must be a UUID", label)
	}
	return nil
}

func catalogText(label, value string, maxRunes int, required bool) (string, error) {
	value = strings.TrimSpace(value)
	if required && value == "" {
		return "", errors.New(label + " is required")
	}
	if len([]rune(value)) > maxRunes {
		return "", fmt.Errorf("%s is too long", label)
	}
	return value, nil
}
