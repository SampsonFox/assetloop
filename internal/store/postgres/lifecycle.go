package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/SampsonFox/assetloop/internal/store/postgres/postgresdb"
	"github.com/google/uuid"
)

func (s *Store) GetPortfolioSummary(ctx context.Context, tenantID string) (application.PortfolioSummary, error) {
	id, err := uuid.Parse(tenantID)
	if err != nil {
		return application.PortfolioSummary{}, fmt.Errorf("parse tenant ID: %w", err)
	}
	row, err := postgresdb.New(s.db).GetPortfolioSummary(ctx, id)
	if err != nil {
		return application.PortfolioSummary{}, err
	}
	return application.PortfolioSummary{
		AssetCount: int(row.AssetCount), ExpenseMinor: row.ExpenseMinor, IncomeMinor: row.IncomeMinor,
		NetMinor: row.NetMinor, BaseCurrency: row.BaseCurrency,
	}, nil
}

func (s *Store) TenantBaseCurrency(ctx context.Context, tenantID string) (string, bool, error) {
	id, err := uuid.Parse(tenantID)
	if err != nil {
		return "", false, fmt.Errorf("parse tenant ID: %w", err)
	}
	row, err := postgresdb.New(s.db).GetTenantBaseCurrency(ctx, id)
	return row.BaseCurrency, row.BaseCurrencyLocked, err
}

func (s *Store) CreateAssetEventType(ctx context.Context, eventType domain.AssetEventTypeDefinition) error {
	id, tenantID, err := catalogIDs(eventType.ID, eventType.TenantID)
	if err != nil {
		return err
	}
	userID, err := uuid.Parse(eventType.CreatedByUserID)
	if err != nil {
		return fmt.Errorf("parse event type user ID: %w", err)
	}
	return postgresdb.New(s.db).CreateAssetEventType(ctx, postgresdb.CreateAssetEventTypeParams{
		ID: id, TenantID: tenantID, Name: eventType.Name, NormalizedName: eventType.NormalizedName,
		CashflowDirection: string(eventType.Cashflow), CreatedByUserID: userID, CreatedAt: eventType.CreatedAt,
	})
}

func (s *Store) ListAssetEventTypes(ctx context.Context, tenantID string) ([]domain.AssetEventTypeDefinition, error) {
	id, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("parse tenant ID: %w", err)
	}
	rows, err := postgresdb.New(s.db).ListAssetEventTypes(ctx, id)
	if err != nil {
		return nil, err
	}
	result := make([]domain.AssetEventTypeDefinition, 0, len(rows))
	for _, row := range rows {
		result = append(result, postgresEventType(row))
	}
	return result, nil
}

func (s *Store) AppendAssetEvent(ctx context.Context, transaction domain.AssetTransaction, event domain.AssetEvent) error {
	return s.lifecycleTx(ctx, func(q *postgresdb.Queries) error {
		if err := createPostgresTransaction(ctx, q, transaction); err != nil {
			return err
		}
		params, err := postgresEventParams(event)
		if err != nil {
			return err
		}
		if err := q.CreateAssetEvent(ctx, params); err != nil {
			return err
		}
		tenantID, err := uuid.Parse(event.TenantID)
		if err != nil {
			return err
		}
		if event.BaseAmountMinor == 0 {
			return nil
		}
		return q.LockTenantBaseCurrency(ctx, postgresdb.LockTenantBaseCurrencyParams{ID: tenantID, BaseCurrency: event.BaseCurrency})
	})
}

func (s *Store) CorrectAssetEvent(ctx context.Context, transaction domain.AssetTransaction, voidEvent, replacement domain.AssetEvent) error {
	return s.lifecycleTx(ctx, func(q *postgresdb.Queries) error {
		if err := createPostgresTransaction(ctx, q, transaction); err != nil {
			return err
		}
		voidParams, err := postgresEventParams(voidEvent)
		if err != nil {
			return err
		}
		if err := q.CreateAssetEvent(ctx, voidParams); err != nil {
			return err
		}
		replacementParams, err := postgresEventParams(replacement)
		if err != nil {
			return err
		}
		if err := q.CreateAssetEvent(ctx, replacementParams); err != nil {
			return err
		}
		tenantID, _ := uuid.Parse(replacement.TenantID)
		if replacement.BaseAmountMinor == 0 {
			return nil
		}
		return q.LockTenantBaseCurrency(ctx, postgresdb.LockTenantBaseCurrencyParams{ID: tenantID, BaseCurrency: replacement.BaseCurrency})
	})
}

func postgresEventType(row postgresdb.AssetEventType) domain.AssetEventTypeDefinition {
	return domain.AssetEventTypeDefinition{
		ID: row.ID.String(), TenantID: row.TenantID.String(), Name: row.Name, NormalizedName: row.NormalizedName,
		Cashflow: domain.AssetEventCashflow(row.CashflowDirection), CreatedByUserID: row.CreatedByUserID.String(), CreatedAt: row.CreatedAt,
	}
}

func (s *Store) GetAssetEvent(ctx context.Context, tenantID, eventID string) (domain.AssetEvent, error) {
	tenantUUID, eventUUID, err := postgresIDs(tenantID, eventID)
	if err != nil {
		return domain.AssetEvent{}, err
	}
	row, err := postgresdb.New(s.db).GetAssetEvent(ctx, postgresdb.GetAssetEventParams{TenantID: tenantUUID, ID: eventUUID})
	if err != nil {
		return domain.AssetEvent{}, err
	}
	return postgresEvent(row.ID, row.TenantID, row.AssetID, row.TransactionID, row.EventType,
		row.BaseAmountMinor, row.BaseCurrency, row.OriginalAmountMinor, row.OriginalCurrency,
		row.FxRateScaled, row.FxRateDate, row.FxRateSource, row.Notes, row.VoidsEventID,
		row.ReplacesEventID, row.OccurredAt, row.CreatedByUserID, row.CreatedAt, row.IsVoided), nil
}

func (s *Store) ListAssetEvents(ctx context.Context, tenantID, assetID string) ([]domain.AssetEvent, error) {
	tenantUUID, assetUUID, err := postgresIDs(tenantID, assetID)
	if err != nil {
		return nil, err
	}
	rows, err := postgresdb.New(s.db).ListAssetEvents(ctx, postgresdb.ListAssetEventsParams{TenantID: tenantUUID, AssetID: assetUUID})
	if err != nil {
		return nil, err
	}
	result := make([]domain.AssetEvent, 0, len(rows))
	for _, row := range rows {
		result = append(result, postgresEvent(row.ID, row.TenantID, row.AssetID, row.TransactionID, row.EventType,
			row.BaseAmountMinor, row.BaseCurrency, row.OriginalAmountMinor, row.OriginalCurrency,
			row.FxRateScaled, row.FxRateDate, row.FxRateSource, row.Notes, row.VoidsEventID,
			row.ReplacesEventID, row.OccurredAt, row.CreatedByUserID, row.CreatedAt, row.IsVoided))
	}
	return result, nil
}

func (s *Store) ListAssetEventsPage(ctx context.Context, tenantID, assetID string, opts application.EventListOptions) ([]domain.AssetEvent, int, error) {
	tenantUUID, assetUUID, err := postgresIDs(tenantID, assetID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := postgresdb.New(s.db).ListAssetEventsPage(ctx, postgresdb.ListAssetEventsPageParams{
		TenantID: tenantUUID, AssetID: assetUUID, SearchQuery: opts.Query, EventTypeFilter: opts.Type,
		SortKey: opts.Sort, SortDirection: opts.Direction,
		PageSize: int64(opts.PageSize), PageOffset: int64((opts.Page - 1) * opts.PageSize),
	})
	if err != nil {
		return nil, 0, err
	}
	result := make([]domain.AssetEvent, 0, len(rows))
	total := 0
	for _, row := range rows {
		total = int(row.TotalCount)
		result = append(result, postgresEvent(row.ID, row.TenantID, row.AssetID, row.TransactionID, row.EventType,
			row.BaseAmountMinor, row.BaseCurrency, row.OriginalAmountMinor, row.OriginalCurrency,
			row.FxRateScaled, row.FxRateDate, row.FxRateSource, row.Notes, row.VoidsEventID,
			row.ReplacesEventID, row.OccurredAt, row.CreatedByUserID, row.CreatedAt, row.IsVoided))
	}
	return result, total, nil
}

func (s *Store) GetAssetSummary(ctx context.Context, tenantID, assetID string) (domain.AssetSummary, error) {
	tenantUUID, assetUUID, err := postgresIDs(tenantID, assetID)
	if err != nil {
		return domain.AssetSummary{}, err
	}
	row, err := postgresdb.New(s.db).GetAssetSummary(ctx, postgresdb.GetAssetSummaryParams{TenantID: tenantUUID, AssetID: assetUUID})
	if err != nil {
		return domain.AssetSummary{}, err
	}
	return domain.AssetSummary{
		BaseCurrency: row.BaseCurrency, ExpenseMinor: row.ExpenseMinor, IncomeMinor: row.IncomeMinor,
		NetCashflowMinor: row.NetMinor, Status: row.Status,
	}, nil
}

func (s *Store) lifecycleTx(ctx context.Context, fn func(*postgresdb.Queries) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(postgresdb.New(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func createPostgresTransaction(ctx context.Context, q *postgresdb.Queries, transaction domain.AssetTransaction) error {
	id, tenantID, err := catalogIDs(transaction.ID, transaction.TenantID)
	if err != nil {
		return err
	}
	userID, err := uuid.Parse(transaction.CreatedByUserID)
	if err != nil {
		return fmt.Errorf("parse transaction user ID: %w", err)
	}
	return q.CreateAssetTransaction(ctx, postgresdb.CreateAssetTransactionParams{
		ID: id, TenantID: tenantID, OccurredAt: transaction.OccurredAt, Source: transaction.Source,
		ExternalReference: transaction.ExternalReference, Notes: transaction.Notes,
		CreatedByUserID: userID, CreatedAt: transaction.CreatedAt,
	})
}

func postgresEventParams(event domain.AssetEvent) (postgresdb.CreateAssetEventParams, error) {
	id, tenantID, err := catalogIDs(event.ID, event.TenantID)
	if err != nil {
		return postgresdb.CreateAssetEventParams{}, err
	}
	assetID, err := uuid.Parse(event.AssetID)
	if err != nil {
		return postgresdb.CreateAssetEventParams{}, fmt.Errorf("parse asset ID: %w", err)
	}
	transactionID, err := uuid.Parse(event.TransactionID)
	if err != nil {
		return postgresdb.CreateAssetEventParams{}, fmt.Errorf("parse transaction ID: %w", err)
	}
	userID, err := uuid.Parse(event.CreatedByUserID)
	if err != nil {
		return postgresdb.CreateAssetEventParams{}, fmt.Errorf("parse event user ID: %w", err)
	}
	voidsID, err := nullableUUID(event.VoidsEventID)
	if err != nil {
		return postgresdb.CreateAssetEventParams{}, err
	}
	replacesID, err := nullableUUID(event.ReplacesEventID)
	if err != nil {
		return postgresdb.CreateAssetEventParams{}, err
	}
	params := postgresdb.CreateAssetEventParams{
		ID: id, TenantID: tenantID, AssetID: assetID, TransactionID: transactionID,
		EventType: string(event.Type), BaseAmountMinor: event.BaseAmountMinor, BaseCurrency: event.BaseCurrency,
		Notes: event.Notes, VoidsEventID: voidsID, ReplacesEventID: replacesID,
		OccurredAt: event.OccurredAt, CreatedByUserID: userID, CreatedAt: event.CreatedAt,
	}
	if event.FX != nil {
		params.OriginalAmountMinor = sql.NullInt64{Int64: event.FX.OriginalAmountMinor, Valid: true}
		params.OriginalCurrency = sql.NullString{String: event.FX.OriginalCurrency, Valid: true}
		params.FxRateScaled = sql.NullInt64{Int64: event.FX.RateScaled, Valid: true}
		params.FxRateDate = sql.NullTime{Time: event.FX.RateDate, Valid: true}
		params.FxRateSource = sql.NullString{String: event.FX.RateSource, Valid: true}
	}
	return params, nil
}

func postgresEvent(id, tenantID, assetID, transactionID uuid.UUID, eventType string, baseAmount int64,
	baseCurrency string, originalAmount sql.NullInt64, originalCurrency sql.NullString, rate sql.NullInt64,
	rateDate sql.NullTime, rateSource sql.NullString, notes string, voidsID, replacesID uuid.NullUUID,
	occurredAt time.Time, userID uuid.UUID, createdAt time.Time, isVoided bool) domain.AssetEvent {
	event := domain.AssetEvent{
		ID: id.String(), TenantID: tenantID.String(), AssetID: assetID.String(), TransactionID: transactionID.String(),
		Type: domain.AssetEventType(eventType), BaseAmountMinor: baseAmount, BaseCurrency: baseCurrency,
		Notes: notes, OccurredAt: occurredAt, CreatedByUserID: userID.String(), CreatedAt: createdAt, IsVoided: isVoided,
	}
	if voidsID.Valid {
		event.VoidsEventID = voidsID.UUID.String()
	}
	if replacesID.Valid {
		event.ReplacesEventID = replacesID.UUID.String()
	}
	if originalAmount.Valid {
		event.FX = &domain.FXEvidence{
			OriginalAmountMinor: originalAmount.Int64, OriginalCurrency: originalCurrency.String,
			RateScaled: rate.Int64, RateDate: rateDate.Time, RateSource: rateSource.String,
		}
	}
	return event
}

func postgresIDs(first, second string) (uuid.UUID, uuid.UUID, error) {
	firstID, err := uuid.Parse(first)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("parse first ID: %w", err)
	}
	secondID, err := uuid.Parse(second)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("parse second ID: %w", err)
	}
	return firstID, secondID, nil
}
