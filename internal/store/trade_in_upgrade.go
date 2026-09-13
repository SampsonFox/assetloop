package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/pressly/goose/v3"
)

const tradeInMigrationName = "00021_trade_in.sql"

// The paired 00021 SQL files hold the pure DDL. Seeding the two neutral pairing
// types and replacing the tenant trigger need collision-safe normalized keys, and
// the type-table rebuild must be checked for dangling foreign keys before commit.
// Both run here, in Goose's transaction, so a late failure rolls the whole
// migration and its version back. internal/store/open.go suspends SQLite foreign
// keys for the schema version this migration starts from.
func tradeInMigration(driver string) *goose.Migration {
	return goose.NewGoMigration(21, &goose.GoFunc{RunTx: func(ctx context.Context, tx *sql.Tx) error {
		if err := execMigrationDDL(ctx, tx, driver, tradeInMigrationName); err != nil {
			return fmt.Errorf("trade-in schema: %w", err)
		}
		if err := seedTradeInSystemTypes(ctx, tx, driver); err != nil {
			return fmt.Errorf("trade-in type seed: %w", err)
		}
		if err := replaceLifecycleTypeTrigger(ctx, tx, driver); err != nil {
			return fmt.Errorf("trade-in type trigger: %w", err)
		}
		if driver == "sqlite" {
			rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
			if err != nil {
				return err
			}
			defer rows.Close()
			if rows.Next() {
				return fmt.Errorf("trade-in migration would leave a foreign key violation")
			}
			return rows.Err()
		}
		return nil
	}}, nil)
}

// seededLifecycleTypes lists the immutable system types the tenant trigger keeps
// in place. The display name and system code stay canonical even when a custom
// type already owns the matching normalized key.
var seededLifecycleTypes = []struct{ code, name, cashflow string }{
	{"purchase", "purchase", "expense"},
	{"repair", "repair", "expense"},
	{"sale", "sale", "income"},
	{"void", "void", "neutral"},
	{"trade_in_source", "trade_in_source", "neutral"},
	{"trade_in_destination", "trade_in_destination", "neutral"},
}

func seedTradeInSystemTypes(ctx context.Context, tx *sql.Tx, driver string) error {
	enabled := "1"
	generatedID := "lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-8' || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))"
	if driver == "postgres" {
		enabled = "TRUE"
		generatedID = "gen_random_uuid()"
	}
	for _, kind := range seededLifecycleTypes[4:] {
		query := `INSERT INTO asset_event_types
    (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, enabled, updated_at)
SELECT ` + generatedID + `, m.tenant_id, ?, ` + collisionSafeKey(driver, "m.tenant_id", "?") + `, '` + kind.cashflow + `', m.user_id, m.created_at, ?, ` + enabled + `, m.created_at
FROM tenant_memberships m
WHERE NOT EXISTS (
        SELECT 1 FROM tenant_memberships m2
        WHERE m2.tenant_id = m.tenant_id
          AND (m2.created_at < m.created_at OR (m2.created_at = m.created_at AND CAST(m2.user_id AS TEXT) < CAST(m.user_id AS TEXT)))
      )
  AND NOT EXISTS (SELECT 1 FROM asset_event_types t WHERE t.tenant_id = m.tenant_id AND t.system_code = ?)`
		if driver == "postgres" {
			query = numberedPlaceholders(query)
		}
		// The ID is generated once per selected tenant row inside the database: a
		// single caller-side value would collide with the primary key as soon as
		// the upgrade covers more than one existing tenant.
		if _, err := tx.ExecContext(ctx, query, kind.name, kind.code, kind.code, kind.code, kind.code, kind.code); err != nil {
			return err
		}
	}
	return nil
}

func replaceLifecycleTypeTrigger(ctx context.Context, tx *sql.Tx, driver string) error {
	var statements strings.Builder
	if driver == "postgres" {
		statements.WriteString("CREATE OR REPLACE FUNCTION assetloop_seed_lifecycle_types() RETURNS trigger AS $$\nBEGIN\n")
		for _, kind := range seededLifecycleTypes {
			fmt.Fprintf(&statements, `INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, enabled, updated_at) SELECT gen_random_uuid(), NEW.tenant_id, '%s', %s, '%s', NEW.user_id, NEW.created_at, '%s', TRUE, NEW.created_at WHERE NOT EXISTS (SELECT 1 FROM asset_event_types WHERE tenant_id = NEW.tenant_id AND system_code = '%s');`+"\n",
				kind.name, collisionSafeKey(driver, "NEW.tenant_id", "'"+kind.name+"'"), kind.cashflow, kind.code, kind.code)
		}
		statements.WriteString("RETURN NEW;\nEND;\n$$ LANGUAGE plpgsql;\n")
		statements.WriteString("DROP TRIGGER IF EXISTS seed_lifecycle_types ON tenant_memberships;\n")
		statements.WriteString("CREATE TRIGGER seed_lifecycle_types AFTER INSERT ON tenant_memberships FOR EACH ROW EXECUTE FUNCTION assetloop_seed_lifecycle_types();")
	} else {
		statements.WriteString("DROP TRIGGER IF EXISTS seed_lifecycle_types;\n")
		statements.WriteString("CREATE TRIGGER seed_lifecycle_types AFTER INSERT ON tenant_memberships\nBEGIN\n")
		for _, kind := range seededLifecycleTypes {
			fmt.Fprintf(&statements, `INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, enabled, updated_at) SELECT lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-8' || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))), NEW.tenant_id, '%s', %s, '%s', NEW.user_id, NEW.created_at, '%s', 1, NEW.created_at WHERE NOT EXISTS (SELECT 1 FROM asset_event_types WHERE tenant_id = NEW.tenant_id AND system_code = '%s');`+"\n",
				kind.name, collisionSafeKey(driver, "NEW.tenant_id", "'"+kind.name+"'"), kind.cashflow, kind.code, kind.code)
		}
		statements.WriteString("END;")
	}
	_, err := tx.ExecContext(ctx, statements.String())
	return err
}

// collisionSafeKey keeps the canonical display name while selecting a
// normalized internal key that a pre-existing custom type may already own. The
// fallback is suffixed with fresh randomness, so it can never collide with a
// stored row and the custom type is left untouched.
func collisionSafeKey(driver, tenantExpr, literal string) string {
	random := "lower(hex(randomblob(16)))"
	if driver == "postgres" {
		random = "replace(gen_random_uuid()::text, '-', '')"
	}
	return "CASE WHEN NOT EXISTS (SELECT 1 FROM asset_event_types t2 WHERE t2.tenant_id = " + tenantExpr + " AND t2.normalized_name = " + literal + ")" +
		" THEN " + literal + " ELSE " + literal + " || ':' || " + random + " END"
}

// numberedPlaceholders rewrites sqlc-style question marks for PostgreSQL.
func numberedPlaceholders(query string) string {
	for index := 1; strings.Contains(query, "?"); index++ {
		query = strings.Replace(query, "?", fmt.Sprintf("$%d", index), 1)
	}
	return query
}
