package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/SampsonFox/assetloop/internal/store/sqlite/sqlitedb"
)

func (s *Store) GetPortfolioSummary(ctx context.Context, tenantID string) (application.PortfolioSummary, error) {
	row, err := s.queries().GetPortfolioSummary(ctx, tenantID)
	if err != nil {
		return application.PortfolioSummary{}, err
	}
	return application.PortfolioSummary{
		AssetCount: int(row.AssetCount), ExpenseMinor: row.ExpenseMinor, IncomeMinor: row.IncomeMinor,
		NetMinor: row.NetMinor, BaseCurrency: row.BaseCurrency,
	}, nil
}

func (s *Store) TenantBaseCurrency(ctx context.Context, tenantID string) (string, bool, error) {
	row, err := s.queries().GetTenantBaseCurrency(ctx, tenantID)
	return row.BaseCurrency, row.BaseCurrencyLocked != 0, err
}

func (s *Store) CreateAssetEventType(ctx context.Context, eventType domain.AssetEventTypeDefinition) error {
	return s.queries().CreateAssetEventType(ctx, sqlitedb.CreateAssetEventTypeParams{
		ID: eventType.ID, TenantID: eventType.TenantID, Name: eventType.Name,
		NormalizedName: eventType.NormalizedName, CashflowDirection: string(eventType.Cashflow),
		CreatedByUserID: eventType.CreatedByUserID, CreatedAt: sqliteTime(eventType.CreatedAt),
	})
}

func (s *Store) ListAssetEventTypes(ctx context.Context, tenantID string) ([]domain.AssetEventTypeDefinition, error) {
	rows, err := s.queries().ListAssetEventTypes(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.AssetEventTypeDefinition, 0, len(rows))
	for _, row := range rows {
		eventType, err := sqliteEventType(row)
		if err != nil {
			return nil, err
		}
		result = append(result, eventType)
	}
	return result, nil
}

func (s *Store) AppendAssetEvent(ctx context.Context, transaction domain.AssetTransaction, event domain.AssetEvent) error {
	return s.lifecycleTx(ctx, func(q *sqlitedb.Queries) error {
		if err := createSQLiteTransaction(ctx, q, transaction); err != nil {
			return err
		}
		if err := q.CreateAssetEvent(ctx, sqliteEventParams(event)); err != nil {
			return err
		}
		if event.BaseAmountMinor == 0 {
			return nil
		}
		return q.LockTenantBaseCurrency(ctx, sqlitedb.LockTenantBaseCurrencyParams{ID: event.TenantID, BaseCurrency: event.BaseCurrency})
	})
}

func (s *Store) CorrectAssetEvent(ctx context.Context, transaction domain.AssetTransaction, voidEvent, replacement domain.AssetEvent) error {
	return s.lifecycleTx(ctx, func(q *sqlitedb.Queries) error {
		if err := createSQLiteTransaction(ctx, q, transaction); err != nil {
			return err
		}
		if err := q.CreateAssetEvent(ctx, sqliteEventParams(voidEvent)); err != nil {
			return err
		}
		if err := q.CreateAssetEvent(ctx, sqliteEventParams(replacement)); err != nil {
			return err
		}
		if replacement.BaseAmountMinor == 0 {
			return nil
		}
		return q.LockTenantBaseCurrency(ctx, sqlitedb.LockTenantBaseCurrencyParams{ID: replacement.TenantID, BaseCurrency: replacement.BaseCurrency})
	})
}

func sqliteEventType(row sqlitedb.AssetEventType) (domain.AssetEventTypeDefinition, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, row.CreatedAt)
	if err != nil {
		return domain.AssetEventTypeDefinition{}, fmt.Errorf("parse event type created_at: %w", err)
	}
	return domain.AssetEventTypeDefinition{
		ID: row.ID, TenantID: row.TenantID, Name: row.Name, NormalizedName: row.NormalizedName,
		Cashflow: domain.AssetEventCashflow(row.CashflowDirection), CreatedByUserID: row.CreatedByUserID, CreatedAt: createdAt,
	}, nil
}

func (s *Store) GetAssetEvent(ctx context.Context, tenantID, eventID string) (domain.AssetEvent, error) {
	row, err := s.queries().GetAssetEvent(ctx, sqlitedb.GetAssetEventParams{TenantID: tenantID, ID: eventID})
	if err != nil {
		return domain.AssetEvent{}, err
	}
	return sqliteEvent(row.ID, row.TenantID, row.AssetID, row.TransactionID, row.EventType,
		row.BaseAmountMinor, row.BaseCurrency, row.OriginalAmountMinor, row.OriginalCurrency,
		row.FxRateScaled, row.FxRateDate, row.FxRateSource, row.Notes, row.VoidsEventID,
		row.ReplacesEventID, row.OccurredAt, row.CreatedByUserID, row.CreatedAt, row.IsVoided)
}

func (s *Store) ListAssetEvents(ctx context.Context, tenantID, assetID string) ([]domain.AssetEvent, error) {
	rows, err := s.queries().ListAssetEvents(ctx, sqlitedb.ListAssetEventsParams{TenantID: tenantID, AssetID: assetID})
	if err != nil {
		return nil, err
	}
	result := make([]domain.AssetEvent, 0, len(rows))
	for _, row := range rows {
		event, err := sqliteEvent(row.ID, row.TenantID, row.AssetID, row.TransactionID, row.EventType,
			row.BaseAmountMinor, row.BaseCurrency, row.OriginalAmountMinor, row.OriginalCurrency,
			row.FxRateScaled, row.FxRateDate, row.FxRateSource, row.Notes, row.VoidsEventID,
			row.ReplacesEventID, row.OccurredAt, row.CreatedByUserID, row.CreatedAt, row.IsVoided)
		if err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, nil
}

func (s *Store) ListAssetEventsPage(ctx context.Context, tenantID, assetID string, opts application.EventListOptions) ([]domain.AssetEvent, int, error) {
	showVoided := int64(0)
	if opts.ShowVoided {
		showVoided = 1
	}
	rows, err := s.queries().ListAssetEventsPage(ctx, sqlitedb.ListAssetEventsPageParams{
		TenantID: tenantID, AssetID: assetID, SearchQuery: opts.Query, EventTypeFilter: opts.Type,
		SortKey: opts.Sort, SortDirection: opts.Direction, ShowVoided: showVoided,
		PageSize: int64(opts.PageSize), PageOffset: int64((opts.Page - 1) * opts.PageSize),
	})
	if err != nil {
		return nil, 0, err
	}
	result := make([]domain.AssetEvent, 0, len(rows))
	total := 0
	for _, row := range rows {
		total = int(row.TotalCount)
		event, err := sqliteEvent(row.ID, row.TenantID, row.AssetID, row.TransactionID, row.EventType,
			row.BaseAmountMinor, row.BaseCurrency, row.OriginalAmountMinor, row.OriginalCurrency,
			row.FxRateScaled, row.FxRateDate, row.FxRateSource, row.Notes, row.VoidsEventID,
			row.ReplacesEventID, row.OccurredAt, row.CreatedByUserID, row.CreatedAt, row.IsVoided)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, event)
	}
	return result, total, nil
}

func (s *Store) GetAssetSummary(ctx context.Context, tenantID, assetID string) (domain.AssetSummary, error) {
	row, err := s.queries().GetAssetSummary(ctx, sqlitedb.GetAssetSummaryParams{TenantID: tenantID, AssetID: assetID})
	if err != nil {
		return domain.AssetSummary{}, err
	}
	return domain.AssetSummary{
		BaseCurrency: row.BaseCurrency, ExpenseMinor: row.ExpenseMinor, IncomeMinor: row.IncomeMinor,
		NetCashflowMinor: row.NetMinor, Status: row.Status,
	}, nil
}

func (s *Store) lifecycleTx(ctx context.Context, fn func(*sqlitedb.Queries) error) error {
	if s.tx != nil {
		return fn(s.queries())
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(sqlitedb.New(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func createSQLiteTransaction(ctx context.Context, q *sqlitedb.Queries, transaction domain.AssetTransaction) error {
	return q.CreateAssetTransaction(ctx, sqlitedb.CreateAssetTransactionParams{
		ID: transaction.ID, TenantID: transaction.TenantID, OccurredAt: sqliteTime(transaction.OccurredAt),
		Source: transaction.Source, ExternalReference: transaction.ExternalReference, Notes: transaction.Notes,
		CreatedByUserID: transaction.CreatedByUserID, CreatedAt: sqliteTime(transaction.CreatedAt),
	})
}

func sqliteEventParams(event domain.AssetEvent) sqlitedb.CreateAssetEventParams {
	params := sqlitedb.CreateAssetEventParams{
		ID: event.ID, TenantID: event.TenantID, AssetID: event.AssetID, TransactionID: event.TransactionID,
		EventType: string(event.Type), BaseAmountMinor: event.BaseAmountMinor, BaseCurrency: event.BaseCurrency,
		Notes: event.Notes, VoidsEventID: nullableString(event.VoidsEventID),
		ReplacesEventID: nullableString(event.ReplacesEventID), OccurredAt: sqliteTime(event.OccurredAt),
		CreatedByUserID: event.CreatedByUserID, CreatedAt: sqliteTime(event.CreatedAt),
	}
	if event.FX != nil {
		params.OriginalAmountMinor = sql.NullInt64{Int64: event.FX.OriginalAmountMinor, Valid: true}
		params.OriginalCurrency = sql.NullString{String: event.FX.OriginalCurrency, Valid: true}
		params.FxRateScaled = sql.NullInt64{Int64: event.FX.RateScaled, Valid: true}
		params.FxRateDate = sql.NullString{String: event.FX.RateDate.UTC().Format("2006-01-02"), Valid: true}
		params.FxRateSource = sql.NullString{String: event.FX.RateSource, Valid: true}
	}
	return params
}

func sqliteEvent(id, tenantID, assetID, transactionID, eventType string, baseAmount int64, baseCurrency string,
	originalAmount sql.NullInt64, originalCurrency sql.NullString, rate sql.NullInt64, rateDate, rateSource sql.NullString,
	notes string, voidsID, replacesID sql.NullString, occurredAt, userID, createdAt string, isVoided bool) (domain.AssetEvent, error) {
	occurred, err := time.Parse(time.RFC3339Nano, occurredAt)
	if err != nil {
		return domain.AssetEvent{}, fmt.Errorf("parse event occurred_at: %w", err)
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.AssetEvent{}, fmt.Errorf("parse event created_at: %w", err)
	}
	event := domain.AssetEvent{
		ID: id, TenantID: tenantID, AssetID: assetID, TransactionID: transactionID,
		Type: domain.AssetEventType(eventType), BaseAmountMinor: baseAmount, BaseCurrency: baseCurrency,
		Notes: notes, VoidsEventID: voidsID.String, ReplacesEventID: replacesID.String,
		OccurredAt: occurred, CreatedByUserID: userID, CreatedAt: created, IsVoided: isVoided,
	}
	if originalAmount.Valid {
		date, err := time.Parse("2006-01-02", rateDate.String)
		if err != nil {
			return domain.AssetEvent{}, fmt.Errorf("parse FX rate date: %w", err)
		}
		event.FX = &domain.FXEvidence{
			OriginalAmountMinor: originalAmount.Int64, OriginalCurrency: originalCurrency.String,
			RateScaled: rate.Int64, RateDate: date, RateSource: rateSource.String,
		}
	}
	return event, nil
}
