package application

import (
	"context"
	"github.com/SampsonFox/assetloop/internal/domain"
	"time"
)

type MarketQuery struct{ Keyword, FilterCriteria string }
type MarketQuote struct {
	ModelDesc, Currency, Provider, ProviderVersion, Evidence string
	MaxMinor                                                 int64
	MinMinor                                                 *int64
	ObservedAt                                               time.Time
	SourceDate                                               *string
	SampleCount                                              *int64
}
type MarketDataProvider interface {
	FetchQuote(context.Context, MarketQuery) (MarketQuote, error)
}
type FXRate struct {
	Scaled       int64
	Date, Source string
}
type FXProvider interface {
	Rate(context.Context, string, string, string) (FXRate, error)
}
type MarketStore interface {
	WithMarketWrite(context.Context, string, func(MarketStore) error) error
	TenantBaseCurrency(context.Context, string) (string, bool, error)
	LockMarketBaseCurrency(context.Context, string, string) error
	GetAsset(context.Context, string, string) (domain.Asset, error)
	GetAssetSummary(context.Context, string, string) (domain.AssetSummary, error)
	ListAssetEvents(context.Context, string, string) ([]domain.AssetEvent, error)
	GetMarketItem(context.Context, string, string) (domain.MarketItem, error)
	ListMarketItems(context.Context, string) ([]domain.MarketItem, error)
	ListMarketTenants(context.Context) ([]string, error)
	CreateMarketItem(context.Context, domain.MarketItem) error
	UpdateMarketItem(context.Context, domain.MarketItem) error
	BindAssetMarket(context.Context, string, string, string) error
	MarketAssetIDs(context.Context, string, string) ([]string, error)
	PutMarketPrice(context.Context, domain.MarketPrice) error
	ListMarketPrices(context.Context, string, string) ([]domain.MarketPrice, error)
	ClaimMarketLease(context.Context, string, string, string, time.Time, time.Time) (bool, error)
	FinishMarketLease(context.Context, string, string, string, time.Time, string) error
}
