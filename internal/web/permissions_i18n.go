package web

import "github.com/SampsonFox/assetloop/internal/application"

func init() {
	zh := map[string]string{
		"asset.delete": "删除物品", "asset.delete_permanent": "永久删除", "asset.delete_confirm": "我确认永久删除此物品及其全部生命周期记录。",
		"asset.delete_warning":          "此操作无法撤销。将清除物品资料、全部生命周期记录及物品专属关联，收支统计也会更新。",
		"asset.delete_shared":           "公共类型、型号、标签、型号图片、3D 资源库和共享行情会保留。",
		"validation.last_administrator": "必须保留至少一名管理员。请先将其他成员设为管理员。",
		"validation.asset_deleted":      "此物品已永久删除，原请求不能再次执行。",
	}
	en := map[string]string{
		"asset.delete": "Delete item", "asset.delete_permanent": "Permanently delete", "asset.delete_confirm": "I confirm permanent deletion of this item and all its lifecycle records.",
		"asset.delete_warning":          "This cannot be undone. Item details, all lifecycle records and item-owned links will be removed, and totals will change.",
		"asset.delete_shared":           "Shared types, models, tags, model images, 3D library resources and market history are retained.",
		"validation.last_administrator": "Keep at least one administrator. Promote another member first.",
		"validation.asset_deleted":      "This item was permanently deleted. The original request cannot be repeated.",
	}
	for k, v := range zh {
		messages[application.LocaleZhCN][k] = v
	}
	for k, v := range en {
		messages[application.LocaleEn][k] = v
	}
}
