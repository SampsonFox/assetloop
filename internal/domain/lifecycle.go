package domain

import "time"

type AssetEventType string

const (
	AssetEventPurchase AssetEventType = "purchase"
	AssetEventRepair   AssetEventType = "repair"
	AssetEventSale     AssetEventType = "sale"
	AssetEventVoid     AssetEventType = "void"
)

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

type ImportDraft struct {
	ID                     string
	TenantID               string
	AssetID                string
	EventType              AssetEventType
	AmountMinor            int64
	Currency               string
	OccurredAt             time.Time
	Source                 string
	ExternalReference      string
	Notes                  string
	RawText                string
	Status                 string
	CreatedByUserID        string
	CreatedAt              time.Time
	ConfirmedTransactionID string
}
