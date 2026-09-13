package application

import (
	"context"
	"errors"

	"github.com/SampsonFox/assetloop/internal/domain"
)

type MarketItemDetail struct {
	Item     domain.MarketItem
	Prices   []domain.MarketPrice
	AssetIDs []string
}

func (s *MarketService) Detail(ctx context.Context, actor Principal, id string) (MarketItemDetail, error) {
	var result MarketItemDetail
	if err := actor.Require(CapabilityView); err != nil {
		return result, err
	}
	var err error
	result.Item, err = s.store.GetMarketItem(ctx, actor.TenantID, id)
	if err != nil {
		return result, err
	}
	result.Prices, err = s.store.ListMarketPrices(ctx, actor.TenantID, id)
	if err != nil {
		return result, err
	}
	result.AssetIDs, err = s.store.MarketAssetIDs(ctx, actor.TenantID, id)
	return result, err
}

type SaveMarketDiscoveryCommand struct {
	DraftID, Name string
	AcceptScope   bool
}

var errMarketPreparationRequired = errors.New("market preparation required")
var errMarketRefreshReplayed = errors.New("market refresh replayed")

func (s *ManagementService) CreateMarketItem(ctx context.Context, actor Principal, key string, market *MarketService, cmd SaveMarketDiscoveryCommand) (domain.MarketItem, error) {
	run := func(fn func(ManagementStore) (domain.MarketItem, error)) (domain.MarketItem, error) {
		return managementWrite(ctx, s, actor, key, "create_market_item", CapabilityManageCatalog, cmd, fn)
	}
	// Check authorization and durable replay before accessing an expiring draft or provider.
	result, err := run(func(ManagementStore) (domain.MarketItem, error) {
		return domain.MarketItem{}, errMarketPreparationRequired
	})
	if !errors.Is(err, errMarketPreparationRequired) {
		return result, err
	}
	if market == nil {
		return result, ErrMarketUnavailable
	}
	return market.saveMarketDiscovery(ctx, actor, cmd.DraftID, cmd.Name, cmd.AcceptScope, func(item domain.MarketItem, price domain.MarketPrice) (domain.MarketItem, error) {
		return run(func(store ManagementStore) (domain.MarketItem, error) {
			// Fetch/specification revalidation/FX happened before this short transaction.
			return (&MarketService{store: store}).commitCreate(ctx, actor, "", item, price)
		})
	})
}

func (s *ManagementService) UpdateMarketItem(ctx context.Context, actor Principal, key, id, name string, enabled bool) (domain.MarketItem, error) {
	cmd := struct {
		ID, Name string
		Enabled  bool
	}{id, name, enabled}
	return managementWrite(ctx, s, actor, key, "update_market_item", CapabilityManageCatalog, cmd, func(store ManagementStore) (domain.MarketItem, error) {
		if err := (&MarketService{store: store}).Update(ctx, actor, id, name, enabled); err != nil {
			return domain.MarketItem{}, err
		}
		return store.GetMarketItem(ctx, actor.TenantID, id)
	})
}

func (s *ManagementService) BindAssetMarket(ctx context.Context, actor Principal, key, assetID, marketID string) error {
	cmd := struct{ AssetID, MarketID string }{assetID, marketID}
	_, err := managementWrite(ctx, s, actor, key, "bind_asset_market", CapabilityManageAssets, cmd, func(store ManagementStore) (bool, error) {
		err := (&MarketService{store: store}).Bind(ctx, actor, assetID, marketID)
		return err == nil, err
	})
	return err
}

func (s *ManagementService) RefreshMarketPrice(ctx context.Context, actor Principal, key, id string, market *MarketService) (domain.MarketPrice, error) {
	run := func(fn func(ManagementStore) (domain.MarketPrice, error)) (domain.MarketPrice, error) {
		return managementWrite(ctx, s, actor, key, "refresh_market_price", CapabilityManageCatalog, id, fn)
	}
	result, err := run(func(ManagementStore) (domain.MarketPrice, error) {
		return domain.MarketPrice{}, errMarketPreparationRequired
	})
	if !errors.Is(err, errMarketPreparationRequired) {
		return result, err
	}
	if market == nil {
		return result, ErrMarketUnavailable
	}
	err = market.refresh(ctx, actor.TenantID, id, func(save func(MarketStore) error) error {
		var err error
		committed := false
		result, err = run(func(store ManagementStore) (domain.MarketPrice, error) {
			if err := save(store); err != nil {
				return domain.MarketPrice{}, err
			}
			committed = true
			prices, err := store.ListMarketPrices(ctx, actor.TenantID, id)
			if err != nil {
				return domain.MarketPrice{}, err
			}
			if len(prices) == 0 {
				return domain.MarketPrice{}, ErrMarketInvalid
			}
			return prices[0], nil
		})
		if err == nil && !committed {
			return errMarketRefreshReplayed
		}
		return err
	})
	return result, err
}
