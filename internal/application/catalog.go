package application

import (
	"context"
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

const defaultAssetPageSize = 25

type CatalogSnapshot struct {
	Categories []domain.ItemCategory
	Models     []domain.ProductModel
	Variants   []domain.ProductVariant
	Assets     []domain.Asset
}

type CategoryIconOption struct {
	Key   string
	Label string
}

var CategoryIconOptions = []CategoryIconOption{
	{Key: "package", Label: "其他"},
	{Key: "smartphone", Label: "手机"},
	{Key: "laptop", Label: "电脑"},
	{Key: "tablet", Label: "平板"},
	{Key: "monitor", Label: "显示器"},
	{Key: "headphones", Label: "耳机"},
	{Key: "camera", Label: "相机"},
	{Key: "watch", Label: "手表"},
	{Key: "gamepad-2", Label: "游戏设备"},
	{Key: "car", Label: "汽车"},
	{Key: "bike", Label: "自行车"},
	{Key: "home", Label: "家居"},
	{Key: "book", Label: "书籍"},
	{Key: "wrench", Label: "工具"},
}

type CreateCategory struct {
	Name    string
	IconKey string
}

type UpdateCategory struct {
	ID      string
	Name    string
	IconKey string
}

type CreateModel struct {
	CategoryID string
	Name       string
}

type UpdateModel struct {
	ID         string
	CategoryID string
	Name       string
}

type CreateVariant struct {
	ModelID string
	Name    string
}

type UpdateVariant struct {
	ID      string
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

type UpdateCatalogAsset struct {
	ID              string
	VariantID       string
	DisplayName     string
	SerialNumber    string
	Color           string
	PurchaseChannel string
	Notes           string
}

var allowedAssetListStatuses = map[string]struct{}{
	"":           {},
	"all":        {},
	"unacquired": {},
	"active":     {},
	"sold":       {},
	"repairing":  {},
}

var allowedAssetSorts = map[string]struct{}{
	"created": {}, "name": {}, "model": {}, "status": {}, "net": {}, "cost": {},
}

var allowedModelSorts = map[string]struct{}{
	"category": {}, "name": {}, "created": {},
}

func NewCatalogService(store CatalogStore) *CatalogService {
	return &CatalogService{store: store, now: time.Now}
}

func (s *CatalogService) CreateCategory(ctx context.Context, actor Principal, cmd CreateCategory) (domain.ItemCategory, error) {
	if err := actor.Require(CapabilityManageCatalog); err != nil {
		return domain.ItemCategory{}, err
	}
	name, err := catalogText("category name", cmd.Name, 120, true)
	if err != nil {
		return domain.ItemCategory{}, err
	}
	iconKey, err := categoryIcon(cmd.IconKey)
	if err != nil {
		return domain.ItemCategory{}, err
	}
	category := domain.ItemCategory{ID: newID(), TenantID: actor.TenantID, Name: name, IconKey: iconKey, CreatedAt: s.now().UTC()}
	if err := s.store.CreateCategory(ctx, category); err != nil {
		return domain.ItemCategory{}, fmt.Errorf("create category: %w", err)
	}
	return category, nil
}

func (s *CatalogService) UpdateCategory(ctx context.Context, actor Principal, cmd UpdateCategory) (domain.ItemCategory, error) {
	if err := actor.Require(CapabilityManageCatalog); err != nil {
		return domain.ItemCategory{}, err
	}
	if err := validID("category ID", cmd.ID); err != nil {
		return domain.ItemCategory{}, err
	}
	name, err := catalogText("category name", cmd.Name, 120, true)
	if err != nil {
		return domain.ItemCategory{}, err
	}
	iconKey, err := categoryIcon(cmd.IconKey)
	if err != nil {
		return domain.ItemCategory{}, err
	}
	category := domain.ItemCategory{ID: cmd.ID, TenantID: actor.TenantID, Name: name, IconKey: iconKey}
	if err := s.store.UpdateCategory(ctx, category); err != nil {
		return domain.ItemCategory{}, fmt.Errorf("update category: %w", err)
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

func (s *CatalogService) UpdateModel(ctx context.Context, actor Principal, cmd UpdateModel) (domain.ProductModel, error) {
	if err := actor.Require(CapabilityManageCatalog); err != nil {
		return domain.ProductModel{}, err
	}
	if err := validID("model ID", cmd.ID); err != nil {
		return domain.ProductModel{}, err
	}
	if err := validID("category ID", cmd.CategoryID); err != nil {
		return domain.ProductModel{}, err
	}
	name, err := catalogText("model name", cmd.Name, 160, true)
	if err != nil {
		return domain.ProductModel{}, err
	}
	model := domain.ProductModel{ID: cmd.ID, TenantID: actor.TenantID, CategoryID: cmd.CategoryID, Name: name}
	if err := s.store.UpdateModel(ctx, model); err != nil {
		return domain.ProductModel{}, fmt.Errorf("update model: %w", err)
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

func (s *CatalogService) UpdateVariant(ctx context.Context, actor Principal, cmd UpdateVariant) (domain.ProductVariant, error) {
	if err := actor.Require(CapabilityManageCatalog); err != nil {
		return domain.ProductVariant{}, err
	}
	if err := validID("variant ID", cmd.ID); err != nil {
		return domain.ProductVariant{}, err
	}
	if err := validID("model ID", cmd.ModelID); err != nil {
		return domain.ProductVariant{}, err
	}
	name, err := catalogText("variant name", cmd.Name, 160, true)
	if err != nil {
		return domain.ProductVariant{}, err
	}
	variant := domain.ProductVariant{ID: cmd.ID, TenantID: actor.TenantID, ModelID: cmd.ModelID, Name: name}
	if err := s.store.UpdateVariant(ctx, variant); err != nil {
		return domain.ProductVariant{}, fmt.Errorf("update variant: %w", err)
	}
	return variant, nil
}

func (s *CatalogService) DeleteVariant(ctx context.Context, actor Principal, variantID string) error {
	if err := actor.Require(CapabilityManageCatalog); err != nil {
		return err
	}
	if err := validID("variant ID", variantID); err != nil {
		return err
	}
	deleted, err := s.store.DeleteVariant(ctx, actor.TenantID, variantID)
	if err != nil {
		return fmt.Errorf("delete variant: %w", err)
	}
	if !deleted {
		return NewInputError("validation.variant_in_use")
	}
	return nil
}

func (s *CatalogService) CreateAsset(ctx context.Context, actor Principal, cmd CreateCatalogAsset) (domain.Asset, error) {
	if err := actor.Require(CapabilityManageCatalog); err != nil {
		return domain.Asset{}, err
	}
	values, err := validateAssetFields(cmd.VariantID, cmd.DisplayName, cmd.SerialNumber, cmd.Color, cmd.PurchaseChannel, cmd.Notes)
	if err != nil {
		return domain.Asset{}, err
	}
	asset := domain.Asset{
		ID: newID(), TenantID: actor.TenantID, VariantID: values.VariantID,
		DisplayName: values.DisplayName, SerialNumber: values.SerialNumber, Color: values.Color,
		PurchaseChannel: values.PurchaseChannel, Notes: values.Notes, CreatedAt: s.now().UTC(),
	}
	if err := s.store.CreateCatalogAsset(ctx, asset); err != nil {
		return domain.Asset{}, fmt.Errorf("create asset: %w", err)
	}
	return s.GetAsset(ctx, actor, asset.ID)
}

func categoryIcon(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "package", nil
	}
	for _, option := range CategoryIconOptions {
		if value == option.Key {
			return value, nil
		}
	}
	return "", NewInputError("validation.category_icon")
}

func validateAssetFields(variantID, displayName, serialNumber, color, purchaseChannel, notes string) (domain.Asset, error) {
	if err := validID("variant ID", variantID); err != nil {
		return domain.Asset{}, err
	}
	values := domain.Asset{VariantID: strings.TrimSpace(variantID)}
	fields := []struct {
		label    string
		value    string
		maxRunes int
		required bool
		target   *string
	}{
		{"display name", displayName, 200, true, &values.DisplayName},
		{"serial number", serialNumber, 200, false, &values.SerialNumber},
		{"color", color, 120, false, &values.Color},
		{"purchase channel", purchaseChannel, 160, false, &values.PurchaseChannel},
		{"notes", notes, 2000, false, &values.Notes},
	}
	for _, field := range fields {
		normalized, err := catalogText(field.label, field.value, field.maxRunes, field.required)
		if err != nil {
			return domain.Asset{}, err
		}
		*field.target = normalized
	}
	return values, nil
}

func (s *CatalogService) UpdateAsset(ctx context.Context, actor Principal, cmd UpdateCatalogAsset) (domain.Asset, error) {
	if err := actor.Require(CapabilityManageCatalog); err != nil {
		return domain.Asset{}, err
	}
	if err := validID("asset ID", cmd.ID); err != nil {
		return domain.Asset{}, err
	}
	values, err := validateAssetFields(cmd.VariantID, cmd.DisplayName, cmd.SerialNumber, cmd.Color, cmd.PurchaseChannel, cmd.Notes)
	if err != nil {
		return domain.Asset{}, err
	}
	asset := domain.Asset{ID: cmd.ID, TenantID: actor.TenantID, VariantID: values.VariantID, DisplayName: values.DisplayName, SerialNumber: values.SerialNumber, Color: values.Color, PurchaseChannel: values.PurchaseChannel, Notes: values.Notes}
	if err := s.store.UpdateCatalogAsset(ctx, asset); err != nil {
		return domain.Asset{}, fmt.Errorf("update asset: %w", err)
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

func (s *CatalogService) Categories(ctx context.Context, actor Principal) ([]domain.ItemCategory, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return nil, err
	}
	categories, err := s.store.ListCategories(ctx, actor.TenantID)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	return categories, nil
}

func (s *CatalogService) ListAssetsWithSummary(ctx context.Context, actor Principal, opts AssetListOptions) (AssetListResult, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return AssetListResult{}, err
	}
	opts.Page, opts.PageSize = normalizePage(opts.Page, opts.PageSize)
	opts.Query = strings.TrimSpace(opts.Query)
	opts.Status = strings.TrimSpace(opts.Status)
	if _, allowed := allowedAssetListStatuses[opts.Status]; !allowed {
		return AssetListResult{}, NewInputError("validation.filter_invalid")
	}
	var err error
	opts.Sort, opts.Direction, err = normalizeSort(opts.Sort, opts.Direction, "created", "desc", allowedAssetSorts)
	if err != nil {
		return AssetListResult{}, err
	}
	return s.store.ListAssetsWithSummary(ctx, actor.TenantID, opts)
}

func (s *CatalogService) ListModelsWithVariants(ctx context.Context, actor Principal, opts ModelListOptions) (ModelListResult, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return ModelListResult{}, err
	}
	opts.Page, opts.PageSize = normalizePage(opts.Page, opts.PageSize)
	opts.Query = strings.TrimSpace(opts.Query)
	opts.CategoryID = strings.TrimSpace(opts.CategoryID)
	if opts.CategoryID != "" {
		if err := validID("category ID", opts.CategoryID); err != nil {
			return ModelListResult{}, NewInputError("validation.filter_invalid")
		}
	}
	var err error
	opts.Sort, opts.Direction, err = normalizeSort(opts.Sort, opts.Direction, "category", "asc", allowedModelSorts)
	if err != nil {
		return ModelListResult{}, err
	}
	return s.store.ListModelsWithVariants(ctx, actor.TenantID, opts)
}

func normalizePage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = defaultAssetPageSize
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}

func normalizeSort(sort, direction, defaultSort, defaultDirection string, allowed map[string]struct{}) (string, string, error) {
	sort = strings.TrimSpace(strings.ToLower(sort))
	direction = strings.TrimSpace(strings.ToLower(direction))
	if sort == "" {
		sort = defaultSort
	}
	if direction == "" {
		direction = defaultDirection
	}
	if _, ok := allowed[sort]; !ok || (direction != "asc" && direction != "desc") {
		return "", "", NewInputError("validation.filter_invalid")
	}
	return sort, direction, nil
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
		return NewInputError("validation.id_invalid", label)
	}
	return nil
}

func catalogText(label, value string, maxRunes int, required bool) (string, error) {
	value = strings.TrimSpace(value)
	if required && value == "" {
		return "", NewInputError("validation.field_required", label)
	}
	if len([]rune(value)) > maxRunes {
		return "", NewInputError("validation.field_too_long", label, maxRunes)
	}
	return value, nil
}
