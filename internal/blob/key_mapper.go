package blob

import (
	"fmt"
	"regexp"

	"github.com/google/uuid"
)

type ObjectKeyMapper struct{}

func (ObjectKeyMapper) Model3DResource(tenantID, resourceID, sha string) (string, error) {
	if _, err := (ObjectKeyMapper{}).ProductModel3D(tenantID, resourceID, sha); err != nil {
		return "", err
	}
	return fmt.Sprintf("tenants/%s/model-3d-resources/%s/%s.glb", tenantID, resourceID, sha), nil
}

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (ObjectKeyMapper) ProductModel3D(tenantID, modelID, sha string) (string, error) {
	if _, err := uuid.Parse(tenantID); err != nil {
		return "", fmt.Errorf("invalid tenant ID: %w", err)
	}
	if _, err := uuid.Parse(modelID); err != nil {
		return "", fmt.Errorf("invalid model ID: %w", err)
	}
	if !sha256Pattern.MatchString(sha) {
		return "", fmt.Errorf("invalid SHA-256")
	}
	return fmt.Sprintf("tenants/%s/models/%s/%s.glb", tenantID, modelID, sha), nil
}
