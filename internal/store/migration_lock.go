package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"time"
)

// Keep the lock file: unlinking it can give waiting processes different inodes.
// Kernel locks are released when a process exits, including after a crash.
func migrationLock(ctx context.Context, db *sql.DB, dialect string) (func(), error) {
	if dialect == "postgres" {
		conn, err := db.Conn(ctx)
		if err != nil {
			return nil, err
		}
		var key int64
		if err := conn.QueryRowContext(ctx, "SELECT hashtextextended(current_database() || ':' || current_schema() || ':assetloop:migrations', 0)").Scan(&key); err != nil {
			conn.Close()
			return nil, err
		}
		if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", key); err != nil {
			conn.Close()
			return nil, err
		}
		return func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := conn.ExecContext(cleanup, "SELECT pg_advisory_unlock($1)", key); err != nil {
				// Do not return a possibly locked session to the pool.
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			}
			conn.Close()
		}, nil
	}
	var seq int
	var name, path string
	if err := db.QueryRowContext(ctx, "PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
		return nil, err
	}
	if path == "" {
		return func() {}, nil
	}
	file, err := os.OpenFile(path+".migration.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open migration lock: %w", err)
	}
	for {
		locked, err := tryFileLock(file)
		if err != nil {
			file.Close()
			return nil, fmt.Errorf("acquire migration lock: %w", err)
		}
		if locked {
			return func() { file.Close() }, nil
		}
		select {
		case <-ctx.Done():
			file.Close()
			return nil, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}
