package domain

import "time"

type ItemCategory struct {
	ID        string
	TenantID  string
	Name      string
	CreatedAt time.Time
}

type ProductModel struct {
	ID           string
	TenantID     string
	CategoryID   string
	CategoryName string
	Name         string
	CreatedAt    time.Time
}

type ProductVariant struct {
	ID           string
	TenantID     string
	CategoryID   string
	CategoryName string
	ModelID      string
	ModelName    string
	Name         string
	CreatedAt    time.Time
}

type Asset struct {
	ID              string
	TenantID        string
	CategoryID      string
	Category        string
	ModelID         string
	Model           string
	VariantID       string
	Variant         string
	DisplayName     string
	SerialNumber    string
	Color           string
	PurchaseChannel string
	Notes           string
	CreatedAt       time.Time
}
