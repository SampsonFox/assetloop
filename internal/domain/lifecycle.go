package domain

import "time"

type AssetEventType string

type AssetEventCashflow string

const (
	AssetEventPurchase AssetEventType = "purchase"
	AssetEventRepair   AssetEventType = "repair"
	AssetEventSale     AssetEventType = "sale"
	AssetEventVoid     AssetEventType = "void"

	// Neutral pairing types are immutable system types written only by the
	// dedicated trade-in service. They carry no amount, so they can never change
	// cost, acquisition status or holding duration.
	AssetEventTradeInSource      AssetEventType = "trade_in_source"
	AssetEventTradeInDestination AssetEventType = "trade_in_destination"

	AssetEventExpense AssetEventCashflow = "expense"
	AssetEventIncome  AssetEventCashflow = "income"
	AssetEventNeutral AssetEventCashflow = "neutral"
)

// TradeInDirection names the role of one asset inside a trade-in pair. The value
// is derived from the paired system type, never from a display name.
type TradeInDirection string

// TradeInState is the lifecycle state of a paired relation. It never replaces the
// append-only void/replacement history; a cancelled pair keeps both rows visible.
type TradeInState string

const (
	TradeInDirectionSource      TradeInDirection = "source"
	TradeInDirectionDestination TradeInDirection = "destination"

	TradeInStateActive    TradeInState = "active"
	TradeInStateCancelled TradeInState = "cancelled"
)

// IsTradeInSystemType identifies the neutral pairing types. Custom types are never
// recognized by their name.
func IsTradeInSystemType(value AssetEventType) bool {
	return value == AssetEventTradeInSource || value == AssetEventTradeInDestination
}

type AssetEventTypeDefinition struct {
	ID              string
	TenantID        string
	Name            string
	NormalizedName  string
	Cashflow        AssetEventCashflow
	BuiltIn         bool
	SystemCode      AssetEventType
	Enabled         bool
	ReferenceCount  int64
	UpdatedAt       time.Time
	CreatedByUserID string
	CreatedAt       time.Time
}

type AssetTransaction struct {
	ID                string
	TenantID          string
	OccurredAt        time.Time
	Source            string
	ExternalReference string
	Notes             string
	CreatedByUserID   string
	CreatedAt         time.Time
}

type FXEvidence struct {
	OriginalAmountMinor int64
	OriginalCurrency    string
	RateScaled          int64
	RateDate            time.Time
	RateSource          string
}

type AssetEvent struct {
	SystemType      AssetEventType
	TypeID          string
	ID              string
	TenantID        string
	AssetID         string
	TransactionID   string
	Type            AssetEventType
	BaseAmountMinor int64
	BaseCurrency    string
	FX              *FXEvidence
	Notes           string
	VoidsEventID    string
	ReplacesEventID string
	OccurredAt      time.Time
	CreatedByUserID string
	CreatedAt       time.Time
	IsVoided        bool

	// Trade-in relation projection. RelatedAssetID is an optional neutral pointer
	// to the counterpart asset of this event's asset; it is deliberately not a
	// foreign key, so purging either endpoint never cascades or blocks. The name
	// and spec columns are the write-time snapshot used after a purge, while
	// RelatedAssetDeleted reports that the live asset is gone.
	RelatedAssetID      string
	RelatedAssetName    string
	RelatedAssetSpec    string
	RelatedAssetDeleted bool
	TradeInLinkID       string
	TradeInState        TradeInState

	// Grouping-transaction metadata read back with the event. A trade-in
	// association owns its own order reference, independent of the buy/sell
	// money, so an edit form can prefill it instead of dropping it.
	Source            string
	ExternalReference string
}

type AssetSummary struct {
	BaseCurrency     string
	ExpenseMinor     int64
	IncomeMinor      int64
	NetCashflowMinor int64
	Status           string
}

// StorageType is a compatibility marker, never a custom display name.
func (e AssetEvent) StorageType() string {
	switch e.Kind() {
	case AssetEventPurchase, AssetEventRepair, AssetEventSale, AssetEventVoid:
		return string(e.Kind())
	}
	return "custom"
}

// Kind uses the immutable linked system code; the fallback supports unpersisted fixtures.
func (e AssetEvent) Kind() AssetEventType {
	if e.TypeID != "" {
		return e.SystemType
	}
	return e.Type
}

// TradeInRole maps the immutable system code to the paired-event direction.
func (e AssetEvent) TradeInRole() (TradeInDirection, bool) {
	switch e.Kind() {
	case AssetEventTradeInSource:
		return TradeInDirectionSource, true
	case AssetEventTradeInDestination:
		return TradeInDirectionDestination, true
	}
	return "", false
}

// RelatedAssetLabel is the read projection of the optional counterpart: the
// current name while the asset exists, otherwise the write-time snapshot. The
// boolean reports a purged target so callers never present a live link to a
// deleted asset. A missing relation returns an empty label.
func (e AssetEvent) RelatedAssetLabel() (string, bool) {
	if e.RelatedAssetID == "" {
		return "", false
	}
	return e.RelatedAssetName, e.RelatedAssetDeleted
}

// RelatedAssetSpecLabel returns the current model/specification label of the
// counterpart, or its write-time snapshot after a purge.
func (e AssetEvent) RelatedAssetSpecLabel() string {
	return e.RelatedAssetSpec
}

// IsActiveTradeInPair reports a currently effective neutral relation.
func (e AssetEvent) IsActiveTradeInPair() bool {
	if e.IsVoided || e.TradeInLinkID == "" {
		return false
	}
	if _, ok := e.TradeInRole(); !ok {
		return false
	}
	return e.TradeInState == TradeInStateActive
}

// TradeInRole reports the paired-event direction a selectable system type
// carries, so transports can expose the two trade-in types with their direction
// metadata instead of a technical code.
func (t AssetEventTypeDefinition) TradeInRole() (TradeInDirection, bool) {
	switch t.SystemCode {
	case AssetEventTradeInSource:
		return TradeInDirectionSource, true
	case AssetEventTradeInDestination:
		return TradeInDirectionDestination, true
	}
	return "", false
}
