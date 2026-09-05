package web

import "github.com/SampsonFox/assetloop/internal/application"

func init() {
	for locale, values := range map[application.Locale]map[string]string{
		application.LocaleZhCN: {
			"cost.title": "持有成本", "cost.daily": "日均持有成本", "cost.per_day": "/ 天", "cost.net": "净成本", "cost.days": "持有天数", "cost.day_unit": "天",
			"cost.holding": "截至今日 · 未扣除二手残值", "cost.sold": "已卖出 · 天数截止卖出日", "cost.unknown": "缺少有效日期，暂无法计算日均", "cost.gain": "回收超过支出",
			"cost.trend": "累计净成本", "cost.trend_help": "支出增加成本，收入降低成本；此图不是二手估值。", "cost.data": "查看明细数据", "cost.breakdown": "支出构成", "cost.share": "占比", "cost.empty": "还没有金额记录", "cost.details": "记录详情",
		},
		application.LocaleEn: {
			"cost.title": "Ownership cost", "cost.daily": "Daily ownership cost", "cost.per_day": "/ day", "cost.net": "Net cost", "cost.days": "Days owned", "cost.day_unit": "days",
			"cost.holding": "To date · resale value not deducted", "cost.sold": "Sold · days stop at sale", "cost.unknown": "Valid dates needed to calculate daily cost", "cost.gain": "Recovery exceeds expenses",
			"cost.trend": "Cumulative net cost", "cost.trend_help": "Expenses increase cost; income reduces it. This is not a market valuation.", "cost.data": "View underlying data", "cost.breakdown": "Expense breakdown", "cost.share": "Share", "cost.empty": "No monetary records yet", "cost.details": "Event details",
		},
	} {
		for key, value := range values {
			messages[locale][key] = value
		}
	}
}
