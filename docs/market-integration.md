# 二手行情实施契约

首版转转 market_price；来源隔离，按采集日期记录最新一期成交最高价，不称作当日最高成交价。
显式的资产→二手物品多对一关系，查询配置改变创建新序列，旧记录保留。
详情只显示提供方图标、参考价及观察时间。成本看板不读取行情。
Frankfurter v2 自动汇率，整数最小货币单位和定点汇率，缺汇率保留原币并稍后补取。
每日北京时间09:00由系统计划任务调用同一Go二进制 refresh-market；跨进程租约。
两种数据库同版本前向迁移、升级测试和共享应用场景。UAT/生产分别授权。

## Provider evidence

Read-only probe 2026-09-08: zai-transfer-mcp 1.0.0 / protocol 2025-03-26.
iPhone 15 Pro + 256GB returned modelDesc=iPhone 15 Pro 256G,
dealMinPrice=3452, dealMaxPrice=6458. No currency, unit, source date or sample count.
The jump URL redirects to the public homepage on desktop and mobile user agents.
CNY yuan is an unverified contract assumption until official display confirmation;
ZHUANZHUAN_PRICE_UNIT_CONFIRMED defaults false and blocks application price writes.
The adapter remains testable with sanitized fixtures. Never copy tokens into evidence.

## Configuration and scheduled execution

Set ZHUANZHUAN_MCP_TOKEN in the process environment, ignored .env, or ignored
.env.zhuanzhuan.local beside the selected configuration file. The environment wins.
Set ZHUANZHUAN_PRICE_UNIT_CONFIRMED=true only after official CNY/yuan verification.
Web shows configuration state only. Credentials never enter quote evidence.

Use an installed binary in a stable path, with absolute SQLite DB_DSN and
ATTACHMENT_LOCAL_ROOT values in its configuration. Do not move or replace an existing
preview database to set up scheduling. Commands resolve relative paths from the
configuration directory and never migrate a database; run the normal upgrade first.

```powershell
.\assetloop.exe refresh-market --config C:\AssetLoop\.env
.\assetloop.exe refresh-market --config C:\AssetLoop\.env --tenant <tenant-id> --item <market-item-id>
.\assetloop.exe install-scheduler --config C:\AssetLoop\.env --dry-run
.\assetloop.exe install-scheduler --config C:\AssetLoop\.env
```

The Windows installer creates a task scoped to that absolute configuration path,
daily at 09:00 Asia/Shanghai. It runs as the current signed-in user without stored
passwords or elevated privileges, skips overlapping instances and starts when a
missed time becomes available. The machine must be on and that user signed in;
application startup also catches up the current date. Installation requires both
credentials and unit confirmation. The dry run prints XML with paths, not secrets.

Linux/server deployment can use systemd units (deployment itself needs its normal
environment authorization). Keep the service in the application's deployment
directory and invoke the same installed binary:

```ini
# assetloop-market.service
[Unit]
Description=AssetLoop daily secondhand quotes
[Service]
Type=oneshot
User=assetloop
WorkingDirectory=/opt/assetloop
ExecStart=/opt/assetloop/assetloop refresh-market --config /opt/assetloop/.env
TimeoutStartSec=2h
```

```ini
# assetloop-market.timer
[Unit]
Description=Refresh AssetLoop quotes at 09:00 Shanghai
[Timer]
OnCalendar=*-*-* 09:00:00 Asia/Shanghai
Persistent=true
[Install]
WantedBy=timers.target
```

No timestamps are fabricated for missed days. A snapshot is keyed by tenant, market
item and Shanghai acquisition date. Manual refresh can replace the current day's
price; older successes cannot overwrite a newer timestamp. Five-minute database
leases are fenced by token, external work has a three-minute deadline, and HTTP
requests never hold a long write transaction. Transient failures retry at most
three times; authentication failure ends the batch.

Only explicitly bound, enabled items are automatically collected. All-sold series
continue through the 90-day boundary; another held device restores eligibility.
Manual disabling retains observations. Historical pending FX is repaired before
the next quote request, using the original observation date.

## Verification evidence (development, 2026-09-08)

- Live Zhuanzhuan preview succeeded through the Go application and Web drawer:
  iPhone 15 Pro / 256GB -> iPhone 15 Pro 256G, max 6458, min 3452.
  The app's display assumes CNY yuan; official unit confirmation is still pending.
  Formal prices remain empty and OS scheduling has not been activated.
- A real Frankfurter v2 time-series response succeeded. Expanded providers are
  objects containing key, date and rate. The recorded public fixture tests this
  actual shape, latest non-future date selection and fixed-point rates.
  2026-09-08 CNY/USD sample: 0.14909; normalized source retains provider keys.
- SQLite application, upgrade and cumulative full-element tests pass. The shared
  scenario covers two devices, config separation, tenant isolation, same-day
  replacement, failed/mismatched quotes, leases, 90-day eligibility, pending FX and
  base-currency locking. Web tests cover actual form posts and viewer denial.
- Existing local preview upgraded 14 -> 15 with automatic backup. All original
  asset/catalog/tag/resource rows, all 15 lifecycle events and the existing GLB
  hash match the before snapshot. The GLB HTTP endpoint returns the same bytes.
- PostgreSQL 17 live validation passed after the test host became reachable: complete
  Store conformance (including market), 14->15 market upgrade, specification upgrade,
  and the cumulative full-element scenario. Tests used a temporary network-isolated
  container and a dedicated non-superuser role/database, without shared credentials
  or host ports. The container and uploaded test binaries were removed afterwards.
  SQLite and PostgreSQL have both executed the market scenario; no skips count as passes.
- Browser QA verified query preview, return to editable conditions, disabled
  confirmation while unverified, existing detail empty state and loaded 3D model.
  Asset editing correctly prefills model and selected tags into a child quote drawer;
  cancelling leaves the original asset unchanged.
  One Impeccable detector pass ran in regex fallback (HTML parser dependencies
  absent); it is not a computed-contrast or accessibility certification.

The fixed source icon is the official site's favicon from
[Zhuanzhuan](https://m.zhuanzhuan.com/favicon.ico), stored locally for provider
identification. No product pictures are fetched.
Frankfurter reference: [official documentation](https://frankfurter.dev/).
