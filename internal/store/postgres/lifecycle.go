package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/SampsonFox/assetloop/internal/store/postgres/postgresdb"
	"github.com/google/uuid"
)

func (s *Store) TenantBaseCurrency(ctx context.Context, tenantID string) (string, bool, error) {
	id, err := uuid.Parse(tenantID)
	if err != nil {
		return "", false, fmt.Errorf("parse tenant ID: %w", err)
	}
	row, err := postgresdb.New(s.db).GetTenantBaseCurrency(ctx, id)
	return row.BaseCurrency, row.BaseCurrencyLocked, err
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
		return q.LockTenantBaseCurrency(ctx, postgresdb.LockTenantBaseCurrencyParams{ID: tenantID, BaseCurrency: replacement.BaseCurrency})
	})
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

func (s *Store) CreateImportDraft(ctx context.Context, draft domain.ImportDraft) error {
	id, tenantID, err := catalogIDs(draft.ID, draft.TenantID)
	if err != nil {
		return err
	}
	assetID, err := uuid.Parse(draft.AssetID)
	if err != nil {
		return fmt.Errorf("parse asset ID: %w", err)
	}
	userID, err := uuid.Parse(draft.CreatedByUserID)
	if err != nil {
		return fmt.Errorf("parse user ID: %w", err)
	}
	return postgresdb.New(s.db).CreateImportDraft(ctx, postgresdb.CreateImportDraftParams{
		ID: id, TenantID: tenantID, AssetID: assetID, EventType: string(draft.EventType), AmountMinor: draft.AmountMinor,
		Currency: draft.Currency, OccurredAt: draft.OccurredAt, Source: draft.Source,
		ExternalReference: draft.ExternalReference, Notes: draft.Notes, RawText: draft.RawText,
		Status: draft.Status, CreatedByUserID: userID, CreatedAt: draft.CreatedAt,
	})
}

func (s *Store) ListPendingImportDrafts(ctx context.Context, tenantID string) ([]domain.ImportDraft, error) {
	id, err := uuid.Parse(tenantID)
	if err != nil {
		return nil, fmt.Errorf("parse tenant ID: %w", err)
	}
	rows, err := postgresdb.New(s.db).ListPendingImportDrafts(ctx, id)
	if err != nil {
		return nil, err
	}
	result := make([]domain.ImportDraft, 0, len(rows))
	for _, row := range rows {
		result = append(result, postgresDraft(row))
	}
	return result, nil
}

func (s *Store) GetImportDraft(ctx context.Context, tenantID, draftID string) (domain.ImportDraft, error) {
	tenantUUID, draftUUID, err := postgresIDs(tenantID, draftID)
	if err != nil {
		return domain.ImportDraft{}, err
	}
	row, err := postgresdb.New(s.db).GetImportDraft(ctx, postgresdb.GetImportDraftParams{TenantID: tenantUUID, ID: draftUUID})
	if err != nil {
		return domain.ImportDraft{}, err
	}
	return postgresDraft(row), nil
}

func (s *Store) ConfirmImportDraft(ctx context.Context, draftID string, transaction domain.AssetTransaction, event domain.AssetEvent) error {
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
		tenantID, draftUUID, err := postgresIDs(transaction.TenantID, draftID)
		if err != nil {
			return err
		}
		transactionID, _ := uuid.Parse(transaction.ID)
		rows, err := q.ConfirmImportDraft(ctx, postgresdb.ConfirmImportDraftParams{
			ConfirmedTransactionID: uuid.NullUUID{UUID: transactionID, Valid: true}, TenantID: tenantID, ID: draftUUID,
		})
		if err != nil {
			return err
		}
		if rows != 1 {
			return errors.New("import draft is not pending")
		}
		return q.LockTenantBaseCurrency(ctx, postgresdb.LockTenantBaseCurrencyParams{ID: tenantID, BaseCurrency: event.BaseCurrency})
	})
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

func postgresDraft(row postgresdb.ImportDraft) domain.ImportDraft {
	draft := domain.ImportDraft{
		ID: row.ID.String(), TenantID: row.TenantID.String(), AssetID: row.AssetID.String(), EventType: domain.AssetEventType(row.EventType),
		AmountMinor: row.AmountMinor, Currency: row.Currency, OccurredAt: row.OccurredAt, Source: row.Source,
		ExternalReference: row.ExternalReference, Notes: row.Notes, RawText: row.RawText, Status: row.Status,
		CreatedByUserID: row.CreatedByUserID.String(), CreatedAt: row.CreatedAt,
	}
	if row.ConfirmedTransactionID.Valid {
		draft.ConfirmedTransactionID = row.ConfirmedTransactionID.UUID.String()
	}
	return draft
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
