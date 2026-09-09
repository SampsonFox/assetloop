package application

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/SampsonFox/assetloop/internal/domain"
	"reflect"
	"strings"
	"sync"
	"time"
)

// All provider calls use the same per-service limit and retry policy, outside DB transactions.
func marketExternal[T any](ctx context.Context, s *MarketService, fn func() (T, error)) (T, error) {
	var zero T
	s.mu.Lock()
	defer s.mu.Unlock()
	for attempt := 0; attempt < 3; attempt++ {
		if wait := time.Until(s.next); wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return zero, ctx.Err()
			case <-timer.C:
			}
		}
		s.next = time.Now().Add(s.options.MinInterval)
		result, e := fn()
		if e == nil {
			return result, nil
		}
		if attempt == 2 || (!errors.Is(e, ErrMarketTemporary) && !errors.Is(e, ErrMarketRateLimit)) {
			return zero, e
		}
		s.next = time.Now().Add(time.Duration(attempt+1) * time.Second)
	}
	return zero, ErrMarketTemporary
}

type MarketDiscoveryState struct {
	ID, Step           string
	SearchQuery, Query MarketQuery
	Results            *ProductSearchResult
	Detail             *MarketProductDetail
	Quote              *MarketQuote
	Selected           int
}
type marketDiscoveryDraft struct {
	mu           sync.Mutex
	tenant, user string
	expires      time.Time
	state        MarketDiscoveryState
}

// Drafts are bounded, short-lived and principal-scoped. Provider tuples and confirmed
// snapshots stay on the server; forms carry only the random draft ID and a row index.
func (s *MarketService) NewMarketDiscovery(ctx context.Context, a Principal, q MarketQuery, direct bool) (MarketDiscoveryState, error) {
	if e := a.Require(CapabilityManageCatalog); e != nil {
		return MarketDiscoveryState{}, e
	}
	if len(q.Keyword) > 800 || len(q.FilterCriteria) > 2000 {
		return MarketDiscoveryState{}, ErrMarketInvalid
	}
	s.draftMu.Lock()
	defer s.draftMu.Unlock()
	now := s.options.Now()
	count := 0
	for id, d := range s.drafts {
		if !d.expires.After(now) {
			delete(s.drafts, id)
		} else if d.tenant == a.TenantID && d.user == a.UserID {
			count++
		}
	}
	// ponytail: bounded in-process drafts; multi-instance routing would need shared ephemeral storage.
	if len(s.drafts) >= 256 || count >= 24 {
		return MarketDiscoveryState{}, ErrMarketBusy
	}
	step := "search"
	if direct {
		step = "query"
	}
	state := MarketDiscoveryState{ID: newID(), Step: step, SearchQuery: q, Query: q, Selected: -1}
	s.drafts[state.ID] = &marketDiscoveryDraft{tenant: a.TenantID, user: a.UserID, expires: now.Add(30 * time.Minute), state: state}
	return state, nil
}
func (s *MarketService) discovery(a Principal, id string) (*marketDiscoveryDraft, error) {
	if e := a.Require(CapabilityManageCatalog); e != nil {
		return nil, e
	}
	s.draftMu.Lock()
	defer s.draftMu.Unlock()
	d := s.drafts[id]
	if d == nil || d.tenant != a.TenantID || d.user != a.UserID || !d.expires.After(s.options.Now()) {
		return nil, ErrMarketDraft
	}
	return d, nil
}
func discoveryCopy(state MarketDiscoveryState) MarketDiscoveryState {
	b, _ := json.Marshal(state)
	var out MarketDiscoveryState
	_ = json.Unmarshal(b, &out)
	return out
}
func (s *MarketService) MarketDiscovery(ctx context.Context, a Principal, id string) (MarketDiscoveryState, error) {
	d, e := s.discovery(a, id)
	if e != nil {
		return MarketDiscoveryState{}, e
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return discoveryCopy(d.state), nil
}
func (s *MarketService) SearchProducts(ctx context.Context, a Principal, id string, q MarketQuery, next bool) (MarketDiscoveryState, error) {
	d, e := s.discovery(a, id)
	if e != nil {
		return MarketDiscoveryState{}, e
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	q, e = marketQuery(q)
	if e != nil {
		return discoveryCopy(d.state), e
	}
	if s.options.Discovery == nil {
		return discoveryCopy(d.state), ErrMarketUnavailable
	}
	token := ""
	if next {
		if d.state.Results == nil || !d.state.Results.HasNext || q != d.state.SearchQuery {
			return discoveryCopy(d.state), ErrMarketInvalid
		}
		token = d.state.Results.NextPageToken
	}
	d.state.Quote = nil
	d.state.Detail = nil
	d.state.Step = "search"
	d.state.Selected = -1
	result, e := marketExternal(ctx, s, func() (ProductSearchResult, error) {
		return s.options.Discovery.SearchProducts(ctx, ProductSearchQuery{Keyword: q.Keyword, FilterCriteria: q.FilterCriteria, PageToken: token})
	})
	if e != nil {
		return discoveryCopy(d.state), e
	}
	d.state.SearchQuery = q
	d.state.Query = q
	d.state.Results = &result
	return discoveryCopy(d.state), nil
}
func (s *MarketService) GetProductDetail(ctx context.Context, a Principal, id string, index int) (MarketDiscoveryState, error) {
	d, e := s.discovery(a, id)
	if e != nil {
		return MarketDiscoveryState{}, e
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if s.options.Discovery == nil {
		return discoveryCopy(d.state), ErrMarketUnavailable
	}
	if d.state.Results == nil || index < 0 || index >= len(d.state.Results.Items) {
		return discoveryCopy(d.state), ErrMarketInvalid
	}
	d.state.Quote = nil
	d.state.Detail = nil
	d.state.Selected = -1
	d.state.Step = "search"
	ref := d.state.Results.Items[index].Reference
	result, e := marketExternal(ctx, s, func() (MarketProductDetail, error) { return s.options.Discovery.GetProductDetail(ctx, ref) })
	if e != nil {
		return discoveryCopy(d.state), e
	}
	if result.Selection.ProductID != ref.ID || result.Selection.Provider != d.state.Results.Items[index].Provider {
		return discoveryCopy(d.state), ErrMarketInvalid
	}
	d.state.Detail = &result
	d.state.Selected = index
	d.state.Step = "detail"
	return discoveryCopy(d.state), nil
}

// SpecKind is a presentation/prefill policy over explicit provider fields, not title inference.
func MarketSpecKind(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "型号", "产品型号", "model":
		return "model"
	case "存储容量", "机身内存", "存储空间", "storage":
		return "storage"
	case "运行内存", "ram":
		return "ram"
	case "颜色", "机身颜色", "color", "colour":
		return "color"
	case "版本", "购买渠道", "销售版本", "version":
		return "version"
	case "成色", "condition":
		return "condition"
	}
	return ""
}
func MarketSpecValue(spec domain.MarketSpecification) string {
	if strings.TrimSpace(spec.Value) != "" && spec.Value != "null" {
		return spec.Value
	}
	var selected []string
	for _, v := range spec.ValueOptions {
		if v.Selected != nil && *v.Selected == 1 && v.Value != "" {
			selected = append(selected, v.Value)
		}
	}
	return strings.Join(selected, "、")
}
func (s *MarketService) EditMarketDiscovery(ctx context.Context, a Principal, id, action string, input ...MarketQuery) (MarketDiscoveryState, error) {
	d, e := s.discovery(a, id)
	if e != nil {
		return MarketDiscoveryState{}, e
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.state.Quote = nil
	switch action {
	case "direct":
		if len(input) > 0 {
			q := input[0]
			if len(q.Keyword) > 800 || len(q.FilterCriteria) > 2000 {
				return discoveryCopy(d.state), ErrMarketInvalid
			}
			d.state.Query = q
		}
		d.state.Detail = nil
		d.state.Selected = -1
		d.state.Step = "query"
	case "search":
		d.state.Detail = nil
		d.state.Selected = -1
		d.state.Step = "search"
	case "edit":
		d.state.Step = "query"
	case "use":
		if d.state.Detail == nil {
			return discoveryCopy(d.state), ErrMarketInvalid
		}
		q := d.state.SearchQuery
		var filters []string
		for _, spec := range d.state.Detail.Selection.Specifications {
			value := MarketSpecValue(spec)
			if value == "" {
				continue
			}
			switch MarketSpecKind(spec.Name) {
			case "model":
				q.Keyword = value
			case "storage", "ram", "version", "color":
				filters = append(filters, spec.Name+":"+value)
			}
		}
		q.FilterCriteria = strings.Join(filters, ",")
		d.state.Query = q
		d.state.Step = "query"
	default:
		return discoveryCopy(d.state), ErrMarketInvalid
	}
	return discoveryCopy(d.state), nil
}
func (s *MarketService) PreviewMarketDiscovery(ctx context.Context, a Principal, id string, q MarketQuery) (MarketDiscoveryState, error) {
	d, e := s.discovery(a, id)
	if e != nil {
		return MarketDiscoveryState{}, e
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.state.Quote = nil
	d.state.Step = "query"
	q, e = marketQuery(q)
	if e != nil {
		return discoveryCopy(d.state), e
	}
	d.state.Query = q
	quote, e := s.fetch(ctx, q)
	if e != nil {
		return discoveryCopy(d.state), e
	}
	d.state.Quote = &quote
	d.state.Step = "preview"
	return discoveryCopy(d.state), nil
}
func sameSelection(a, b domain.MarketSelection) bool {
	a.ObservedAt = time.Time{}
	b.ObservedAt = time.Time{}
	return reflect.DeepEqual(a, b)
}
func (s *MarketService) SaveMarketDiscovery(ctx context.Context, a Principal, id, name string, accept bool) (domain.MarketItem, error) {
	d, e := s.discovery(a, id)
	if e != nil {
		return domain.MarketItem{}, e
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !accept {
		return domain.MarketItem{}, ErrMarketScope
	}
	if d.state.Step != "preview" || d.state.Quote == nil {
		return domain.MarketItem{}, ErrMarketMismatch
	}
	selectionJSON := ""
	if d.state.Detail != nil {
		ref := d.state.Results.Items[d.state.Selected].Reference
		fresh, e := marketExternal(ctx, s, func() (MarketProductDetail, error) { return s.options.Discovery.GetProductDetail(ctx, ref) })
		if e != nil {
			d.state.Quote = nil
			d.state.Step = "query"
			return domain.MarketItem{}, e
		}
		if !sameSelection(d.state.Detail.Selection, fresh.Selection) {
			d.state.Detail = &fresh
			d.state.Quote = nil
			d.state.Step = "detail"
			return domain.MarketItem{}, ErrMarketSelectionChanged
		}
		b, e := json.Marshal(fresh.Selection)
		if e != nil {
			return domain.MarketItem{}, ErrMarketInvalid
		}
		selectionJSON = string(b)
	}
	item, e := s.create(ctx, a, CreateMarketItem{Name: name, Query: d.state.Query, ConfirmedModel: d.state.Quote.ModelDesc}, selectionJSON)
	if errors.Is(e, ErrMarketMismatch) {
		d.state.Quote = nil
		d.state.Step = "query"
	}
	return item, e
}
