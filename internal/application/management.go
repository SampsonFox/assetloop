package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/SampsonFox/assetloop/internal/domain"
)

type ManagementRequest struct{ TenantID, UserID, Key, Hash, ResultJSON string }
type ManagementStore interface {
	CatalogStore
	WithManagementWrite(context.Context, string, func(ManagementStore) error) error
	FindManagementRequest(context.Context, string, string, string) (ManagementRequest, bool, error)
	SaveManagementRequest(context.Context, ManagementRequest) error
}

// ManagementService adds durable command replay to existing management use cases.
// Business validation remains in the called services, not in a second MCP model.
type ManagementService struct{ store ManagementStore }

func NewManagementService(store ManagementStore) *ManagementService {
	return &ManagementService{store: store}
}

func managementWrite[T any](ctx context.Context, s *ManagementService, actor Principal, key, operation string, capability Capability, command any, run func(ManagementStore) (T, error)) (T, error) {
	var result T
	if err := actor.Require(capability); err != nil {
		return result, err
	}
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 128 || strings.IndexFunc(key, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
		return result, NewInputError("validation.request_key")
	}
	payload, err := json.Marshal(struct {
		Operation string
		Command   any
	}{operation, command})
	if err != nil {
		return result, err
	}
	digest := sha256.Sum256(payload)
	receipt := ManagementRequest{TenantID: actor.TenantID, UserID: actor.UserID, Key: key, Hash: hex.EncodeToString(digest[:])}
	err = s.store.WithManagementWrite(ctx, actor.TenantID, func(store ManagementStore) error {
		previous, found, err := store.FindManagementRequest(ctx, actor.TenantID, actor.UserID, key)
		if err != nil {
			return err
		}
		if found {
			if previous.Hash != receipt.Hash {
				return NewInputError("validation.request_conflict")
			}
			return json.Unmarshal([]byte(previous.ResultJSON), &result)
		}
		result, err = run(store)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return err
		}
		receipt.ResultJSON = string(encoded)
		return store.SaveManagementRequest(ctx, receipt)
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return result, nil
}

func (s *ManagementService) CreateCategory(ctx context.Context, actor Principal, key string, cmd CreateCategory) (domain.ItemCategory, error) {
	return managementWrite(ctx, s, actor, key, "create_category", CapabilityManageCatalog, cmd, func(store ManagementStore) (domain.ItemCategory, error) {
		return NewCatalogService(store).CreateCategory(ctx, actor, cmd)
	})
}
func (s *ManagementService) UpdateCategory(ctx context.Context, actor Principal, key string, cmd UpdateCategory) (domain.ItemCategory, error) {
	return managementWrite(ctx, s, actor, key, "update_category", CapabilityManageCatalog, cmd, func(store ManagementStore) (domain.ItemCategory, error) {
		return NewCatalogService(store).UpdateCategory(ctx, actor, cmd)
	})
}
func (s *ManagementService) CreateModel(ctx context.Context, actor Principal, key string, cmd CreateModel) (domain.ProductModel, error) {
	return managementWrite(ctx, s, actor, key, "create_model", CapabilityManageCatalog, cmd, func(store ManagementStore) (domain.ProductModel, error) {
		return NewCatalogService(store).CreateModel(ctx, actor, cmd)
	})
}
