package mcp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type MarketQueryInput struct {
	DraftID        string `json:"draft_id,omitempty" jsonschema:"Draft returned by search/select/preview; expires after 30 minutes or restart. Omit for a new direct preview or search."`
	Keyword        string `json:"keyword"`
	FilterCriteria string `json:"filter_criteria,omitempty" jsonschema:"Only explicitly selected specifications; never infer missing RAM, capacity, color or version."`
	Next           bool   `json:"next,omitempty" jsonschema:"Search only: request the next candidate page using the same draft and query."`
}
type SelectMarketProductInput struct {
	DraftID string `json:"draft_id"`
	Index   int    `json:"index" jsonschema:"Zero-based candidate index from the current search result."`
}
type CreateMarketInput struct {
	DraftID     string `json:"draft_id"`
	Name        string `json:"name"`
	AcceptScope bool   `json:"accept_scope" jsonschema:"True only after user accepts the actual matched market model shown by preview, which may be broader than the product specifications."`
	RequestKey  string `json:"request_key" jsonschema:"Stable command key; reuse identical payload on retries, including after draft expiry."`
}
type UpdateMarketInput struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	RequestKey string `json:"request_key"`
}
type BindMarketInput struct {
	AssetID      string `json:"asset_id"`
	MarketItemID string `json:"market_item_id" jsonschema:"Existing saved market series ID. Empty string explicitly unbinds; never automatically match by model."`
	RequestKey   string `json:"request_key"`
}
type RefreshMarketInput struct {
	ID         string `json:"id"`
	RequestKey string `json:"request_key" jsonschema:"Reuse for retry; a new deliberate refresh requires a new key."`
}

type MarketItemResult struct {
	ID             string                  `json:"id"`
	Name           string                  `json:"name"`
	Provider       string                  `json:"provider"`
	Keyword        string                  `json:"keyword"`
	FilterCriteria string                  `json:"filter_criteria"`
	MatchedModel   string                  `json:"matched_model"`
	Region         string                  `json:"region"`
	Enabled        bool                    `json:"enabled"`
	LastSuccess    time.Time               `json:"last_success"`
	LastError      string                  `json:"last_error,omitempty"`
	Selection      *domain.MarketSelection `json:"selection,omitempty"`
}
type MarketPriceResult struct {
	MarketItemID    string             `json:"market_item_id,omitempty"`
	ObservationDate string             `json:"observation_date,omitempty"`
	ObservedAt      time.Time          `json:"observed_at"`
	MaxMinor        int64              `json:"max_minor"`
	MinMinor        *int64             `json:"min_minor"`
	Currency        string             `json:"currency"`
	BaseCurrency    string             `json:"base_currency,omitempty"`
	BaseMinor       *int64             `json:"base_minor"`
	FX              *domain.FXEvidence `json:"fx,omitempty"`
	Provider        string             `json:"provider"`
	ProviderVersion string             `json:"provider_version"`
	Provenance      string             `json:"provenance"`
	SourceDate      *string            `json:"source_date"`
	SampleCount     *int64             `json:"sample_count"`
}

func marketItemResult(m domain.MarketItem) MarketItemResult {
	r := MarketItemResult{ID: m.ID, Name: m.Name, Provider: m.Provider, Keyword: m.Keyword, FilterCriteria: m.FilterCriteria, MatchedModel: m.ModelDesc, Region: m.Region, Enabled: m.Enabled, LastSuccess: m.LastSuccess, LastError: m.LastError}
	if m.SelectionJSON != "" {
		_ = json.Unmarshal([]byte(m.SelectionJSON), &r.Selection)
	}
	return r
}
func marketPriceResult(p domain.MarketPrice) MarketPriceResult {
	return MarketPriceResult{MarketItemID: p.MarketItemID, ObservationDate: p.ObservationDate, ObservedAt: p.ObservedAt, MaxMinor: p.MaxMinor, MinMinor: p.MinMinor, Currency: p.Currency, BaseCurrency: p.BaseCurrency, BaseMinor: p.BaseMinor, FX: p.FX, Provider: p.Provider, ProviderVersion: p.ProviderVersion, Provenance: p.Provenance, SourceDate: p.SourceDate, SampleCount: p.SampleCount}
}

type marketCandidate struct {
	Index            int    `json:"index"`
	Title            string `json:"title"`
	Currency         string `json:"currency"`
	AskingPriceMinor *int64 `json:"asking_price_minor"`
}
type MarketDiscoveryResult struct {
	DraftID        string                  `json:"draft_id"`
	Step           string                  `json:"step"`
	Keyword        string                  `json:"keyword"`
	FilterCriteria string                  `json:"filter_criteria"`
	Candidates     []marketCandidate       `json:"candidates,omitempty"`
	HasNext        bool                    `json:"has_next"`
	Selection      *domain.MarketSelection `json:"selection,omitempty"`
	MatchedModel   string                  `json:"matched_model,omitempty"`
	Quote          *MarketPriceResult      `json:"quote,omitempty"`
}

func marketDiscoveryResult(d application.MarketDiscoveryState) MarketDiscoveryResult {
	r := MarketDiscoveryResult{DraftID: d.ID, Step: d.Step, Keyword: d.Query.Keyword, FilterCriteria: d.Query.FilterCriteria}
	if d.Results != nil {
		r.HasNext = d.Results.HasNext
		for i, item := range d.Results.Items {
			r.Candidates = append(r.Candidates, marketCandidate{i, item.Title, item.Currency, item.PriceMinor})
		}
	}
	if d.Detail != nil {
		r.Selection = &d.Detail.Selection
	}
	if q := d.Quote; q != nil {
		r.MatchedModel = q.ModelDesc
		r.Quote = &MarketPriceResult{ObservedAt: q.ObservedAt, MaxMinor: q.MaxMinor, MinMinor: q.MinMinor, Currency: q.Currency, Provider: q.Provider, ProviderVersion: q.ProviderVersion, Provenance: "provider_latest_period_deal_max/v1", SourceDate: q.SourceDate, SampleCount: q.SampleCount}
	}
	return r
}

func registerMarket(server *sdk.Server, s Services) {
	available := func() error {
		if s.Market == nil {
			return application.ErrMarketUnavailable
		}
		return nil
	}
	register(server, "list_market_items", "List saved shared secondhand market series. These are reference prices, not lifecycle costs or realized proceeds.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, _ Empty) (any, error) {
		if err := available(); err != nil {
			return nil, err
		}
		items, err := s.Market.List(ctx, p)
		out := make([]MarketItemResult, 0, len(items))
		for _, item := range items {
			out = append(out, marketItemResult(item))
		}
		return out, err
	})
	register(server, "get_market_item", "Read a saved series, explicit asset bindings and daily price history. max_minor is the provider's latest-period maximum, not today's highest sale. Missing dates/sample counts/FX remain null.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q IDInput) (any, error) {
		if err := available(); err != nil {
			return nil, err
		}
		d, err := s.Market.Detail(ctx, p, q.ID)
		if err != nil {
			return nil, err
		}
		prices := make([]MarketPriceResult, 0, len(d.Prices))
		for _, price := range d.Prices {
			prices = append(prices, marketPriceResult(price))
		}
		return struct {
			Item     MarketItemResult    `json:"item"`
			Prices   []MarketPriceResult `json:"prices"`
			AssetIDs []string            `json:"asset_ids"`
		}{marketItemResult(d.Item), prices, d.AssetIDs}, nil
	})
	register(server, "get_asset_market_price", "Read the latest saved reference price for an explicitly bound asset; null when unbound or without prices. Does not refresh or change exact holding costs.", ScopeRead, application.CapabilityView, func(ctx context.Context, p application.Principal, q IDInput) (any, error) {
		if err := available(); err != nil {
			return nil, err
		}
		price, err := s.Market.LatestForAsset(ctx, p, q.ID)
		if err != nil {
			return nil, err
		}
		if price == nil {
			return nil, nil
		}
		return marketPriceResult(*price), nil
	})
	register(server, "search_market_products", "Search external candidate listings for user selection; asking prices are not saved market quotes. Returns a temporary draft, not a business record. Requires catalog permission.", ScopeCatalog, application.CapabilityManageCatalog, func(ctx context.Context, p application.Principal, q MarketQueryInput) (any, error) {
		if err := available(); err != nil {
			return nil, err
		}
		query := application.MarketQuery{Keyword: q.Keyword, FilterCriteria: q.FilterCriteria}
		if q.DraftID == "" {
			if q.Next {
				return nil, application.ErrMarketDraft
			}
			d, err := s.Market.NewMarketDiscovery(ctx, p, query, false)
			if err != nil {
				return nil, err
			}
			q.DraftID = d.ID
		}
		d, err := s.Market.SearchProducts(ctx, p, q.DraftID, query, q.Next)
		return marketDiscoveryResult(d), err
	}, true)
	register(server, "select_market_product", "Fetch explicit specifications of a selected candidate and prefill its query. Missing specifications stay missing. Does not save a market series or bind an asset.", ScopeCatalog, application.CapabilityManageCatalog, func(ctx context.Context, p application.Principal, q SelectMarketProductInput) (any, error) {
		if err := available(); err != nil {
			return nil, err
		}
		if _, err := s.Market.GetProductDetail(ctx, p, q.DraftID, q.Index); err != nil {
			return nil, err
		}
		d, err := s.Market.EditMarketDiscovery(ctx, p, q.DraftID, "use")
		return marketDiscoveryResult(d), err
	}, true)
	register(server, "preview_market_price", "Preview a query without saving. Use selected draft plus its explicit keyword/filter, or omit draft_id for direct model lookup. Show matched_model and original specs to the user; broader scope requires explicit acceptance before create_market_item.", ScopeCatalog, application.CapabilityManageCatalog, func(ctx context.Context, p application.Principal, q MarketQueryInput) (any, error) {
		if err := available(); err != nil {
			return nil, err
		}
		query := application.MarketQuery{Keyword: q.Keyword, FilterCriteria: q.FilterCriteria}
		if q.DraftID == "" {
			d, err := s.Market.NewMarketDiscovery(ctx, p, query, true)
			if err != nil {
				return nil, err
			}
			q.DraftID = d.ID
		}
		d, err := s.Market.PreviewMarketDiscovery(ctx, p, q.DraftID, query)
		return marketDiscoveryResult(d), err
	}, true)
	register(server, "create_market_item", "Save an explicitly accepted preview after rechecking specifications and matched model. Reuses identical saved queries. Does not bind; call bind_asset_market separately. Stable request_key prevents duplicate writes even after draft expiry.", ScopeCatalog, application.CapabilityManageCatalog, func(ctx context.Context, p application.Principal, q CreateMarketInput) (any, error) {
		if s.Management == nil {
			return nil, application.ErrMarketUnavailable
		}
		item, err := s.Management.CreateMarketItem(ctx, p, q.RequestKey, s.Market, application.SaveMarketDiscoveryCommand{DraftID: q.DraftID, Name: q.Name, AcceptScope: q.AcceptScope})
		return marketItemResult(item), err
	})
	register(server, "update_market_item", "Rename or enable/disable a shared series without changing query identity or deleting history. Use a new preview/series for changed configuration.", ScopeCatalog, application.CapabilityManageCatalog, func(ctx context.Context, p application.Principal, q UpdateMarketInput) (any, error) {
		if s.Management == nil {
			return nil, application.ErrMarketUnavailable
		}
		item, err := s.Management.UpdateMarketItem(ctx, p, q.RequestKey, q.ID, q.Name, q.Enabled)
		return marketItemResult(item), err
	})
	register(server, "bind_asset_market", "Explicitly bind an asset to a saved shared market series, or pass empty market_item_id to unbind. Does not record purchase/sale or alter costs.", ScopeCatalog, application.CapabilityManageAssets, func(ctx context.Context, p application.Principal, q BindMarketInput) (any, error) {
		if s.Management == nil {
			return nil, application.ErrMarketUnavailable
		}
		err := s.Management.BindAssetMarket(ctx, p, q.RequestKey, q.AssetID, q.MarketItemID)
		return err == nil, err
	})
	register(server, "refresh_market_price", "Refresh one enabled series using shared rate limits and a fenced database lease. Retains old successful prices on failure. Reuse request_key for retry; new deliberate refresh uses a new key. Never changes lifecycle cashflows.", ScopeCatalog, application.CapabilityManageCatalog, func(ctx context.Context, p application.Principal, q RefreshMarketInput) (any, error) {
		if s.Management == nil {
			return nil, application.ErrMarketUnavailable
		}
		price, err := s.Management.RefreshMarketPrice(ctx, p, q.RequestKey, q.ID, s.Market)
		return marketPriceResult(price), err
	})
}
