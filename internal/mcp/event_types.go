package mcp

import (
	"context"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateEventTypeInput struct {
	RequestKey string `json:"request_key"`
	Name       string `json:"name"`
	Cashflow   string `json:"cashflow" jsonschema:"expense, income or neutral"`
}
type UpdateEventTypeInput struct {
	CreateEventTypeInput
	ID string `json:"id"`
}
type EnableEventTypeInput struct {
	RequestKey string `json:"request_key"`
	ID         string `json:"id"`
	Enabled    bool   `json:"enabled"`
}

func registerEventTypes(server *sdk.Server, s Services) {
	register(server, "create_event_type", "Create a confirmed custom lifecycle type. Reuse request_key on retries.", ScopeLifecycle, application.CapabilityManageLifecycle, func(ctx context.Context, p application.Principal, q CreateEventTypeInput) (any, error) {
		return s.Management.CreateEventType(ctx, p, q.RequestKey, application.CreateAssetEventType{Name: q.Name, Cashflow: domain.AssetEventCashflow(q.Cashflow)})
	})
	register(server, "update_event_type", "Rename a custom lifecycle type; changing cashflow is forbidden after use. Built-in types cannot change. Reuse request_key on retries.", ScopeLifecycle, application.CapabilityManageLifecycle, func(ctx context.Context, p application.Principal, q UpdateEventTypeInput) (any, error) {
		return s.Management.UpdateEventType(ctx, p, q.RequestKey, q.ID, application.UpdateEventType{Name: q.Name, Cashflow: domain.AssetEventCashflow(q.Cashflow)})
	})
	register(server, "set_event_type_enabled", "Enable or disable a custom lifecycle type without altering history. Built-in types cannot change. Reuse request_key on retries.", ScopeLifecycle, application.CapabilityManageLifecycle, func(ctx context.Context, p application.Principal, q EnableEventTypeInput) (any, error) {
		return s.Management.SetEventTypeEnabled(ctx, p, q.RequestKey, q.ID, q.Enabled)
	})
}
