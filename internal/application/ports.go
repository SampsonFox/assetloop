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
	CreateMember(context.Context, User, Membership, SecurityEvent) error
	ListMembers(context.Context, string) ([]Member, error)
	RecordSecurityEvent(context.Context, SecurityEvent) error
}

type CatalogStore interface {
	CreateCategory(context.Context, domain.ItemCategory) error
	CreateModel(context.Context, domain.ProductModel) error
	CreateVariant(context.Context, domain.ProductVariant) error
	CreateCatalogAsset(context.Context, domain.Asset) error
	ListCategories(context.Context, string) ([]domain.ItemCategory, error)
	ListModels(context.Context, string) ([]domain.ProductModel, error)
	ListVariants(context.Context, string) ([]domain.ProductVariant, error)
	ListAssets(context.Context, string) ([]domain.Asset, error)
	GetAsset(context.Context, string, string) (domain.Asset, error)
}
