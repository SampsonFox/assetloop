package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/store/postgres/postgresdb"
	"time"
)

// PurgeAsset holds the same tenant lock as lifecycle and specification writes.
func (s *Store) PurgeAsset(ctx context.Context, actor application.Principal, id string, now time.Time) error {
	ids, err := authIDs(actor.TenantID, id, actor.UserID)
	if err != nil {
		return err
	}
	tenant, asset, user := ids[0], ids[1], ids[2]
	return s.WithManagementWrite(ctx, actor.TenantID, func(store application.ManagementStore) error {
		st := store.(*Store)
		q := st.queries()
		role, err := q.GetMemberRole(ctx, postgresdb.GetMemberRoleParams{TenantID: tenant, UserID: user})
		if err != nil {
			return err
		}
		if role != "owner" {
			return application.ErrForbidden
		}
		deleted, err := q.AssetDeletionExists(ctx, postgresdb.AssetDeletionExistsParams{TenantID: tenant, AssetID: asset})
		if err != nil {
			return err
		}
		if deleted != 0 {
			return nil
		}
		if _, err := st.GetAsset(ctx, actor.TenantID, id); err != nil {
			return err
		}
		transactions, err := q.AssetDeletionTransactions(ctx, postgresdb.AssetDeletionTransactionsParams{TenantID: tenant, AssetID: asset})
		if err != nil {
			return err
		}
		receipts, err := q.AssetManagementReceipts(ctx, tenant)
		if err != nil {
			return err
		}
		for _, receipt := range receipts {
			var value any
			if err := json.Unmarshal([]byte(receipt.ResultJson), &value); err != nil {
				return err
			}
			if receiptContainsAsset(value, id) {
				if err := q.RedactManagementReceipt(ctx, postgresdb.RedactManagementReceiptParams{TenantID: tenant, UserID: receipt.UserID, RequestKey: receipt.RequestKey}); err != nil {
					return err
				}
			}
		}
		if err := q.MarkAssetDeleted(ctx, postgresdb.MarkAssetDeletedParams{TenantID: tenant, AssetID: asset, ActorUserID: user, DeletedAt: now}); err != nil {
			return err
		}
		if err := q.SaveDeletedLifecycleRequests(ctx, postgresdb.SaveDeletedLifecycleRequestsParams{TenantID: tenant, AssetID: asset}); err != nil {
			return err
		}
		if err := q.PurgeLifecycleRequests(ctx, postgresdb.PurgeLifecycleRequestsParams{TenantID: tenant, AssetID: asset}); err != nil {
			return err
		}
		if err := q.PurgeAssetDrafts(ctx, postgresdb.PurgeAssetDraftsParams{TenantID: tenant, AssetID: asset}); err != nil {
			return err
		}
		if err := q.PurgeAssetTags(ctx, postgresdb.PurgeAssetTagsParams{TenantID: tenant, AssetID: asset}); err != nil {
			return err
		}
		if err := q.PurgeAssetMarket(ctx, postgresdb.PurgeAssetMarketParams{TenantID: tenant, AssetID: asset}); err != nil {
			return err
		}
		if err := q.PurgeAssetEvents(ctx, postgresdb.PurgeAssetEventsParams{TenantID: tenant, AssetID: asset}); err != nil {
			return err
		}

		n, err := q.PurgeAsset(ctx, postgresdb.PurgeAssetParams{TenantID: tenant, AssetID: asset})
		if err != nil {
			return err
		}
		if n != 1 {
			return sql.ErrNoRows
		}
		for _, transaction := range transactions {
			if err := q.PurgeUnusedTransaction(ctx, postgresdb.PurgeUnusedTransactionParams{TenantID: tenant, TransactionID: transaction}); err != nil {
				return err
			}
		}
		return nil
	})
}
func receiptContainsAsset(value any, id string) bool {
	switch v := value.(type) {
	case string:
		return v == id
	case map[string]any:
		for _, nested := range v {
			if receiptContainsAsset(nested, id) {
				return true
			}
		}
	case []any:
		for _, nested := range v {
			if receiptContainsAsset(nested, id) {
				return true
			}
		}
	}
	return false
}
