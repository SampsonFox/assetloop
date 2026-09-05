package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/SampsonFox/assetloop/internal/domain"
)

func (s *LifecycleService) write(ctx context.Context, actor Principal, operation, eventID string, cmd RecordEvent, fn func(*LifecycleService) (domain.AssetEvent, error)) (domain.AssetEvent, error) {
	if err := validID("tenant ID", actor.TenantID); err != nil {
		return domain.AssetEvent{}, err
	}
	key := strings.TrimSpace(cmd.RequestKey)
	if len(key) > 128 || strings.IndexFunc(key, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
		return domain.AssetEvent{}, NewInputError("validation.request_key")
	}
	cmd.RequestKey = ""
	cmd.OccurredAt = cmd.OccurredAt.UTC()
	cmd.FXRateDate = cmd.FXRateDate.UTC()
	payload, err := json.Marshal(struct {
		Operation, EventID string
		Command            RecordEvent
	}{operation, eventID, cmd})
	if err != nil {
		return domain.AssetEvent{}, err
	}
	digest := sha256.Sum256(payload)
	request := LifecycleRequest{TenantID: actor.TenantID, UserID: actor.UserID, Key: key, Hash: hex.EncodeToString(digest[:])}
	return s.store.WithLifecycleWrite(ctx, actor.TenantID, func(store LifecycleStore) (domain.AssetEvent, error) {
		if key != "" {
			previous, found, err := store.FindLifecycleRequest(ctx, actor.TenantID, actor.UserID, key)
			if err != nil {
				return domain.AssetEvent{}, err
			}
			if found {
				if previous.Hash != request.Hash {
					return domain.AssetEvent{}, NewInputError("validation.request_conflict")
				}
				return store.GetAssetEvent(ctx, actor.TenantID, previous.EventID)
			}
		}
		event, err := fn(&LifecycleService{store: store, now: s.now})
		if err != nil {
			return domain.AssetEvent{}, err
		}
		if key != "" {
			request.EventID = event.ID
			if err := store.SaveLifecycleRequest(ctx, request); err != nil {
				return domain.AssetEvent{}, err
			}
		}
		return event, nil
	})
}
