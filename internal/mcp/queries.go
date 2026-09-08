package mcp

import (
	"context"
	"github.com/SampsonFox/assetloop/internal/application"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type Empty struct{}
type IDInput struct {
	ID string `json:"id" jsonschema:"Stable object ID returned by a query; do not guess an ID."`
}
type AssetQuery struct {
	Query     string `json:"query,omitempty"`
	Status    string `json:"status,omitempty"`
	Sort      string `json:"sort,omitempty"`
	Direction string `json:"direction,omitempty"`
	Page      int    `json:"page,omitempty"`
	PageSize  int    `json:"page_size,omitempty"`
}
type ModelQuery struct {
	Query      string `json:"query,omitempty"`
	CategoryID string `json:"category_id,omitempty"`
	TagID      string `json:"tag_id,omitempty"`
	Sort       string `json:"sort,omitempty"`
	Direction  string `json:"direction,omitempty"`
	Page       int    `json:"page,omitempty"`
	PageSize   int    `json:"page_size,omitempty"`
}
type TagQuery struct {
	Query    string `json:"query,omitempty"`
	Status   string `json:"status,omitempty"`
	TypeID   string `json:"type_id,omitempty"`
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
}
type EventQuery struct {
	AssetID    string `json:"asset_id"`
	Query      string `json:"query,omitempty"`
	TypeID     string `json:"type_id,omitempty"`
	Sort       string `json:"sort,omitempty"`
	Direction  string `json:"direction,omitempty"`
	ShowVoided bool   `json:"show_voided,omitempty"`
	Page       int    `json:"page,omitempty"`
	PageSize   int    `json:"page_size,omitempty"`
}
type ResourceQuery struct {
	Query    string `json:"query,omitempty"`
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
}

func registerQueries(server *sdk.Server, s Services) {
	register(server, "get_context", "Read current account, data space, granted scopes and base currency. No credentials are returned.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, _ Empty) (any, error) {
		currency, locked, err := s.Lifecycle.BaseCurrency(ctx, p)
		return struct {
			UserID   string           `json:"user_id"`
			TenantID string           `json:"tenant_id"`
			Role     application.Role `json:"role"`
			Scopes   []string         `json:"scopes"`
			Currency string           `json:"base_currency"`
			Locked   bool             `json:"base_currency_locked"`
		}{p.UserID, p.TenantID, p.Role, ctx.Value(identityKey{}).(Identity).Scopes, currency, locked}, err
	})
	register(server, "list_assets", "Search paged assets and exact cost summaries. Money is in integer minor units.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q AssetQuery) (any, error) {
		return s.Catalog.ListAssetsWithSummary(ctx, p, application.AssetListOptions{Query: q.Query, Status: q.Status, Sort: q.Sort, Direction: q.Direction, Page: q.Page, PageSize: q.PageSize})
	})
	register(server, "get_asset", "Read an asset with its selected typed tags.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q IDInput) (any, error) {
		return s.Specifications.Asset(ctx, p, q.ID)
	})
	register(server, "list_categories", "List existing categories and stable IDs for model creation.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, _ Empty) (any, error) {
		return s.Catalog.Categories(ctx, p)
	})
	register(server, "search_product_models", "Search existing models before choosing or creating a model.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q ModelQuery) (any, error) {
		page, err := s.Catalog.ListModelsPage(ctx, p, application.ModelListOptions{Query: q.Query, CategoryID: q.CategoryID, TagID: q.TagID, Sort: q.Sort, Direction: q.Direction, Page: q.Page, PageSize: q.PageSize})
		result := struct {
			Models []ModelResult `json:"models"`
			Total  int           `json:"total"`
		}{Models: []ModelResult{}, Total: page.Total}
		for _, model := range page.Models {
			result.Models = append(result.Models, modelResult(model))
		}
		return result, err
	})
	register(server, "get_product_model", "Read model details by stable ID.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q IDInput) (any, error) {
		model, err := s.Specifications.Model(ctx, p, q.ID)
		return modelResult(model), err
	})
	register(server, "list_tag_types", "Read typed specification dimensions and reference counts.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q TagQuery) (any, error) {
		return s.Specifications.ListTypes(ctx, p, application.SpecificationListOptions{Query: q.Query, Status: q.Status, Page: q.Page, PageSize: q.PageSize})
	})
	register(server, "search_specification_tags", "Search reusable tags by type, text and enabled status.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q TagQuery) (any, error) {
		return s.Specifications.ListTags(ctx, p, application.SpecificationListOptions{Query: q.Query, Status: q.Status, TypeID: q.TypeID, Page: q.Page, PageSize: q.PageSize})
	})
	register(server, "list_events", "Read paged lifecycle history including optional voided originals.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q EventQuery) (any, error) {
		return s.Lifecycle.TimelinePage(ctx, p, q.AssetID, application.EventListOptions{Query: q.Query, Type: q.TypeID, Sort: q.Sort, Direction: q.Direction, ShowVoided: q.ShowVoided, Page: q.Page, PageSize: q.PageSize})
	})
	register(server, "get_event", "Read one lifecycle event and its preserved economic evidence.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q IDInput) (any, error) {
		return s.Lifecycle.GetEvent(ctx, p, q.ID)
	})
	register(server, "list_event_types", "Read system and custom event types and their enabled states.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q TagQuery) (any, error) {
		return s.Lifecycle.EventTypePage(ctx, p, application.EventTypeListOptions{Query: q.Query, Status: q.Status, Page: q.Page, PageSize: q.PageSize})
	})
	register(server, "get_asset_cost", "Calculate full lifecycle cost, unaffected by event list filters; no market valuation.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q IDInput) (any, error) {
		return s.Lifecycle.CostDashboard(ctx, p, q.ID)
	})
	register(server, "get_portfolio_summary", "Read aggregate exact cashflow and asset count for the current data space.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, _ Empty) (any, error) {
		return s.Lifecycle.PortfolioSummary(ctx, p)
	})
	register(server, "list_3d_resources", "Search existing 3D resources; file upload remains in Web.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q ResourceQuery) (any, error) {
		page, err := s.Media.ListResources(ctx, p, application.Model3DResourceListOptions{Query: q.Query, Page: q.Page, PageSize: q.PageSize})
		result := struct {
			Resources []ResourceResult `json:"resources"`
			Total     int              `json:"total"`
		}{Resources: []ResourceResult{}, Total: page.Total}
		for _, resource := range page.Resources {
			result.Resources = append(result.Resources, resourceResult(resource))
		}
		return result, err
	})
	register(server, "get_3d_resource", "Read existing 3D resource metadata.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q IDInput) (any, error) {
		resource, err := s.Media.GetResource(ctx, p, q.ID)
		return resourceResult(resource), err
	})
	register(server, "get_3d_references", "Read references before changing or deleting a resource.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q IDInput) (any, error) {
		return s.Media.References(ctx, p, q.ID)
	})
	register(server, "get_asset_appearance", "Read the effective resource and source of an asset appearance.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q IDInput) (any, error) {
		appearance, err := s.Specifications.EffectiveForAsset(ctx, p, q.ID)
		result := AppearanceResult{Source: appearance.Source, Conflict: appearance.Conflict, RuleIDs: append([]string{}, appearance.RuleIDs...)}
		if appearance.Resource != nil {
			resource := resourceResult(*appearance.Resource)
			result.Resource = &resource
		}
		return result, err
	})
}
