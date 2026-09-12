package mcp

import (
	"time"

	"github.com/SampsonFox/assetloop/internal/domain"
)

// Explicit transport projections keep storage identities out of tool responses,
// including nested media on product models and effective appearance results.
type ResourceResult struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Status         string    `json:"status"`
	ReferenceCount int       `json:"reference_count"`
	SizeBytes      int64     `json:"size_bytes"`
	SourceURL      string    `json:"source_url"`
	Author         string    `json:"author"`
	License        string    `json:"license"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
type ModelResult struct {
	ID           string    `json:"id"`
	CategoryID   string    `json:"category_id"`
	CategoryName string    `json:"category_name"`
	CategoryIcon string    `json:"category_icon"`
	Name         string    `json:"name"`
	ResourceID   string    `json:"resource_id"`
	CreatedAt    time.Time `json:"created_at"`
}
type AppearanceResult struct {
	Resource *ResourceResult `json:"resource"`
	Source   string          `json:"source"`
	Conflict bool            `json:"conflict"`
	RuleIDs  []string        `json:"rule_ids"`
}

func resourceResult(r domain.Model3DResource) ResourceResult {
	return ResourceResult{ID: r.ID, Name: r.Name, Status: r.Status, ReferenceCount: r.ReferenceCount, SizeBytes: r.SizeBytes, SourceURL: r.SourceURL, Author: r.Author, License: r.License, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
func modelResult(m domain.ProductModel) ModelResult {
	return ModelResult{ID: m.ID, CategoryID: m.CategoryID, CategoryName: m.CategoryName, CategoryIcon: m.CategoryIcon, Name: m.Name, ResourceID: m.Model3DResourceID, CreatedAt: m.CreatedAt}
}
