package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/SampsonFox/assetloop/migrations"
	"github.com/pressly/goose/v3"
)

const specificationContractMigrationName = "00014_retire_product_variants.sql"

func specificationContractMigration(driver string) *goose.Migration {
	return goose.NewGoMigration(14, &goose.GoFunc{RunTx: func(ctx context.Context, tx *sql.Tx) error {
		ddl, err := migrations.FS.ReadFile(driver + "/" + specificationContractMigrationName)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(ddl)); err != nil {
			return fmt.Errorf("retire product variants: %w", err)
		}
		if driver == "sqlite" {
			rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
			if err != nil {
				return err
			}
			defer rows.Close()
			if rows.Next() {
				return fmt.Errorf("specification retirement would leave a foreign key violation")
			}
			return rows.Err()
		}
		return nil
	}}, nil)
}
