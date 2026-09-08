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
