package application

import (
	"context"
	"time"

	"github.com/SampsonFox/assetloop/internal/domain"
)

type Store interface {
	CreateAsset(context.Context, domain.Asset) (domain.Asset, error)
	GetAsset(context.Context, string, string) (domain.Asset, error)
}

type AuthStore interface {
	AuthNeedsSetup(context.Context) (bool, error)
	BootstrapAuth(context.Context, Tenant, User, Membership, SecurityEvent) error
	FindAccount(context.Context, string) (Account, error)
	FirstPrincipal(context.Context) (Principal, error)
	CreateSession(context.Context, Session) error
	GetSessionPrincipal(context.Context, string, time.Time) (Principal, error)
	DeleteSession(context.Context, string) error
	UpdateUserPreferences(context.Context, string, Locale, Theme) error
	CreateMember(context.Context, User, Membership, SecurityEvent) error
	ListMembers(context.Context, string, MemberListOptions) (MemberListResult, error)
	RecordSecurityEvent(context.Context, SecurityEvent) error
}

type CatalogStore interface {
	CreateCategory(context.Context, domain.ItemCategory) error
	UpdateCategory(context.Context, domain.ItemCategory) error
	CreateModel(context.Context, domain.ProductModel) error
	UpdateModel(context.Context, domain.ProductModel) error
	CreateVariant(context.Context, domain.ProductVariant) error
	UpdateVariant(context.Context, domain.ProductVariant) error
	DeleteVariant(context.Context, string, string) (bool, error)
	CreateCatalogAsset(context.Context, domain.Asset) error
	UpdateCatalogAsset(context.Context, domain.Asset) error
	ListCategories(context.Context, string) ([]domain.ItemCategory, error)
	ListModels(context.Context, string) ([]domain.ProductModel, error)
	ListVariants(context.Context, string) ([]domain.ProductVariant, error)
	ListAssets(context.Context, string) ([]domain.Asset, error)
	ListAssetsWithSummary(context.Context, string, AssetListOptions) (AssetListResult, error)
	ListModelsWithVariants(context.Context, string, ModelListOptions) (ModelListResult, error)
	GetAsset(context.Context, string, string) (domain.Asset, error)
}

type AssetListOptions struct {
	Query     string
	Status    string
	Sort      string
	Direction string
	Page      int
	PageSize  int
}

type AssetWithSummary struct {
	Asset   domain.Asset
	Summary domain.AssetSummary
}

type AssetListResult struct {
	Assets []AssetWithSummary
	Total  int
}

type ModelListOptions struct {
	Query      string
	CategoryID string
	Sort       string
	Direction  string
	Page       int
	PageSize   int
}

type ModelListResult struct {
	Models   []domain.ProductModel
	Variants []domain.ProductVariant
	Total    int
}

type MemberListOptions struct {
	Query     string
	Role      string
	Sort      string
	Direction string
	Page      int
	PageSize  int
}

type MemberListResult struct {
	Members []Member
	Total   int
}

type PortfolioSummary struct {
	AssetCount   int
	ExpenseMinor int64
	IncomeMinor  int64
	NetMinor     int64
	BaseCurrency string
}

type EventListOptions struct {
	Query      string
	Type       string
	Sort       string
	Direction  string
	ShowVoided bool
	Page       int
	PageSize   int
}

type EventListResult struct {
	Events  []domain.AssetEvent
	Summary domain.AssetSummary
	Total   int
}

type LifecycleStore interface {
	WithLifecycleWrite(context.Context, string, func(LifecycleStore) (domain.AssetEvent, error)) (domain.AssetEvent, error)
	FindLifecycleRequest(context.Context, string, string, string) (LifecycleRequest, bool, error)
	SaveLifecycleRequest(context.Context, LifecycleRequest) error
	GetAsset(context.Context, string, string) (domain.Asset, error)
	TenantBaseCurrency(context.Context, string) (string, bool, error)
	CreateAssetEventType(context.Context, domain.AssetEventTypeDefinition) error
	ListAssetEventTypes(context.Context, string) ([]domain.AssetEventTypeDefinition, error)
	AppendAssetEvent(context.Context, domain.AssetTransaction, domain.AssetEvent) error
	GetAssetEvent(context.Context, string, string) (domain.AssetEvent, error)
	ListAssetEvents(context.Context, string, string) ([]domain.AssetEvent, error)
	ListAssetEventsPage(context.Context, string, string, EventListOptions) ([]domain.AssetEvent, int, error)
	GetAssetSummary(context.Context, string, string) (domain.AssetSummary, error)
	GetPortfolioSummary(context.Context, string) (PortfolioSummary, error)
	CorrectAssetEvent(context.Context, domain.AssetTransaction, domain.AssetEvent, domain.AssetEvent) error
}

type LifecycleRequest struct {
	TenantID string
	UserID   string
	Key      string
	Hash     string
	EventID  string
}
