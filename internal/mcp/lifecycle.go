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
	AmountMinor       int64  `json:"amount_minor" jsonschema:"Positive integer minor units, never decimal major units. Event cashflow determines income or expense; a neutral type records exactly 0."`
	Currency          string `json:"currency" jsonschema:"ISO currency code for the original amount."`
	OccurredAt        string `json:"occurred_at" jsonschema:"RFC3339 timestamp with explicit timezone."`
	FXRateScaled      int64  `json:"fx_rate_scaled,omitempty" jsonschema:"Base currency per original currency unit multiplied by 100000000, required for foreign currency."`
	FXRateDate        string `json:"fx_rate_date,omitempty" jsonschema:"YYYY-MM-DD date for confirmed FX evidence."`
	FXRateSource      string `json:"fx_rate_source,omitempty"`
	FXConfirmed       bool   `json:"fx_confirmed,omitempty"`
	ExternalReference string `json:"external_reference,omitempty"`
	Notes             string `json:"notes,omitempty" jsonschema:"Details of this individual event: the product or service name and relevant context. A purchased service or accessory keeps its specific product name here; the reusable cost category is chosen with type_id."`
	// RelatedAssetID is the optional neutral counterpart of this event on another
	// existing asset of the same data space. Omitting it on a correction preserves
	// the current relation; an explicit empty string clears it. Paired trade-in
	// events stay owned by the dedicated trade-in tools.
	RelatedAssetID *string `json:"related_asset_id,omitempty" jsonschema:"Optional ID of another existing asset this record relates to. Omitted on a correction preserves the current relation; an explicit empty string clears it. A paired trade-in event is refused here."`
}

type EventInput struct {
	EventFields
	AssetID string `json:"asset_id"`
	TypeID  string `json:"type_id" jsonschema:"Existing enabled event type ID from list_event_types. A type is a reusable action category (purchase, repair, sale, or a user-confirmed custom service or accessory cost), not a product-specific name. Built-in purchase means acquiring the item and is recorded only once per item."`
}

type CorrectEventInput struct {
	EventID     string      `json:"event_id" jsonschema:"Original event ID to void and replace; history is preserved."`
	TypeID      string      `json:"type_id,omitempty" jsonschema:"Optional replacement type ID. Omit it to keep the original type. Use it only to reclassify an already recorded misclassification into an enabled tenant-owned custom type with the same cash-flow direction, or into a neutral custom type when the original amount is zero; built-in targets are refused."`
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
	return application.RecordEvent{RequestKey: input.RequestKey, AmountMinor: input.AmountMinor, Currency: input.Currency, OccurredAt: when, FXRateScaled: input.FXRateScaled, FXRateDate: rateDate, FXRateSource: input.FXRateSource, FXConfirmed: input.FXConfirmed, Source: "mcp", ExternalReference: input.ExternalReference, Notes: input.Notes, RelatedAssetID: input.RelatedAssetID}, nil
}

func registerLifecycle(server *sdk.Server, s Services) {
	register(server, "record_event", "Persist a user-confirmed lifecycle event. First use list_event_types and reuse a matching enabled type. The built-in purchase type means acquiring the item and is recorded only once per item. Purchased services, accessories and similar costs use reusable custom expense types that the user has explicitly confirmed; individual product or service names stay in notes. A zero amount is valid only for a user-confirmed custom neutral type, such as a free gift. Repair and sale require the acquisition; after a sale no new built-in purchase, repair or sale is accepted, while custom post-sale cost events stay available. Confirm screenshot-derived fields with the user before calling. Asset creation is a separate command. Reuse request_key on retries.", ScopeLifecycle, application.CapabilityManageLifecycle, func(ctx context.Context, p application.Principal, input EventInput) (any, error) {
		cmd, err := input.command()
		if err != nil {
			return nil, err
		}
		cmd.AssetID, cmd.TypeID = input.AssetID, input.TypeID
		return s.Lifecycle.Record(ctx, p, cmd)
	})
	register(server, "correct_event", "Correct a user-confirmed event by atomically voiding the original and appending its replacement. History is never overwritten and the original asset and its economic evidence do not change. Omit type_id to keep the original event type, including for a disabled type. The optional type_id reclassifies an already recorded misclassification into an enabled tenant-owned custom type with the same cash-flow direction, or into a neutral custom type when the original amount is zero; built-in targets and expense/income sign changes are refused, and the last purchase cannot be removed while a repair or sale depends on it. Reuse the replacement request_key on retries.", ScopeLifecycle, application.CapabilityManageLifecycle, func(ctx context.Context, p application.Principal, input CorrectEventInput) (any, error) {
		cmd, err := input.Replacement.command()
		if err != nil {
			return nil, err
		}
		cmd.TypeID = strings.TrimSpace(input.TypeID)
		return s.Lifecycle.Correct(ctx, p, input.EventID, cmd)
	})
}
