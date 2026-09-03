package web

import (
	"errors"
	"fmt"
	"strings"

	"github.com/SampsonFox/assetloop/internal/application"
	"golang.org/x/text/language"
)

const localeCookie = "assetloop_locale"

var localeMatcher = language.NewMatcher([]language.Tag{
	language.MustParse(string(application.LocaleZhCN)),
	language.English,
})

var messages = map[application.Locale]map[string]string{
	application.LocaleZhCN: {
		"app.name": "AssetLoop 物迹", "nav.main": "主导航", "nav.assets": "物品",
		"menu.label": "用户菜单", "menu.catalog": "物品类型管理", "menu.members": "成员管理",
		"menu.preferences": "界面偏好", "menu.language": "语言", "menu.theme": "主题", "menu.logout": "退出登录", "menu.save": "保存偏好",
		"locale.zh-CN": "简体中文", "locale.en": "English", "theme.system": "跟随系统", "theme.light": "浅色", "theme.dark": "深色",
		"role.owner": "Owner", "role.editor": "Editor", "role.viewer": "Viewer",
		"common.save": "保存", "common.cancel": "取消", "common.close": "关闭", "common.edit": "编辑", "common.select": "请选择",
		"common.back": "返回", "common.back_assets": "返回物品列表", "common.notes": "备注", "common.none": "暂无", "common.actions": "操作", "common.delete": "删除",
		"common.skip_main": "跳到主要内容", "common.discard_changes": "尚有未保存的更改，确定放弃吗？", "common.saving": "正在保存…",
		"table.sort": "排序字段", "table.direction": "排序方向", "table.ascending": "升序", "table.descending": "降序", "table.created": "创建时间",
		"title.assets": "我的物品", "title.overview": "概览", "title.catalog": "物品类型配置",
		"title.members": "成员", "title.login": "登录", "title.setup": "初始化", "title.error": "错误", "title.forbidden": "无权限",
		"login.eyebrow": "欢迎回来", "login.heading": "登录 AssetLoop", "login.username": "用户名", "login.password": "密码", "login.submit": "登录",
		"setup.eyebrow": "首次启动", "setup.heading": "创建你的物迹空间", "setup.help": "第一个账号会成为 Owner。本位币在第一条金额记录后锁定。",
		"setup.tenant_name": "空间名称", "setup.tenant_default": "我的物迹", "setup.base_currency": "本位币", "setup.owner_username": "Owner 用户名", "setup.password": "密码", "setup.submit": "完成初始化",
		"overview.heading": "你的物品，沿时间留下清楚的账。", "overview.help": "从类别、型号和规格建立具体物品；在具体物品下记录完整生命周期。",
		"overview.assets": "具体物品", "overview.view_assets": "查看物品列表", "overview.expense": "累计支出", "overview.expense_help": "有效买入与维修记录。",
		"overview.net": "净现金流", "overview.income": "收入", "overview.base_currency_help": "全部统计统一使用本位币。",
		"assets.count_suffix": "件具体物品", "assets.view_label": "显示方式", "assets.list": "列表", "assets.grid": "卡片", "assets.create": "录入物品",
		"assets.cards_label": "物品卡片", "assets.serial_number": "序列号", "assets.serial_missing": "尚未记录序列号", "assets.edit": "编辑物品", "assets.back_detail": "返回物品详情",
		"assets.item": "物品", "assets.taxonomy": "类别 / 型号 / 规格", "assets.cost": "累计成本", "assets.no_cost": "尚未录入", "assets.add_event": "新增事件", "assets.color": "颜色", "assets.channel": "渠道",
		"assets.search": "搜索", "assets.search_placeholder": "搜索名称、型号、规格、序列号…", "assets.status": "状态", "assets.all_statuses": "全部状态", "assets.filter": "筛选", "assets.clear_filters": "清除筛选",
		"assets.net_settlement": "最终结算", "assets.pagination": "物品分页", "assets.page": "第", "assets.previous": "上一页", "assets.next": "下一页", "assets.no_results_heading": "没有匹配的物品", "assets.no_results_help": "换个关键词或状态条件试试。",
		"assets.empty_heading": "还没有物品", "assets.empty_help_editor": "录入第一件具体物品，之后就可以添加买入、维修和卖出记录。",
		"assets.empty_help_viewer": "当前还没有可查看的物品。", "assets.create_first": "录入第一个物品", "assets.concrete": "具体物品",
		"assets.variant": "所属规格", "assets.add_type": "新增物品类型", "assets.display_name": "显示名称", "assets.display_placeholder": "我的主力手机",
		"assets.purchase_channel": "购买渠道", "assets.no_types": "当前还没有物品类型", "assets.no_types_help": "直接在这里新增类别、型号和规格，完成后会回到当前录入表单。",
		"catalog.eyebrow": "管理", "catalog.help": "以型号为主记录；类别标识型号归属，规格直接挂在型号下面。", "catalog.add_model": "新增型号",
		"catalog.edit_category": "编辑类别", "catalog.edit_model": "编辑型号", "catalog.variant": "规格", "catalog.variant_help": "容量等影响二手价格的规格分别维护。",
		"catalog.add_variant": "新增规格", "catalog.edit_variant": "编辑规格", "catalog.empty_heading": "还没有型号", "catalog.empty_help": "新增第一个型号，并为它选择或新增所属类别。",
		"catalog.add_first_model": "新增第一个型号", "catalog.category_add": "新增类别", "catalog.category_name": "类别名称", "catalog.category_placeholder": "手机", "catalog.icon": "图标",
		"catalog.model_category": "所属类别", "catalog.model_name": "型号名称", "catalog.model_placeholder": "iPhone 17 Pro", "catalog.variant_model": "所属型号", "catalog.variant_name": "规格名称",
		"catalog.search": "搜索型号", "catalog.search_placeholder": "搜索型号或类别…", "catalog.all_categories": "全部类别", "catalog.pagination": "型号分页", "catalog.count_suffix": "个型号", "catalog.no_results_heading": "没有匹配的型号", "catalog.no_results_help": "换个关键词或类别条件试试。",
		"catalog.variant_list": "规格管理", "catalog.variant_empty": "暂无规格", "catalog.variant_empty_help": "当前型号还没有规格。", "catalog.delete_variant_confirm": "确定删除这个规格吗？", "validation.variant_in_use": "该规格已被具体物品使用，不能删除。",
		"icon.package": "其他", "icon.smartphone": "手机", "icon.laptop": "电脑", "icon.tablet": "平板", "icon.monitor": "显示器", "icon.headphones": "耳机", "icon.camera": "相机",
		"icon.watch": "手表", "icon.gamepad-2": "游戏设备", "icon.car": "汽车", "icon.bike": "自行车", "icon.home": "家居", "icon.book": "书籍", "icon.wrench": "工具",
		"members.eyebrow": "租户安全", "members.heading": "成员与角色", "members.username": "用户名", "members.role": "角色", "members.joined": "加入时间",
		"members.add": "新增成员", "members.initial_password": "初始密码", "members.create": "创建成员", "members.search": "搜索成员", "members.search_placeholder": "搜索用户名…", "members.all_roles": "全部角色", "members.no_results": "没有匹配的成员", "members.pagination": "成员分页",
		"asset.taxonomy": "归属规格", "asset.category": "类别", "asset.model": "型号", "asset.variant": "规格", "asset.no_notes": "暂无备注",
		"asset.income": "累计收入", "asset.income_help": "有效卖出记录。", "asset.base_locked": "本位币已锁定", "asset.base_locks_after_first": "首笔金额后锁定本位币",
		"asset.timeline": "生命周期时间线", "asset.time": "时间", "asset.event": "事件", "asset.base_amount": "本位币金额", "asset.fx_evidence": "原币与汇率证据",
		"asset.search_events": "搜索记录", "asset.search_events_placeholder": "搜索备注或汇率来源…", "asset.all_event_types": "全部事件类型", "asset.pagination": "生命周期记录分页", "asset.cashflow_summary": "生命周期现金流汇总",
		"asset.voided": "已作废", "asset.correct": "更正", "asset.no_events": "尚无生命周期记录。", "asset.add_event": "新增生命周期记录", "asset.add_event_short": "新增记录",
		"asset.event_type": "事件类型", "asset.event_type_add": "新增类型", "asset.event_type_create": "新增事件类型", "asset.event_type_name": "类型名称", "asset.event_type_placeholder": "例如：保养", "asset.event_cashflow": "金额影响", "asset.event_cashflow_help": "决定这类记录如何计入生命周期现金流。", "asset.amount_zero_help": "无金额类型会以 0 记录，不锁定本位币。", "asset.occurred_at": "发生时间", "asset.amount_positive": "金额（正数）", "asset.original_currency": "原始货币",
		"asset.source": "交易来源", "asset.reference": "订单/凭证号", "asset.fx_rate": "汇率（1 原币 = 本位币）", "asset.fx_date": "汇率日期", "asset.fx_source": "汇率来源",
		"asset.fx_required": "仅非本位币必填", "asset.fx_confirm": "我确认使用上述汇率换算为", "asset.append": "追加记录", "asset.model_image_caption": "型号示意图；具体颜色以物品记录为准。",
		"event.purchase": "买入", "event.repair": "维修", "event.sale": "卖出", "event.void": "作废",
		"cashflow.expense": "支出", "cashflow.income": "收入", "cashflow.neutral": "无金额",
		"status.active": "持有中", "status.repairing": "维修中", "status.sold": "已卖出", "status.unacquired": "未入账",
		"correct.eyebrow": "追加式更正", "correct.heading": "更正记录", "correct.back": "返回物品", "correct.help": "原记录不会被覆盖。确认后系统会原子地追加一条作废记录和一条替代记录。",
		"correct.original_amount": "原金额", "correct.amount": "正确金额（正数）", "correct.fx_rate": "汇率", "correct.fx_confirm": "我确认使用上述汇率", "correct.source": "来源", "correct.notes": "更正说明", "correct.submit": "作废并替代",
		"error.eyebrow": "请求未完成", "error.internal": "系统暂时无法完成请求，请稍后重试。", "error.not_found_asset": "物品不存在", "error.not_found_event": "可更正记录不存在",
		"error.forbidden_catalog": "当前角色不能维护物品类型配置", "error.forbidden_lifecycle": "当前角色不能修改生命周期记录", "error.forbidden_correct": "当前角色不能更正生命周期记录",
		"error.forbidden_asset": "当前角色不能修改具体物品", "error.forbidden_members": "只有 Owner 可以管理成员",
		"error.csrf": "安全校验失败，请刷新页面后重试。", "error.login": "用户名或密码错误", "error.login_rate": "登录尝试过多，请稍后再试。", "error.return_to": "返回地址无效。",
		"validation.tenant_name_required": "空间名称不能为空。", "validation.locale_invalid": "不支持该语言。", "validation.theme_invalid": "不支持该主题。", "validation.role_invalid": "角色必须是 Owner、Editor 或 Viewer。",
		"validation.password_length": "密码至少需要 12 个字符。", "validation.username_length": "用户名长度需要在 3 到 64 个字符之间。", "validation.username_characters": "用户名不能包含空白或控制字符。",
		"validation.currency_iso": "本位币必须是三位 ISO 货币代码。", "validation.datetime": "日期时间格式无效。", "validation.fx_date": "汇率日期格式无效。",
		"validation.amount_required": "金额不能为空。", "validation.amount_positive": "金额必须为正数。", "validation.amount_invalid": "金额格式无效。", "validation.currency_invalid": "货币必须是三位 ISO 代码。",
		"validation.fx_invalid": "汇率格式无效。", "validation.fx_positive": "汇率必须为正数。", "validation.fx_confirm": "必须确认汇率换算。", "validation.fx_evidence": "汇率日期和来源不能为空。",
		"validation.event_purchase_exists": "物品已经存在有效买入记录。", "validation.event_purchase_first": "维修或卖出前必须先记录买入。", "validation.event_after_sale": "已卖出的物品不能再添加维修或卖出记录。",
		"validation.occurred_required": "发生时间不能为空。", "validation.occurred_future": "发生时间不能晚于当前时间。", "validation.event_type": "请选择已有的事件类型。", "validation.event_type_exists": "这个事件类型已经存在。", "validation.event_cashflow": "金额影响必须是支出、收入或无金额。", "validation.amount_zero": "无金额事件的金额必须为 0。", "validation.input_invalid": "输入内容无效，请检查后重试。",
		"validation.category_icon": "不支持该类别图标。", "validation.id_invalid": "记录标识无效。", "validation.filter_invalid": "筛选条件无效。", "validation.field_required": "必填字段不能为空。", "validation.field_too_long": "输入内容超过长度限制。",
		"preferences.updated": "界面偏好已保存。",
	},
	application.LocaleEn: {
		"app.name": "AssetLoop", "nav.main": "Primary navigation", "nav.assets": "Assets",
		"menu.label": "User menu", "menu.catalog": "Asset type settings", "menu.members": "Members",
		"menu.preferences": "Interface preferences", "menu.language": "Language", "menu.theme": "Theme", "menu.logout": "Log out", "menu.save": "Save preferences",
		"locale.zh-CN": "简体中文", "locale.en": "English", "theme.system": "System", "theme.light": "Light", "theme.dark": "Dark",
		"role.owner": "Owner", "role.editor": "Editor", "role.viewer": "Viewer",
		"common.save": "Save", "common.cancel": "Cancel", "common.close": "Close", "common.edit": "Edit", "common.select": "Select",
		"common.back": "Back", "common.back_assets": "Back to assets", "common.notes": "Notes", "common.none": "None", "common.actions": "Actions", "common.delete": "Delete",
		"common.skip_main": "Skip to main content", "common.discard_changes": "You have unsaved changes. Discard them?", "common.saving": "Saving…",
		"table.sort": "Sort by", "table.direction": "Direction", "table.ascending": "Ascending", "table.descending": "Descending", "table.created": "Created",
		"title.assets": "My assets", "title.overview": "Overview", "title.catalog": "Asset type settings",
		"title.members": "Members", "title.login": "Log in", "title.setup": "Setup", "title.error": "Error", "title.forbidden": "Access denied",
		"login.eyebrow": "Welcome back", "login.heading": "Log in to AssetLoop", "login.username": "Username", "login.password": "Password", "login.submit": "Log in",
		"setup.eyebrow": "First launch", "setup.heading": "Create your AssetLoop workspace", "setup.help": "The first account becomes the Owner. The base currency locks after the first monetary record.",
		"setup.tenant_name": "Workspace name", "setup.tenant_default": "My AssetLoop", "setup.base_currency": "Base currency", "setup.owner_username": "Owner username", "setup.password": "Password", "setup.submit": "Complete setup",
		"overview.heading": "A clear record of every asset over time.", "overview.help": "Create assets from categories, models, and specifications, then record their complete lifecycle.",
		"overview.assets": "Assets", "overview.view_assets": "View assets", "overview.expense": "Total expense", "overview.expense_help": "Valid purchase and repair records.",
		"overview.net": "Net cash flow", "overview.income": "Income", "overview.base_currency_help": "All statistics use the base currency.",
		"assets.count_suffix": "assets", "assets.view_label": "Display", "assets.list": "List", "assets.grid": "Cards", "assets.create": "Add asset",
		"assets.cards_label": "Asset cards", "assets.serial_number": "Serial number", "assets.serial_missing": "No serial number", "assets.edit": "Edit asset", "assets.back_detail": "Back to asset details",
		"assets.item": "Asset", "assets.taxonomy": "Category / model / specification", "assets.cost": "Total cost", "assets.no_cost": "Not recorded", "assets.add_event": "Add event", "assets.color": "Color", "assets.channel": "Channel",
		"assets.search": "Search", "assets.search_placeholder": "Search name, model, specification, serial…", "assets.status": "Status", "assets.all_statuses": "All statuses", "assets.filter": "Filter", "assets.clear_filters": "Clear filters",
		"assets.net_settlement": "Net settlement", "assets.pagination": "Asset pagination", "assets.page": "Page", "assets.previous": "Previous", "assets.next": "Next", "assets.no_results_heading": "No matching assets", "assets.no_results_help": "Try another keyword or status.",
		"assets.empty_heading": "No assets yet", "assets.empty_help_editor": "Add the first asset, then record its purchases, repairs, and sales.",
		"assets.empty_help_viewer": "There are no assets to view yet.", "assets.create_first": "Add first asset", "assets.concrete": "Asset",
		"assets.variant": "Specification", "assets.add_type": "Add asset type", "assets.display_name": "Display name", "assets.display_placeholder": "My primary phone",
		"assets.purchase_channel": "Purchase channel", "assets.no_types": "No asset types yet", "assets.no_types_help": "Add a category, model, and specification here, then return to this form.",
		"catalog.eyebrow": "Admin", "catalog.help": "Models are the primary records; categories group models, and specifications belong to models.", "catalog.add_model": "Add model",
		"catalog.edit_category": "Edit category", "catalog.edit_model": "Edit model", "catalog.variant": "Specifications", "catalog.variant_help": "Maintain capacity and other specifications that affect resale value separately.",
		"catalog.add_variant": "Add specification", "catalog.edit_variant": "Edit specification", "catalog.empty_heading": "No models yet", "catalog.empty_help": "Add the first model and select or create its category.",
		"catalog.add_first_model": "Add first model", "catalog.category_add": "Add category", "catalog.category_name": "Category name", "catalog.category_placeholder": "Phone", "catalog.icon": "Icon",
		"catalog.model_category": "Category", "catalog.model_name": "Model name", "catalog.model_placeholder": "iPhone 17 Pro", "catalog.variant_model": "Model", "catalog.variant_name": "Specification name",
		"catalog.search": "Search models", "catalog.search_placeholder": "Search model or category…", "catalog.all_categories": "All categories", "catalog.pagination": "Model pagination", "catalog.count_suffix": "models", "catalog.no_results_heading": "No matching models", "catalog.no_results_help": "Try another keyword or category.",
		"catalog.variant_list": "Manage specifications", "catalog.variant_empty": "No specifications", "catalog.variant_empty_help": "This model has no specifications yet.", "catalog.delete_variant_confirm": "Delete this specification?", "validation.variant_in_use": "This specification is used by an asset and cannot be deleted.",
		"icon.package": "Other", "icon.smartphone": "Phone", "icon.laptop": "Computer", "icon.tablet": "Tablet", "icon.monitor": "Monitor", "icon.headphones": "Headphones", "icon.camera": "Camera",
		"icon.watch": "Watch", "icon.gamepad-2": "Gaming", "icon.car": "Car", "icon.bike": "Bicycle", "icon.home": "Home", "icon.book": "Books", "icon.wrench": "Tools",
		"members.eyebrow": "Tenant security", "members.heading": "Members and roles", "members.username": "Username", "members.role": "Role", "members.joined": "Joined",
		"members.add": "Add member", "members.initial_password": "Initial password", "members.create": "Create member", "members.search": "Search members", "members.search_placeholder": "Search username…", "members.all_roles": "All roles", "members.no_results": "No matching members", "members.pagination": "Member pagination",
		"asset.taxonomy": "Specification", "asset.category": "Category", "asset.model": "Model", "asset.variant": "Specification", "asset.no_notes": "No notes",
		"asset.income": "Total income", "asset.income_help": "Valid sale records.", "asset.base_locked": "Base currency is locked", "asset.base_locks_after_first": "Base currency locks after the first amount",
		"asset.timeline": "Lifecycle timeline", "asset.time": "Time", "asset.event": "Event", "asset.base_amount": "Base-currency amount", "asset.fx_evidence": "Original currency and FX evidence",
		"asset.search_events": "Search records", "asset.search_events_placeholder": "Search notes or FX source…", "asset.all_event_types": "All event types", "asset.pagination": "Lifecycle record pagination", "asset.cashflow_summary": "Lifecycle cash flow summary",
		"asset.voided": "Voided", "asset.correct": "Correct", "asset.no_events": "No lifecycle records yet.", "asset.add_event": "Add lifecycle record", "asset.add_event_short": "Add record",
		"asset.event_type": "Event type", "asset.event_type_add": "Add type", "asset.event_type_create": "Add event type", "asset.event_type_name": "Type name", "asset.event_type_placeholder": "For example: Maintenance", "asset.event_cashflow": "Cash-flow effect", "asset.event_cashflow_help": "Controls how records of this type contribute to lifecycle cash flow.", "asset.amount_zero_help": "No-amount types are recorded as zero and do not lock the base currency.", "asset.occurred_at": "Occurred at", "asset.amount_positive": "Amount (positive)", "asset.original_currency": "Original currency",
		"asset.source": "Transaction source", "asset.reference": "Order / reference", "asset.fx_rate": "FX rate (1 original = base)", "asset.fx_date": "FX date", "asset.fx_source": "FX source",
		"asset.fx_required": "Required for foreign currency", "asset.fx_confirm": "I confirm conversion using this rate to", "asset.append": "Add record", "asset.model_image_caption": "Model reference image; the asset record is authoritative for color.",
		"event.purchase": "Purchase", "event.repair": "Repair", "event.sale": "Sale", "event.void": "Void",
		"cashflow.expense": "Expense", "cashflow.income": "Income", "cashflow.neutral": "No amount",
		"status.active": "Active", "status.repairing": "In repair", "status.sold": "Sold", "status.unacquired": "Not acquired",
		"correct.eyebrow": "Append-only correction", "correct.heading": "Correct record", "correct.back": "Back to asset", "correct.help": "The original record is preserved. Confirming appends a void record and a replacement atomically.",
		"correct.original_amount": "Original amount", "correct.amount": "Correct amount (positive)", "correct.fx_rate": "FX rate", "correct.fx_confirm": "I confirm this FX rate", "correct.source": "Source", "correct.notes": "Correction reason", "correct.submit": "Void and replace",
		"error.eyebrow": "Request not completed", "error.internal": "The system could not complete the request. Please try again.", "error.not_found_asset": "Asset not found", "error.not_found_event": "Correctable record not found",
		"error.forbidden_catalog": "Your role cannot manage asset types", "error.forbidden_lifecycle": "Your role cannot change lifecycle records", "error.forbidden_correct": "Your role cannot correct lifecycle records",
		"error.forbidden_asset": "Your role cannot edit assets", "error.forbidden_members": "Only an Owner can manage members",
		"error.csrf": "Security validation failed. Refresh the page and try again.", "error.login": "Invalid username or password", "error.login_rate": "Too many login attempts. Try again later.", "error.return_to": "Invalid return address.",
		"validation.tenant_name_required": "Workspace name is required.", "validation.locale_invalid": "That language is not supported.", "validation.theme_invalid": "That theme is not supported.", "validation.role_invalid": "Role must be Owner, Editor, or Viewer.",
		"validation.password_length": "Password must contain at least 12 characters.", "validation.username_length": "Username must contain 3 to 64 characters.", "validation.username_characters": "Username must not contain whitespace or control characters.",
		"validation.currency_iso": "Base currency must be a three-letter ISO code.", "validation.datetime": "Invalid date and time.", "validation.fx_date": "Invalid FX date.",
		"validation.amount_required": "Amount is required.", "validation.amount_positive": "Amount must be positive.", "validation.amount_invalid": "Invalid amount.", "validation.currency_invalid": "Currency must be a three-letter ISO code.",
		"validation.fx_invalid": "Invalid FX rate.", "validation.fx_positive": "FX rate must be positive.", "validation.fx_confirm": "FX conversion must be confirmed.", "validation.fx_evidence": "FX rate date and source are required.",
		"validation.event_purchase_exists": "The asset already has an active purchase record.", "validation.event_purchase_first": "The asset must be purchased before a repair or sale.", "validation.event_after_sale": "A sold asset cannot receive another repair or sale record.",
		"validation.occurred_required": "Occurred time is required.", "validation.occurred_future": "Occurred time cannot be in the future.", "validation.event_type": "Select an existing event type.", "validation.event_type_exists": "That event type already exists.", "validation.event_cashflow": "Cash-flow effect must be expense, income, or no amount.", "validation.amount_zero": "A no-amount event must have an amount of zero.", "validation.input_invalid": "Invalid input. Check the form and try again.",
		"validation.category_icon": "Unsupported category icon.", "validation.id_invalid": "Invalid record identifier.", "validation.filter_invalid": "Invalid filter.", "validation.field_required": "A required field is empty.", "validation.field_too_long": "Input exceeds the length limit.",
		"preferences.updated": "Interface preferences saved.",
	},
}

func supportedLocale(value string) (application.Locale, bool) {
	locale := application.Locale(strings.TrimSpace(value))
	_, ok := messages[locale]
	return locale, ok
}

func matchLocale(acceptLanguage string) application.Locale {
	_, index := language.MatchStrings(localeMatcher, acceptLanguage)
	if index == 1 {
		return application.LocaleEn
	}
	return application.LocaleZhCN
}

func stringsFor(locale application.Locale) map[string]string {
	if pack, ok := messages[locale]; ok {
		return pack
	}
	return messages[application.LocaleZhCN]
}

func textFor(locale application.Locale, key string, args ...any) string {
	value, ok := stringsFor(locale)[key]
	if !ok {
		value = messages[application.LocaleZhCN][key]
	}
	if value == "" {
		value = key
	}
	if len(args) > 0 && strings.Contains(value, "%") {
		return fmt.Sprintf(value, args...)
	}
	return value
}

func inputErrorText(locale application.Locale, err error) (string, bool) {
	var input application.InputError
	if !errors.As(err, &input) {
		return "", false
	}
	return textFor(locale, input.Code, input.Args...), true
}
