# 数据健康层 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为市场榜单、指标和价格提醒提供可验证的新鲜度、来源和缓存状态，确保旧数据绝不被标记为实时。

**Architecture:** 在 `market` 包建立纯函数的新鲜度模型；store 的 schema 6 追加结构化组件健康数据；service 为每项数据独立生成健康状态。Vue 显示短状态徽标，原始网络错误只进入本地诊断。

**Tech Stack:** Go 1.25、SQLite（modernc）、Vue 3、TypeScript、Vitest。

**Spec:** `docs/superpowers/specs/2026-08-23-quick-record-data-health-design.md`

## Global Constraints

- 只读取公开行情；不连接券商、不下单、不后台高频轮询。
- 只有当天且来源时间足够新才可称 `live`。
- 缓存/过期报价不得触发价格提醒。
- 页面不得显示请求 URL、EOF 或重试栈；详细信息只进本地诊断。
- 迁移必须兼容 schema 5，并保留现有观察数据。

---

### Task 1: 新鲜度领域模型与实时防误标

**Files:**
- Create: `backend/internal/market/health.go`
- Create: `backend/internal/market/health_test.go`
- Modify: `backend/internal/market/provider.go`
- Modify: `backend/internal/service/market.go`
- Test: `backend/internal/service/market_test.go`

**Interfaces:**
- Produces `market.HealthState`: `live`, `delayed`, `cached`, `unavailable`.
- Produces `market.ComponentHealth` with state, source, source time, last success, message and detail code.
- Produces `market.AssessFreshness(now, tradeDate, sourceTime, maxAge)`.

- [ ] **Step 1: Write failing pure-function tests**

```go
func TestAssessFreshnessRejectsPreviousTradeDate(t *testing.T) {
    now := time.Date(2026, 10, 8, 10, 0, 0, 0, shanghai)
    got := AssessFreshness(now, "2026-09-30", time.Date(2026, 9, 30, 15, 0, 0, 0, shanghai), 5*time.Minute)
    if got.State != HealthCached { t.Fatalf("state=%s", got.State) }
}
```

- [ ] **Step 2: Run the focused test to verify failure**

Run: `cd backend && go test ./internal/market -run TestAssessFreshnessRejectsPreviousTradeDate -count=1`  
Expected: FAIL because the health model does not exist.

- [ ] **Step 3: Implement the health model**

```go
type HealthState string
const ( HealthLive HealthState = "live"; HealthDelayed HealthState = "delayed"; HealthCached HealthState = "cached"; HealthUnavailable HealthState = "unavailable" )
type ComponentHealth struct { State HealthState `json:"state"`; Source string `json:"source,omitempty"`; SourceTime *time.Time `json:"sourceTime,omitempty"`; LastSuccessfulAt *time.Time `json:"lastSuccessfulAt,omitempty"`; Message string `json:"message"`; DetailCode string `json:"detailCode,omitempty"` }
```

Compare dates in `Asia/Shanghai`. A same-day source older than `maxAge` is delayed; a previous-date source never returns live.

- [ ] **Step 4: Gate live snapshot writes with freshness**

Add a service test with a Wednesday 10:00 clock and a fallback provider returning the prior trading date. Assert `RefreshLiveMarket` does not call `SaveMarketSnapshotForMode` for it and returns cached/unavailable status.

- [ ] **Step 5: Run focused tests**

Run: `cd backend && go test ./internal/market ./internal/service -run 'TestAssessFreshness|TestRefreshLiveMarket' -count=1`  
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/market/health.go backend/internal/market/health_test.go backend/internal/market/provider.go backend/internal/service/market.go backend/internal/service/market_test.go
git commit -m "feat: classify market data freshness"
```

### Task 2: 持久化结构化刷新状态

**Files:**
- Modify: `backend/internal/store/migrations.go`
- Modify: `backend/internal/store/store.go`
- Modify: `backend/internal/store/market.go`
- Modify: `backend/internal/store/store_test.go`
- Modify: `backend/internal/store/market_history_test.go`

**Interfaces:**
- Raises `store.CurrentSchemaVersion` from 5 to 6.
- Extends `MarketRefreshStatusRow` with `Components map[string]market.ComponentHealth`.
- Changes `SaveMarketRefreshStatus` to accept component health, while reading legacy `errors_json` as an empty health map.

- [ ] **Step 1: Write failing schema-5 upgrade test**

Create a schema-5 `market_refresh_status` fixture with `errors_json`. After `Migrate`, assert `health_json` exists, the old row remains readable and components defaults to an empty map.

- [ ] **Step 2: Run focused migration test to verify failure**

Run: `cd backend && go test ./internal/store -run TestMigrationAddsMarketHealth -count=1`  
Expected: FAIL because schema version 6 and `health_json` are absent.

- [ ] **Step 3: Implement append-only upgrade**

Implement `ensureMarketRefreshHealth` using `PRAGMA table_info(market_refresh_status)` and add `health_json TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(health_json))` only if missing. Record migration 6 after success. Marshal/unmarshal component maps with an empty-map fallback.

- [ ] **Step 4: Add persistence test**

Save a fresh stock component and a cached quote component. Reload and assert state, source, source time and last-success time match the saved values.

- [ ] **Step 5: Run store suite and commit**

Run: `cd backend && go test ./internal/store -count=1`  
Expected: PASS.

```bash
git add backend/internal/store/migrations.go backend/internal/store/store.go backend/internal/store/market.go backend/internal/store/store_test.go backend/internal/store/market_history_test.go
git commit -m "feat: persist market component health"
```

### Task 3: 独立容错、来源与提醒安全性

**Files:**
- Modify: `backend/internal/service/market.go`
- Modify: `backend/internal/service/monitor.go`
- Modify: `backend/internal/market/eastmoney_history.go`
- Modify: `backend/internal/market/sina.go`
- Test: `backend/internal/service/market_test.go`
- Test: `backend/internal/service/monitor_test.go`
- Test: `backend/internal/market/sina_test.go`

**Interfaces:**
- `MarketResult` and `LiveMarketResult` return `health map[string]market.ComponentHealth`.
- Component keys: `stock`, `etf`, `quotes`, `ashare_turnover`, `southbound_net_buy`, `live_stock`, `live_etf`, `monitor_quotes`.
- Service accepts an optional quote fallback implementing `FetchQuotes(context.Context, []InstrumentKey) ([]Quote, error)`.

- [ ] **Step 1: Write failing partial-success and stale-alert tests**

```go
func TestRefreshMarketMarksQuotesCachedWhenRankingsSucceed(t *testing.T) {
    svc := openExecutionService(t)
    svc.SetMarketProvider(fakeMarketProvider{stock: freshStock, etf: freshETF, quotesErr: errors.New("EOF")})
    result, err := svc.RefreshMarket(context.Background())
    if err != nil || result.Health["quotes"].State != market.HealthCached { t.Fatalf("%#v %v", result.Health, err) }
}

func TestMonitorDoesNotTriggerWithPreviousTradeDayQuote(t *testing.T) {
    svc, plan := serviceWithQualifiedTencentPosition(t)
    svc.SetMarketProvider(fakeMarketProvider{quotes: []market.Quote{oldRiskExitQuote(plan)}})
    result, err := svc.RunMonitorOnce(context.Background())
    if err != nil || result.Triggered != 0 { t.Fatalf("%#v %v", result, err) }
}
```

- [ ] **Step 2: Run focused tests to verify failure**

Run: `cd backend && go test ./internal/service -run 'TestRefreshMarketMarksQuotesCached|TestMonitorDoesNotTriggerWithPreviousTradeDayQuote' -count=1`  
Expected: FAIL because only a flat error map exists.

- [ ] **Step 3: Implement per-component fallback**

Set source/time from saved quotes or metrics. If a component source fails, return cached only when a local observation exists, otherwise unavailable. Convert raw provider errors to stable `PRIMARY_TEMPORARY_FAILURE`, `FALLBACK_FAILURE` or `NO_LOCAL_CACHE` codes. Do not fail a successful ranking because a quote update failed.

- [ ] **Step 4: Implement safe quote fallback and provenance**

Attempt the configured quote fallback after primary failure; reject a fallback quote unless its market, code, normalized currency and freshness are valid. Make `combineMetricPoints` derive `eastmoney-public-derived`, `sina-public-derived`, or `mixed-public-derived` from the two component sources. Never alert on non-live quotes.

- [ ] **Step 5: Run backend market tests and commit**

Run: `cd backend && go test ./internal/market ./internal/service -count=1`  
Expected: PASS.

```bash
git add backend/internal/service/market.go backend/internal/service/monitor.go backend/internal/market/eastmoney_history.go backend/internal/market/sina.go backend/internal/service/market_test.go backend/internal/service/monitor_test.go backend/internal/market/sina_test.go
git commit -m "feat: isolate market source health"
```

### Task 4: 市场页健康状态和本地诊断

**Files:**
- Create: `src/renderer/components/market/MarketHealthBadge.vue`
- Create: `src/renderer/components/market/MarketHealthBadge.spec.ts`
- Modify: `src/renderer/types.ts`
- Modify: `src/renderer/composables/useMarketHistory.ts`
- Modify: `src/renderer/views/MarketView.vue`
- Modify: `src/renderer/views/MarketView.spec.ts`
- Modify: `README.md`
- Modify: `docs/验收清单.md`

**Interfaces:**
- TypeScript `MarketComponentHealth` mirrors the backend shape.
- `MarketHealthBadge` receives `{ health: MarketComponentHealth; compact?: boolean }`.

- [ ] **Step 1: Write failing UI tests**

```ts
expect(screen.getByText('本地缓存 · 最后成功于 2026/08/22 15:03')).toBeTruthy()
expect(screen.queryByText(/push2\.eastmoney\.com/)).toBeNull()
```

Test successful rankings plus cached quotes and expect “榜单已更新；持仓参考价暂未更新”，not “最近刷新异常”。

- [ ] **Step 2: Run focused tests to verify failure**

Run: `pnpm vitest run src/renderer/views/MarketView.spec.ts src/renderer/components/market/MarketHealthBadge.spec.ts`  
Expected: FAIL because typed health and the badge do not exist.

- [ ] **Step 3: Implement safe copy and diagnostics**

Replace the flat `errors.join('；')` message with a summary derived from health states. Render a badge per table/metric; use a collapsed diagnostic block containing only stable code, source and times.

- [ ] **Step 4: Verify, document and commit**

Run: `pnpm test && pnpm typecheck && pnpm test:go`  
Expected: PASS.

Update the acceptance checklist for schema 6, stale data, partial refresh and alert freshness.

```bash
git add src/renderer/components/market/MarketHealthBadge.vue src/renderer/components/market/MarketHealthBadge.spec.ts src/renderer/types.ts src/renderer/composables/useMarketHistory.ts src/renderer/views/MarketView.vue src/renderer/views/MarketView.spec.ts README.md docs/验收清单.md
git commit -m "feat: show market data health"
```
