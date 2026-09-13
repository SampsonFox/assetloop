package application

import (
	"context"
	"errors"
	"fmt"
	"github.com/SampsonFox/assetloop/internal/domain"
	"strings"
	"sync"
	"time"
)

var (
	ErrMarketAuth      = errors.New("market.authentication")
	ErrMarketRateLimit = errors.New("market.rate_limit")
	ErrMarketTemporary = errors.New("market.temporary")
	ErrMarketInvalid   = errors.New("market.invalid_response")

	ErrMarketUnavailable      = errors.New("market.unconfigured")
	ErrMarketMismatch         = errors.New("market.model_changed")
	ErrMarketBusy             = errors.New("market.busy")
	ErrMarketFX               = errors.New("market.fx_pending")
	ErrMarketProductGone      = errors.New("market.product_gone")
	ErrMarketDraft            = errors.New("market.draft_expired")
	ErrMarketSelectionChanged = errors.New("market.selection_changed")
	ErrMarketScope            = errors.New("market.accept_scope")
)
var MarketLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

func MarketDate(t time.Time) string { return t.In(MarketLocation).Format("2006-01-02") }

type MarketOptions struct {
	Now         func() time.Time
	MinInterval time.Duration
	Discovery   ProductDiscoveryProvider
}
type MarketService struct {
	store    MarketStore
	provider MarketDataProvider
	fx       FXProvider
	options  MarketOptions
	mu       sync.Mutex
	next     time.Time
	draftMu  sync.Mutex
	drafts   map[string]*marketDiscoveryDraft
}

func NewMarketService(store MarketStore, p MarketDataProvider, fx FXProvider, o MarketOptions) *MarketService {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Discovery == nil {
		o.Discovery, _ = p.(ProductDiscoveryProvider)
	}
	return &MarketService{drafts: make(map[string]*marketDiscoveryDraft), store: store, provider: p, fx: fx, options: o}
}
func (s *MarketService) Configured() bool { return s.provider != nil }
func marketQuery(q MarketQuery) (MarketQuery, error) {
	var e error
	q.Keyword, e = catalogText("keyword", q.Keyword, 200, true)
	if e != nil {
		return q, e
	}
	q.FilterCriteria, e = catalogText("filter criteria", q.FilterCriteria, 500, false)
	return q, e
}
func (s *MarketService) Preview(ctx context.Context, actor Principal, q MarketQuery) (MarketQuote, error) {
	if e := actor.Require(CapabilityManageCatalog); e != nil {
		return MarketQuote{}, e
	}
	q, e := marketQuery(q)
	if e != nil {
		return MarketQuote{}, e
	}
	return s.fetch(ctx, q)
}
func (s *MarketService) fetch(ctx context.Context, q MarketQuery) (MarketQuote, error) {
	if s.provider == nil {
		return MarketQuote{}, ErrMarketUnavailable
	}
	return marketExternal(ctx, s, func() (MarketQuote, error) {
		quote, e := s.provider.FetchQuote(ctx, q)
		if e != nil {
			return MarketQuote{}, e
		}
		currency, currencyErr := domain.NormalizeCurrency(quote.Currency)
		if strings.TrimSpace(quote.Provider) == "" || currencyErr != nil || currency != quote.Currency || quote.MaxMinor <= 0 || strings.TrimSpace(quote.ModelDesc) == "" || quote.ObservedAt.IsZero() || quote.ObservedAt.After(s.options.Now().Add(time.Minute)) {
			return MarketQuote{}, ErrMarketInvalid
		}
		if quote.MinMinor != nil && (*quote.MinMinor <= 0 || *quote.MinMinor > quote.MaxMinor) {
			return MarketQuote{}, ErrMarketInvalid
		}
		return quote, nil
	})
}

type CreateMarketItem struct {
	Name           string
	Query          MarketQuery
	ConfirmedModel string
	AssetID        string
}

func (s *MarketService) Create(ctx context.Context, a Principal, cmd CreateMarketItem) (domain.MarketItem, error) {
	return s.create(ctx, a, cmd, "")
}
func (s *MarketService) create(ctx context.Context, a Principal, cmd CreateMarketItem, selectionJSON string, commit ...func(domain.MarketItem, domain.MarketPrice) (domain.MarketItem, error)) (domain.MarketItem, error) {
	if e := a.Require(CapabilityManageCatalog); e != nil {
		return domain.MarketItem{}, e
	}
	name, e := catalogText("market item name", cmd.Name, 200, true)
	if e != nil {
		return domain.MarketItem{}, e
	}
	q, e := marketQuery(cmd.Query)
	if e != nil {
		return domain.MarketItem{}, e
	}
	if strings.TrimSpace(cmd.ConfirmedModel) == "" {
		return domain.MarketItem{}, ErrMarketMismatch
	}
	quote, e := s.fetch(ctx, q)
	if e != nil {
		return domain.MarketItem{}, e
	}
	if quote.ModelDesc != cmd.ConfirmedModel {
		return domain.MarketItem{}, ErrMarketMismatch
	}
	base, _, e := s.store.TenantBaseCurrency(ctx, a.TenantID)
	if e != nil {
		return domain.MarketItem{}, e
	}
	item := domain.MarketItem{SelectionJSON: selectionJSON, ID: newID(), TenantID: a.TenantID, Name: name, Provider: quote.Provider, Keyword: q.Keyword, FilterCriteria: q.FilterCriteria, ModelDesc: quote.ModelDesc, Region: "CN", Enabled: true, CreatedAt: s.options.Now().UTC(), LastSuccess: quote.ObservedAt}
	price := s.price(item, quote, base)
	s.convert(ctx, &price)
	if len(commit) > 0 {
		return commit[0](item, price)
	}
	return s.commitCreate(ctx, a, cmd.AssetID, item, price)
}

func (s *MarketService) commitCreate(ctx context.Context, a Principal, assetID string, item domain.MarketItem, price domain.MarketPrice) (domain.MarketItem, error) {
	e := s.store.WithMarketWrite(ctx, a.TenantID, func(st MarketStore) error {
		// The tenant lock makes exact-query deduplication and first-price currency locking atomic.
		items, err := st.ListMarketItems(ctx, a.TenantID)
		if err != nil {
			return err
		}
		for _, existing := range items {
			if existing.Provider == item.Provider && existing.Keyword == item.Keyword && existing.FilterCriteria == item.FilterCriteria && existing.Region == item.Region {
				if existing.ModelDesc != item.ModelDesc {
					return ErrMarketMismatch
				}
				item = existing
				break
			}
		}
		if item.ID == price.MarketItemID {
			if err = st.CreateMarketItem(ctx, item); err != nil {
				return err
			}
			if err = s.savePrice(ctx, st, price); err != nil {
				return err
			}
		}
		if assetID != "" {
			if _, err = st.GetAsset(ctx, a.TenantID, assetID); err != nil {
				return err
			}
			if err = st.BindAssetMarket(ctx, a.TenantID, assetID, item.ID); err != nil {
				return err
			}
		}
		return nil
	})
	return item, e
}

func (s *MarketService) price(m domain.MarketItem, q MarketQuote, base string) domain.MarketPrice {
	return domain.MarketPrice{TenantID: m.TenantID, MarketItemID: m.ID, ObservationDate: MarketDate(q.ObservedAt), ObservedAt: q.ObservedAt.UTC(), MaxMinor: q.MaxMinor, MinMinor: q.MinMinor, Currency: q.Currency, BaseCurrency: base, Provider: q.Provider, ProviderVersion: q.ProviderVersion, Provenance: "provider_latest_period_deal_max/v1", Evidence: q.Evidence, SourceDate: q.SourceDate, SampleCount: q.SampleCount}
}
func (s *MarketService) convert(ctx context.Context, p *domain.MarketPrice) bool {
	rate := FXRate{Scaled: domain.FXRateScale, Date: p.ObservationDate, Source: "identity"}
	if p.Currency != p.BaseCurrency {
		if s.fx == nil {
			return false
		}
		var e error
		rate, e = s.fx.Rate(ctx, p.Currency, p.BaseCurrency, p.ObservationDate)
		if e != nil {
			return false
		}
	}
	date, e := time.Parse("2006-01-02", rate.Date)
	if e != nil || rate.Date > p.ObservationDate || rate.Scaled <= 0 || rate.Source == "" {
		return false
	}
	amount, e := domain.ConvertMinor(p.MaxMinor, p.Currency, p.BaseCurrency, rate.Scaled)
	if e != nil {
		return false
	}
	p.BaseMinor = &amount
	p.FX = &domain.FXEvidence{OriginalAmountMinor: p.MaxMinor, OriginalCurrency: p.Currency, RateScaled: rate.Scaled, RateDate: date, RateSource: rate.Source}
	return true
}
func (s *MarketService) savePrice(ctx context.Context, st MarketStore, p domain.MarketPrice) error {
	base, _, e := st.TenantBaseCurrency(ctx, p.TenantID)
	if e != nil {
		return e
	}
	if base != p.BaseCurrency {
		return errors.New("market.base_currency_changed")
	}
	if e = st.PutMarketPrice(ctx, p); e != nil {
		return e
	}
	return st.LockMarketBaseCurrency(ctx, p.TenantID, p.BaseCurrency)
}
func (s *MarketService) List(ctx context.Context, a Principal) ([]domain.MarketItem, error) {
	if e := a.Require(CapabilityView); e != nil {
		return nil, e
	}
	return s.store.ListMarketItems(ctx, a.TenantID)
}
func (s *MarketService) LatestForAsset(ctx context.Context, a Principal, id string) (*domain.MarketPrice, error) {
	if e := a.Require(CapabilityView); e != nil {
		return nil, e
	}
	asset, e := s.store.GetAsset(ctx, a.TenantID, id)
	if e != nil {
		return nil, e
	}
	if asset.MarketItemID == "" {
		return nil, nil
	}
	rows, e := s.store.ListMarketPrices(ctx, a.TenantID, asset.MarketItemID)
	if e != nil || len(rows) == 0 {
		return nil, e
	}
	return &rows[0], nil
}
func (s *MarketService) Update(ctx context.Context, a Principal, id, name string, enabled bool) error {
	if e := a.Require(CapabilityManageCatalog); e != nil {
		return e
	}
	name, e := catalogText("market item name", name, 200, true)
	if e != nil {
		return e
	}
	return s.store.WithMarketWrite(ctx, a.TenantID, func(st MarketStore) error {
		m, e := st.GetMarketItem(ctx, a.TenantID, id)
		if e != nil {
			return e
		}
		m.Name = name
		m.Enabled = enabled
		return st.UpdateMarketItem(ctx, m)
	})
}
func (s *MarketService) Bind(ctx context.Context, a Principal, asset, id string) error {
	if e := a.Require(CapabilityManageAssets); e != nil {
		return e
	}
	return s.store.WithMarketWrite(ctx, a.TenantID, func(st MarketStore) error {
		if _, e := st.GetAsset(ctx, a.TenantID, asset); e != nil {
			return e
		}
		if id != "" {
			if _, e := st.GetMarketItem(ctx, a.TenantID, id); e != nil {
				return e
			}
		}
		return st.BindAssetMarket(ctx, a.TenantID, asset, id)
	})
}
func (s *MarketService) Refresh(ctx context.Context, a Principal, id string) error {
	if e := a.Require(CapabilityManageCatalog); e != nil {
		return e
	}
	return s.refresh(ctx, a.TenantID, id)
}
func (s *MarketService) refresh(ctx context.Context, tenant, id string, commit ...func(func(MarketStore) error) error) (result error) {
	if s.provider == nil {
		return ErrMarketUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	token := newID()
	now := s.options.Now()
	ok, e := s.store.ClaimMarketLease(ctx, tenant, id, token, now, now.Add(5*time.Minute))
	if e != nil {
		return e
	}
	if !ok {
		return ErrMarketBusy
	}
	defer func() {
		if result != nil {
			cleanup, done := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer done()
			_ = s.store.FinishMarketLease(cleanup, tenant, id, token, time.Time{}, MarketErrorCode(result))
		}
	}()
	item, e := s.store.GetMarketItem(ctx, tenant, id)
	if e != nil {
		return e
	}
	old, e := s.store.ListMarketPrices(ctx, tenant, id)
	if e != nil {
		return e
	}
	var repaired []domain.MarketPrice
	for _, p := range old {
		if p.BaseMinor == nil && s.convert(ctx, &p) {
			repaired = append(repaired, p)
		}
	}
	// Repair historical FX independently, retaining each observation's original date and amount.
	if len(repaired) > 0 {
		e = s.store.WithMarketWrite(ctx, tenant, func(st MarketStore) error {
			m, e := st.GetMarketItem(ctx, tenant, id)
			if e != nil {
				return e
			}
			if m.LeaseToken != token || !m.LeaseUntil.After(s.options.Now()) {
				return ErrMarketBusy
			}
			for _, p := range repaired {
				if e = s.savePrice(ctx, st, p); e != nil {
					return e
				}
			}
			return nil
		})
		if e != nil {
			return e
		}
	}
	quote, e := s.fetch(ctx, MarketQuery{Keyword: item.Keyword, FilterCriteria: item.FilterCriteria})
	if e != nil {
		return e
	}
	if quote.Provider != item.Provider || quote.ModelDesc != item.ModelDesc {
		return ErrMarketMismatch
	}
	base, _, e := s.store.TenantBaseCurrency(ctx, tenant)
	if e != nil {
		return e
	}
	price := s.price(item, quote, base)
	s.convert(ctx, &price)
	save := func(st MarketStore) error {
		m, e := st.GetMarketItem(ctx, tenant, id)
		if e != nil {
			return e
		}
		if !m.Enabled || m.LeaseToken != token || !m.LeaseUntil.After(s.options.Now()) {
			return ErrMarketBusy
		}
		if e = s.savePrice(ctx, st, price); e != nil {
			return e
		}
		return st.FinishMarketLease(ctx, tenant, id, token, price.ObservedAt, "")
	}
	if len(commit) > 0 {
		err := commit[0](save)
		if errors.Is(err, errMarketRefreshReplayed) {
			// Another request committed after our preflight. Release only our own
			// fenced lease while preserving the winner's last status and timestamp.
			current, readErr := s.store.GetMarketItem(ctx, tenant, id)
			if readErr != nil {
				return readErr
			}
			return s.store.FinishMarketLease(ctx, tenant, id, token, current.LastSuccess, current.LastError)
		}
		return err
	}
	return s.store.WithMarketWrite(ctx, tenant, save)
}
func MarketErrorCode(e error) string {
	for _, known := range []error{ErrMarketAuth, ErrMarketRateLimit, ErrMarketTemporary, ErrMarketInvalid, ErrMarketUnavailable, ErrMarketMismatch, ErrMarketBusy, ErrMarketFX, ErrMarketProductGone, ErrMarketDraft, ErrMarketSelectionChanged, ErrMarketScope} {
		if errors.Is(e, known) {
			return known.Error()
		}
	}
	if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
		return "market.temporary"
	}
	return "market.failed"
}

// RefreshDue is the trusted process entry point. Web requests never accept a tenant override.
func (s *MarketService) RefreshDue(ctx context.Context, tenant, id string) (int, error) {
	if s.provider == nil {
		return 0, ErrMarketUnavailable
	}
	tenants := []string{tenant}
	if tenant == "" {
		var e error
		tenants, e = s.store.ListMarketTenants(ctx)
		if e != nil {
			return 0, e
		}
	}
	count := 0
	var failures []error
	for _, tenant := range tenants {
		items, e := s.store.ListMarketItems(ctx, tenant)
		if e != nil {
			return count, e
		}
		for _, m := range items {
			if id != "" && m.ID != id {
				continue
			}
			if !m.Enabled {
				continue
			}
			if id == "" {
				due, e := s.due(ctx, m)
				if e != nil {
					failures = append(failures, e)
					continue
				}
				if !due {
					continue
				}
			}
			if e = s.refresh(ctx, tenant, m.ID); e != nil {
				if errors.Is(e, ErrMarketAuth) {
					return count, e
				}
				if !errors.Is(e, ErrMarketBusy) {
					failures = append(failures, fmt.Errorf("%s: %s", m.ID, MarketErrorCode(e)))
				}
			} else {
				count++
			}
			if ctx.Err() != nil {
				return count, ctx.Err()
			}
		}
	}
	return count, errors.Join(failures...)
}
func (s *MarketService) due(ctx context.Context, m domain.MarketItem) (bool, error) {
	prices, e := s.store.ListMarketPrices(ctx, m.TenantID, m.ID)
	if e != nil {
		return false, e
	}
	if len(prices) > 0 && prices[0].ObservationDate == MarketDate(s.options.Now()) {
		return false, nil
	}
	ids, e := s.store.MarketAssetIDs(ctx, m.TenantID, m.ID)
	if e != nil {
		return false, e
	}
	if len(ids) == 0 {
		return false, nil
	}
	var latestSale time.Time
	for _, id := range ids {
		summary, e := s.store.GetAssetSummary(ctx, m.TenantID, id)
		if e != nil {
			return false, e
		}
		if summary.Status != "sold" {
			return true, nil
		}
		events, e := s.store.ListAssetEvents(ctx, m.TenantID, id)
		if e != nil {
			return false, e
		}
		for _, ev := range events {
			if !ev.IsVoided && ev.Kind() == domain.AssetEventSale && ev.OccurredAt.After(latestSale) {
				latestSale = ev.OccurredAt
			}
		}
	}
	return !latestSale.IsZero() && !s.options.Now().After(latestSale.AddDate(0, 0, 90)), nil
}
