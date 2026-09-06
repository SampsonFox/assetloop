package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/SampsonFox/assetloop/internal/domain"
	"github.com/SampsonFox/assetloop/migrations"
	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
)

const specificationMigrationName = "00013_specification_tags.sql"

// DDL, Unicode-aware backfill, verification and Goose's version write share one
// transaction. The paired SQL files also remain the schema source for sqlc.
func specificationMigration(driver string) *goose.Migration {
	return goose.NewGoMigration(13, &goose.GoFunc{RunTx: func(ctx context.Context, tx *sql.Tx) error {
		ddl, err := migrations.FS.ReadFile(driver + "/" + specificationMigrationName)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(ddl)); err != nil {
			return fmt.Errorf("specification schema: %w", err)
		}
		if err := backfillSpecifications(ctx, tx, driver); err != nil {
			return fmt.Errorf("specification backfill: %w", err)
		}
		if driver == "sqlite" {
			rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
			if err != nil {
				return err
			}
			defer rows.Close()
			if rows.Next() {
				return fmt.Errorf("specification migration would leave a foreign key violation")
			}
			return rows.Err()
		}
		return nil
	}}, nil)
}

type legacySpecification struct {
	id, tenant, model, name, color, resource, colorTag string
}

func backfillSpecifications(ctx context.Context, tx *sql.Tx, driver string) error {
	exec := func(query string, args ...any) error {
		if driver == "postgres" {
			for index := 1; strings.Contains(query, "?"); index++ {
				query = strings.Replace(query, "?", fmt.Sprintf("$%d", index), 1)
			}
		}
		_, err := tx.ExecContext(ctx, query, args...)
		return err
	}
	rows, err := tx.QueryContext(ctx, "SELECT id FROM tenants ORDER BY id")
	if err != nil {
		return err
	}
	var tenants []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		tenants = append(tenants, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	typeIDs := map[string]string{}
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	for _, tenant := range tenants {
		for _, kind := range []struct {
			code, name string
			appearance bool
		}{{"color", "颜色", true}, {"legacy-specification", "旧规格描述", false}} {
			id := uuid.NewString()
			typeIDs[tenant+"/"+kind.code] = id
			if err := exec("INSERT INTO specification_tag_types(id,tenant_id,name,normalized_name,multiple,affects_appearance,enabled,system_code,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)", id, tenant, kind.name, domain.NormalizeSpecificationName(kind.name), false, kind.appearance, true, kind.code, stamp, stamp); err != nil {
				return err
			}
		}
	}
	rows, err = tx.QueryContext(ctx, "SELECT id,tenant_id,model_id,name,color,COALESCE(CAST(model_3d_resource_id AS TEXT),'') FROM product_variants ORDER BY tenant_id,model_id,id")
	if err != nil {
		return err
	}
	var variants []legacySpecification
	for rows.Next() {
		var variant legacySpecification
		if err := rows.Scan(&variant.id, &variant.tenant, &variant.model, &variant.name, &variant.color, &variant.resource); err != nil {
			rows.Close()
			return err
		}
		variants = append(variants, variant)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	tagIDs := map[string]string{}
	for index := range variants {
		variant := &variants[index]
		for _, description := range []struct{ code, text string }{{"legacy-specification", variant.name}, {"color", variant.color}} {
			name := strings.TrimSpace(description.text)
			if name == "" {
				continue
			}
			typeID := typeIDs[variant.tenant+"/"+description.code]
			normalized := domain.NormalizeSpecificationName(name)
			key := typeID + "/" + normalized
			tagID := tagIDs[key]
			if tagID == "" {
				tagID = uuid.NewString()
				tagIDs[key] = tagID
				if err := exec("INSERT INTO specification_tags(id,tenant_id,type_id,name,normalized_name,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)", tagID, variant.tenant, typeID, name, normalized, true, stamp, stamp); err != nil {
					return err
				}
			}
			if description.code == "color" {
				variant.colorTag = tagID
			}
			if err := exec("INSERT INTO model_allowed_tags(tenant_id,model_id,tag_id) VALUES(?,?,?) ON CONFLICT DO NOTHING", variant.tenant, variant.model, tagID); err != nil {
				return err
			}
			if err := exec("INSERT INTO legacy_variant_tags(tenant_id,variant_id,tag_id) VALUES(?,?,?)", variant.tenant, variant.id, tagID); err != nil {
				return err
			}
		}
	}
	if err := exec("INSERT INTO asset_specification_tags(tenant_id,asset_id,model_id,tag_id) SELECT a.tenant_id,a.id,a.model_id,t.tag_id FROM assets a JOIN legacy_variant_tags t ON t.tenant_id=a.tenant_id AND t.variant_id=a.variant_id"); err != nil {
		return err
	}
	groups := map[string][]legacySpecification{}
	for _, variant := range variants {
		if variant.resource != "" {
			groups[variant.tenant+"/"+variant.model+"/"+variant.colorTag] = append(groups[variant.tenant+"/"+variant.model+"/"+variant.colorTag], variant)
		}
	}
	for _, group := range groups {
		first := group[0]
		resources := map[string]bool{}
		for _, variant := range group {
			resources[variant.resource] = true
		}
		if first.colorTag != "" && len(resources) == 1 {
			id := uuid.NewString()
			if err := exec("INSERT INTO model_appearance_defaults(id,tenant_id,model_id,resource_id,created_at,updated_at) VALUES(?,?,?,?,?,?)", id, first.tenant, first.model, first.resource, stamp, stamp); err != nil {
				return err
			}
			if err := exec("INSERT INTO model_appearance_conditions(tenant_id,rule_id,model_id,tag_id) VALUES(?,?,?,?)", first.tenant, id, first.model, first.colorTag); err != nil {
				return err
			}
			continue
		}
		reason := "conflicting-appearance"
		if first.colorTag == "" {
			reason = "no-appearance-condition"
		}
		for _, variant := range group {
			if err := exec("INSERT INTO legacy_variant_media(tenant_id,variant_id,model_id,resource_id,reason) VALUES(?,?,?,?,?)", variant.tenant, variant.id, variant.model, variant.resource, reason); err != nil {
				return err
			}
			if err := exec("UPDATE assets SET model_3d_resource_id=? WHERE tenant_id=? AND variant_id=? AND model_3d_resource_id IS NULL", variant.resource, variant.tenant, variant.id); err != nil {
				return err
			}
		}
	}
	// Seed classifications from explicit legacy references, not guessed GLB content.
	return exec(`INSERT INTO resource_categories(tenant_id,resource_id,category_id)
 SELECT tenant_id,model_3d_resource_id,category_id FROM product_models WHERE model_3d_resource_id IS NOT NULL
 UNION SELECT v.tenant_id,v.model_3d_resource_id,m.category_id FROM product_variants v JOIN product_models m ON m.tenant_id=v.tenant_id AND m.id=v.model_id WHERE v.model_3d_resource_id IS NOT NULL
 UNION SELECT a.tenant_id,a.model_3d_resource_id,m.category_id FROM assets a JOIN product_models m ON m.tenant_id=a.tenant_id AND m.id=a.model_id WHERE a.model_3d_resource_id IS NOT NULL
 ON CONFLICT DO NOTHING`)
}
