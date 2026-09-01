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
	if cfg.Driver == "sqlite" {
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
	return nil
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
