package web

import "github.com/SampsonFox/assetloop/internal/application"

func init() {
	messages[application.LocaleZhCN]["image.download_failed"] = "服务器未能下载图片。请检查公开 HTTPS 直链及服务器网络；浏览器能打开但导入失败时，可下载后上传文件。"
	messages[application.LocaleEn]["image.download_failed"] = "The server could not download the image. Check the public HTTPS URL and server network, or download it in your browser and upload the file."
	messages[application.LocaleZhCN]["image.import"] = "从链接导入"
	messages[application.LocaleZhCN]["image.import_help"] = "由服务器下载公开 HTTPS 图片直链，不支持网页地址、登录或内网链接。请确认你有权使用这张图片。"
	messages[application.LocaleZhCN]["image.url"] = "图片直链"
	messages[application.LocaleEn]["image.import"] = "Import from URL"
	messages[application.LocaleEn]["image.import_help"] = "The server downloads a public HTTPS image URL, not a webpage. Login and private-network URLs are unsupported. Ensure you may use the image."
	messages[application.LocaleEn]["image.url"] = "Direct image URL"
	for locale, values := range map[application.Locale]map[string]string{
		application.LocaleZhCN: {"image.title": "型号图片", "image.help": "同型号物品共用这张图片；没有 3D 或加载失败时显示。颜色仅供参考。", "image.file": "选择图片", "image.limit": "PNG、JPEG 或 WebP，最大 8 MiB、1600 万像素。上传失败后请重新选择文件。", "image.source": "来源网页（可选）", "image.empty": "还没有型号图片", "image.clear": "移除图片", "image.clear_help": "移除仅解除展示关联，不删除历史文件，也不影响 3D。", "image.invalid": "未能保存，请检查图片格式、大小和来源网址后重试。", "image.unavailable": "图片暂时不可用，请稍后重试。", "image.invalid_source": "请输入不含登录凭证的 HTTP 或 HTTPS 来源网址。"},
		application.LocaleEn:   {"image.title": "Model image", "image.help": "Shared by this model’s assets; shown when 3D is absent or fails. Color is for reference.", "image.file": "Choose image", "image.limit": "PNG, JPEG or WebP, up to 8 MiB and 16 megapixels. Reselect the file after an upload error.", "image.source": "Source page (optional)", "image.empty": "No model image yet", "image.clear": "Remove image", "image.clear_help": "Removing detaches the image. Historical files and 3D remain unchanged.", "image.invalid": "Could not save. Check the image format, size and source URL, then retry.", "image.unavailable": "Image temporarily unavailable. Please retry.", "image.invalid_source": "Enter an HTTP or HTTPS source URL without login credentials."},
	} {
		for key, value := range values {
			messages[locale][key] = value
		}
	}
}
