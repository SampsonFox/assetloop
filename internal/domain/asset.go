package domain

import "time"

type Asset struct {
	ID          string
	TenantID    string
	CategoryID  string
	Category    string
	ModelID     string
	Model       string
	VariantID   string
	Variant     string
	DisplayName string
	CreatedAt   time.Time
}
