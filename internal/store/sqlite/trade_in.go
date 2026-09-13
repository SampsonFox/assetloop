package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/SampsonFox/assetloop/internal/store/sqlite/sqlitedb"
)

// CreateAssetEvents inserts one grouping transaction and every supplied event in
// the same database transaction, so the two events of a trade-in pair are never
// half-written. A non-zero amount still locks the tenant base currency.
func (s *Store) CreateAssetEvents(ctx context.Context, transaction domain.AssetTransaction, events []domain.AssetEvent) error {
	if len(events) == 0 {
		return fmt.Errorf("create asset events: no events supplied")
	}
	return s.lifecycleTx(ctx, func(q *sqlitedb.Queries) error {
		if err := createSQLiteTransaction(ctx, q, transaction); err != nil {
			return err
		}
		for _, event := range events {
			if err := q.CreateAssetEvent(ctx, sqliteEventParams(event)); err != nil {
				return err
			}
		}
		for _, event := range events {
			if event.BaseAmountMinor == 0 {
				continue
			}
			return q.LockTenantBaseCurrency(ctx, sqlitedb.LockTenantBaseCurrencyParams{ID: event.TenantID, BaseCurrency: event.BaseCurrency})
		}
		return nil
	})
}

// TradeInLinks projects effective pairs involving one asset. Cancelled pairs are
// only returned when the caller explicitly asks for history; a purged endpoint
// keeps its surviving relation with the write-time snapshot and a deletion flag.
func (s *Store) TradeInLinks(ctx context.Context, tenantID, assetID string, includeCancelled bool) ([]application.TradeInLinkView, error) {
	include := int64(0)
	if includeCancelled {
		include = 1
	}
	rows, err := s.queries().ListTradeInLinksForAsset(ctx, sqlitedb.ListTradeInLinksForAssetParams{
		TenantID: tenantID, AssetID: assetID, IncludeCancelled: include,
	})
	if err != nil {
		return nil, err
	}
	result := make([]application.TradeInLinkView, 0, len(rows))
	var live []string
	for _, row := range rows {
		view := application.TradeInLinkView{
			LinkID: row.LinkID.String, State: row.LinkState,
			SourceEventID: row.SourceEventID, DestinationEventID: row.DestinationEventID,
			NewAssetID: row.NewAssetID, NewAssetName: row.NewAssetLabel, NewAssetSpec: row.NewAssetSpecLabel,
			NewAssetDeleted: row.NewAssetDeleted != 0,
			OldAssetID:      row.OldAssetID, OldAssetName: row.OldAssetLabel, OldAssetSpec: row.OldAssetSpecLabel,
			OldAssetDeleted: row.OldAssetDeleted != 0,
		}
		if !view.NewAssetDeleted {
			live = append(live, view.NewAssetID)
		}
		if !view.OldAssetDeleted {
			live = append(live, view.OldAssetID)
		}
		result = append(result, view)
	}
	summaries, err := s.AssetTagSummaries(ctx, tenantID, live)
	if err != nil {
		return nil, err
	}
	for index := range result {
		if !result[index].NewAssetDeleted {
			if summary := strings.TrimSpace(summaries[result[index].NewAssetID]); summary != "" {
				result[index].NewAssetSpec = summary
			}
		}
		if !result[index].OldAssetDeleted {
			if summary := strings.TrimSpace(summaries[result[index].OldAssetID]); summary != "" {
				result[index].OldAssetSpec = summary
			}
		}
	}
	return result, nil
}
