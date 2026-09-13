package application

import (
	"context"
	"strings"

	"github.com/SampsonFox/assetloop/internal/domain"
)

// assetTagSummaryStore is the optional bulk projection that resolves the joined
// specification label of many assets in one read. Adapters that cannot provide it
// keep the model name, so the fallback stays correct for every Store.
type assetTagSummaryStore interface {
	AssetTagSummaries(context.Context, string, []string) (map[string]string, error)
}

func assetTagSummaries(ctx context.Context, store LifecycleStore, tenantID string, assetIDs []string) (map[string]string, error) {
	provider, ok := store.(assetTagSummaryStore)
	if !ok || len(assetIDs) == 0 {
		return map[string]string{}, nil
	}
	unique := make([]string, 0, len(assetIDs))
	seen := map[string]bool{}
	for _, id := range assetIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return map[string]string{}, nil
	}
	return provider.AssetTagSummaries(ctx, tenantID, unique)
}

// relatedAssetName is the minimal correct display label: the asset's own name, or
// its model when no name was given.
func relatedAssetName(asset domain.Asset) string {
	if name := strings.TrimSpace(asset.DisplayName); name != "" {
		return name
	}
	return strings.TrimSpace(asset.Model)
}

// relatedAssetSpecification prefers the asset's current tag summary and falls back
// to the model name when no tag describes it.
func relatedAssetSpecification(asset domain.Asset, summaries map[string]string) string {
	if summary := strings.TrimSpace(summaries[asset.ID]); summary != "" {
		return summary
	}
	return strings.TrimSpace(asset.Model)
}

// TagSummaries resolves the joined specification labels of many assets from one
// snapshot, in the same tag order the normal asset read path hydrates. Adapters
// use it to answer the optional AssetTagSummaries bulk projection.
func TagSummaries(state SpecificationSnapshot, assetIDs []string) map[string]string {
	wanted := map[string]bool{}
	for _, id := range assetIDs {
		if id != "" {
			wanted[id] = true
		}
	}
	selected := map[string]map[string]bool{}
	for _, link := range state.Links {
		if link.Kind != "asset" || !wanted[link.TargetID] {
			continue
		}
		if selected[link.TargetID] == nil {
			selected[link.TargetID] = map[string]bool{}
		}
		selected[link.TargetID][link.TagID] = true
	}
	result := map[string]string{}
	for id, tags := range selected {
		labels := make([]string, 0, len(tags))
		for _, tag := range state.Tags {
			if tags[tag.ID] {
				labels = append(labels, tag.Name)
			}
		}
		result[id] = strings.Join(labels, " · ")
	}
	return result
}
