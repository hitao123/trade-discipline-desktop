# 市场历史行情与资金流图表 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** 在 macOS 本地交易纪律应用中增量保存近 1/3 个月证券日线、沪深成交额与南向成交净买额，并在市场榜单提供可切换、离线回退的折线图。

**Architecture:** 东方财富原始字段只在 Go market provider 内解析，service 协调“在线刷新 + SQLite 追加观察 + 缓存回退”，HTTP API 输出稳定领域类型。Vue 通过一个 composable 管理榜单、市场概览和当前证券走势，图表使用无第三方依赖的原生 SVG。

**Tech Stack:** Go 1.25、net/http、SQLite、Vue 3.5 Composition API、TypeScript 5.9、Vitest、Vue Testing Library、Electron 37。

## Global Constraints

- 只在用户刷新榜单或选择证券走势时请求数据，不建立后台轮询。
- A 股成交额范围为沪深两市，不包含北交所。
- 南向资金使用港股通（沪）与港股通（深）成交净买额之和，金额默认人民币。
- 数据点采用追加观察，不原地覆盖；同日修正必须保留旧观察。
- 公开接口失败时不得删除本地成功数据，页面必须说明缓存与错误。
- 现有股票前 20、ETF 前 10、CSV 导入、加入观察和“不可下单”行为保持兼容。
- 不引入图表依赖，不连接券商，不生成买卖信号。
- 保留工作区中用户已有的未提交界面、图标和打包修改。

---

### Task 1: Schema 2 与追加观察存储

**Files:**
- Modify: backend/internal/store/migrations.go
- Modify: backend/internal/store/store.go
- Modify: backend/internal/store/store_test.go
- Modify: backend/internal/store/market.go
- Create: backend/internal/store/market_history_test.go
- Modify: backend/internal/api/router.go
- Modify: backend/cmd/discipline-server/main.go

**Interfaces:**
- Produces: store.CurrentSchemaVersion 常量。
- Produces: Store.SaveDailyBars(ctx, bars, fetchedAt) (ObservationStats, error)。
- Produces: Store.LatestDailyBars(ctx, key, since) ([]market.DailyBar, time.Time, error)。
- Produces: Store.SaveMetricPoints(ctx, points, fetchedAt) (ObservationStats, error)。
- Produces: Store.LatestMetricPoints(ctx, metric, since) ([]market.MetricPoint, time.Time, error)。

- [ ] **Step 1: 写迁移失败测试**

在 store_test.go 要求迁移后存在 market_daily_bar_observations、market_metric_observations，并且 schema_migrations 最大版本为 2。测试先在当前 schema 1 上失败。

~~~go
func TestMigrationAddsAppendOnlyMarketHistorySchema(t *testing.T) {
    db := openTestStore(t)
    for _, table := range []string{"market_daily_bar_observations", "market_metric_observations"} {
        var name string
        if err := db.DB().QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil {
            t.Fatalf("missing %s: %v", table, err)
        }
    }
    var version int
    if err := db.DB().QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil || version != 2 {
        t.Fatalf("schema version=%d err=%v", version, err)
    }
}
~~~

- [ ] **Step 2: 运行迁移测试并确认 RED**

Run: cd backend && go test ./internal/store -run TestMigrationAddsAppendOnlyMarketHistorySchema -count=1

Expected: FAIL，提示缺少新观察表或版本仍为 1。

- [ ] **Step 3: 实现 schema 2**

新增两个追加观察表、日期查询索引和 CurrentSchemaVersion = 2。Migrate 同时 INSERT OR IGNORE 版本 2；health 与 sidecar ready message 使用该常量。

- [ ] **Step 4: 写追加、去重和修正保留测试并确认 RED**

测试用同一证券同一日的 10.00 元、完全重复的 10.00 元和修正后的 10.20 元依次保存。断言观察表有两行，LatestDailyBars 只返回 10.20 元；指标表执行同样验证。

~~~go
first := market.DailyBar{TradeDate: "2026-08-11", Market: "SH", Code: "600001", CloseMinor: 1000, Source: "fixture"}
corrected := first
corrected.CloseMinor = 1020
~~~

- [ ] **Step 5: 运行存储测试并确认 RED**

Run: cd backend && go test ./internal/store -run 'Test(DailyBar|Metric)Observations' -count=1

Expected: FAIL，提示 SaveDailyBars、SaveMetricPoints 或查询方法尚不存在。

- [ ] **Step 6: 实现最小追加观察存储**

ObservationStats 包含 Inserted、Duplicates、FromDate、ToDate。INSERT OR IGNORE 后用 RowsAffected 统计；查询使用 ROW_NUMBER() OVER (PARTITION BY trade_date ORDER BY fetched_at DESC, rowid DESC) = 1。

- [ ] **Step 7: 运行 Task 1 测试**

Run: cd backend && go test ./internal/store ./internal/api -count=1

Expected: PASS。

- [ ] **Step 8: 检查本任务 diff**

Run: git diff --check -- backend/internal/store backend/internal/api/router.go backend/cmd/discipline-server/main.go

Expected: 无输出，exit 0。

### Task 2: 东方财富证券日线与市场指标解析

**Files:**
- Modify: backend/internal/market/provider.go
- Create: backend/internal/market/eastmoney_history.go
- Modify: backend/internal/market/market_test.go
- Create: backend/internal/market/eastmoney_history_test.go

**Interfaces:**
- Produces: market.DailyBar、market.MetricKind、market.MetricPoint、market.MetricBatch。
- Extends Provider with FetchDailyBars(ctx, key, limit) and FetchMarketMetrics(ctx, limit)。
- EastmoneyProvider 增加 HistoryURL、DataCenterURL 可注入字段，测试使用 httptest.Server。

- [ ] **Step 1: 写证券日线解析失败测试**

伪服务器返回 klines 字符串。断言 600000 的日期、收盘价 10.25 元和成交额 12.5 亿元被转换为 CloseMinor=1025、TurnoverFen=125000000000；断言请求包含 secid=1.600000、klt=101、lmt=120。

- [ ] **Step 2: 运行日线测试并确认 RED**

Run: cd backend && go test ./internal/market -run TestFetchDailyBars -count=1

Expected: FAIL，提示 FetchDailyBars 尚不存在。

- [ ] **Step 3: 实现日线解析**

使用 fields2=f51,f52,f53,f54,f55,f56 解析 date/open/close/high/low/volume/turnover。SH 映射 1、SZ 映射 0、HK 映射 116；金额乘 100 转为分，价格乘 100 转为最小货币单位。响应为空、列数不足或数字非法时返回带上下文的错误。

- [ ] **Step 4: 运行日线测试并确认 GREEN**

Run: cd backend && go test ./internal/market -run TestFetchDailyBars -count=1

Expected: PASS。

- [ ] **Step 5: 写沪深成交额与南向净买额失败测试**

测试服务器按请求返回上证综指、深证综指日线，以及 MUTUAL_TYPE=002/004 的 NET_DEAL_AMT。手算断言沪深成交额相加、南向两个通道正负相加，并产生主指标与四个分项；不同交易日不能错配。

- [ ] **Step 6: 运行指标测试并确认 RED**

Run: cd backend && go test ./internal/market -run TestFetchMarketMetrics -count=1

Expected: FAIL，提示 FetchMarketMetrics 尚不存在或返回为空。

- [ ] **Step 7: 实现市场指标抓取与合并**

四个独立来源请求并发但不循环重试；MetricBatch.Errors 按 sh_turnover、sz_turnover、southbound_sh_net_buy、southbound_sz_net_buy 记录。只有组成同一主指标的分项均成功时才生成 ashare_turnover 或 southbound_net_buy；成功分项仍返回并可保存。

- [ ] **Step 8: 添加收盘完整性过滤测试和实现**

注入当前时间：Asia/Shanghai 15:00 前排除当日沪深数据，16:20 前排除当日南向数据；历史日期不受影响。分别先观察测试失败，再实现过滤。

- [ ] **Step 9: 运行 Task 2 测试**

Run: cd backend && go test ./internal/market -count=1

Expected: PASS。

### Task 3: Service 缓存回退与 HTTP API

**Files:**
- Modify: backend/internal/service/market.go
- Modify: backend/internal/service/market_test.go
- Modify: backend/internal/api/router.go
- Modify: backend/internal/api/router_test.go

**Interfaces:**
- Produces: Service.MarketOverview(ctx, rangeName)。
- Produces: Service.MarketHistory(ctx, key, rangeName)。
- Produces: Service.RefreshMarketHistory(ctx, key)。
- Extends POST /api/market/refresh response with overview。
- Adds GET /api/market/overview、GET /api/market/history、POST /api/market/history/refresh。

- [ ] **Step 1: 扩展 fake provider 并写概览部分失败测试**

fake 返回成功沪深成交额和失败南向资金。先断言 RefreshMarket 保存成交额、返回南向错误；第二次让 provider 全部失败，断言仍返回第一次缓存。

- [ ] **Step 2: 运行 service 测试并确认 RED**

Run: cd backend && go test ./internal/service -run TestRefreshMarketKeepsCachedOverviewOnPartialFailure -count=1

Expected: FAIL，当前 MarketResult 没有 Overview。

- [ ] **Step 3: 实现概览刷新和范围过滤**

解析 rangeName 只接受 1m、3m；按 s.now() 的自然月起点查询。RefreshMarket 在榜单后调用 provider.FetchMarketMetrics，保存成功点，再读取本地最新观察构造 Overview；指标失败只进入 Errors。

- [ ] **Step 4: 写证券历史按需刷新与回退测试**

第一次成功抓取并保存；第二次 provider 返回错误。断言 RefreshMarketHistory 仍返回缓存 Points、Cached=true、Error 非空，且数据库记录未删除。

- [ ] **Step 5: 运行证券历史测试并确认 RED**

Run: cd backend && go test ./internal/service -run TestRefreshMarketHistoryFallsBackToCache -count=1

Expected: FAIL，方法尚不存在。

- [ ] **Step 6: 实现证券历史服务**

GET 方法只读缓存；POST 刷新调用 FetchDailyBars(key, 120)，成功后追加观察，失败时读取缓存。若失败且无缓存，返回业务错误；若有缓存，返回成功 envelope 并携带 Error。

- [ ] **Step 7: 写 API 契约测试**

使用真实临时 SQLite 和 fake provider 验证非法 range 返回 400、历史刷新 body 未知字段返回 400、成功响应包含 points/cached/lastSuccessfulAt，overview 同时包含两条主序列。

- [ ] **Step 8: 运行 API 测试并确认 RED**

Run: cd backend && go test ./internal/api -run 'TestMarket(Overview|History)' -count=1

Expected: FAIL，路由尚未注册。

- [ ] **Step 9: 注册路由与参数校验**

查询参数 market 仅允许 SH/SZ/HK，code 必须非空，range 仅允许 1m/3m。JSON body 使用现有 decodeJSON 严格拒绝未知字段。

- [ ] **Step 10: 运行 Task 3 测试**

Run: cd backend && go test ./internal/service ./internal/api -count=1

Expected: PASS。

### Task 4: SVG 图表几何与展示组件

**Files:**
- Create: src/renderer/lib/chart.ts
- Create: src/renderer/lib/chart.spec.ts
- Create: src/renderer/components/market/MarketLineChart.vue
- Create: src/renderer/components/market/MarketLineChart.spec.ts
- Create: src/renderer/components/market/MarketOverviewPanel.vue
- Create: src/renderer/components/market/InstrumentHistoryPanel.vue
- Modify: src/renderer/types.ts

**Interfaces:**
- Produces: buildLineGeometry(points, width, height, padding)。
- MarketLineChart props: points、formatValue、label、valueKind、loading。
- MarketOverviewPanel props: overview、range、loading；emit rangeChange。
- InstrumentHistoryPanel props: history、range、loading；emit rangeChange、refresh。

- [ ] **Step 1: 写纯几何失败测试**

手算验证两个点、单点、全相同值和跨零数据不会出现 NaN/Infinity；跨零返回 zeroY 且位于绘图区内。

- [ ] **Step 2: 运行几何测试并确认 RED**

Run: pnpm test -- src/renderer/lib/chart.spec.ts

Expected: FAIL，chart.ts 尚不存在。

- [ ] **Step 3: 实现最小图表几何**

输出 path、points、min、max、zeroY。单点放在横向中心；相同值上下各扩 5%；空数组返回空 path。

- [ ] **Step 4: 写组件可访问性与正负测试并确认 RED**

渲染真实 MarketLineChart，断言 role=img 的 aria-label、空状态、首尾日期、正负值文本和零轴存在，不断言内部 mock。

- [ ] **Step 5: 实现三个展示组件**

使用 script setup lang=ts、类型化 props/emits、scoped class styles。SVG viewBox 响应容器；数据点为可聚焦 button/元素并暴露日期和值。prefers-reduced-motion 下关闭折线淡入。

- [ ] **Step 6: 运行 Task 4 测试**

Run: pnpm test -- src/renderer/lib/chart.spec.ts src/renderer/components/market/MarketLineChart.spec.ts

Expected: PASS。

### Task 5: Vue 数据编排与榜单交互

**Files:**
- Create: src/renderer/composables/useMarketHistory.ts
- Create: src/renderer/composables/useMarketHistory.spec.ts
- Modify: src/renderer/components/market/MarketTable.vue
- Modify: src/renderer/views/MarketView.vue
- Modify: src/renderer/views/MarketView.spec.ts
- Modify: docs/组件边界.md

**Interfaces:**
- Produces: useMarketHistory() 返回 market、overview、history、tab、range、selectedKey、busy、error、notice 及 load、refreshAll、selectHistory、setRange。
- MarketTable 新增 selectedKey，可发出 selectHistory 和 watch。

- [ ] **Step 1: 写 composable 失败测试**

用完整 API 响应 fixture 验证 load 先读榜单与概览；选择证券先 GET 缓存，缓存为空才 POST 刷新；同一会话同一天重复选择不再次 POST；切换范围只重新读取缓存。

- [ ] **Step 2: 运行 composable 测试并确认 RED**

Run: pnpm test -- src/renderer/composables/useMarketHistory.spec.ts

Expected: FAIL，composable 尚不存在。

- [ ] **Step 3: 实现 composable**

原始可替换数据使用 shallowRef，派生 activeSnapshot 使用 computed，网络副作用集中在 actions。错误按概览和历史来源合并为用户可读提示；没有后台 interval。

- [ ] **Step 4: 扩展 MarketView 集成测试并确认 RED**

断言两张市场概览图、1个月/3个月按钮、点击“查看走势”后证券名称与折线出现，同时页面仍没有“买入/下单”按钮且“加入观察”有效。

- [ ] **Step 5: 改造 MarketTable 与 MarketView**

MarketView 只组合 PageHeader、MarketOverviewPanel、InstrumentHistoryPanel、MarketTable 和 composable。MarketTable 选中行有 aria-selected 与视觉标记；新增“查看走势”按钮但不直接请求 API。

- [ ] **Step 6: 运行 Task 5 测试**

Run: pnpm test -- src/renderer/views/MarketView.spec.ts src/renderer/composables/useMarketHistory.spec.ts

Expected: PASS。

- [ ] **Step 7: 运行 Vue 类型检查**

Run: pnpm typecheck

Expected: PASS。

### Task 6: 文档、构建与完成审计

**Files:**
- Modify: README.md（仅追加本功能段落，保留用户现有修改）
- Modify: docs/验收清单.md（仅追加历史行情验收）
- Modify: resources/bin/discipline-server（由构建脚本生成）

**Interfaces:**
- Documents: 数据来源、按需刷新、1m/3m、缓存回退、南向净买额口径和公开接口限制。

- [ ] **Step 1: 更新用户文档**

说明刷新榜单会更新市场概览；点击榜单证券才补历史；本地数据位于 discipline.db；公开接口失败显示缓存；图表不构成投资建议。

- [ ] **Step 2: 运行完整自动验证**

Run: pnpm verify

Expected: Go、Vitest、vue-tsc 和完整 build 全部 exit 0。

- [ ] **Step 3: 运行 macOS sidecar 冒烟**

Run: pnpm smoke:mac

Expected: sidecar ready、鉴权、健康检查和关闭均通过。

- [ ] **Step 4: 构建 macOS 安装包**

Run: pnpm build:mac

Expected: release/Plain Rule-0.1.0-arm64.dmg 生成且命令 exit 0。

- [ ] **Step 5: 完成要求对照**

逐项检查：SQLite 追加保存、1m/3m 个股/ETF 折线、沪深成交额、南向净买额、缓存回退、迁移、API、测试、文档、macOS 构建。每一项记录直接文件或命令证据。

- [ ] **Step 6: 检查最终差异**

Run: git diff --check

Expected: 无空白错误。再运行 git status --short，区分本功能文件与用户原有未提交文件。
