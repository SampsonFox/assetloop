package application

import (
	"context"
	"github.com/SampsonFox/assetloop/internal/domain"
	"time"
)

// CostDashboard intentionally ignores the timeline's search, sort and page.
func (s *LifecycleService) CostDashboard(ctx context.Context, actor Principal, assetID string) (domain.CostDashboard, error) {
	events, summary, err := s.Timeline(ctx, actor, assetID)
	if err != nil {
		return domain.CostDashboard{}, err
	}
	return domain.CalculateCostDashboard(events, summary.BaseCurrency, s.now(), time.Local)
}
