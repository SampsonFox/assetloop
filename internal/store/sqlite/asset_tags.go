package sqlite

import (
	"context"
	"strings"

	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
)

// AssetTagSummaries implements the optional bulk projection used to snapshot and
// read a related asset's specification label in one snapshot read.
func (s *Store) AssetTagSummaries(ctx context.Context, tenantID string, assetIDs []string) (map[string]string, error) {
	state, err := s.SpecificationSnapshot(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return application.TagSummaries(state, assetIDs), nil
}

// decorateRelatedSpecification replaces the model-name fallback with the current
// tag summary while the related asset still exists. A purged target keeps its
// write-time snapshot.
func (s *Store) decorateRelatedSpecification(ctx context.Context, tenantID string, events []domain.AssetEvent) error {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		if event.RelatedAssetID != "" && !event.RelatedAssetDeleted {
			ids = append(ids, event.RelatedAssetID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	summaries, err := s.AssetTagSummaries(ctx, tenantID, ids)
	if err != nil {
		return err
	}
	for index := range events {
		if events[index].RelatedAssetID == "" || events[index].RelatedAssetDeleted {
			continue
		}
		if summary := strings.TrimSpace(summaries[events[index].RelatedAssetID]); summary != "" {
			events[index].RelatedAssetSpec = summary
		}
	}
	return nil
}
