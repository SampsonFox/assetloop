package store

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SampsonFox/assetloop/internal/config"
	"github.com/SampsonFox/assetloop/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

func Open(cfg config.Database) (*sql.DB, error) {
	if cfg.Driver == "sqlite" {
		if path := sqlitePath(cfg.DSN); path != "" {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return nil, fmt.Errorf("create database directory: %w", err)
			}
		}
		db, err := sql.Open("sqlite", cfg.DSN)
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(1)
		if _, err := db.Exec("PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;"); err != nil {
			db.Close()
			return nil, fmt.Errorf("configure sqlite: %w", err)
		}
		return db, nil
	}
	if cfg.Driver == "postgres" {
		return sql.Open("pgx", cfg.DSN)
	}
	return nil, fmt.Errorf("unsupported database driver %q", cfg.Driver)
}

func Migrate(ctx context.Context, db *sql.DB, cfg config.Database) error {
	unlock, err := migrationLock(ctx, db, cfg.Driver)
	if err != nil {
		return err
	}
	defer unlock()
	current, target, err := schemaVersions(ctx, db, cfg.Driver)
	if err != nil {
		return err
	}
	if current == target {
		return nil
	}
	if cfg.Driver == "sqlite" && current > 0 {
		if err := backupSQLite(ctx, db, cfg.DSN); err != nil {
			return err
		}
	}
	migrationFS, err := fs.Sub(migrations.FS, cfg.Driver)
	if err != nil {
		return fmt.Errorf("open %s migrations: %w", cfg.Driver, err)
	}
	dialect := goose.DialectPostgres
	if cfg.Driver == "sqlite" {
		dialect = goose.DialectSQLite3
	}
	provider, err := goose.NewProvider(dialect, db, migrationFS)
	if err != nil {
		return fmt.Errorf("initialize migrations: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate %s: %w", cfg.Driver, err)
	}
	if cfg.Driver == "sqlite" {
		if err := verifySQLite(ctx, db); err != nil {
			return err
		}
	}
	return CheckSchema(ctx, db, cfg.Driver)
}

// CheckSchema is read-only: application startup never initializes PostgreSQL tables.
func CheckSchema(ctx context.Context, db *sql.DB, driver string) error {
	current, target, err := schemaVersions(ctx, db, driver)
	if err != nil {
		return err
	}
	if current != target {
		return fmt.Errorf("database schema %d requires migration to %d; run assetloop migrate", current, target)
	}
	return nil
}

func verifySQLite(ctx context.Context, db *sql.DB) error {
	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("check SQLite integrity: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("SQLite integrity check: %s", result)
	}
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("check SQLite foreign keys: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("SQLite foreign key check failed")
	}
	return rows.Err()
}

func sqlitePath(dsn string) string {
	if dsn == "" || dsn == ":memory:" || strings.Contains(dsn, "mode=memory") {
		return ""
	}
	path := strings.TrimPrefix(dsn, "file:")
	if before, _, ok := strings.Cut(path, "?"); ok {
		path = before
	}
	return filepath.Clean(path)
}

func backupSQLite(ctx context.Context, db *sql.DB, dsn string) error {
	path := sqlitePath(dsn)
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect sqlite database: %w", err)
	}
	if info.Size() == 0 {
		return nil
	}

	backupPath := path + ".backup-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", backupPath); err != nil {
		return fmt.Errorf("create consistent sqlite backup: %w", err)
	}
	backup, err := sql.Open("sqlite", backupPath)
	if err != nil {
		return fmt.Errorf("open sqlite backup for verification: %w", err)
	}
	defer backup.Close()
	var result string
	if err := backup.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("verify sqlite backup: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("verify sqlite backup: integrity_check returned %q", result)
	}
	return nil
}
