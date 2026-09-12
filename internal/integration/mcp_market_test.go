package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	transport "github.com/SampsonFox/assetloop/internal/mcp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpMarketFixture struct {
	mu     sync.Mutex
	calls  int
	err    error
	model  string
	amount int64
	color  string
}

type beforeMarketClaimStore struct {
	application.MarketStore
	before func(context.Context) error
}

func (s beforeMarketClaimStore) ClaimMarketLease(ctx context.Context, tenant, id, token string, now, until time.Time) (bool, error) {
	if err := s.before(ctx); err != nil {
		return false, err
	}
	return s.MarketStore.ClaimMarketLease(ctx, tenant, id, token, now, until)
}

func newMCPMarketFixture() *mcpMarketFixture {
	return &mcpMarketFixture{model: "Fixture phone", amount: 500000, color: "blue"}
}
func (f *mcpMarketFixture) FetchQuote(context.Context, application.MarketQuery) (application.MarketQuote, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return application.MarketQuote{Provider: "zhuanzhuan", ProviderVersion: "fixture", Currency: "CNY", ModelDesc: f.model, MaxMinor: f.amount, ObservedAt: time.Now().UTC(), Evidence: "private-provider-evidence"}, f.err
}
func (f *mcpMarketFixture) SearchProducts(context.Context, application.ProductSearchQuery) (application.ProductSearchResult, error) {
	return application.ProductSearchResult{Items: []application.MarketProduct{{Reference: application.ProductReference{ID: "synthetic-product", Metric: "private-provider-metric"}, Provider: "zhuanzhuan", Title: "Fixture phone listing", Currency: "CNY"}}}, nil
}
func (f *mcpMarketFixture) GetProductDetail(context.Context, application.ProductReference) (application.MarketProductDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return application.MarketProductDetail{Currency: "CNY", Selection: domain.MarketSelection{Provider: "zhuanzhuan", ProductID: "synthetic-product", Title: "Fixture phone listing", ObservedAt: time.Now().UTC(), Specifications: []domain.MarketSpecification{{Name: "model", Value: "Fixture phone"}, {Name: "storage", Value: "256GB"}, {Name: "color", Value: f.color}}}}, nil
}

func runMCPMarketWalkthrough(t *testing.T, ctx context.Context, client *sdk.ClientSession, call func(string, any, any), assetID string, fixture *mcpMarketFixture, market *application.MarketService, manager *application.ManagementService, actor application.Principal) {
	t.Helper()
	var state struct {
		Data transport.MarketDiscoveryResult
	}
	call("search_market_products", transport.MarketQueryInput{Keyword: "Fixture phone"}, &state)
	if len(state.Data.Candidates) != 1 {
		t.Fatal("candidate missing")
	}
	call("select_market_product", transport.SelectMarketProductInput{DraftID: state.Data.DraftID, Index: 0}, &state)
	if !strings.Contains(state.Data.FilterCriteria, "256GB") || state.Data.Selection == nil {
		t.Fatal("explicit specification missing")
	}
	call("preview_market_price", transport.MarketQueryInput{DraftID: state.Data.DraftID, Keyword: state.Data.Keyword, FilterCriteria: state.Data.FilterCriteria}, &state)
	if state.Data.MatchedModel != "Fixture phone" || state.Data.Quote == nil || state.Data.Quote.MaxMinor != 500000 {
		t.Fatal("broader quote scope missing")
	}
	cmd := transport.CreateMarketInput{DraftID: state.Data.DraftID, Name: "Shared phone price", RequestKey: "mcp-market-create"}
	expectError := func(name string, input any, code string) {
		t.Helper()
		r, err := client.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: input})
		if err != nil || !r.IsError {
			t.Fatalf("%s did not reject input: %v", name, err)
		}
		data, _ := json.Marshal(r.StructuredContent)
		if !strings.Contains(string(data), code) {
			t.Fatalf("%s wrong error: %s", name, data)
		}
	}
	expectError("create_market_item", cmd, "market.accept_scope")
	cmd.AcceptScope = true
	fixture.color = "black"
	expectError("create_market_item", cmd, "market.selection_changed")
	// A changed specification forces a fresh preview before the same uncommitted command can succeed.
	call("preview_market_price", transport.MarketQueryInput{DraftID: state.Data.DraftID, Keyword: "Fixture phone", FilterCriteria: "storage:256GB,color:black"}, &state)
	var item struct{ Data transport.MarketItemResult }
	call("create_market_item", cmd, &item)
	id := item.Data.ID
	if id == "" || item.Data.Selection == nil || item.Data.Selection.Specifications[2].Value != "black" {
		t.Fatal("selection snapshot lost")
	}
	calls := fixture.calls
	call("create_market_item", cmd, &item)
	if item.Data.ID != id || fixture.calls != calls {
		t.Fatal("replayed create contacted provider or changed identity")
	}
	bad := cmd
	bad.Name = "Conflicting command"
	expectError("create_market_item", bad, "validation.request_conflict")
	call("bind_asset_market", transport.BindMarketInput{AssetID: assetID, MarketItemID: id, RequestKey: "mcp-market-bind"}, new(any))
	call("list_market_items", transport.Empty{}, new(any))
	var detail struct {
		Data struct {
			Item     transport.MarketItemResult
			Prices   []transport.MarketPriceResult
			AssetIDs []string `json:"asset_ids"`
		}
	}
	call("get_market_item", transport.IDInput{ID: id}, &detail)
	if len(detail.Data.Prices) != 1 || len(detail.Data.AssetIDs) != 1 || detail.Data.AssetIDs[0] != assetID {
		t.Fatal("shared history/binding missing")
	}
	var price struct{ Data transport.MarketPriceResult }
	call("get_asset_market_price", transport.IDInput{ID: assetID}, &price)
	if price.Data.BaseMinor == nil || *price.Data.BaseMinor != 500000 || price.Data.SampleCount != nil || price.Data.SourceDate != nil {
		t.Fatal("price provenance/FX changed")
	}
	fixture.amount = 510000
	refresh := transport.RefreshMarketInput{ID: id, RequestKey: "mcp-market-refresh"}
	call("refresh_market_price", refresh, &price)
	if price.Data.MaxMinor != 510000 {
		t.Fatal("fresh quote missing")
	}
	calls = fixture.calls
	call("refresh_market_price", refresh, &price)
	if fixture.calls != calls {
		t.Fatal("refresh retry performed external I/O")
	}
	// Provider failure retains the successful price and does not produce a success receipt.
	fixture.err = application.ErrMarketAuth
	refresh.RequestKey = "mcp-market-failed-refresh"
	expectError("refresh_market_price", refresh, "market.authentication")
	call("get_asset_market_price", transport.IDInput{ID: assetID}, &price)
	if price.Data.MaxMinor != 510000 {
		t.Fatal("failed refresh destroyed old quote")
	}
	fixture.err = nil
	call("refresh_market_price", refresh, &price)
	call("update_market_item", transport.UpdateMarketInput{ID: id, Name: "Renamed shared series", Enabled: false, RequestKey: "mcp-market-disable"}, &item)
	if item.Data.Enabled {
		t.Fatal("disable ignored")
	}
	call("update_market_item", transport.UpdateMarketInput{ID: id, Name: item.Data.Name, Enabled: true, RequestKey: "mcp-market-enable"}, &item)
	call("bind_asset_market", transport.BindMarketInput{AssetID: assetID, MarketItemID: "", RequestKey: "mcp-market-unbind"}, new(any))
	var unbound struct{ Data *transport.MarketPriceResult }
	call("get_asset_market_price", transport.IDInput{ID: assetID}, &unbound)
	if unbound.Data != nil {
		t.Fatal("unbind ignored")
	}
	call("bind_asset_market", transport.BindMarketInput{AssetID: assetID, MarketItemID: id, RequestKey: "mcp-market-rebind"}, new(any))
	// Durable results must replay after process restart, without the in-memory draft/provider.
	replayed, err := manager.CreateMarketItem(ctx, actor, cmd.RequestKey, nil, application.SaveMarketDiscoveryCommand{DraftID: cmd.DraftID, Name: cmd.Name, AcceptScope: true})
	if err != nil || replayed.ID != id {
		t.Fatal("expired draft replay failed", err)
	}
}

// testMarketManagement uses both real Store adapters through the existing transaction suite.
func testMarketManagement(t *testing.T, store, second application.ManagementStore, actor application.Principal) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	f := newMCPMarketFixture()
	market := application.NewMarketService(store, f, nil, application.MarketOptions{})
	manager := application.NewManagementService(store, nil)
	draft, err := market.NewMarketDiscovery(ctx, actor, application.MarketQuery{Keyword: "Rollback phone"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = market.PreviewMarketDiscovery(ctx, actor, draft.ID, application.MarketQuery{Keyword: "Rollback phone"}); err != nil {
		t.Fatal(err)
	}
	cmd := application.SaveMarketDiscoveryCommand{DraftID: draft.ID, Name: "Rollback series", AcceptScope: true}
	failed := application.NewManagementService(failedReceiptStore{store}, nil)
	if _, err := failed.CreateMarketItem(ctx, actor, "market-rollback", market, cmd); err == nil {
		t.Fatal("receipt failure ignored")
	}
	items, err := market.List(ctx, actor)
	if err != nil || len(items) != 0 {
		t.Fatal("market create escaped failed receipt", err)
	}
	item, err := manager.CreateMarketItem(ctx, actor, "market-rollback", market, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = failed.UpdateMarketItem(ctx, actor, "market-update-rollback", item.ID, "Wrong rename", false); err == nil {
		t.Fatal("update receipt failure ignored")
	}
	detail, err := market.Detail(ctx, actor, item.ID)
	if err != nil || detail.Item.Name != cmd.Name || !detail.Item.Enabled {
		t.Fatal("update escaped receipt", err)
	}
	f.amount = 600000
	if _, err = failed.RefreshMarketPrice(ctx, actor, "market-refresh-rollback", item.ID, market); err == nil {
		t.Fatal("refresh receipt failure ignored")
	}
	detail, err = market.Detail(ctx, actor, item.ID)
	if err != nil || detail.Prices[0].MaxMinor != 500000 {
		t.Fatal("price escaped receipt", err)
	}
	if _, err = manager.RefreshMarketPrice(ctx, actor, "market-refresh-rollback", item.ID, market); err != nil {
		t.Fatal(err)
	}
	if err = store.WithManagementWrite(ctx, actor.TenantID, func(tx application.ManagementStore) error {
		if err := tx.WithMarketWrite(ctx, "different-tenant", func(application.MarketStore) error { return nil }); !errors.Is(err, application.ErrForbidden) {
			t.Fatal("nested market crossed tenant")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Independent Store/service reconstructs the original receipt with no draft/provider.
	replayed, err := application.NewManagementService(second, nil).CreateMarketItem(ctx, actor, "market-rollback", nil, cmd)
	if err != nil || replayed.ID != item.ID {
		t.Fatal("cross-connection replay lost", err)
	}
	// A second process finishes between this request's receipt preflight and lease
	// acquisition. The late replay must release its own newly acquired lease.
	otherManager := application.NewManagementService(second, nil)
	otherMarket := application.NewMarketService(second, f, nil, application.MarketOptions{})
	delayed := application.NewMarketService(beforeMarketClaimStore{MarketStore: store, before: func(ctx context.Context) error {
		_, err := otherManager.RefreshMarketPrice(ctx, actor, "late-market-refresh", item.ID, otherMarket)
		return err
	}}, f, nil, application.MarketOptions{})
	if _, err := manager.RefreshMarketPrice(ctx, actor, "late-market-refresh", item.ID, delayed); err != nil {
		t.Fatal(err)
	}
	detail, err = market.Detail(ctx, actor, item.ID)
	if err != nil || detail.Item.LeaseToken != "" {
		t.Fatal("late duplicate retained a market lease", err)
	}
	for _, tc := range []struct {
		name   string
		actor  application.Principal
		scopes []string
	}{
		{"read-only scope", actor, []string{transport.ScopeRead}},
		{"viewer", application.Principal{UserID: actor.UserID, TenantID: actor.TenantID, Role: application.RoleViewer}, []string{transport.ScopeRead, transport.ScopeCatalog}},
		{"other tenant", application.Principal{UserID: actor.UserID, TenantID: "99999999-9999-4999-8999-999999999998", Role: application.RoleOwner}, []string{transport.ScopeRead, transport.ScopeCatalog}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host := httptest.NewServer(transport.NewHandler(transport.Services{Market: market, Management: manager}, func(context.Context, *http.Request) (transport.Identity, error) {
				return transport.Identity{Principal: tc.actor, Scopes: tc.scopes}, nil
			}))
			defer host.Close()
			client, err := sdk.NewClient(&sdk.Implementation{Name: "market-policy", Version: "1"}, nil).Connect(ctx, &sdk.StreamableClientTransport{Endpoint: host.URL}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			for name, input := range map[string]any{"update_market_item": transport.UpdateMarketInput{ID: item.ID, Name: "Forbidden", Enabled: true, RequestKey: "market-policy"}, "refresh_market_price": transport.RefreshMarketInput{ID: item.ID, RequestKey: "market-policy-refresh"}} {
				r, err := client.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: input})
				if err != nil || !r.IsError {
					t.Fatalf("%s authorization bypass", name)
				}
			}
			if tc.name == "other tenant" {
				r, err := client.CallTool(ctx, &sdk.CallToolParams{Name: "get_market_item", Arguments: transport.IDInput{ID: item.ID}})
				if err != nil || !r.IsError {
					t.Fatal("cross-tenant price read")
				}
			}
		})
	}
}
