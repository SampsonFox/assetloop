package mcp

import (
	"context"
	"github.com/SampsonFox/assetloop/internal/application"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type CategoryInput struct {
	RequestKey string `json:"request_key" jsonschema:"Stable key for this confirmed command; reuse on retries."`
	Name       string `json:"name"`
	IconKey    string `json:"icon_key,omitempty"`
}
type UpdateCategoryInput struct {
	CategoryInput
	ID string `json:"id"`
}
type CreateModelInput struct {
	RequestKey string `json:"request_key"`
	CategoryID string `json:"category_id"`
	Name       string `json:"name"`
}

func registerCatalog(server *sdk.Server, s Services) {
	register(server, "create_category", "Create a confirmed category. Search existing categories first. Reuse request_key on retry.", ScopeCatalog, application.CapabilityManageCatalog, func(ctx context.Context, p application.Principal, q CategoryInput) (any, error) {
		return s.Management.CreateCategory(ctx, p, q.RequestKey, application.CreateCategory{Name: q.Name, IconKey: q.IconKey})
	})
	register(server, "update_category", "Update an existing category name/icon. Reuse request_key on retry.", ScopeCatalog, application.CapabilityManageCatalog, func(ctx context.Context, p application.Principal, q UpdateCategoryInput) (any, error) {
		return s.Management.UpdateCategory(ctx, p, q.RequestKey, application.UpdateCategory{ID: q.ID, Name: q.Name, IconKey: q.IconKey})
	})
	register(server, "create_product_model", "Create a confirmed product model under an existing category. Search existing models first. Reuse request_key on retry.", ScopeCatalog, application.CapabilityManageCatalog, func(ctx context.Context, p application.Principal, q CreateModelInput) (any, error) {
		return s.Management.CreateModel(ctx, p, q.RequestKey, application.CreateModel{CategoryID: q.CategoryID, Name: q.Name})
	})
}
