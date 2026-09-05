package web

import "github.com/SampsonFox/assetloop/internal/application"

func init() {
	for locale, values := range map[application.Locale]map[string]string{
		application.LocaleZhCN: {
			"resource.preview": "预览", "resource.current": "当前绑定",
			"resource.source_asset": "具体物品专用", "resource.source_variant": "继承自规格", "resource.source_model": "继承自型号",
			"resource.loaded": "3D 模型已载入", "resource.viewer_hint": "拖动或方向键旋转 · +/− 缩放 · Home 重置",
			"resource.size": "大小", "resource.direct": "当前层级绑定", "resource.inherited": "继承上级资源", "resource.target_not_found": "未找到要设置的型号、规格或物品。",
			"resource.title": "3D 资源库", "resource.help": "上传一次，在型号、规格和具体物品之间重复使用。",
			"resource.name": "资源名称", "resource.search": "搜索名称、作者或许可证", "resource.pagination": "3D 资源分页",
			"resource.binding": "选择绑定资源", "resource.precedence": "具体物品优先于规格，规格优先于型号。解除绑定后使用上一级资源；资源文件仍保留在库中。",
			"resource.bind": "绑定此资源", "resource.unbind": "解除当前层级绑定", "resource.manage_binding": "设置 3D 资源",
			"resource.active": "可用", "resource.pending": "等待删除", "resource.preview_edit": "预览与编辑",
			"resource.empty": "没有匹配的 3D 资源", "resource.empty_help": "调整搜索条件，或上传一个自包含 GLB 文件。",
			"resource.upload": "上传资源", "resource.upload_bind": "上传并绑定", "resource.upload_help": "仅支持自包含 GLB，最大 25 MiB。上传失败后需重新选择文件。",
			"resource.back": "返回资源库", "resource.edit_help": "名称和署名更新会应用于所有引用。文件不可替换；需要新文件时请上传新资源。",
			"resource.references": "引用位置", "resource.no_references": "当前没有引用，可以删除。", "resource.referenced_help": "请先解除这些绑定，再删除资源。",
			"resource.delete_confirm": "确定永久删除这个未被引用的资源和文件吗？", "resource.retry_delete": "重试删除",
			"resource.pending_help":  "资源正在等待文件清理，暂不可预览或绑定。可以重试删除。",
			"resource.delete_failed": "删除未完成。若仍有引用，请先解除绑定；若文件清理失败，请重试删除。",
			"resource.unavailable":   "3D 文件存储尚未配置。", "resource.not_found": "未找到这个 3D 资源。",
			"resource.upload_invalid": "上传未完成。请选择不超过 25 MiB 的自包含 GLB 文件后重试。", "resource.file_required": "请选择一个 GLB 文件。",
			"resource.invalid":            "操作未完成。请检查资源名称、来源网址及 GLB 文件，并确认资源仍可用后重试。",
			"resource.preview_error":      "无法显示 3D 预览。请刷新页面重试；资源记录仍可管理。",
		},
		application.LocaleEn: {
			"resource.preview": "Preview", "resource.current": "Currently bound",
			"resource.source_asset": "Dedicated to this asset", "resource.source_variant": "Inherited from specification", "resource.source_model": "Inherited from model",
			"resource.loaded": "3D model loaded", "resource.viewer_hint": "Drag or arrows to rotate · +/− zoom · Home resets",
			"resource.size": "Size", "resource.direct": "Bound at this level", "resource.inherited": "Inherited resource", "resource.target_not_found": "The model, specification, or asset was not found.",
			"resource.title": "3D resource library", "resource.help": "Upload once and reuse across models, specifications, and assets.",
			"resource.name": "Resource name", "resource.search": "Search name, author, or license", "resource.pagination": "3D resource pages",
			"resource.binding": "Choose a resource", "resource.precedence": "Asset overrides specification; specification overrides model. Unbinding restores the inherited resource and keeps the file in the library.",
			"resource.bind": "Bind this resource", "resource.unbind": "Unbind at this level", "resource.manage_binding": "Set 3D resource",
			"resource.active": "Available", "resource.pending": "Pending deletion", "resource.preview_edit": "Preview and edit",
			"resource.empty": "No matching 3D resources", "resource.empty_help": "Adjust your search or upload a self-contained GLB file.",
			"resource.upload": "Upload resource", "resource.upload_bind": "Upload and bind", "resource.upload_help": "Self-contained GLB only, up to 25 MiB. Select the file again after a failed upload.",
			"resource.back": "Back to library", "resource.edit_help": "Name and attribution updates apply to every reference. Files cannot be replaced; upload a new resource for a different file.",
			"resource.references": "References", "resource.no_references": "No references. This resource can be deleted.", "resource.referenced_help": "Unbind these references before deleting the resource.",
			"resource.delete_confirm": "Permanently delete this unreferenced resource and its file?", "resource.retry_delete": "Retry deletion",
			"resource.pending_help":  "File cleanup is pending. Preview and binding are unavailable. You can retry deletion.",
			"resource.delete_failed": "Deletion did not finish. Unbind any remaining references, or retry deletion if file cleanup failed.",
			"resource.unavailable":   "3D file storage is not configured.", "resource.not_found": "This 3D resource was not found.",
			"resource.upload_invalid": "Upload did not finish. Choose a self-contained GLB up to 25 MiB and retry.", "resource.file_required": "Choose a GLB file.",
			"resource.invalid":            "The operation did not finish. Check the name, source URL, and GLB file, confirm the resource is available, and retry.",
			"resource.preview_error":      "3D preview is unavailable. Refresh to retry; you can still manage this resource.",
		},
	} {
		for key, value := range values {
			messages[locale][key] = value
		}
	}
}
