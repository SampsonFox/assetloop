package application

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// TradeInLinkQuery addresses one persisted relation by the asset that owns the
// page and the stable link ID both paired events carry. It is a read-only
// projection: the two paired events remain the source of truth.
type TradeInLinkQuery struct {
	OwningAssetID string
	LinkID        string
}

// TradeInLink reads one relation of an owning asset, including a cancelled pair
// from history, so a transport can render the dedicated edit or cancel form
// without assembling relationship state itself. Capability and owning-asset
// scope are validated here; the projection itself stays the store adapter's.
func (s *ManagementService) TradeInLink(ctx context.Context, actor Principal, query TradeInLinkQuery) (TradeInLinkView, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return TradeInLinkView{}, err
	}
	owning := strings.TrimSpace(query.OwningAssetID)
	linkID := strings.TrimSpace(query.LinkID)
	if err := validID("asset ID", owning); err != nil {
		return TradeInLinkView{}, err
	}
	if linkID == "" {
		return TradeInLinkView{}, NewInputError("validation.trade_in_link_stale")
	}
	if err := s.requireQueryAsset(ctx, actor, owning); err != nil {
		return TradeInLinkView{}, err
	}
	// History is included so a cancelled pair stays addressable by its stable
	// link ID; the default effective view is a separate, narrower read.
	links, err := s.store.TradeInLinks(ctx, actor.TenantID, owning, true)
	if err != nil {
		return TradeInLinkView{}, fmt.Errorf("list trade-in links: %w", err)
	}
	for _, link := range links {
		if link.LinkID != linkID {
			continue
		}
		if link.NewAssetID != owning && link.OldAssetID != owning {
			continue
		}
		return link, nil
	}
	return TradeInLinkView{}, NewInputError("validation.trade_in_link_stale")
}

// TradeInLinksForAsset lists the relations one asset takes part in. Cancelled
// pairs are only returned when the caller explicitly asks for history.
func (s *ManagementService) TradeInLinksForAsset(ctx context.Context, actor Principal, assetID string, includeCancelled bool) ([]TradeInLinkView, error) {
	if err := actor.Require(CapabilityView); err != nil {
		return nil, err
	}
	assetID = strings.TrimSpace(assetID)
	if err := validID("asset ID", assetID); err != nil {
		return nil, err
	}
	if err := s.requireQueryAsset(ctx, actor, assetID); err != nil {
		return nil, err
	}
	links, err := s.store.TradeInLinks(ctx, actor.TenantID, assetID, includeCancelled)
	if err != nil {
		return nil, fmt.Errorf("list trade-in links: %w", err)
	}
	return links, nil
}

// requireQueryAsset proves the referenced asset exists inside the caller's tenant
// before a relation read is attempted, so a cross-tenant or purged ID is refused
// and never disclosed.
func (s *ManagementService) requireQueryAsset(ctx context.Context, actor Principal, assetID string) error {
	if _, err := s.store.GetAsset(ctx, actor.TenantID, assetID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return NewInputError("validation.trade_in_asset_unavailable")
		}
		return fmt.Errorf("get trade-in asset: %w", err)
	}
	return nil
}
