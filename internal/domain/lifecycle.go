package domain

import "time"

type AssetEventType string

type AssetEventCashflow string

const (
	AssetEventPurchase AssetEventType = "purchase"
	AssetEventRepair   AssetEventType = "repair"
	AssetEventSale     AssetEventType = "sale"
	AssetEventVoid     AssetEventType = "void"

	AssetEventExpense AssetEventCashflow = "expense"
	AssetEventIncome  AssetEventCashflow = "income"
	AssetEventNeutral AssetEventCashflow = "neutral"
)

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
