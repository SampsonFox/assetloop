# AssetLoop / 物迹 项目计划

状态：基础工程已发布，进入认证、资产目录和生命周期纵向交付
最后更新：2026-09-01

本文只管理产品范围和实施阶段。非协商边界以 `../AGENTS.md` 为准，系统结构以 `ARCHITECTURE.md` 为准，当前路径导航以 `../CODEMAP.md` 为准。

## 1. 项目目标

构建一个自用优先、可开源、可平滑演进为 SaaS 的物品生命周期与持有成本系统。

用户通过 AI 对话提交订单、商品、价格或维修凭证截图。AI Harness 负责识别、补全和确认信息，通过语义化 MCP 工具写入系统。Web 页面负责人工复核、维护物品、查看二手行情、持有成本和生命周期记录。

系统应做到：

- 同一份业务代码支持本地单用户版和未来 SaaS。
- 默认使用 SQLite，同时正式适配 PostgreSQL。
- 默认将附件保存在本地，同时适配阿里云 OSS。
- OneBound 只是首个行情数据源，可被其他数据源替换或并存。
- 所有统计统一使用租户本位币，原始货币信息可追溯。
- 应用升级时自动、安全地升级旧数据结构，不破坏已有数据。

## 2. 已确认的产品边界

### 2.1 第一阶段包含

- 截图识别后的待确认入库流程。
- 物品分类、型号、规格、具体物品和生命周期管理。
- 买入、卖出、维修及其他资产事件。
- 原币、本位币和汇率留痕。
- OneBound 二手挂牌数据接入。
- 二手价格曲线、当前估值、持有成本和日均减值展示。
- Web 管理页面。
- 本地账号登录、租户成员和 Owner/Editor/Viewer 权限。
- 面向 AI Harness 的语义化 MCP 工具。
- SQLite/PostgreSQL、Local/OSS 的兼容层。
- 跨平台单二进制发布。

### 2.2 第一阶段不包含

- 应用内置大模型调用和模型 API Key 管理。
- 裸 SQL MCP 工具。
- Go `.so` 动态插件。
- 对任意数据库的名义兼容。
- 第三方数据源插件市场。
- 复杂会计总账或企业 ERP 功能。

## 3. 总体架构

```text
AI Harness / Browser / Scheduler
              |
       Web + MCP + CLI
              |
      Application Services
       /        |         \
    Store    BlobStore   MarketProvider
    /  \       /  \          /   \
SQLite PG   Local OSS    OneBound Other
```

依赖方向固定为：入口层调用应用服务，应用服务调用领域接口，基础设施适配器实现接口。领域层不得依赖具体数据库、阿里云 SDK 或 OneBound 字段。

## 4. 技术选型

### 4.1 后端

- 语言：Go。
- HTTP：标准库 `net/http`。
- HTML：标准库 `html/template`，服务端渲染。
- 局部交互：HTMX，静态资源嵌入二进制。
- 图表：Chart.js，按需引入。
- 日志：标准库 `log/slog`。
- 认证：本地密码哈希、服务端不透明 Session 和表单 CSRF；SaaS 身份源后续接入。
- 数据库访问：`database/sql`。
- SQL 代码生成：sqlc；生成代码提交到 Git。
- 数据迁移：Goose，迁移文件嵌入二进制。
- SQLite 驱动：纯 Go 的 `modernc.org/sqlite`。
- PostgreSQL 驱动：`pgx/v5/stdlib`。
- MCP：Go MCP SDK，提供业务语义工具。

不引入运行时 ORM、React、Node 构建链或微服务拆分。

### 4.2 分发

同一个可执行程序提供子命令：

```text
assetloop serve
assetloop mcp
assetloop migrate
assetloop refresh
assetloop install-scheduler
```

通过 CI 构建 Windows、Linux、macOS 的独立二进制和可选 Docker 镜像。

## 5. 领域模型

### 5.1 核心层级

```text
Tenant
└── ItemCategory
    └── ProductModel
        └── ProductVariant
            └── Asset
                └── AssetEvent
```

- `ItemCategory`：手机、相机、耳机等类别，不预置固定类别。
- `ProductModel`：如 iPhone 17 Pro。
- `ProductVariant`：影响二手价格的确切规格，如 256GB 和 512GB；分别绑定行情。
- `Asset`：用户实际拥有的一台具体物品，可包含序列号、颜色和购买渠道。
- `AssetEvent`：买入、卖出、维修、退货、升级、状态变更等生命周期事件。

颜色默认是具体物品属性，不拆成价格规格；只有确定颜色显著影响市场价格时才升级为规格维度。

### 5.2 分类和成色

- AI 可以建议创建新类别，但必须由用户确认。
- 已有类别内，信息完整时 AI 可以创建型号和规格。
- 信息不完整时先补充检索或询问用户。
- 每个类别可以维护自己的成色等级和映射规则。

### 5.3 交易与事件

- `transactions` 表示一次业务交易，可关联多个物品事件。
- `asset_events` 是物品生命周期的事实来源。
- 金额使用最小货币单位的有符号整数：支出为负，收入为正。
- 已确认事件不直接覆盖；更正采用作废并创建替代记录，以保留审计轨迹。

## 6. 金额、本位币和汇率

### 6.1 本位币

- 本地版自动创建一个默认租户。
- “系统全局本位币”在数据模型中是租户级全局设置。
- 默认本位币为 CNY。
- 第一条金额记录写入后，本位币锁定，普通设置操作不能直接修改。
- 系统内资产估值、收益、成本和报表全部按本位币计算。

### 6.2 非本位币入库

当原始货币不同于本位币时：

1. 用户确认采用哪一天的汇率。
2. 默认采用当前日期汇率。
3. 计算本位币金额并落库。
4. 同时保留原始金额、原始货币、汇率、汇率日期和来源。

建议字段：

```text
amount_base_minor
base_currency
amount_original_minor nullable
original_currency nullable
fx_rate nullable
fx_rate_date nullable
fx_rate_source nullable
```

自动行情点可按行情日期自动取得汇率，但仍必须保存换算证据。

## 7. 数据库兼容层

### 7.1 正式支持范围

- 默认：SQLite。
- SaaS：PostgreSQL。
- MySQL 仅保留未来适配位置，当前不承诺支持。

业务层使用按业务用例定义的 `Store` 接口，不暴露通用表级 CRUD：

```go
type Store interface {
    ConfirmImportDraft(ctx context.Context, cmd ConfirmImportDraft) error
    AppendAssetEvent(ctx context.Context, cmd AppendAssetEvent) error
    ListAssets(ctx context.Context, query AssetQuery) ([]Asset, error)
    SaveMarketSnapshot(ctx context.Context, snapshot MarketSnapshot) error
    ListDuePriceSeries(ctx context.Context, now time.Time) ([]PriceSeries, error)
}
```

SQLite 和 PostgreSQL 分别维护查询 SQL 和 sqlc 生成包。事务边界由具体 Store 操作封装。

### 7.2 SaaS 数据边界

- 所有业务表从第一版起包含 `tenant_id`。
- 所有唯一约束包含 `tenant_id`。
- 应用服务显式接收租户 ID。
- ID 由应用生成 UUID，不依赖数据库自增。
- 金额使用 `BIGINT` 最小货币单位。
- 时间统一为 UTC。
- 核心逻辑避免依赖触发器、存储过程和数据库特有生成列。

### 7.3 用户和租户权限

- `users` 保存全局登录身份，`tenant_memberships` 保存租户角色。
- 第一版固定 `owner`、`editor`、`viewer` 三个角色，不提供自定义权限编辑器。
- Web 从 Session 解析用户和租户；浏览器提交的 `tenant_id` 不作为授权依据。
- Application 服务检查能力，Store 仍在每条业务查询中强制 `tenant_id`。
- 平台运维权限与租户角色分离，第一版不提供跨租户数据浏览后台。
- 本地认证只能在 loopback 监听地址上关闭。

## 8. 数据迁移和版本升级

迁移按数据库方言分开，但保持相同逻辑版本号：

```text
migrations/
├── sqlite/
│   ├── 00001_initial.sql
│   └── 00002_add_original_currency.sql
└── postgres/
    ├── 00001_initial.sql
    └── 00002_add_original_currency.sql
```

### 8.1 SQLite 升级

应用启动时：

1. 获取跨进程升级锁。
2. 检查程序支持的 schema 版本。
3. 使用 SQLite Backup API 创建升级前备份。
4. 只执行向前迁移。
5. 运行完整性和外键检查。
6. 成功后启动；失败则停止并保留备份。

旧版程序遇到更高版本数据库时必须拒绝启动。

### 8.2 PostgreSQL 升级

- 由发布流水线中的单独迁移任务执行。
- 使用 PostgreSQL advisory lock 避免多个实例同时迁移。
- 应用实例启动只校验版本，不自行修改生产表结构。
- 采用 expand-contract：先增加兼容结构，再回填和切换，后续版本才删除旧结构。
- 生产环境不依赖 destructive down，修复使用新的向前迁移。

### 8.3 CI 验证

- 两种数据库都从空库建库。
- 两种数据库都运行同一套 Store 一致性测试。
- 保存上一正式版本数据库样本并执行升级测试。
- 检查同一迁移版本是否同时存在于两个方言目录。
- 检查 sqlc 生成代码没有过期。

## 9. 附件存储兼容层

### 9.1 支持范围

- 默认附件存储：本地文件系统。
- OSS 实现：默认阿里云 OSS。
- Bucket 默认私有。

应用服务只调用 `BlobStore`：

```go
type BlobStore interface {
    Put(ctx context.Context, key string, body io.Reader, metadata BlobMetadata) (BlobInfo, error)
    Open(ctx context.Context, key string) (io.ReadCloser, error)
    Stat(ctx context.Context, key string) (BlobInfo, error)
    Delete(ctx context.Context, key string) error
}
```

首批实现：

```text
LocalBlobStore
AliyunOSSBlobStore
```

### 9.2 统一对象键映射

本地和 OSS 共用唯一的 `ObjectKeyMapper`：

```text
tenants/{tenant_id}/attachments/{yyyy}/{mm}/{attachment_id}/original.{ext}
tenants/{tenant_id}/attachments/{yyyy}/{mm}/{attachment_id}/thumbnail.webp
```

同一个对象键可以映射为：

```text
本地：{local_root}/{object_key}
OSS： oss://{bucket}/{prefix}/{object_key}
```

业务表不保存本地绝对路径、Bucket 域名或临时签名 URL。

### 9.3 附件元数据

```text
attachments
- id
- tenant_id
- owner_type
- owner_id
- purpose
- store_id
- object_key
- original_filename
- mime_type
- byte_size
- sha256
- status
- created_at
- deleted_at
```

`store_id` 记录附件实际所在位置。修改默认存储只影响新附件，历史附件仍从原 Store 读取。

本地迁移到 OSS 时使用相同对象键：复制文件、校验大小和 SHA-256、更新 `store_id`，最后才允许清理旧文件。

### 9.4 安全要求

- 限制文件大小和允许的 MIME 类型。
- 不使用原始文件名拼接对象路径。
- 防止路径穿越。
- OSS 密钥不得写入数据库、日志或 Git。
- 不保存永久公开 URL。
- 下载经应用鉴权后代理，或生成短时签名 URL。

## 10. 行情数据源兼容层

OneBound 是第一个 `MarketDataProvider`：

```go
type MarketDataProvider interface {
    Descriptor() ProviderDescriptor
    FetchListings(ctx context.Context, query MarketQuery) (MarketBatch, error)
}
```

标准请求包含：

- 型号和规格指纹。
- 成色代码。
- 市场地区。
- 观察时间。
- 分页和数量限制。

标准结果包含：

- 数据源 ID 和版本。
- 原始挂牌 ID、标题、URL、价格和货币。
- 成色、地区、发布时间。
- 原始响应留痕。
- 下一页令牌。

OneBound 适配器只负责鉴权、请求、分页、限流、字段转换和错误分类。以下处理属于统一核心管线：

```text
规格匹配
→ 排除配件和服务
→ 去重
→ 成色映射
→ 异常值过滤
→ 计算中位数/P25/P75/样本量/置信度
→ 换算本位币
→ 保存价格点
```

不同数据源的行情不得静默混合。价格点必须保留数据源实例、版本、原始货币、汇率和样本统计。

第一版使用编译期注册，不使用 Go 动态插件。将来需要第三方独立扩展时，再增加版本化 HTTP Provider 协议。

## 11. 行情曲线和估值

- 行情序列由规格、成色、市场地区和数据源共同确定。
- 每日保存聚合价格点，不伪造历史回填。
- 第一个真实采样日是曲线起点。
- 某规格下所有物品卖出后，仍继续刷新 90 天，再停止自动刷新。
- 估值默认使用最新且满足最低样本量的首选数据源中位数。
- 页面展示中位数、区间、样本量、更新时间和数据源，避免制造虚假精度。

定时任务调用同一个应用服务：本地版由系统计划任务调用 CLI，SaaS 由 Worker/Cron 调用。

## 12. 配置管理

### 12.1 部署配置

本地版使用 `.env`；仓库只提交 `.env.example`，真实 `.env` 必须加入 `.gitignore`。

生产或 SaaS 可以直接注入真实环境变量和 Secret Manager，不要求磁盘上存在 `.env`。

配置优先级：

```text
真实环境变量
→ 指定的 .env 文件
→ 默认位置的 .env
→ 程序默认值
```

示例：

```dotenv
APP_ENV=local
HTTP_ADDR=127.0.0.1:8080
LOG_LEVEL=info

DB_DRIVER=sqlite
DB_DSN=./data/assetloop.db

ATTACHMENT_DEFAULT_STORE=local
ATTACHMENT_LOCAL_ROOT=./data/blobs

OSS_PROVIDER=aliyun
ALIYUN_OSS_ENDPOINT=https://oss-cn-shanghai.aliyuncs.com
ALIYUN_OSS_REGION=cn-shanghai
ALIYUN_OSS_BUCKET=assetloop
ALIYUN_OSS_ACCESS_KEY_ID=
ALIYUN_OSS_ACCESS_KEY_SECRET=
ALIYUN_OSS_PATH_PREFIX=

ONEBOUND_ENABLED=false
ONEBOUND_BASE_URL=
ONEBOUND_APP_KEY=
ONEBOUND_APP_SECRET=
```

### 12.2 业务配置

以下配置保存在数据库而不是 `.env`：

- 本位币及锁定状态。
- 行情数据源启用状态和优先级。
- 默认市场地区。
- 行情刷新周期。
- 分类成色等级。
- 估值统计口径。

本地版属于默认租户设置；SaaS 版属于各自租户设置。

## 13. Web 和 MCP

### 13.1 Web 页面

- 待确认导入。
- 物品列表和筛选。
- 分类、型号和规格维护。
- 具体物品详情和生命周期时间线。
- 买入、维修和卖出记录。
- 附件预览和下载。
- 行情曲线和样本说明。
- 持有成本、日均费用、账面损益和实际损益。
- 租户业务设置。
- 登录、首次初始化和租户成员管理。

### 13.2 MCP 工具

MCP 只暴露语义化工具，例如：

```text
create_import_draft
confirm_import_draft
search_product_variants
create_product_variant
record_purchase
record_repair
record_sale
attach_evidence
get_asset
list_assets
get_asset_valuation
refresh_market_price
```

写入工具必须经过输入验证和业务规则；不提供任意 SQL 写权限。

## 14. 建议目录结构

```text
cmd/assetloop/
internal/
├── domain/
├── application/
├── config/
├── store/
│   ├── sqlite/
│   └── postgres/
├── blob/
│   ├── local/
│   └── aliyun/
├── market/
│   ├── onebound/
│   └── manual/
├── web/
├── mcp/
└── scheduler/
migrations/
├── sqlite/
└── postgres/
web/
├── templates/
└── static/
docs/
```

## 15. 实施阶段

### 阶段 1：基础工程

- 建立 Go 模块和单二进制命令框架。
- 实现配置加载和 `.env.example`。
- 建立领域类型和租户边界。
- 建立 SQLite/PostgreSQL 初始迁移。
- 建立 Store 接口和双数据库一致性测试。

验收：两个数据库都能完成建库、升级、创建并查询最小物品记录。

### 阶段 2A：认证、RBAC 和 Web 外壳

- 首次初始化默认租户和 Owner。
- 本地账号密码、服务端 Session、退出和 CSRF。
- Owner/Editor/Viewer 应用层能力检查。
- 租户成员管理、安全审计和跨租户隔离测试。
- 服务端渲染的基础布局、导航和错误页面。

验收：Owner 可以登录并管理成员；Editor 不能管理成员；Viewer 不能执行写操作；任意角色都不能越租户读取数据。

### 阶段 2B：资产目录管理

- 分类、型号、规格和具体物品应用服务。
- 分类、型号、规格、物品列表、创建和详情页面。
- SQLite/PostgreSQL Store 一致性和升级测试。

验收：用户可以在 Web 中建立类别、型号、不同价格规格及其下的具体物品，并保持完整租户隔离。

### 阶段 2C：核心资产生命周期

- 交易与资产事件。
- 本位币、原币、汇率确认和审计。
- 待确认导入流程。
- 买入、维修、卖出、作废替换和生命周期时间线页面。

验收：可以在 Web 中完整记录一件物品从买入、维修到卖出的生命周期，并正确计算本位币现金流；更正保留原始事件。

### 阶段 3：附件系统

- 实现统一 ObjectKeyMapper。
- 实现 LocalBlobStore。
- 实现 AliyunOSSBlobStore。
- 实现附件元数据、校验和下载。
- 增加 Local 到 OSS 的校验迁移命令。

验收：同一业务流程可以在不改业务代码的情况下切换附件 Store，历史附件仍可读取。

### 阶段 4：行情和估值

- 定义 MarketDataProvider v1。
- 实现 OneBound 和手工数据源。
- 实现匹配、清洗、聚合、汇率换算和价格曲线。
- 实现刷新调度和停止策略。

验收：一个规格可以生成可追溯的二手行情曲线和当前本位币估值。

### 阶段 5：Web 和 MCP

- 完成附件、行情和待确认导入等剩余 Web 页面及人工复核。
- 完成语义化 MCP 工具。
- 验证 Harness 截图识别到确认入库的完整链路。

验收：用户可以通过对话提交截图，在 Web 中复核，并查看物品、行情和生命周期分析。

### 阶段 6：发布与 SaaS 准备

- GitHub Actions 多平台发布。
- 固化 `dev -> uat -> prod` 晋级、受保护分支和隔离打包环境。
- SQLite 升级备份和恢复验证。
- PostgreSQL 发布迁移 Job。
- Docker 镜像和健康检查。
- 加入认证、租户解析、限流和 Secret Store 后部署 SaaS。

## 16. 第一阶段完成标准

- 本地默认配置可零外部服务启动。
- SQLite 老数据升级前自动备份，升级失败不继续运行。
- PostgreSQL 与 SQLite 通过相同业务一致性测试。
- 本位币统计无浮点金额误差，原币和汇率可追溯。
- 附件可在本地和阿里云 OSS 间切换、读取和校验。
- OneBound 可替换，核心行情算法不依赖其字段。
- MCP 无裸 SQL 写入能力。
- Windows、Linux、macOS 用户无需安装 Go 或 Node 即可运行发布包。
