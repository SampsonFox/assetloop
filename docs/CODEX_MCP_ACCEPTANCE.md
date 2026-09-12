# Codex MCP 配置、踩坑与纯工具验收

日期：2026-09-10；Windows Codex Desktop、本机 8081 隔离实例。
这是后续编写 skill 的事实素材，不是已安装的 skill，也不是远程/生产操作授权。

## 当前结论

2026-09-12 更新：当前能力与 UAT 范围见 [子版本总结](MCP_RELEASE_NOTES.md)。
当前共 40 工具，支持公开 GLB URL 导入；图片补全是 Web 上传/导入而非 MCP。
事件类型与备注的含义已写进服务器工具描述及字段 schema，用户不需要旧对话。
本地图片替换已按“先看原图、核对型号颜色、再上传、再查展示”验证。
以下保留原生 39 工具时期的验收事实，不据此宣称所有新增能力均经原生工具实测。

后续增量：服务器端 URL 导入已新增 `import_3d_resource_from_url`，见 `MCP.md`。
以下 39 工具和无导入能力的记录描述此前验收时点，不代表新版本工具目录。
更新运行程序后，当前任务需重新加载才能看到第 40 个工具；该增量的实测应另记。

用户明确接受“当前对话只用 MCP 完成一条完整业务流程”作为本次验收标准。
已完成：OAuth 登录、当前任务加载 39 个工具、直接工具读写、创建物品、买入、
维修、更正、卖出、历史保留、幂等重试、冲突拒绝和精确成本核对。
本轮业务操作全部使用 `mcp__assetloop_acceptance__*`，没有浏览器业务操作、
直接 REST、SQL 或独立 SDK 代替原生调用。终端/文件工具仅用于诊断和文档。

本轮没有逐一调用 39 个工具。之前 SDK 的全工具测试是另一层证据。
实时撤销授权、自动刷新长期运行验证、最终清理仍未执行，不据此声称完成。
3D 文件上传没有 MCP 工具，只能管理已有资源；UAT/生产发布仍需单独批准。

## 配置顺序（供 skill 固化）

1. 确认目标工作树、分支、8081 所属进程、可执行文件和数据库/Blob 绝对路径。
   保留已有验收账号和数据，不因连接错误删库；不要影响 8080。
2. 按 [MCP.md](MCP.md) 设置：

   ```dotenv
   AUTH_MODE=local
   HTTP_ADDR=127.0.0.1:8081
   MCP_ENABLED=true
   MCP_ISSUER=http://127.0.0.1:8081
   MCP_CLIENT_ID=codex-local
   MCP_REDIRECT_URI=http://127.0.0.1/callback
   ```

   另行指定 `DB_DSN`、`ATTACHMENT_LOCAL_ROOT` 的隔离绝对路径。
   启动时显式使用 `assetloop serve`；真正新库才走 `/setup`。
3. 验证 `/.well-known/oauth-authorization-server` 和
   `/.well-known/oauth-protected-resource/mcp` 返回 200 JSON，核对 issuer/resource。
   无凭证 MCP 返回带资源元数据地址的 401 是正常认证要求。
4. 先检查当前 CLI 帮助和配置，已存在则复用。本次用户删除后要求重建：

   ```sh
   codex mcp get assetloop-acceptance
   codex mcp add assetloop-acceptance --url http://127.0.0.1:8081/mcp --oauth-client-id codex-local --oauth-resource http://127.0.0.1:8081/mcp
   ```

   `add` 会尝试自动开始 OAuth，不并行发起第二次登录。
   需要重新登录时：`codex mcp login assetloop-acceptance`。
5. 打开这次输出的新授权链接，核对账号、客户端、权限。本次为测试账号和
   `assets:read`、`assets:catalog`、`assets:lifecycle`。
   安全审核要求确认时，向用户说明具体范围；拒绝后不可换通道绕过。
   用户确认同范围授权后继续。不要打印 Cookie、访问/刷新令牌或回调 code。
6. 保持 CLI 登录进程活着直到回调。成功依据是
   `Successfully logged in to MCP server 'assetloop-acceptance'.`，不是按钮点击。
   本次动态回调端口由 CLI 选择；服务允许已注册 loopback 路径的可变端口。
7. Desktop 需要让当前任务重新加载。本次实际 UI 是“插件 → MCP”，关闭再开启
   `assetloop-acceptance` 后，指定任务日志变成 `ready`，原生工具出现。
   不声称存在未观察到的 Restart 按钮，不盲目反复切换。
8. 直接调用 `get_context`、`list_assets` 核对实际账号、数据空间和权限。
   配置存在、登录成功、metadata 200、SDK 测试通过，都不能单独证明当前任务直连。

OAuth 凭证由 Codex 管理，不显示在 Bearer 环境变量/headers 输入框。
字段为空不代表没有认证；Web Cookie 不是 MCP 凭证。Client ID 不是秘密。
不要读取原始凭证存储或把令牌粘到配置来掩盖 OAuth 故障。

## Windows 服务存活

Codex 更新后旧服务停止过；之后临时终端启动的实例也曾停止，后一次没有捕获
退出原因，不能断言每次都是更新或应用崩溃。同一工具调用内 200 不证明跨回合存活。
最终由 `Win32_Process.Create` 启动已核实绝对路径的 PowerShell 隐藏后台进程，
运行忽略目录中的环境脚本和已构建程序，显式工作目录、DB/Blob 路径及 `serve`，
日志写入忽略目录。没有创建计划任务、开机项或第二个应用服务。
首次 Windows PowerShell 启动未成功，换成已确认的解释器后成功，前者原因未证实。
跨多个回合验证同一 PID 及发现端点后才继续。不要将临时 PID、个人绝对路径写死进 skill。

## 排障表：先定位失败层

| 现象 | 本次证据/原因 | 处理原则 |
|---|---|---|
| `failed to refresh OAuth tokens` | 旧实现拒绝任何非空 refresh `scope`；Codex 实际发送该字段 | 接受原授权标准化 scope 集合的重复，拒绝扩权/未知 scope；保留角色、资源及轮换校验 |
| `failed to resolve OAuth metadata before using stored credentials` | 对应失败时 8081 未监听 | 先核对端口/两个元数据端点与日志时间，不直接推断 JSON 坏了或凭证坏了 |
| 服务恢复后仍报旧错误 | 当前任务没有新握手 | 服务稳定后重新加载，再看当前任务新日志及直接调用 |
| 重建后 `unknown MCP server` | CLI 配置已创建，当前任务未加载 | 区分磁盘配置、凭证、运行中的工具目录 |
| 回调页拒绝连接 | 8081 仍 200，临时回调端口关闭；CLI 报等待回调超时 | 新建登录流程，用新链接；不刷新旧 code、不重启业务服务 |
| 重复 `resource` | CLI 显式资源和发现结果可产生同值重复 | 接受相同资源重复，拒绝不同值；不放开其它重复参数 |
| 授权 submitter 丢失 | 禁用提交按钮导致按钮值未提交 | 保留允许/拒绝语义并覆盖回归测试 |
| 授权 POST 不透明 Origin | OAuth 页原 `no-referrer` 策略 | 该页面改 `same-origin`，继续拒绝 null/跨源；不放宽全局 Origin |
| 浏览器阻止回调 | 曾涉及 consent CSP，但不是所有浏览器错误都已归因 | 只允许验证后的回调 origin/port；不能任意开放全部 loopback |
| app-server 操控失败 | 独立进程 resume 当前任务遇到 active writer；proxy 曾 socket 连接失败 | 不破坏锁、不关闭主进程、不用新任务/SDK 冒充当前对话 |

scope/Referrer 修复见 `db08ff8`；对应测试在 `internal/mcp/oauth_test.go`、
`internal/web/oauth_test.go` 及 application OAuth 测试。人工重连不等于长期刷新验证。
本机 Desktop 日志位于
`%LOCALAPPDATA%/Packages/OpenAI.Codex_2p2nqsd0c76g0/LocalCache/Local/Codex/Logs/YYYY/MM/DD/`；
不同安装方式/版本需重新定位。按服务器、任务和时间过滤
`mcp_server_startup_status_updated`/enabled 状态，避免整段输出凭证相关日志。
日志为 UTC，用户时间为 Asia/Shanghai，比较前换算。本次 19:57 当前任务出现 ready。

## 纯 MCP 生命周期验收证据

使用单独命名的虚构测试物品及序列号；公开记录不保留实际物品、账户、
租户或事件标识。未来 skill 必须读取/创建返回的 ID，不能硬编码示例 ID。

| 步骤 | 工具 | 实际结果 |
|---|---|---|
| 身份/去重/类型 | `get_context`, `list_categories`, `search_product_models`, `list_event_types` | owner、CNY、三个权限；新型号无匹配，事件类型使用返回 ID |
| 新建 | `create_category`, `create_product_model`, `save_asset` | 一件新物品；同键重试创建返回同一 ID |
| 买入 | `record_event` | 100000 分 CNY；同键重试返回同一事件 ID |
| 维修 | `record_event` | 原始 10000 分 CNY |
| 更正 | `correct_event` | 8000 分，替代记录引用原事件；同键重试完整结构相同 |
| 卖出 | `record_event` | 收入 70000 分 CNY |
| 冲突拒绝 | 原买入键改金额 `record_event` | `isError=true`, `invalid_input`, `validation.request_conflict`, 不可重试 |
| 完整历史 | `list_events(show_voided=true)` | 返回 4 项；原维修 IsVoided=true、原金额保留，替代记录引用原 ID |
| 筛选 | `list_events` 仅 sale | 1 项，摘要仍为完整支出/收入 |
| 单件结算 | `get_asset_cost`, `list_assets` | Sold=true/sold，支出 108000、收入 70000、成本 38000 分；序列号恰好一件 |
| 汇总 | `get_portfolio_summary` | 物品 2→3；支出 1424→109424；收入 0→70000；净现金流 -1424→-39424 分 |

`get_asset_cost.NetMinor` 是净成本（正 38000）；空间汇总 `NetMinor` 是净现金流
（本次增量负 38000）。字段同名不能混淆符号。返回结构的精确金额、状态、
原记录保留、重试结果和汇总增量均在本轮直接断言通过。

## 后续 skill 边界与清理

- 业务必须先查 ID，写入稳定 request_key；超时复用原键及完整输入。
- 金额整数分、时间带时区；更正走 void-and-replace，不覆盖历史。
- 只用 MCP 不允许暗中以浏览器/REST/SQL/SDK 补齐业务动作。
- 当前没有资产/类别删除或账号重置 MCP，本轮保留可追溯验收数据。
  之前用户允许清理测试数据，但最终清理未执行；应另行安排精确范围的维护清理，
  不冒充纯 MCP 步骤、不波及其它数据或未经要求删除连接配置。
- 实时撤销后 MCP 拒绝、Web 会话仍有效的验证留待单独执行。
  删除 Codex 配置不等于服务端撤销。保存验收证据后再清理目标授权和数据。
- 一次验收成功不自动触发 UAT、生产或架构变更。
