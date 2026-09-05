package web

import (
	"github.com/SampsonFox/assetloop/internal/domain"
	"math"
	"strings"
	"testing"
	"time"
)

func TestCostChartProjection(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := costChart(domain.CostDashboard{Points: []domain.CostPoint{{At: start, NetMinor: 100}, {At: start.AddDate(0, 0, 1), NetMinor: 150}, {At: start.AddDate(0, 0, 2), NetMinor: -20}}})
	if len(c.Points) != 3 || c.Min != -20 || c.Max != 150 || !strings.Contains(c.Path, " H 320 V ") || c.Points[0].X != 20 || c.Points[2].X != 620 {
		t.Fatalf("projection: %+v", c)
	}
	if strings.ContainsAny(c.Path, "CQ") {
		t.Fatal("cost trend must not interpolate curves")
	}
	if costRatio(math.MaxInt64, math.MinInt64, math.MaxInt64, 600) != 600 {
		t.Fatal("extreme projection overflow")
	}
	one := costChart(domain.CostDashboard{Points: []domain.CostPoint{{At: start, NetMinor: 100}}})
	if len(one.Points) != 1 || one.Points[0].X != 320 {
		t.Fatal("single observation must remain a visible point")
	}
	if costChart(domain.CostDashboard{}).Path != "" {
		t.Fatal("empty chart must not invent points")
	}
}
