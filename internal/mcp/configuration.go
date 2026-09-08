package mcp

import (
	"context"

	"github.com/SampsonFox/assetloop/internal/application"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type ModelConfigurationResult struct {
	ModelID             string                 `json:"model_id"`
	TagIDs              []string               `json:"tag_ids"`
	AppearanceOverrides map[string]bool        `json:"appearance_overrides"`
	Defaults            []AppearanceRuleResult `json:"appearance_defaults"`
}
type AppearanceRuleResult struct {
	ID         string   `json:"id"`
	ResourceID string   `json:"resource_id"`
	TagIDs     []string `json:"tag_ids"`
}
type ResourceConfigurationResult struct {
	ResourceID  string   `json:"resource_id"`
	TagIDs      []string `json:"tag_ids"`
	CategoryIDs []string `json:"category_ids"`
}
type SpecificationReferencesInput struct {
	ID       string `json:"id"`
	Kind     string `json:"kind" jsonschema:"tag or type"`
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
}
type SpecificationReferenceResult struct {
	Kind     string `json:"kind"`
	TargetID string `json:"target_id"`
	ModelID  string `json:"model_id"`
	TagID    string `json:"tag_id"`
}

func registerConfiguration(server *sdk.Server, s Services) {
	register(server, "get_model_configuration", "Read complete allowed tag IDs, explicit appearance overrides (false differs from absent), and confirmed appearance rules before editing a model.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q IDInput) (any, error) {
		c, err := s.Specifications.ModelConfiguration(ctx, p, q.ID)
		result := ModelConfigurationResult{ModelID: q.ID, TagIDs: append([]string{}, c.Specification.AllowedTagIDs...), AppearanceOverrides: c.Specification.AppearanceOverrides, Defaults: []AppearanceRuleResult{}}
		for _, rule := range c.Defaults {
			result.Defaults = append(result.Defaults, AppearanceRuleResult{ID: rule.ID, ResourceID: rule.ResourceID, TagIDs: rule.TagIDs})
		}
		return result, err
	})
	register(server, "get_resource_configuration", "Read complete descriptive tag and category associations before replacing resource metadata. These associations do not imply bindings.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q IDInput) (any, error) {
		c, err := s.Specifications.ResourceConfiguration(ctx, p, q.ID)
		return ResourceConfigurationResult{ResourceID: q.ID, TagIDs: append([]string{}, c.TagIDs...), CategoryIDs: append([]string{}, c.CategoryIDs...)}, err
	})
	register(server, "get_specification_references", "Read paged tag or tag-type references before confirming a shared rename or changing selection policy.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q SpecificationReferencesInput) (any, error) {
		c, err := s.Specifications.References(ctx, p, q.Kind, q.ID, application.SpecificationListOptions{Page: q.Page, PageSize: q.PageSize})
		result := struct {
			References []SpecificationReferenceResult `json:"references"`
			Total      int                            `json:"total"`
		}{References: []SpecificationReferenceResult{}, Total: c.Total}
		for _, ref := range c.References {
			result.References = append(result.References, SpecificationReferenceResult{Kind: ref.Kind, TargetID: ref.TargetID, ModelID: ref.ModelID, TagID: ref.TagID})
		}
		return result, err
	})
}
