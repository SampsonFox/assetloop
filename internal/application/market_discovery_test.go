package application

import (
	"context"
	"errors"
	"github.com/SampsonFox/assetloop/internal/domain"
	"testing"
	"time"
)

type discoveryStub struct {
	queries               []ProductSearchQuery
	detailErr, errorQuote error
	ref                   ProductReference
}

func (p *discoveryStub) SearchProducts(ctx context.Context, q ProductSearchQuery) (ProductSearchResult, error) {
	p.queries = append(p.queries, q)
	id := "first"
	if q.PageToken != "" {
		id = "second"
	}
	return ProductSearchResult{Items: []MarketProduct{{Reference: ProductReference{ID: id, Metric: id + "-metric", BusinessType: "fixture"}, Provider: "fixture", Title: id}}, HasNext: q.PageToken == "", NextPageToken: "page-two"}, nil
}
func (p *discoveryStub) GetProductDetail(ctx context.Context, ref ProductReference) (MarketProductDetail, error) {
	p.ref = ref
	return MarketProductDetail{Selection: domain.MarketSelection{Provider: "fixture", ProductID: ref.ID, Title: "Phone", ObservedAt: time.Now()}}, p.detailErr
}
func (p *discoveryStub) FetchQuote(context.Context, MarketQuery) (MarketQuote, error) {
	return MarketQuote{Provider: "fixture", ModelDesc: "Phone", Currency: "CNY", MaxMinor: 100, ObservedAt: time.Now()}, p.errorQuote
}
func TestDiscoveryIsolationPaginationAndInvalidation(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	actor := Principal{UserID: "one", TenantID: "tenant", Role: RoleOwner}
	p := &discoveryStub{}
	s := NewMarketService(nil, p, nil, MarketOptions{Now: func() time.Time { return now }})
	q := MarketQuery{Keyword: "phone"}
	state, e := s.NewMarketDiscovery(ctx, actor, q, false)
	if e != nil {
		t.Fatal(e)
	}
	state, e = s.SearchProducts(ctx, actor, state.ID, q, false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SearchProducts(ctx, actor, state.ID, MarketQuery{Keyword: "changed"}, true); e == nil {
		t.Fatal("cursor reused with changed query")
	}
	state, e = s.SearchProducts(ctx, actor, state.ID, q, true)
	if e != nil || p.queries[1].PageToken != "page-two" {
		t.Fatal(e, p.queries)
	}
	state, e = s.GetProductDetail(ctx, actor, state.ID, 0)
	if e != nil || p.ref.ID != "second" || p.ref.Metric != "second-metric" {
		t.Fatal(e, p.ref)
	}
	// Returned DTOs cannot modify the authoritative server draft.
	state.Detail.Selection.Title = "tampered"
	again, _ := s.MarketDiscovery(ctx, actor, state.ID)
	if again.Detail.Selection.Title == "tampered" {
		t.Fatal("shared mutable DTO")
	}
	other := actor
	other.UserID = "two"
	if _, e = s.MarketDiscovery(ctx, other, state.ID); !errors.Is(e, ErrMarketDraft) {
		t.Fatal("cross-user draft", e)
	}
	state, e = s.PreviewMarketDiscovery(ctx, actor, state.ID, q)
	if e != nil || state.Quote == nil {
		t.Fatal(e)
	}
	p.errorQuote = ErrMarketInvalid
	state, e = s.PreviewMarketDiscovery(ctx, actor, state.ID, MarketQuery{Keyword: "different"})
	if e == nil || state.Quote != nil || state.Step != "query" {
		t.Fatal("failed query retained confirmation")
	}
	p.detailErr = ErrMarketProductGone
	state, e = s.GetProductDetail(ctx, actor, state.ID, 0)
	if e != ErrMarketProductGone || state.Detail != nil {
		t.Fatal("unavailable product retained")
	}
	state, e = s.EditMarketDiscovery(ctx, actor, state.ID, "direct", MarketQuery{Keyword: "manual"})
	if e != nil || state.Detail != nil || state.Query.Keyword != "manual" {
		t.Fatal("direct fallback", e)
	}
	now = now.Add(31 * time.Minute)
	if _, e = s.MarketDiscovery(ctx, actor, state.ID); !errors.Is(e, ErrMarketDraft) {
		t.Fatal("expired draft accepted")
	}
}
func TestSpecPrefillUsesExplicitSelectedValues(t *testing.T) {
	zero, one := 0, 1
	spec := domain.MarketSpecification{Name: "颜色", ValueOptions: []domain.MarketSpecificationOption{{Value: "黑色", Selected: &zero}, {Value: "蓝色", Selected: &one}}}
	if MarketSpecValue(spec) != "蓝色" {
		t.Fatal("unselected option used")
	}
	spec.ValueOptions[1].Selected = nil
	if MarketSpecValue(spec) != "" {
		t.Fatal("invented missing spec")
	}
	for _, name := range []string{"电池容量", "系统版本", "Phone 256GB 黑色"} {
		if MarketSpecKind(name) != "" {
			t.Fatal("unrelated spec interpreted", name)
		}
	}
}
