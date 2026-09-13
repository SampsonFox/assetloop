package application

import (
	"context"

	"github.com/SampsonFox/assetloop/internal/domain"
)

// TradeInLinkView is the read projection of one effective trade-in pair. The
// relation has no table of its own: the two paired events are the source of
// truth, so the projection can only be assembled from persisted events.
type TradeInLinkView struct {
	LinkID             string
	State              string
	SourceEventID      string
	DestinationEventID string
	NewAssetID         string
	NewAssetName       string
	NewAssetSpec       string
	NewAssetDeleted    bool
	OldAssetID         string
	OldAssetName       string
	OldAssetSpec       string
	OldAssetDeleted    bool
}

// TradeInStore is the persistence surface of the trade-in use case. Policy and
// ordinary event appends reuse the lifecycle port; only the grouped multi-event
// append and the pair projection are new. Implementations MUST tenant-scope
// every operation and MUST insert the pairs inside the caller's transaction.
type TradeInStore interface {
	LifecycleStore
	// CreateAssetEvents inserts one grouping transaction and every supplied event
	// in a single database transaction, so a pair can never be half-written.
	CreateAssetEvents(context.Context, domain.AssetTransaction, []domain.AssetEvent) error
	// TradeInLinks lists effective pairs involving one asset. Cancelled pairs are
	// only returned when the caller explicitly asks for them.
	TradeInLinks(context.Context, string, string, bool) ([]TradeInLinkView, error)
}
