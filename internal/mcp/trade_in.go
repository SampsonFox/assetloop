package mcp

import (
	"context"
	"strings"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Trade-in tool inputs and public results. Inputs are thin snake_case DTOs of the
// documented application command: no matching, aggregation or policy lives here,
// and direction is never inferred from names or event order.

// TradeInNewEventInput is one newly recorded economic side. It has no request
// key of its own: the whole trade-in command shares the outer request_key.
type TradeInNewEventInput struct {
	AmountMinor  int64  `json:"amount_minor,omitempty" jsonschema:"Positive integer minor units of the new record, never decimal major units. Required on record_trade_in; omit on preview_trade_in and the shared preview reports it as missing."`
	Currency     string `json:"currency,omitempty" jsonschema:"ISO currency code of the original amount. Required on record_trade_in."`
	OccurredAt   string `json:"occurred_at,omitempty" jsonschema:"RFC3339 timestamp with an explicit timezone for this economic record. Required on record_trade_in."`
	FXRateScaled int64  `json:"fx_rate_scaled,omitempty" jsonschema:"Base currency per original unit multiplied by 100000000, required when the original currency is not the base currency."`
	FXRateDate   string `json:"fx_rate_date,omitempty" jsonschema:"YYYY-MM-DD date of the confirmed rate."`
	FXRateSource string `json:"fx_rate_source,omitempty"`
	FXConfirmed  bool   `json:"fx_confirmed,omitempty"`
	Reference    string `json:"external_reference,omitempty"`
	Notes        string `json:"notes,omitempty"`
}

// TradeInEconomicInput is one explicitly selected side of a pair. Provide exactly
// one of ExistingEventID or NewEvent; a side with neither is a preview-only gap.
type TradeInEconomicInput struct {
	AssetID         string                `json:"asset_id" jsonschema:"Existing asset ID of this side of the pair."`
	ExistingEventID string                `json:"existing_event_id,omitempty" jsonschema:"Effective purchase or sale event ID to reuse. Provide this or new_event, never both."`
	NewEvent        *TradeInNewEventInput `json:"new_event,omitempty" jsonschema:"New economic record when no valid one exists yet."`
}

type PreviewTradeInInput struct {
	CurrentAssetID    string                 `json:"current_asset_id" jsonschema:"The asset this command is recorded from."`
	Direction         string                 `json:"direction" jsonschema:"source when the current asset is the new item and every counterpart is an old item; destination when the current asset is the old item and every counterpart is a new item."`
	Current           TradeInEconomicInput   `json:"current,omitempty" jsonschema:"This asset's own economic side: a purchase for source, a sale for destination."`
	Counterparts      []TradeInEconomicInput `json:"counterparts" jsonschema:"One to fifty explicit counterpart assets. Every pair is selected explicitly; nothing is inferred from names, notes or order references."`
	OccurredAt        string                 `json:"occurred_at,omitempty" jsonschema:"RFC3339 trade-in date, independent of when the money occurred."`
	ExternalReference string                 `json:"external_reference,omitempty"`
	Notes             string                 `json:"notes,omitempty"`
}

type RecordTradeInInput struct {
	PreviewTradeInInput
	RequestKey string `json:"request_key" jsonschema:"Stable unique key for this confirmed command. Reuse exactly on retries; never reuse for different content."`
}

type CorrectTradeInLinkInput struct {
	RequestKey                 string `json:"request_key" jsonschema:"Stable unique key for this confirmed command. Reuse exactly on retries."`
	LinkID                     string `json:"link_id" jsonschema:"Stable relation ID returned by record_trade_in, get_event or list_events."`
	ExpectedSourceEventID      string `json:"expected_source_event_id" jsonschema:"Source paired event ID this client last read. A changed link is refused instead of silently re-pointed."`
	ExpectedDestinationEventID string `json:"expected_destination_event_id" jsonschema:"Destination paired event ID this client last read."`
	NewAssetID                 string `json:"new_asset_id" jsonschema:"Asset that carries the purchase after the correction. It must already have an effective purchase."`
	OldAssetID                 string `json:"old_asset_id" jsonschema:"Asset that carries the sale after the correction. It must already have an effective sale."`
	OccurredAt                 string `json:"occurred_at,omitempty" jsonschema:"RFC3339 date of the association; independent of the money records."`
	ExternalReference          string `json:"external_reference,omitempty"`
	Notes                      string `json:"notes,omitempty"`
}

type CancelTradeInLinkInput struct {
	RequestKey                 string `json:"request_key" jsonschema:"Stable unique key for this confirmed command. Reuse exactly on retries."`
	LinkID                     string `json:"link_id" jsonschema:"Stable relation ID returned by record_trade_in, get_event or list_events."`
	ExpectedSourceEventID      string `json:"expected_source_event_id" jsonschema:"Source paired event ID this client last read."`
	ExpectedDestinationEventID string `json:"expected_destination_event_id" jsonschema:"Destination paired event ID this client last read."`
	OccurredAt                 string `json:"occurred_at,omitempty"`
	Notes                      string `json:"notes,omitempty" jsonschema:"Reason kept with the cancelled pair. Money is retained."`
}

// Public result DTOs: relation IDs, both paired event IDs, the economic event IDs
// and their created/reused status. No internal identifiers or storage metadata.
type TradeInPreviewPair struct {
	NewAssetID    string   `json:"new_asset_id"`
	NewAssetName  string   `json:"new_asset_name"`
	NewAssetSpec  string   `json:"new_asset_spec"`
	OldAssetID    string   `json:"old_asset_id"`
	OldAssetName  string   `json:"old_asset_name"`
	OldAssetSpec  string   `json:"old_asset_spec"`
	LinkID        string   `json:"link_id,omitempty"`
	NewEventID    string   `json:"new_event_id,omitempty"`
	OldEventID    string   `json:"old_event_id,omitempty"`
	NewSelection  string   `json:"new_selection"`
	OldSelection  string   `json:"old_selection"`
	MissingFields []string `json:"missing_fields,omitempty"`
}

type TradeInPreview struct {
	Direction      string               `json:"direction"`
	CurrentAssetID string               `json:"current_asset_id"`
	Ready          bool                 `json:"ready"`
	Pairs          []TradeInPreviewPair `json:"pairs"`
	MissingFields  []string             `json:"missing_fields,omitempty"`
}

type TradeInPairResult struct {
	LinkID             string `json:"link_id"`
	LinkStatus         string `json:"link_status"`
	SourceEventID      string `json:"source_event_id"`
	DestinationEventID string `json:"destination_event_id"`
	NewAssetID         string `json:"new_asset_id"`
	OldAssetID         string `json:"old_asset_id"`
	NewAssetName       string `json:"new_asset_name"`
	OldAssetName       string `json:"old_asset_name"`
	NewEconomicEventID string `json:"new_economic_event_id,omitempty"`
	NewEconomicStatus  string `json:"new_economic_status"`
	OldEconomicEventID string `json:"old_economic_event_id,omitempty"`
	OldEconomicStatus  string `json:"old_economic_status"`
}

type TradeInResult struct {
	Direction      string              `json:"direction"`
	CurrentAssetID string              `json:"current_asset_id"`
	Pairs          []TradeInPairResult `json:"pairs"`
}

func tradeInDirectionOf(value string) (domain.TradeInDirection, error) {
	switch domain.TradeInDirection(strings.TrimSpace(value)) {
	case domain.TradeInDirectionSource:
		return domain.TradeInDirectionSource, nil
	case domain.TradeInDirectionDestination:
		return domain.TradeInDirectionDestination, nil
	default:
		return "", application.NewInputError("validation.trade_in_direction")
	}
}

func parseTradeInTimestamp(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, application.NewInputError("validation.occurred_at")
	}
	return parsed, nil
}

func (input *TradeInNewEventInput) recordEvent() (application.RecordEvent, error) {
	occurredAt, err := parseTradeInTimestamp(input.OccurredAt)
	if err != nil {
		return application.RecordEvent{}, err
	}
	var rateDate time.Time
	if value := strings.TrimSpace(input.FXRateDate); value != "" {
		rateDate, err = time.Parse("2006-01-02", value)
		if err != nil {
			return application.RecordEvent{}, application.NewInputError("validation.fx_date")
		}
	}
	return application.RecordEvent{
		AmountMinor: input.AmountMinor, Currency: strings.TrimSpace(input.Currency), OccurredAt: occurredAt,
		FXRateScaled: input.FXRateScaled, FXRateDate: rateDate, FXRateSource: strings.TrimSpace(input.FXRateSource),
		FXConfirmed: input.FXConfirmed, Source: "mcp",
		ExternalReference: strings.TrimSpace(input.Reference), Notes: strings.TrimSpace(input.Notes),
	}, nil
}

func (input TradeInEconomicInput) selection() (application.EconomicSelection, error) {
	selection := application.EconomicSelection{
		AssetID:         strings.TrimSpace(input.AssetID),
		ExistingEventID: strings.TrimSpace(input.ExistingEventID),
	}
	if selection.ExistingEventID != "" && input.NewEvent != nil {
		return application.EconomicSelection{}, application.NewInputError("validation.trade_in_selection_conflict")
	}
	if input.NewEvent == nil {
		return selection, nil
	}
	record, err := input.NewEvent.recordEvent()
	if err != nil {
		return application.EconomicSelection{}, err
	}
	selection.NewEvent = &record
	return selection, nil
}

// previewCommand builds the documented command. A selection may be incomplete:
// the shared preview reports exactly what a write would still reject.
func (input PreviewTradeInInput) previewCommand() (application.TradeInCommand, error) {
	direction, err := tradeInDirectionOf(input.Direction)
	if err != nil {
		return application.TradeInCommand{}, err
	}
	occurredAt, err := parseTradeInTimestamp(input.OccurredAt)
	if err != nil {
		return application.TradeInCommand{}, err
	}
	command := application.TradeInCommand{
		CurrentAssetID:    strings.TrimSpace(input.CurrentAssetID),
		Direction:         direction,
		OccurredAt:        occurredAt,
		ExternalReference: strings.TrimSpace(input.ExternalReference),
		Notes:             strings.TrimSpace(input.Notes),
	}
	current, err := input.Current.selection()
	if err != nil {
		return application.TradeInCommand{}, err
	}
	command.Current = current
	for _, counterpart := range input.Counterparts {
		selection, err := counterpart.selection()
		if err != nil {
			return application.TradeInCommand{}, err
		}
		command.Counterparts = append(command.Counterparts, selection)
	}
	return command, nil
}

func (input RecordTradeInInput) recordCommand() (application.TradeInCommand, error) {
	if strings.TrimSpace(input.RequestKey) == "" {
		return application.TradeInCommand{}, application.NewInputError("validation.request_key")
	}
	command, err := input.PreviewTradeInInput.previewCommand()
	if err != nil {
		return application.TradeInCommand{}, err
	}
	command.RequestKey = strings.TrimSpace(input.RequestKey)
	return command, nil
}

func tradeInPreviewResult(preview application.TradeInPreview) TradeInPreview {
	result := TradeInPreview{
		Direction:      string(preview.Direction),
		CurrentAssetID: preview.CurrentAssetID,
		Ready:          preview.Ready,
		Pairs:          make([]TradeInPreviewPair, 0, len(preview.Pairs)),
		MissingFields:  preview.MissingFields,
	}
	for _, pair := range preview.Pairs {
		result.Pairs = append(result.Pairs, TradeInPreviewPair{
			NewAssetID: pair.NewAssetID, NewAssetName: pair.NewAssetName, NewAssetSpec: pair.NewAssetSpec,
			OldAssetID: pair.OldAssetID, OldAssetName: pair.OldAssetName, OldAssetSpec: pair.OldAssetSpec,
			LinkID: pair.LinkID, NewEventID: pair.NewEventID, OldEventID: pair.OldEventID,
			NewSelection: pair.NewSelection, OldSelection: pair.OldSelection, MissingFields: pair.MissingFields,
		})
	}
	return result
}

func tradeInResult(result application.TradeInResult) TradeInResult {
	mapped := TradeInResult{
		Direction:      string(result.Direction),
		CurrentAssetID: result.CurrentAssetID,
		Pairs:          make([]TradeInPairResult, 0, len(result.Pairs)),
	}
	for _, pair := range result.Pairs {
		mapped.Pairs = append(mapped.Pairs, TradeInPairResult{
			LinkID: pair.LinkID, LinkStatus: pair.LinkStatus,
			SourceEventID: pair.SourceEventID, DestinationEventID: pair.DestinationEventID,
			NewAssetID: pair.NewAssetID, OldAssetID: pair.OldAssetID,
			NewAssetName: pair.NewAssetName, OldAssetName: pair.OldAssetName,
			NewEconomicEventID: pair.NewEconomicEventID, NewEconomicStatus: pair.NewEconomicStatus,
			OldEconomicEventID: pair.OldEconomicEventID, OldEconomicStatus: pair.OldEconomicStatus,
		})
	}
	return mapped
}

func registerTradeIn(server *sdk.Server, s Services) {
	register(server, "preview_trade_in", "Read the selected buy/sell money and the fields a confirmed trade-in still needs, without writing. Direction is explicit: source means the current asset is the new item and every counterpart is an old item; destination means the current asset is the old item and every counterpart is a new item. Each counterpart is an explicitly selected pair; nothing is inferred from names, notes or order references. A selection may omit detailed money fields so this preview reports what is missing. Money is integer minor units and times are RFC3339 with a timezone.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, input PreviewTradeInInput) (any, error) {
		command, err := input.previewCommand()
		if err != nil {
			return nil, err
		}
		preview, err := s.Management.PreviewTradeIn(ctx, p, command)
		if err != nil {
			return nil, err
		}
		return tradeInPreviewResult(preview), nil
	})
	register(server, "record_trade_in", "Persist one user-confirmed trade-in: reuse each explicitly chosen effective purchase or sale, append the missing ones, and create the paired neutral relations atomically. Direction is explicit: source means the current asset is the new item (its own record is the purchase) and every counterpart is an old item carrying the sale; destination means the current asset is the old item and every counterpart is a new item. Supply either existing_event_id or new_event per asset, never both; a new_event needs integer minor-unit amount, ISO currency and an RFC3339 occurrence time. A sale requires an existing purchase first. Confirm every money field with the user before calling, reuse request_key on retries, and read get_event or list_events for the relation state afterwards.", ScopeLifecycle, application.CapabilityManageLifecycle, func(ctx context.Context, p application.Principal, input RecordTradeInInput) (any, error) {
		command, err := input.recordCommand()
		if err != nil {
			return nil, err
		}
		result, err := s.Management.RecordTradeIn(ctx, p, input.RequestKey, command)
		if err != nil {
			return nil, err
		}
		return tradeInResult(result), nil
	})
	register(server, "correct_trade_in_link", "Correct one confirmed trade-in relation by atomically voiding both paired events and appending replacements under the same link_id. Only the linked assets, date, order reference and notes change: buy and sell money is never created or modified, and both targets must already hold the required effective purchase and sale. Pass the link_id with both expected paired event IDs; a changed link is refused instead of silently re-pointed. Reuse request_key on retries.", ScopeLifecycle, application.CapabilityManageLifecycle, func(ctx context.Context, p application.Principal, input CorrectTradeInLinkInput) (any, error) {
		occurredAt, err := parseTradeInTimestamp(input.OccurredAt)
		if err != nil {
			return nil, err
		}
		result, err := s.Management.CorrectTradeInLink(ctx, p, input.RequestKey, application.CorrectTradeInLinkCommand{
			LinkID:                     strings.TrimSpace(input.LinkID),
			ExpectedSourceEventID:      strings.TrimSpace(input.ExpectedSourceEventID),
			ExpectedDestinationEventID: strings.TrimSpace(input.ExpectedDestinationEventID),
			NewAssetID:                 strings.TrimSpace(input.NewAssetID),
			OldAssetID:                 strings.TrimSpace(input.OldAssetID),
			OccurredAt:                 occurredAt,
			ExternalReference:          strings.TrimSpace(input.ExternalReference),
			Notes:                      strings.TrimSpace(input.Notes),
		})
		if err != nil {
			return nil, err
		}
		return tradeInResult(result), nil
	})
	register(server, "cancel_trade_in_link", "Cancel one confirmed trade-in relation by atomically voiding both paired events and appending cancelled replacements under the same link_id. The buy and sell records are retained and no amount changes; the relation leaves the effective view and stays visible in history, and the same pair may be linked again afterwards. Pass the link_id with both expected paired event IDs; money errors use the ordinary lifecycle correction instead. Reuse request_key on retries.", ScopeLifecycle, application.CapabilityManageLifecycle, func(ctx context.Context, p application.Principal, input CancelTradeInLinkInput) (any, error) {
		occurredAt, err := parseTradeInTimestamp(input.OccurredAt)
		if err != nil {
			return nil, err
		}
		result, err := s.Management.CancelTradeInLink(ctx, p, input.RequestKey, application.CancelTradeInLinkCommand{
			LinkID:                     strings.TrimSpace(input.LinkID),
			ExpectedSourceEventID:      strings.TrimSpace(input.ExpectedSourceEventID),
			ExpectedDestinationEventID: strings.TrimSpace(input.ExpectedDestinationEventID),
			OccurredAt:                 occurredAt,
			Notes:                      strings.TrimSpace(input.Notes),
		})
		if err != nil {
			return nil, err
		}
		return tradeInResult(result), nil
	})
}
