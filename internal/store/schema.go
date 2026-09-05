package store

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"strconv"
	"strings"

	"github.com/SampsonFox/assetloop/migrations"
)

func schemaVersions(ctx context.Context, db *sql.DB, driver string) (int64, int64, error) {
	files, err := fs.Glob(migrations.FS, driver+"/*.sql")
	if err != nil {
		return 0, 0, err
	}
	known := make(map[int64]bool)
	var target int64
	for _, name := range files {
		base := strings.TrimPrefix(name, driver+"/")
		version, err := strconv.ParseInt(strings.SplitN(base, "_", 2)[0], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid migration name: %w", err)
		}
		known[version] = true
		if version > target {
			target = version
		}
	}
	if target == 0 {
		return 0, 0, fmt.Errorf("no migrations for %s", driver)
	}
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='goose_db_version')"
	if driver == "postgres" {
		query = "SELECT to_regclass('goose_db_version') IS NOT NULL"
	}
	if err := db.QueryRowContext(ctx, query).Scan(&exists); err != nil {
		return 0, target, fmt.Errorf("inspect schema version table: %w", err)
	}
	if !exists {
		return 0, target, nil
	}
	rows, err := db.QueryContext(ctx, "SELECT version_id, is_applied FROM goose_db_version ORDER BY id")
	if err != nil {
		return 0, target, err
	}
	defer rows.Close()
	applied := make(map[int64]bool)
	for rows.Next() {
		var version int64
		var active bool
		if err := rows.Scan(&version, &active); err != nil {
			return 0, target, err
		}
		if version != 0 && !known[version] {
			return 0, target, fmt.Errorf("database contains unsupported schema version %d; binary supports through %d", version, target)
		}
		applied[version] = active
	}
	if err := rows.Err(); err != nil {
		return 0, target, err
	}
	var current int64
	for version, active := range applied {
		if active && version > current {
			current = version
		}
	}
	for version := range known {
		if version <= current && !applied[version] {
			return current, target, fmt.Errorf("database migration history is missing version %d", version)
		}
	}
	return current, target, nil
}
