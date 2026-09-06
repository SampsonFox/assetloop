package application

import (
	"context"

	"github.com/SampsonFox/assetloop/internal/domain"
)

// A read projection of explicit, separately constrained association tables.
// Kind is not a polymorphic persisted foreign key or a generic mutation command.
type SpecificationLink struct{ Kind, TargetID, ModelID, TagID string }
type AppearanceDimension struct {
	ModelID, TypeID   string
	AffectsAppearance bool
}
type LegacyMediaMapping struct {
	VariantID, ModelID, ResourceID, Reason string
	Resolved                               bool
}
type SpecificationSnapshot struct {
	Types       []domain.SpecificationTagType
	Tags        []domain.SpecificationTag
	Links       []SpecificationLink
	Dimensions  []AppearanceDimension
	Defaults    []domain.AppearanceDefault
	LegacyMedia []LegacyMediaMapping
	CategoryIDs []string
}

type SpecificationStore interface {
	WithSpecificationWrite(context.Context, string, func(SpecificationStore) error) error
	SpecificationSnapshot(context.Context, string) (SpecificationSnapshot, error)
	PutSpecificationType(context.Context, domain.SpecificationTagType) error
	PutSpecificationTag(context.Context, domain.SpecificationTag) error
	AddModelAllowedTag(context.Context, string, string, string) error
	RemoveModelAllowedTag(context.Context, string, string, string) error
	PutAppearanceDimension(context.Context, string, AppearanceDimension) error
	ClearAppearanceDimensions(context.Context, string, string) error
	PutAppearanceDefault(context.Context, domain.AppearanceDefault) error
	DeleteAppearanceDefault(context.Context, string, string) error
	AddAppearanceCondition(context.Context, string, string, string, string) error
	ClearAppearanceConditions(context.Context, string, string) error
	ResolveLegacyMedia(context.Context, string, string) error
	ClearAssetSpecificationTags(context.Context, string, string) error
	AddAssetSpecificationTag(context.Context, string, string, string, string) error
	WriteSelectedAsset(context.Context, domain.Asset, bool) error
	ClearResourceSpecificationTags(context.Context, string, string) error
	AddResourceSpecificationTag(context.Context, string, string, string) error
	ClearResourceCategories(context.Context, string, string) error
	AddResourceCategory(context.Context, string, string, string) error
	GetProductModel(context.Context, string, string) (domain.ProductModel, error)
	GetAsset(context.Context, string, string) (domain.Asset, error)
	GetModel3DResource(context.Context, string, string) (domain.Model3DResource, error)
	ListModel3DResources(context.Context, string, Model3DResourceListOptions) (Model3DResourceListResult, error)
}
