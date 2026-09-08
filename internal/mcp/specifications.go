package mcp

import (
	"context"
	"github.com/SampsonFox/assetloop/internal/application"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type SaveTagTypeInput struct {
	RequestKey          string `json:"request_key"`
	ID                  string `json:"id,omitempty" jsonschema:"Omit to create; supply an existing ID to update."`
	Name                string `json:"name"`
	Multiple            bool   `json:"multiple"`
	AffectsAppearance   bool   `json:"affects_appearance"`
	Enabled             bool   `json:"enabled"`
	ConfirmSharedRename bool   `json:"confirm_shared_rename,omitempty" jsonschema:"Set only after the user confirms renaming a referenced shared value."`
}
type SaveTagInput struct {
	RequestKey          string `json:"request_key"`
	ID                  string `json:"id,omitempty"`
	TypeID              string `json:"type_id"`
	Name                string `json:"name"`
	Enabled             bool   `json:"enabled"`
	ConfirmSharedRename bool   `json:"confirm_shared_rename,omitempty"`
}
type ModelDetails struct {
	CategoryID string `json:"category_id"`
	Name       string `json:"name"`
}
type SaveModelInput struct {
	RequestKey          string          `json:"request_key"`
	ModelID             string          `json:"model_id"`
	TagIDs              []string        `json:"tag_ids" jsonschema:"Complete allowed tag selection, replacing the previous selection."`
	AppearanceOverrides map[string]bool `json:"appearance_overrides" jsonschema:"Complete type-ID to boolean override map; empty restores type defaults."`
	Details             *ModelDetails   `json:"details,omitempty"`
}
type SaveAssetInput struct {
	RequestKey      string   `json:"request_key"`
	ID              string   `json:"id,omitempty" jsonschema:"Omit to create a new item; provide existing ID to update."`
	ModelID         string   `json:"model_id"`
	DisplayName     string   `json:"display_name"`
	SerialNumber    string   `json:"serial_number"`
	PurchaseChannel string   `json:"purchase_channel"`
	Notes           string   `json:"notes"`
	TagIDs          []string `json:"tag_ids" jsonschema:"Complete actual selection of existing allowed tags."`
	ResourceID      *string  `json:"resource_id,omitempty" jsonschema:"Omit to retain 3D binding; empty string restores inheritance; otherwise existing resource ID."`
}
type SaveAppearanceInput struct {
	RequestKey string   `json:"request_key"`
	ID         string   `json:"id,omitempty"`
	ModelID    string   `json:"model_id"`
	ResourceID string   `json:"resource_id"`
	TagIDs     []string `json:"tag_ids"`
}
type DeleteAppearanceInput struct {
	RequestKey string `json:"request_key"`
	ID         string `json:"id"`
}
type ResourceDetails struct {
	Name      string `json:"name"`
	SourceURL string `json:"source_url"`
	Author    string `json:"author"`
	License   string `json:"license"`
}
type SaveResourceInput struct {
	RequestKey  string           `json:"request_key"`
	ResourceID  string           `json:"resource_id"`
	TagIDs      []string         `json:"tag_ids"`
	CategoryIDs []string         `json:"category_ids"`
	Details     *ResourceDetails `json:"details,omitempty"`
}

func registerSpecifications(server *sdk.Server, s Services) {
	register(server, "save_tag_type", "Create/update a confirmed specification type. All flags are explicit. Shared rename needs user confirmation. Reuse request_key on retries.", ScopeCatalog, application.CapabilityManageCatalog, func(ctx context.Context, p application.Principal, q SaveTagTypeInput) (any, error) {
		return s.Management.SaveType(ctx, p, q.RequestKey, application.SaveSpecificationType{ID: q.ID, Name: q.Name, Multiple: q.Multiple, AffectsAppearance: q.AffectsAppearance, Enabled: q.Enabled, ConfirmSharedRename: q.ConfirmSharedRename})
	})
	register(server, "save_specification_tag", "Create/update a confirmed tag. Existing shared renames need explicit confirmation; reuse request_key on retries.", ScopeCatalog, application.CapabilityManageCatalog, func(ctx context.Context, p application.Principal, q SaveTagInput) (any, error) {
		return s.Management.SaveTag(ctx, p, q.RequestKey, application.SaveSpecificationTag{ID: q.ID, TypeID: q.TypeID, Name: q.Name, Enabled: q.Enabled, ConfirmSharedRename: q.ConfirmSharedRename})
	})
	register(server, "save_model_configuration", "Replace model tag allowances and appearance overrides, optionally saving metadata atomically. Read current configuration first; reuse request_key on retries.", ScopeCatalog, application.CapabilityManageCatalog, func(ctx context.Context, p application.Principal, q SaveModelInput) (any, error) {
		cmd := application.SaveModelSpecification{ModelID: q.ModelID, TagIDs: q.TagIDs, AppearanceOverrides: q.AppearanceOverrides}
		if q.Details != nil {
			cmd.Details = &application.ModelConfigurationDetails{CategoryID: q.Details.CategoryID, Name: q.Details.Name}
		}
		err := s.Management.SaveModel(ctx, p, q.RequestKey, cmd)
		return map[string]string{"model_id": q.ModelID}, err
	})
	register(server, "save_asset", "Create/update confirmed item metadata and complete selected tags. This does not record a purchase; record_event is a separate committed command. Reuse request_key on retries.", ScopeCatalog, application.CapabilityManageCatalog, func(ctx context.Context, p application.Principal, q SaveAssetInput) (any, error) {
		return s.Management.SaveAsset(ctx, p, q.RequestKey, application.SaveSpecificationAsset{ID: q.ID, ModelID: q.ModelID, DisplayName: q.DisplayName, SerialNumber: q.SerialNumber, PurchaseChannel: q.PurchaseChannel, Notes: q.Notes, TagIDs: q.TagIDs, ResourceID: q.ResourceID})
	})
	register(server, "save_appearance_default", "Create/update a user-confirmed model appearance rule using an existing 3D resource and complete tag conditions. Reuse request_key on retries.", ScopeCatalog, application.CapabilityManageCatalog, func(ctx context.Context, p application.Principal, q SaveAppearanceInput) (any, error) {
		return s.Management.SaveAppearance(ctx, p, q.RequestKey, application.SaveAppearanceDefault{ID: q.ID, ModelID: q.ModelID, ResourceID: q.ResourceID, TagIDs: q.TagIDs})
	})
	register(server, "delete_appearance_default", "Delete the identified confirmed appearance rule. Does not delete the resource. Reuse request_key on retries.", ScopeCatalog, application.CapabilityManageCatalog, func(ctx context.Context, p application.Principal, q DeleteAppearanceInput) (any, error) {
		err := s.Management.DeleteAppearance(ctx, p, q.RequestKey, q.ID)
		return map[string]string{"deleted_id": q.ID}, err
	})
	register(server, "save_3d_resource_metadata", "Replace descriptive tag/category associations and optionally metadata of an existing resource. No upload or rebinding. Read current values first; reuse request_key on retries.", ScopeCatalog, application.CapabilityManageCatalog, func(ctx context.Context, p application.Principal, q SaveResourceInput) (any, error) {
		cmd := application.SaveResourceSpecification{ResourceID: q.ResourceID, TagIDs: q.TagIDs, CategoryIDs: q.CategoryIDs}
		if q.Details != nil {
			cmd.Details = &application.UpdateModel3DResource{ID: q.ResourceID, Name: q.Details.Name, SourceURL: q.Details.SourceURL, Author: q.Details.Author, License: q.Details.License}
		}
		err := s.Management.SaveResource(ctx, p, q.RequestKey, cmd)
		return map[string]string{"resource_id": q.ResourceID}, err
	})
}
