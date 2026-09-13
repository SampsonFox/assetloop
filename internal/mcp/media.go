package mcp

import (
	"context"

	"github.com/SampsonFox/assetloop/internal/application"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type BindingInput struct {
	Kind     string `json:"kind" jsonschema:"model or asset"`
	TargetID string `json:"target_id"`
}
type BindResourceInput struct {
	BindingInput
	RequestKey string `json:"request_key"`
	ResourceID string `json:"resource_id" jsonschema:"Existing ready resource ID; empty clears the explicit binding and restores inheritance for an asset"`
}
type BindingResult struct {
	Name                string `json:"name"`
	ResourceID          string `json:"resource_id"`
	EffectiveResourceID string `json:"effective_resource_id"`
	Source              string `json:"source"`
	Conflict            bool   `json:"conflict"`
}

type DeleteResourceInput struct {
	RequestKey string `json:"request_key"`
	ResourceID string `json:"resource_id"`
}

func registerMedia(server *sdk.Server, s Services) {
	register(server, "delete_3d_resource", "Delete a confirmed unreferenced 3D resource and its GLB. Irreversible; read references and confirm intent first. Reuse request_key to resume a failed deletion.", ScopeCatalog, application.CapabilityManageCatalog, func(ctx context.Context, p application.Principal, q DeleteResourceInput) (any, error) {
		err := s.Management.DeleteResource(ctx, p, q.RequestKey, q.ResourceID)
		return err == nil, err
	})
	register(server, "get_3d_binding", "Read an explicit binding and its effective resource without storage locations.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q BindingInput) (any, error) {
		b, err := s.Media.Binding(ctx, p, q.Kind, q.TargetID)
		return BindingResult{Name: b.Name, ResourceID: b.ResourceID, EffectiveResourceID: b.EffectiveResourceID, Source: b.Source, Conflict: b.Conflict}, err
	})
	register(server, "bind_3d_resource", "Set or clear a confirmed model default or asset override. Does not delete resources. Reuse request_key on retries.", ScopeCatalog, application.CapabilityManageAssets, func(ctx context.Context, p application.Principal, q BindResourceInput) (any, error) {
		err := s.Management.BindResource(ctx, p, q.RequestKey, application.BindModel3DResource{Kind: q.Kind, TargetID: q.TargetID, ResourceID: q.ResourceID})
		return err == nil, err
	})
}
