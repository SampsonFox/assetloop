package web

import (
	"fmt"
	"github.com/SampsonFox/assetloop/internal/domain"
	"math/big"
	"strings"
	"time"
)

// Integer ratios are used only to project already-calculated amounts into SVG coordinates.
func costRatio(value, low, high int64, size int64) int64 {
	span := new(big.Int).Sub(big.NewInt(high), big.NewInt(low))
	if span.Sign() == 0 {
		return size / 2
	}
	v := new(big.Int).Sub(big.NewInt(value), big.NewInt(low))
	v.Mul(v, big.NewInt(size))
	v.Quo(v, span)
	return v.Int64()
}

type costChartView struct {
	Path       string
	ZeroY      int64
	Min, Max   int64
	Start, End time.Time
	Points     []costChartPoint
}

type costChartPoint struct {
	X, Y  int64
	Point domain.CostPoint
}

func costChart(d domain.CostDashboard) costChartView {
	c := costChartView{}
	if len(d.Points) == 0 {
		return c
	}
	for _, p := range d.Points {
		if p.NetMinor < c.Min {
			c.Min = p.NetMinor
		}
		if p.NetMinor > c.Max {
			c.Max = p.NetMinor
		}
	}
	y := func(v int64) int64 { return 180 - costRatio(v, c.Min, c.Max, 160) }
	c.ZeroY = y(0)
	first, last := d.Points[0].At.Unix(), d.Points[len(d.Points)-1].At.Unix()
	c.Start, c.End = d.Points[0].At, d.Points[len(d.Points)-1].At
	var path strings.Builder
	fmt.Fprintf(&path, "M 20 %d", y(0))
	for _, p := range d.Points {
		x := 20 + costRatio(p.At.Unix(), first, last, 600)
		fmt.Fprintf(&path, " H %d V %d", x, y(p.NetMinor))
		c.Points = append(c.Points, costChartPoint{x, y(p.NetMinor), p})
	}
	c.Path = path.String()
	return c
}

func costPercent(tenths int64) string { return fmt.Sprintf("%d.%d", tenths/10, tenths%10) }
