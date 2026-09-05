package domain

import "time"

type ItemCategory struct {
	ID        string
	TenantID  string
	Name      string
	IconKey   string
	CreatedAt time.Time
}

type ProductModel struct {
	ID                string
	TenantID          string
	CategoryID        string
	CategoryName      string
	CategoryIcon      string
	Name              string
	CreatedAt         time.Time
	Model3D           *ProductModel3D
	Model3DResourceID string
}

type ProductModel3D struct {
	ResourceID string
	StoreID    string
	ObjectKey  string
	SHA256     string
	SizeBytes  int64
	SourceURL  string
	Author     string
	License    string
	UpdatedAt  time.Time
}

type ProductVariant struct {
	Color             string
	Model3DResourceID string
	ID                string
	TenantID          string
	CategoryID        string
	CategoryName      string
	CategoryIcon      string
	ModelID           string
	ModelName         string
	Name              string
	CreatedAt         time.Time
}

type Asset struct {
	Model3DResourceID string
	ID                string
	TenantID          string
	CategoryID        string
	Category          string
	CategoryIcon      string
	ModelID           string
	Model             string
	VariantID         string
	Variant           string
	DisplayName       string
	SerialNumber      string
	Color             string
	PurchaseChannel   string
	Notes             string
	CreatedAt         time.Time
}

type Model3DResource struct {
	ReferenceCount int
	ID             string
	TenantID       string
	Name           string
	Status         string
	ProductModel3D
	CreatedAt time.Time
}
