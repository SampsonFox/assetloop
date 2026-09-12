package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/SampsonFox/assetloop/migrations"
	"github.com/pressly/goose/v3"
)

const marketMigrationName = "00018_market_quotes.sql"
const marketSelectionMigrationName = "00019_market_selection.sql"

// The pre-UAT market branch used 15/16 for quotes/selection, while accepted
// UAT uses those versions for OAuth/receipts. Forward-only 18 repairs that
// known branch shape without rewriting Goose history or existing market rows.
func marketMigration(driver string) *goose.Migration {
	return goose.NewGoMigration(18, &goose.GoFunc{RunTx: func(ctx context.Context, tx *sql.Tx) error {
		exists, err := migrationTableExists(ctx, tx, driver, "market_items")
		if err != nil {
			return err
		}
		if !exists {
			return execMigrationDDL(ctx, tx, driver, marketMigrationName)
		}
		// Require the complete legacy table shape before accepting pre-existing data.
		for _, query := range []string{
			"SELECT id,tenant_id,name,provider,keyword,filter_criteria,model_desc,region,external_id,enabled,created_at,last_attempt,last_success,last_error,lease_token,lease_until FROM market_items WHERE 1=0",
			"SELECT tenant_id,market_item_id,observation_date,observed_at,max_minor,min_minor,currency,base_currency,base_minor,rate_scaled,rate_date,rate_source,provider,provider_version,provenance,evidence,source_date,sample_count FROM market_prices WHERE 1=0",
			"SELECT tenant_id,asset_id,market_item_id FROM asset_market_bindings WHERE 1=0",
		} {
			rows, err := tx.QueryContext(ctx, query)
			if err != nil {
				return fmt.Errorf("unrecognized legacy market schema: %w", err)
			}
			if err := rows.Close(); err != nil {
				return err
			}
		}
		for _, missing := range []struct{ table, file string }{
			{"oauth_grants", "00015_oauth.sql"},
			{"management_requests", "00016_management_requests.sql"},
		} {
			present, err := migrationTableExists(ctx, tx, driver, missing.table)
			if err != nil {
				return err
			}
			if !present {
				if err := execMigrationDDL(ctx, tx, driver, missing.file); err != nil {
					return err
				}
			}
		}
		return nil
	}}, nil)
}

func marketSelectionMigration(driver string) *goose.Migration {
	return goose.NewGoMigration(19, &goose.GoFunc{RunTx: func(ctx context.Context, tx *sql.Tx) error {
		query := "SELECT COUNT(*) FROM pragma_table_info('market_items') WHERE name='selection_json' AND upper(type)='TEXT'"
		if driver == "postgres" {
			query = "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='market_items' AND column_name='selection_json' AND data_type='text'"
		}
		var count int
		if err := tx.QueryRowContext(ctx, query).Scan(&count); err != nil {
			return err
		}
		if count == 1 {
			return nil
		}
		return execMigrationDDL(ctx, tx, driver, marketSelectionMigrationName)
	}}, nil)
}

func migrationTableExists(ctx context.Context, tx *sql.Tx, driver, name string) (bool, error) {
	query := "SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name=?)"
	if driver == "postgres" {
		query = "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=$1)"
	}
	var exists bool
	err := tx.QueryRowContext(ctx, query, name).Scan(&exists)
	return exists, err
}

func execMigrationDDL(ctx context.Context, tx *sql.Tx, driver, name string) error {
	ddl, err := migrations.FS.ReadFile(driver + "/" + name)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, string(ddl))
	return err
}
