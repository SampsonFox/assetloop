package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/SampsonFox/assetloop/internal/store/postgres/postgresdb"
	"github.com/google/uuid"
)

// CreateAssetEvents inserts one grouping transaction and every supplied event in
// the same database transaction, so the two events of a trade-in pair are never
// half-written. A non-zero amount still locks the tenant base currency.
func (s *Store) CreateAssetEvents(ctx context.Context, transaction domain.AssetTransaction, events []domain.AssetEvent) error {
	if len(events) == 0 {
		return fmt.Errorf("create asset events: no events supplied")
	}
	return s.lifecycleTx(ctx, func(q *postgresdb.Queries) error {
		if err := createPostgresTransaction(ctx, q, transaction); err != nil {
			return err
		}
		params := make([]postgresdb.CreateAssetEventParams, 0, len(events))
		for _, event := range events {
			prepared, err := postgresEventParams(event)
			if err != nil {
				return err
			}
			params = append(params, prepared)
		}
		for _, prepared := range params {
			if err := q.CreateAssetEvent(ctx, prepared); err != nil {
				return err
			}
		}
		tenantID, err := uuid.Parse(events[0].TenantID)
		if err != nil {
			return fmt.Errorf("parse tenant ID: %w", err)
		}
		for _, event := range events {
			if event.BaseAmountMinor == 0 {
				continue
			}
			return q.LockTenantBaseCurrency(ctx, postgresdb.LockTenantBaseCurrencyParams{ID: tenantID, BaseCurrency: event.BaseCurrency})
		}
		return nil
	})
}

// TradeInLinks projects effective pairs involving one asset. Cancelled pairs are
// only returned when the caller explicitly asks for history; a purged endpoint
// keeps its surviving relation with the write-time snapshot and a deletion flag.
func (s *Store) TradeInLinks(ctx context.Context, tenantID, assetID string, includeCancelled bool) ([]application.TradeInLinkView, error) {
	tenant, asset, err := postgresIDs(tenantID, assetID)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries().ListTradeInLinksForAsset(ctx, postgresdb.ListTradeInLinksForAssetParams{
		TenantID: tenant, AssetID: asset, IncludeCancelled: includeCancelled,
	})
	if err != nil {
		return nil, err
	}
	result := make([]application.TradeInLinkView, 0, len(rows))
	var live []string
	for _, row := range rows {
		view := application.TradeInLinkView{
			LinkID: row.LinkID.UUID.String(), State: row.LinkState,
			SourceEventID: row.SourceEventID, DestinationEventID: row.DestinationEventID,
			NewAssetID: row.NewAssetID, NewAssetName: row.NewAssetLabel, NewAssetSpec: row.NewAssetSpecLabel,
			NewAssetDeleted: row.NewAssetDeleted,
			OldAssetID:      row.OldAssetID, OldAssetName: row.OldAssetLabel, OldAssetSpec: row.OldAssetSpecLabel,
			OldAssetDeleted: row.OldAssetDeleted,
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
