package store

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/SampsonFox/assetloop/internal/config"
	"github.com/SampsonFox/assetloop/migrations"
	"github.com/pressly/goose/v3"
)

func TestOAuthUpgradeFrom14(t *testing.T) {
	cfg := config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "oauth-upgrade.db")}
	db, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	files, err := fs.Sub(migrations.FS, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, db, files,
		goose.WithExcludeNames([]string{specificationMigrationName, specificationContractMigrationName, "00015_oauth.sql", "00016_management_requests.sql"}),
		goose.WithGoMigrations(specificationMigration("sqlite"), specificationContractMigration("sqlite")))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := db.Exec("PRAGMA foreign_keys=OFF"); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO tenants(id,name,base_currency,created_at) VALUES ('upgrade-tenant','Retained account','CNY','2026-01-01T00:00:00Z')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE oauth_credentials (collision TEXT)"); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db, cfg); err == nil {
		t.Fatal("expected migration collision")
	}
	var version, grants int
	if err := db.QueryRow("SELECT MAX(version_id) FROM goose_db_version").Scan(&version); err != nil || version != 14 {
		t.Fatalf("version after rollback: %d / %v", version, err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name='oauth_grants'").Scan(&grants); err != nil || grants != 0 {
		t.Fatal("partial migration persisted")
	}
	if _, err := db.Exec("DROP TABLE oauth_credentials"); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db, cfg); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := db.QueryRow("SELECT name FROM tenants WHERE id='upgrade-tenant'").Scan(&name); err != nil || name != "Retained account" {
		t.Fatal("existing account changed")
	}
	if err := db.QueryRow("SELECT MAX(version_id) FROM goose_db_version").Scan(&version); err != nil || version != 16 {
		t.Fatal("upgrade not committed")
	}
}
