package store_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
)

const tradeInUpgradeTenant = "22222222-2222-4222-8222-222222222211"
const tradeInUpgradeUser = "22222222-2222-4222-8222-222222222212"
const tradeInUpgradeCustomType = "22222222-2222-4222-8222-222222222213"
const tradeInUpgradeSecondUser = "22222222-2222-4222-8222-222222222214"
const tradeInUpgradeCategory = "22222222-2222-4222-8222-222222222215"
const tradeInUpgradeModel = "22222222-2222-4222-8222-222222222216"
const tradeInUpgradeAsset = "22222222-2222-4222-8222-222222222217"
const tradeInUpgradeTransaction = "22222222-2222-4222-8222-222222222218"
const tradeInUpgradeEvent = "22222222-2222-4222-8222-222222222219"

// A second pre-existing tenant proves the upgrade seeds one row per tenant
// instead of reusing one primary key for the whole database. Its custom type
// does not own a canonical trade-in key, so both the colliding and the plain
// normalized-name paths are covered.
const tradeInUpgradeTenant2 = "22222222-2222-4222-8222-222222222241"
const tradeInUpgradeUser2 = "22222222-2222-4222-8222-222222222242"
const tradeInUpgradeCustomType2 = "22222222-2222-4222-8222-222222222243"
const tradeInUpgradeCategory2 = "22222222-2222-4222-8222-222222222244"
const tradeInUpgradeModel2 = "22222222-2222-4222-8222-222222222245"
const tradeInUpgradeAsset2 = "22222222-2222-4222-8222-222222222246"
const tradeInUpgradeTransaction2 = "22222222-2222-4222-8222-222222222247"
const tradeInUpgradeEvent2 = "22222222-2222-4222-8222-222222222248"

// injectedMigrationIndex is an index the migration itself creates, so creating
// it beforehand makes the migration fail on its own last DDL statement, long
// after the schema has already been altered.
const injectedMigrationIndex = "asset_events_related_asset_idx"
const rollbackProbeTypeOK = "22222222-2222-4222-8222-222222222297"
const rollbackProbeType = "22222222-2222-4222-8222-222222222298"

// TestTradeInMigrationPreservesV20 proves the 00021 upgrade widens the schema,
// seeds the two pairing types for every existing tenant even when a custom type
// already owns the canonical normalized key, and never rewrites the custom types
// or the existing lifecycle rows.
func TestTradeInMigrationPreservesV20(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			cfg := tradeInUpgradeConfig(t, driver)
			db := tradeInUpgradeV20(t, cfg)
			if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
				t.Fatalf("upgrade trade-in schema: %v", err)
			}
			assertTradeInUpgradeV21(t, db, cfg.Driver)
			assertSeededPairingTypes(t, db, cfg.Driver)
			// The tenant trigger must tolerate a custom collision for a tenant that
			// only gets its pairing types repaired on a later membership insert.
			assertTriggerRepairsCollision(t, db, cfg.Driver)
			if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
				t.Fatalf("restart upgraded database: %v", err)
			}
			assertTradeInUpgradeV21(t, db, cfg.Driver)
		})
	}
}

// TestTradeInMigrationFailureRollsBackAndRetries proves a failure raised by the
// migration's own DDL leaves the previous schema version, its rows, its schema
// objects and foreign key enforcement intact, and that the retained version
// succeeds once the conflicting object is removed. The failing object is the
// migration's own index, so nothing before the migration can reject the database
// and the rollback can only have been produced inside the migration.
func TestTradeInMigrationFailureRollsBackAndRetries(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			cfg := tradeInUpgradeConfig(t, driver)
			db := tradeInUpgradeV20(t, cfg)
			if _, err := db.Exec("CREATE INDEX " + injectedMigrationIndex + " ON tenants(id)"); err != nil {
				t.Fatalf("inject the migration-conflicting index: %v", err)
			}
			err := basestore.Migrate(context.Background(), db, cfg)
			if err == nil {
				t.Fatal("a migration conflicting with an existing index succeeded")
			}
			if !strings.Contains(err.Error(), "trade-in schema") {
				t.Fatalf("the failure must originate inside the trade-in migration: %v", err)
			}
			assertTradeInMigrationRolledBack(t, db, cfg.Driver)
			if _, err := db.Exec("DROP INDEX IF EXISTS " + injectedMigrationIndex); err != nil {
				t.Fatalf("remove the injected index: %v", err)
			}
			if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
				t.Fatalf("retry after removing the injected failure: %v", err)
			}
			assertTradeInUpgradeV21(t, db, cfg.Driver)
			assertSeededPairingTypes(t, db, cfg.Driver)
		})
	}
}

func tradeInUpgradeConfig(t *testing.T, driver string) config.Database {
	t.Helper()
	if driver == "postgres" {
		dsn := os.Getenv("TEST_POSTGRES_DSN")
		if dsn == "" {
			if os.Getenv("REQUIRE_POSTGRES_TEST") == "true" {
				t.Fatal("TEST_POSTGRES_DSN is required for trade-in migration coverage")
			}
			t.Skip("TEST_POSTGRES_DSN is not set")
		}
		cfg, cleanup := postgresUpgradeSchema(t, dsn)
		t.Cleanup(cleanup)
		return cfg
	}
	return config.Database{Driver: driver, DSN: filepath.Join(t.TempDir(), "trade-in-upgrade.db")}
}

// tradeInUpgradeSeed is one pre-existing tenant of the schema-20 database.
type tradeInUpgradeSeed struct {
	tenant, user, username, customType, customName string
	category, model, asset, transaction, event     string
}

func tradeInUpgradeSeeds() []tradeInUpgradeSeed {
	return []tradeInUpgradeSeed{
		{
			tenant: tradeInUpgradeTenant, user: tradeInUpgradeUser, username: "trade-in-upgrade-owner",
			customType: tradeInUpgradeCustomType, customName: "trade_in_source",
			category: tradeInUpgradeCategory, model: tradeInUpgradeModel, asset: tradeInUpgradeAsset,
			transaction: tradeInUpgradeTransaction, event: tradeInUpgradeEvent,
		},
		{
			tenant: tradeInUpgradeTenant2, user: tradeInUpgradeUser2, username: "trade-in-upgrade-owner-2",
			customType: tradeInUpgradeCustomType2, customName: "trade_in_extra",
			category: tradeInUpgradeCategory2, model: tradeInUpgradeModel2, asset: tradeInUpgradeAsset2,
			transaction: tradeInUpgradeTransaction2, event: tradeInUpgradeEvent2,
		},
	}
}

// tradeInUpgradeV20 seeds two schema-20 tenants with lifecycle history and a
// custom event type. The first tenant's custom type already owns the canonical
// trade-in normalized key the migration wants for its system type.
func tradeInUpgradeV20(t *testing.T, cfg config.Database) *sql.DB {
	t.Helper()
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	applyMigrationsThrough(t, db, cfg.Driver, 20)
	createdAt := any("2026-09-01T00:00:00Z")
	occurredAt := any("2026-09-01T01:00:00Z")
	if cfg.Driver == "postgres" {
		createdAt = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
		occurredAt = time.Date(2026, 9, 1, 1, 0, 0, 0, time.UTC)
	}
	for _, seed := range tradeInUpgradeSeeds() {
		statements := []struct {
			query string
			args  []any
		}{
			{"INSERT INTO tenants (id, name, base_currency, created_at) VALUES (" + values(cfg.Driver, 4) + ")", []any{seed.tenant, "Trade-in upgrade " + seed.tenant, "CNY", createdAt}},
			{"INSERT INTO users (id, username, username_normalized, password_hash, created_at) VALUES (" + values(cfg.Driver, 5) + ")", []any{seed.user, seed.username, seed.username, "preserved-hash", createdAt}},
			{"INSERT INTO tenant_memberships (tenant_id, user_id, role, created_at) VALUES (" + values(cfg.Driver, 4) + ")", []any{seed.tenant, seed.user, "owner", createdAt}},
			// The custom type intentionally owns the canonical normalized key the
			// migration wants for its system type; it must survive untouched.
			{"INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at) VALUES (" + values(cfg.Driver, 7) + ")", []any{seed.customType, seed.tenant, seed.customName, seed.customName, "neutral", seed.user, createdAt}},
			{"INSERT INTO item_categories (id, tenant_id, name, created_at) VALUES (" + values(cfg.Driver, 4) + ")", []any{seed.category, seed.tenant, "Phone", createdAt}},
			{"INSERT INTO product_models (id, tenant_id, category_id, name, created_at) VALUES (" + values(cfg.Driver, 5) + ")", []any{seed.model, seed.tenant, seed.category, "Upgrade Phone", createdAt}},
			{"INSERT INTO assets (id, tenant_id, model_id, display_name, created_at) VALUES (" + values(cfg.Driver, 5) + ")", []any{seed.asset, seed.tenant, seed.model, "Preserved Upgrade Asset", createdAt}},
			{"INSERT INTO asset_transactions (id, tenant_id, occurred_at, source, created_by_user_id, created_at) VALUES (" + values(cfg.Driver, 6) + ")", []any{seed.transaction, seed.tenant, occurredAt, "manual", seed.user, createdAt}},
		}
		for _, statement := range statements {
			if _, err := db.Exec(statement.query, statement.args...); err != nil {
				t.Fatalf("seed v20 trade-in upgrade: %v", err)
			}
		}
		eventTypeID := tradeInUpgradeTypeID(t, db, cfg.Driver, seed.tenant, "purchase")
		if _, err := db.Exec("INSERT INTO asset_events (id, tenant_id, asset_id, transaction_id, event_type, base_amount_minor, base_currency, notes, occurred_at, created_by_user_id, created_at, event_type_id) VALUES ("+values(cfg.Driver, 12)+")",
			seed.event, seed.tenant, seed.asset, seed.transaction, "purchase", -12345, "CNY", "preserve trade-in history", occurredAt, seed.user, createdAt, eventTypeID); err != nil {
			t.Fatalf("seed v20 trade-in event: %v", err)
		}
	}
	return db
}

func tradeInUpgradeTypeID(t *testing.T, db *sql.DB, driver, tenant, code string) string {
	t.Helper()
	query := "SELECT id FROM asset_event_types WHERE tenant_id = ? AND system_code = ?"
	if driver == "postgres" {
		query = "SELECT id::text FROM asset_event_types WHERE tenant_id = $1 AND system_code = $2"
	}
	var id string
	if err := db.QueryRow(query, tenant, code).Scan(&id); err != nil {
		t.Fatalf("read %s system type: %v", code, err)
	}
	return id
}

func assertTradeInUpgradeV21(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	if driver == "sqlite" {
		var foreignKeys int
		if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil || foreignKeys != 1 {
			t.Fatalf("foreign keys not restored after the upgrade: %d %v", foreignKeys, err)
		}
	}
	var version int
	if err := db.QueryRow("SELECT MAX(version_id) FROM goose_db_version").Scan(&version); err != nil || version != 21 {
		t.Fatalf("schema version after trade-in upgrade: %d %v", version, err)
	}
	// The pre-existing custom types keep their identity, name and code.
	for _, seed := range tradeInUpgradeSeeds() {
		var custom int
		if err := db.QueryRow("SELECT COUNT(*) FROM asset_event_types WHERE tenant_id = "+upgradePlaceholder(driver)+" AND id = "+placeholders(driver, 1, 2)+" AND name = "+placeholders(driver, 1, 3)+" AND normalized_name = "+placeholders(driver, 1, 4)+" AND system_code = ''",
			seed.tenant, seed.customType, seed.customName, seed.customName).Scan(&custom); err != nil || custom != 1 {
			t.Fatalf("custom type %s changed: %d %v", seed.customType, custom, err)
		}
		// The preserved lifecycle rows keep their identity and economic shape.
		var events int
		if err := db.QueryRow("SELECT COUNT(*) FROM asset_events WHERE tenant_id = "+upgradePlaceholder(driver)+" AND asset_id = "+placeholders(driver, 1, 2)+" AND base_amount_minor = -12345 AND base_currency = 'CNY' AND notes = 'preserve trade-in history'",
			seed.tenant, seed.asset).Scan(&events); err != nil || events != 1 {
			t.Fatalf("preserved lifecycle history for %s changed: %d %v", seed.tenant, events, err)
		}
	}
	// The new relation columns exist.
	columns := "SELECT COUNT(*) FROM pragma_table_info('asset_events') WHERE name IN ('related_asset_id','related_asset_name','related_asset_spec','trade_in_link_id','trade_in_state')"
	if driver == "postgres" {
		columns = "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'asset_events' AND column_name IN ('related_asset_id','related_asset_name','related_asset_spec','trade_in_link_id','trade_in_state')"
	}
	var added int
	if err := db.QueryRow(columns).Scan(&added); err != nil || added != 5 {
		t.Fatalf("trade-in relation columns missing: %d %v", added, err)
	}
}

// assertSeededPairingTypes proves the migration seeded both pairing types for
// every existing tenant with canonical names, while the colliding custom type
// keeps the canonical normalized key, and that no two tenants share a row.
func assertSeededPairingTypes(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	for _, seed := range tradeInUpgradeSeeds() {
		for _, code := range []string{"trade_in_source", "trade_in_destination"} {
			var seeded int
			if err := db.QueryRow("SELECT COUNT(*) FROM asset_event_types WHERE tenant_id = "+upgradePlaceholder(driver)+" AND system_code = "+placeholders(driver, 1, 2)+" AND name = "+placeholders(driver, 1, 3),
				seed.tenant, code, code).Scan(&seeded); err != nil || seeded != 1 {
				t.Fatalf("seeded pairing type %s for %s mismatch: %d %v", code, seed.tenant, seeded, err)
			}
		}
	}
	// Two types for two tenants must be four distinct rows: a caller-side ID
	// reused across tenants would collide on the primary key instead.
	var distinct int
	if err := db.QueryRow("SELECT COUNT(DISTINCT id) FROM asset_event_types WHERE system_code IN ('trade_in_source', 'trade_in_destination')").Scan(&distinct); err != nil || distinct != 4 {
		t.Fatalf("seeded pairing type IDs must be unique per tenant: %d %v", distinct, err)
	}
}

// assertTriggerRepairsCollision removes one pairing type, lets a custom type take
// the canonical key, then adds a membership so the trigger must repair the system
// type with a non-colliding normalized key.
func assertTriggerRepairsCollision(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	const tenant = "22222222-2222-4222-8222-222222222221"
	const owner = "22222222-2222-4222-8222-222222222222"
	const member = "22222222-2222-4222-8222-222222222223"
	const custom = "22222222-2222-4222-8222-222222222224"
	createdAt := any("2026-09-02T00:00:00Z")
	if driver == "postgres" {
		createdAt = time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("trigger repair step: %v", err)
		}
	}
	exec("INSERT INTO tenants (id, name, base_currency, created_at) VALUES ("+values(driver, 4)+")", tenant, "Trigger Repair", "CNY", createdAt)
	exec("INSERT INTO users (id, username, username_normalized, password_hash, created_at) VALUES ("+values(driver, 5)+")", owner, "trigger-owner", "trigger-owner", "preserved-hash", createdAt)
	exec("INSERT INTO tenant_memberships (tenant_id, user_id, role, created_at) VALUES ("+values(driver, 4)+")", tenant, owner, "owner", createdAt)
	// A later, partial-legacy repair: the tenant loses its source type and a
	// custom type takes the canonical key.
	exec("DELETE FROM asset_event_types WHERE tenant_id = "+upgradePlaceholder(driver)+" AND system_code = 'trade_in_source'", tenant)
	exec("INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at) VALUES ("+values(driver, 7)+")",
		custom, tenant, "trade_in_source", "trade_in_source", "neutral", owner, createdAt)
	exec("INSERT INTO users (id, username, username_normalized, password_hash, created_at) VALUES ("+values(driver, 5)+")", member, "trigger-member", "trigger-member", "preserved-hash", createdAt)
	exec("INSERT INTO tenant_memberships (tenant_id, user_id, role, created_at) VALUES ("+values(driver, 4)+")", tenant, member, "owner", createdAt)
	var seeded int
	if err := db.QueryRow("SELECT COUNT(*) FROM asset_event_types WHERE tenant_id = "+upgradePlaceholder(driver)+" AND system_code = 'trade_in_source' AND normalized_name <> 'trade_in_source'", tenant).Scan(&seeded); err != nil || seeded != 1 {
		t.Fatalf("trigger did not repair the colliding pairing type: %d %v", seeded, err)
	}
	var customRows int
	if err := db.QueryRow("SELECT COUNT(*) FROM asset_event_types WHERE tenant_id = "+upgradePlaceholder(driver)+" AND id = "+placeholders(driver, 1, 2)+" AND system_code = ''", tenant, custom).Scan(&customRows); err != nil || customRows != 1 {
		t.Fatalf("the custom collision type was converted: %d %v", customRows, err)
	}
}

// assertTradeInMigrationRolledBack inspects a database the failed migration left
// behind. Every check is read-only except the constraint probe, which proves the
// widened system-code domain never leaked into the retained schema.
func assertTradeInMigrationRolledBack(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	var version int
	if err := db.QueryRow("SELECT MAX(version_id) FROM goose_db_version").Scan(&version); err != nil || version != 20 {
		t.Fatalf("schema version after the rolled-back trade-in migration: %d %v", version, err)
	}
	// The migration had already altered the schema when it failed, so absent
	// columns prove its earlier DDL rolled back together with the version.
	columns := "SELECT COUNT(*) FROM pragma_table_info('asset_events') WHERE name IN ('related_asset_id','related_asset_name','related_asset_spec','trade_in_link_id','trade_in_state')"
	injected := "SELECT COUNT(*) FROM sqlite_master WHERE name = '" + injectedMigrationIndex + "'"
	nextTable := "SELECT COUNT(*) FROM sqlite_master WHERE name = 'asset_event_types_next'"
	if driver == "postgres" {
		columns = "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'asset_events' AND column_name IN ('related_asset_id','related_asset_name','related_asset_spec','trade_in_link_id','trade_in_state')"
		injected = "SELECT COUNT(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = '" + injectedMigrationIndex + "'"
		nextTable = "SELECT COUNT(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = current_schema() AND c.relname = 'asset_event_types_next'"
	}
	var added int
	if err := db.QueryRow(columns).Scan(&added); err != nil || added != 0 {
		t.Fatalf("trade-in columns survived the rollback: %d %v", added, err)
	}
	// The colliding object is still there, and the rebuild table never was
	// created, so the failure happened before the type table was replaced.
	var indexRows int
	if err := db.QueryRow(injected).Scan(&indexRows); err != nil || indexRows != 1 {
		t.Fatalf("injected index state after the rollback: %d %v", indexRows, err)
	}
	var rebuild int
	if err := db.QueryRow(nextTable).Scan(&rebuild); err != nil || rebuild != 0 {
		t.Fatalf("the rebuilt type table survived the rollback: %d %v", rebuild, err)
	}
	// No tenant received a pairing type, and every pre-existing row is intact.
	var seeded int
	if err := db.QueryRow("SELECT COUNT(*) FROM asset_event_types WHERE system_code IN ('trade_in_source', 'trade_in_destination')").Scan(&seeded); err != nil || seeded != 0 {
		t.Fatalf("pairing types survived the rollback: %d %v", seeded, err)
	}
	assertTradeInUpgradeRowsPreserved(t, db, driver)
	if driver == "sqlite" {
		var foreignKeys int
		if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil || foreignKeys != 1 {
			t.Fatalf("foreign keys not restored after the rolled-back upgrade: %d %v", foreignKeys, err)
		}
	}
	assertSystemCodeConstraintNarrow(t, db, driver)
}

func assertTradeInUpgradeRowsPreserved(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	for _, seed := range tradeInUpgradeSeeds() {
		var custom int
		if err := db.QueryRow("SELECT COUNT(*) FROM asset_event_types WHERE tenant_id = "+upgradePlaceholder(driver)+" AND id = "+placeholders(driver, 1, 2)+" AND system_code = '' AND name = "+placeholders(driver, 1, 3),
			seed.tenant, seed.customType, seed.customName).Scan(&custom); err != nil || custom != 1 {
			t.Fatalf("custom type %s changed: %d %v", seed.customType, custom, err)
		}
		var events int
		if err := db.QueryRow("SELECT COUNT(*) FROM asset_events WHERE tenant_id = "+upgradePlaceholder(driver)+" AND asset_id = "+placeholders(driver, 1, 2)+" AND notes = 'preserve trade-in history'",
			seed.tenant, seed.asset).Scan(&events); err != nil || events != 1 {
			t.Fatalf("preserved lifecycle history for %s changed: %d %v", seed.tenant, events, err)
		}
	}
}

// assertSystemCodeConstraintNarrow proves the retained schema only accepts the
// original system codes. The exact probe statement is first executed with an
// allowed code, so a rejection can only come from the constraint itself.
func assertSystemCodeConstraintNarrow(t *testing.T, db *sql.DB, driver string) {
	t.Helper()
	createdAt := any("2026-09-01T00:00:00Z")
	enabled := any(1)
	if driver == "postgres" {
		createdAt = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
		enabled = true
	}
	insert := "INSERT INTO asset_event_types (id, tenant_id, name, normalized_name, cashflow_direction, created_by_user_id, created_at, system_code, enabled, updated_at) VALUES (" + values(driver, 10) + ")"
	if _, err := db.Exec(insert, rollbackProbeTypeOK, tradeInUpgradeTenant, "rollback_probe_ok", "rollback_probe_ok", "neutral", tradeInUpgradeUser, createdAt, "", enabled, createdAt); err != nil {
		t.Fatalf("the rollback probe row must be insertable with an allowed code: %v", err)
	}
	if _, err := db.Exec(insert, rollbackProbeType, tradeInUpgradeTenant, "rollback_probe", "rollback_probe", "neutral", tradeInUpgradeUser, createdAt, "trade_in_source", enabled, createdAt); err == nil {
		t.Fatal("the widened system-code constraint survived the rollback")
	}
}

func placeholders(driver string, count, offset int) string {
	result := ""
	for i := 0; i < count; i++ {
		if i > 0 {
			result += ", "
		}
		if driver == "postgres" {
			result += fmt.Sprintf("$%d", offset+i)
		} else {
			result += "?"
		}
	}
	return result
}
