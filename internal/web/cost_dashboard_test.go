package web

import (
	"bytes"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/domain"
	"math"
	"strings"
	"testing"
	"time"
)

func TestCostReportAdaptsToObservationCount(t *testing.T) {
	s, err := New(nil, nil, nil, nil, Options{Specifications: application.NewSpecificationService(nil)})
	if err != nil {
		t.Fatal(err)
	}
	for _, count := range []int{0, 1, 3, 5, 6} {
		cost := domain.CostDashboard{Currency: "CNY"}
		for i := 0; i < count; i++ {
			cost.Points = append(cost.Points, domain.CostPoint{At: time.Date(2026, 9, 1+i, 0, 0, 0, 0, time.UTC), Type: domain.AssetEventType("purchase"), NetMinor: -40000})
		}
		var out bytes.Buffer
		if err := s.templates["asset"].ExecuteTemplate(&out, "cost_dashboard", pageData{Strings: stringsFor(application.LocaleEn), Cost: cost}); err != nil {
			t.Fatal(err)
		}
		html := out.String()
		compact := count > 0 && count <= 5
		if strings.Contains(html, `class="cost-milestones"`) != compact || strings.Contains(html, `<svg class="cost-chart"`) != (count > 5) {
			t.Fatalf("wrong layout for %d observations", count)
		}
		if compact && (strings.Count(html, `class="cost-milestone"`) != count || !strings.Contains(html, "-400.00 CNY") || strings.Contains(html, `class="cost-data"`)) {
			t.Fatalf("sparse report lost values or duplicated data: %s", html)
		}
	}
}

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
