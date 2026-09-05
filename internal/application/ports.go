package application

import (
	"context"
	"io"
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
	CreateMember(context.Context, User, Membership, SecurityEvent) error
	ListMembers(context.Context, string) ([]Member, error)
	RecordSecurityEvent(context.Context, SecurityEvent) error
}

type CatalogStore interface {
	CreateCategory(context.Context, domain.ItemCategory) error
	UpdateCategory(context.Context, domain.ItemCategory) error
	CreateModel(context.Context, domain.ProductModel) error
	UpdateModel(context.Context, domain.ProductModel) error
	CreateVariant(context.Context, domain.ProductVariant) error
	UpdateVariant(context.Context, domain.ProductVariant) error
	CreateCatalogAsset(context.Context, domain.Asset) error
	UpdateCatalogAsset(context.Context, domain.Asset) error
	ListCategories(context.Context, string) ([]domain.ItemCategory, error)
	ListModels(context.Context, string) ([]domain.ProductModel, error)
	ListVariants(context.Context, string) ([]domain.ProductVariant, error)
	ListAssets(context.Context, string) ([]domain.Asset, error)
	GetAsset(context.Context, string, string) (domain.Asset, error)
}

type ModelMediaStore interface {
	GetAsset(context.Context, string, string) (domain.Asset, error)
	GetProductModel(context.Context, string, string) (domain.ProductModel, error)
	UpdateProductModel3D(context.Context, string, string, domain.ProductModel3D) error
}

type BlobMetadata struct{ ContentType string }
type BlobInfo struct{ Size int64 }

type BlobStore interface {
	Put(context.Context, string, io.Reader, BlobMetadata) error
	Open(context.Context, string) (io.ReadCloser, BlobInfo, error)
	Stat(context.Context, string) (BlobInfo, error)
	Delete(context.Context, string) error
}

type BlobStores interface {
	Get(string) (BlobStore, bool)
}

type ObjectKeyMapper interface {
	ProductModel3D(string, string, string) (string, error)
}

type LifecycleStore interface {
	GetAsset(context.Context, string, string) (domain.Asset, error)
	TenantBaseCurrency(context.Context, string) (string, bool, error)
	AppendAssetEvent(context.Context, domain.AssetTransaction, domain.AssetEvent) error
	GetAssetEvent(context.Context, string, string) (domain.AssetEvent, error)
	ListAssetEvents(context.Context, string, string) ([]domain.AssetEvent, error)
	CorrectAssetEvent(context.Context, domain.AssetTransaction, domain.AssetEvent, domain.AssetEvent) error
	CreateImportDraft(context.Context, domain.ImportDraft) error
	ListPendingImportDrafts(context.Context, string) ([]domain.ImportDraft, error)
	GetImportDraft(context.Context, string, string) (domain.ImportDraft, error)
	ConfirmImportDraft(context.Context, string, domain.AssetTransaction, domain.AssetEvent) error
}
