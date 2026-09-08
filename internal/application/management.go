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
	SpecificationStore
	LifecycleStore
	ModelMediaStore
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

func (s *ManagementService) BindResource(ctx context.Context, actor Principal, key string, cmd BindModel3DResource) error {
	_, err := managementWrite(ctx, s, actor, key, "bind_resource", CapabilityManageCatalog, cmd, func(store ManagementStore) (bool, error) {
		// Binding only changes metadata; no blob access is needed in this transaction.
		err := (&ModelMediaService{store: store}).Bind(ctx, actor, cmd)
		return err == nil, err
	})
	return err
}

func (s *ManagementService) CreateEventType(ctx context.Context, actor Principal, key string, cmd CreateAssetEventType) (domain.AssetEventTypeDefinition, error) {
	return managementWrite(ctx, s, actor, key, "create_event_type", CapabilityManageLifecycle, cmd, func(store ManagementStore) (domain.AssetEventTypeDefinition, error) {
		return NewLifecycleService(store).CreateEventType(ctx, actor, cmd)
	})
}
func (s *ManagementService) UpdateEventType(ctx context.Context, actor Principal, key, id string, cmd UpdateEventType) (domain.AssetEventTypeDefinition, error) {
	payload := struct {
		ID      string
		Command UpdateEventType
	}{id, cmd}
	return managementWrite(ctx, s, actor, key, "update_event_type", CapabilityManageLifecycle, payload, func(store ManagementStore) (domain.AssetEventTypeDefinition, error) {
		return NewLifecycleService(store).UpdateEventType(ctx, actor, id, cmd)
	})
}
func (s *ManagementService) SetEventTypeEnabled(ctx context.Context, actor Principal, key, id string, enabled bool) (domain.AssetEventTypeDefinition, error) {
	payload := struct {
		ID      string
		Enabled bool
	}{id, enabled}
	return managementWrite(ctx, s, actor, key, "set_event_type_enabled", CapabilityManageLifecycle, payload, func(store ManagementStore) (domain.AssetEventTypeDefinition, error) {
		return NewLifecycleService(store).SetEventTypeEnabled(ctx, actor, id, enabled)
	})
}

func (s *ManagementService) SaveModel(ctx context.Context, actor Principal, key string, cmd SaveModelSpecification) error {
	_, err := managementWrite(ctx, s, actor, key, "save_model", CapabilityManageCatalog, cmd, func(store ManagementStore) (bool, error) {
		err := NewSpecificationService(store).SaveModel(ctx, actor, cmd)
		return err == nil, err
	})
	return err
}
func (s *ManagementService) SaveResource(ctx context.Context, actor Principal, key string, cmd SaveResourceSpecification) error {
	_, err := managementWrite(ctx, s, actor, key, "save_resource", CapabilityManageCatalog, cmd, func(store ManagementStore) (bool, error) {
		err := NewSpecificationService(store).SaveResource(ctx, actor, cmd)
		return err == nil, err
	})
	return err
}
func (s *ManagementService) DeleteAppearance(ctx context.Context, actor Principal, key, id string) error {
	_, err := managementWrite(ctx, s, actor, key, "delete_appearance", CapabilityManageCatalog, id, func(store ManagementStore) (bool, error) {
		err := NewSpecificationService(store).DeleteAppearance(ctx, actor, id)
		return err == nil, err
	})
	return err
}

func (s *ManagementService) SaveAsset(ctx context.Context, actor Principal, key string, cmd SaveSpecificationAsset) (domain.Asset, error) {
	return managementWrite(ctx, s, actor, key, "save_asset", CapabilityManageCatalog, cmd, func(store ManagementStore) (domain.Asset, error) {
		return NewSpecificationService(store).SaveAsset(ctx, actor, cmd)
	})
}

func (s *ManagementService) SaveType(ctx context.Context, actor Principal, key string, cmd SaveSpecificationType) (domain.SpecificationTagType, error) {
	return managementWrite(ctx, s, actor, key, "save_tag_type", CapabilityManageCatalog, cmd, func(store ManagementStore) (domain.SpecificationTagType, error) {
		return NewSpecificationService(store).SaveType(ctx, actor, cmd)
	})
}

func (s *ManagementService) SaveTag(ctx context.Context, actor Principal, key string, cmd SaveSpecificationTag) (domain.SpecificationTag, error) {
	return managementWrite(ctx, s, actor, key, "save_tag", CapabilityManageCatalog, cmd, func(store ManagementStore) (domain.SpecificationTag, error) {
		return NewSpecificationService(store).SaveTag(ctx, actor, cmd)
	})
}

func (s *ManagementService) SaveAppearance(ctx context.Context, actor Principal, key string, cmd SaveAppearanceDefault) (domain.AppearanceDefault, error) {
	return managementWrite(ctx, s, actor, key, "save_appearance", CapabilityManageCatalog, cmd, func(store ManagementStore) (domain.AppearanceDefault, error) {
		return NewSpecificationService(store).SaveAppearance(ctx, actor, cmd)
	})
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
