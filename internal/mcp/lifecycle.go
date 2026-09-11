package mcp

import (
	"context"
	"strings"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type EventFields struct {
	RequestKey        string `json:"request_key" jsonschema:"Stable unique key for this confirmed command. Reuse exactly on retries; never reuse for different content."`
	AmountMinor       int64  `json:"amount_minor" jsonschema:"Nonnegative integer minor units, never decimal major units. Event type determines income or expense."`
	Currency          string `json:"currency" jsonschema:"ISO currency code for the original amount."`
	OccurredAt        string `json:"occurred_at" jsonschema:"RFC3339 timestamp with explicit timezone."`
	FXRateScaled      int64  `json:"fx_rate_scaled,omitempty" jsonschema:"Base currency per original currency unit multiplied by 100000000, required for foreign currency."`
	FXRateDate        string `json:"fx_rate_date,omitempty" jsonschema:"YYYY-MM-DD date for confirmed FX evidence."`
	FXRateSource      string `json:"fx_rate_source,omitempty"`
	FXConfirmed       bool   `json:"fx_confirmed,omitempty"`
	ExternalReference string `json:"external_reference,omitempty"`
	Notes             string `json:"notes,omitempty" jsonschema:"Details of this individual event: product or service name and relevant context. For a purchased service use the existing purchase type and put the service name here; do not create a type named after the service."`
}

type EventInput struct {
	EventFields
	AssetID string `json:"asset_id"`
	TypeID  string `json:"type_id" jsonschema:"Existing enabled event type ID from list_event_types. A type is a reusable action category (purchase, repair, sale), not a product or service name. Prefer an existing matching type; put purchase details in notes."`
}

type CorrectEventInput struct {
	EventID     string      `json:"event_id" jsonschema:"Original event ID to void and replace; history is preserved."`
	Replacement EventFields `json:"replacement"`
}

func (input EventFields) command() (application.RecordEvent, error) {
	if strings.TrimSpace(input.RequestKey) == "" {
		return application.RecordEvent{}, application.NewInputError("validation.request_key")
	}
	when, err := time.Parse(time.RFC3339, input.OccurredAt)
	if err != nil {
		return application.RecordEvent{}, application.NewInputError("validation.occurred_at")
	}
	var rateDate time.Time
	if input.FXRateDate != "" {
		rateDate, err = time.Parse("2006-01-02", input.FXRateDate)
		if err != nil {
			return application.RecordEvent{}, application.NewInputError("validation.fx_rate_date")
		}
	}
	return application.RecordEvent{RequestKey: input.RequestKey, AmountMinor: input.AmountMinor, Currency: input.Currency, OccurredAt: when, FXRateScaled: input.FXRateScaled, FXRateDate: rateDate, FXRateSource: input.FXRateSource, FXConfirmed: input.FXConfirmed, Source: "mcp", ExternalReference: input.ExternalReference, Notes: input.Notes}, nil
}

func registerLifecycle(server *sdk.Server, s Services) {
	register(server, "record_event", "Persist a user-confirmed lifecycle event. First use list_event_types and reuse a matching enabled action category. Buying a product or service is a purchase; put its specific name in notes, not a new event type. Confirm screenshot-derived fields with the user before calling. Asset creation is a separate command. Reuse request_key on retries.", ScopeLifecycle, application.CapabilityManageLifecycle, func(ctx context.Context, p application.Principal, input EventInput) (any, error) {
		cmd, err := input.command()
		if err != nil {
			return nil, err
		}
		cmd.AssetID, cmd.TypeID = input.AssetID, input.TypeID
		return s.Lifecycle.Record(ctx, p, cmd)
	})
	register(server, "correct_event", "Correct a user-confirmed event by atomically voiding the original and appending its replacement. Never overwrites history; this does not change the original asset or event type. Do not use it to reclassify an event or rename a shared type to correct one record. Reuse the replacement request_key on retries.", ScopeLifecycle, application.CapabilityManageLifecycle, func(ctx context.Context, p application.Principal, input CorrectEventInput) (any, error) {
		cmd, err := input.Replacement.command()
		if err != nil {
			return nil, err
		}
		return s.Lifecycle.Correct(ctx, p, input.EventID, cmd)
	})
}
