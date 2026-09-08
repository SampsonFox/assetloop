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
