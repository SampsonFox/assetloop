package web

import "github.com/SampsonFox/assetloop/internal/application"

func init() {
	for locale, values := range map[application.Locale]map[string]string{
		application.LocaleZhCN: {
			"types.detail": "类型详情", "types.view_selected": "查看／编辑所选类型",
			"types.title": "生命周期类型管理", "types.help": "类型名称由所有引用记录共享。停用不影响历史记录。", "types.search": "搜索类型名称", "types.status": "启用状态", "types.all": "全部状态", "types.enabled": "启用", "types.disabled": "已停用", "types.builtin": "内置", "types.custom": "自定义", "types.origin": "类型来源", "types.references": "引用记录", "types.stop": "停用", "types.restore": "恢复", "types.edit": "编辑类型", "types.empty": "没有符合条件的类型", "types.locked": "已有记录，收支方向不可修改。", "types.shared": "修改名称后，所有引用记录直接显示此名称。", "types.manage": "管理类型", "types.stop_confirm": "停用后不能用于新增记录，历史记录仍保留。确定停用？",
			"validation.event_type_disabled": "该类型已停用，请选择其他类型。", "validation.event_type_in_use": "该类型已有记录，不能修改收支方向。", "validation.event_type_builtin": "内置类型不可修改或停用。",
		},
		application.LocaleEn: {
			"types.detail": "Type details", "types.view_selected": "View/edit selected type",
			"types.title": "Lifecycle event types", "types.help": "Names are shared by all referencing events. Disabling a type preserves its history.", "types.search": "Search type names", "types.status": "Availability", "types.all": "All statuses", "types.enabled": "Enabled", "types.disabled": "Disabled", "types.builtin": "Built-in", "types.custom": "Custom", "types.origin": "Origin", "types.references": "References", "types.stop": "Disable", "types.restore": "Restore", "types.edit": "Edit type", "types.empty": "No matching types", "types.locked": "Used by existing events. Cash-flow direction is locked.", "types.shared": "All referencing events display this name directly.", "types.manage": "Manage types", "types.stop_confirm": "Disable for new events? Existing history will be retained.",
			"validation.event_type_disabled": "This type is disabled. Choose another type.", "validation.event_type_in_use": "This type has recorded events. Its cash-flow direction cannot change.", "validation.event_type_builtin": "Built-in types cannot be edited or disabled.",
		}} {
		for key, value := range values {
			messages[locale][key] = value
		}
	}
}
