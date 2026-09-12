package domain

import "time"

// MarketItem is an explicit shared series. Its query identity is immutable.
type MarketItem struct {
	ID, TenantID, Name, Provider, Keyword, FilterCriteria, ModelDesc, Region, ExternalID string
	Enabled                                                                              bool
	CreatedAt, LastAttempt, LastSuccess                                                  time.Time
	LastError                                                                            string
	LeaseToken                                                                           string
	LeaseUntil                                                                           time.Time
	SelectionJSON                                                                        string // Immutable, optional MarketSelection snapshot; empty for older/direct queries.
}

type MarketSpecificationOption struct {
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	Selected    *int   `json:"selected,omitempty"`
}
type MarketSpecification struct {
	Name         string                      `json:"name"`
	Value        string                      `json:"value"`
	Explanations []string                    `json:"explanations,omitempty"`
	ValueOptions []MarketSpecificationOption `json:"valueOptions,omitempty"`
}
type MarketSelection struct {
	Provider       string                `json:"provider"`
	ProductID      string                `json:"productId"`
	BusinessType   string                `json:"businessType,omitempty"`
	Title          string                `json:"title"`
	Condition      string                `json:"condition,omitempty"`
	Specifications []MarketSpecification `json:"specifications"`
	ObservedAt     time.Time             `json:"observedAt"`
}
type MarketPrice struct {
	TenantID, MarketItemID, ObservationDate         string
	ObservedAt                                      time.Time
	MaxMinor                                        int64
	MinMinor                                        *int64
	Currency, BaseCurrency                          string
	BaseMinor                                       *int64
	FX                                              *FXEvidence
	Provider, ProviderVersion, Provenance, Evidence string
	SourceDate                                      *string
	SampleCount                                     *int64
}
