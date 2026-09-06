package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Validate before any migration writes so an unknown legacy type names its record.
func checkLegacyEventTypes(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, "SELECT tenant_id, normalized_name FROM asset_event_types")
	if err != nil {
		return err
	}
	types := map[string]bool{}
	for rows.Next() {
		var tenant, name string
		if err := rows.Scan(&tenant, &name); err != nil {
			rows.Close()
			return err
		}
		types[tenant+"/"+name] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = db.QueryContext(ctx, "SELECT id, tenant_id, event_type FROM asset_events")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, tenant, name string
		if err := rows.Scan(&id, &tenant, &name); err != nil {
			return err
		}
		switch name {
		case "purchase", "repair", "sale", "void":
			continue
		}
		if !types[tenant+"/"+strings.ToLower(strings.TrimSpace(name))] {
			return fmt.Errorf("event type migration: cannot associate event %s in tenant %s", id, tenant)
		}
	}
	return rows.Err()
}
