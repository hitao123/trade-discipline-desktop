# 开仓前确认与持仓中纪律复核 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在本地 macOS 应用中加入 30 秒开仓前确认、应用运行期间的关键价格监控、系统通知和不可变持仓复核记录。

**Architecture:** Go sidecar 负责持仓相关证券的低频报价刷新、关键线交叉判定和 SQLite 追加式记录；Electron 主进程读取未投递提醒并发送 macOS 通知；Vue 负责计划确认、持仓状态与复核表单。监控仅在应用运行时工作，公开报价过期或失败时不产生新触发。

**Tech Stack:** Go 1.24、SQLite（modernc.org/sqlite）、Vue 3、TypeScript、Vitest、Electron 37、electron-builder。

**Spec:** `docs/superpowers/specs/2026-08-19-discipline-loop-design.md`

## Global Constraints

- 不向券商发送订单；通知只提示核对计划，不给出买卖指令。
- 应用退出后停止监控；不引入云端、账户登录或常驻 macOS 服务。
- 使用现有 `market.Provider.FetchQuotes`；报价时间超过 20 分钟或获取失败时不得以缓存价格生成新提醒。
- 默认刷新间隔为 10 分钟；仅允许 `off`、`10m`、`15m`、`30m`。
- 所有确认、提醒和复核均追加保存并写审计事件；不得覆盖历史记录。
- 保留现有工作区改动；本计划执行期间不提交、重置或推送 Git。

---

## File Structure

| 文件 | 责任 |
| --- | --- |
| `backend/internal/domain/plan.go` | 扩展计划草稿的可选目标退出价位。 |
| `backend/internal/domain/discipline.go` | 定义确认、提醒、复核与监控状态的领域类型。 |
| `backend/internal/store/migrations.go` | 新增 v3 SQLite 表、索引和迁移记录。 |
| `backend/internal/store/discipline.go` | 追加式确认、提醒、复核和设置读写。 |
| `backend/internal/service/discipline.go` | 确认、监控评估、提醒投递和复核业务规则。 |
| `backend/internal/service/monitor.go` | sidecar 生命周期内的低频监控循环。 |
| `backend/internal/api/router.go` | 暴露本地确认、监控和复核 API。 |
| `backend/cmd/discipline-server/main.go` | 启动及停止监控循环。 |
| `src/main/index.ts` | 轮询未投递提醒、发送 macOS 通知并打开持仓页。 |
| `src/preload/index.ts`、`src/preload/index.d.ts` | 暴露安全的通知点击导航桥接。 |
| `src/renderer/components/plans/PreTradeConfirmation.vue` | 30 秒确认卡片与三项勾选。 |
| `src/renderer/components/positions/MonitorReviewPanel.vue` | 持仓监控状态、待复核提醒及决定表单。 |
| `src/renderer/components/plans/PlanForm.vue` | 录入目标/估值退出区间。 |
| `src/renderer/views/PlansView.vue`、`src/renderer/views/PositionsView.vue`、`src/renderer/views/SettingsView.vue` | 组合上述交互与本地 API。 |
| `src/renderer/types.ts` | 前端 API 类型。 |

## Task 1: 定义领域类型、计划价位与 SQLite v3 追加式表

**Files:**
- Create: `backend/internal/domain/discipline.go`
- Modify: `backend/internal/domain/plan.go`
- Modify: `backend/internal/store/migrations.go`
- Modify: `backend/internal/store/store.go`
- Test: `backend/internal/store/store_test.go`

**Interfaces:**
- Produces `domain.PreTradeConfirmation`, `domain.PriceAlertEvent`, `domain.PositionReviewEvent` 和 `domain.MonitorSettings`。
- Extends `domain.TradePlanDraft` with `TargetExitLowMinor int64` and `TargetExitHighMinor int64`.
- Raises `store.CurrentSchemaVersion` from `2` to `3`.

- [ ] **Step 1: Write failing migration and JSON-compatibility tests**

Add a test that opens a new store, asserts schema version 3, and verifies all three new tables exist. Add a domain JSON round-trip test that omits target prices and asserts both new fields remain `0`, preserving existing plans.

```go
func TestMigrationAddsDisciplineLoopTables(t *testing.T) {
	db := openTestStore(t)
	for _, table := range []string{"pre_trade_confirmations", "price_alert_events", "position_review_events"} {
		var name string
		if err := db.DB().QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name); err != nil || name != table {
			t.Fatalf("missing %s: %v", table, err)
		}
	}
}
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run: `go test ./internal/store -run TestMigrationAddsDisciplineLoopTables -count=1`

Expected: FAIL because the tables and schema version do not exist.

- [ ] **Step 3: Add domain types and v3 migration**

Create `domain/discipline.go` with these exact types and values:

```go
type AlertKind string
const (
	AlertRiskExit AlertKind = "risk_exit"
	AlertTargetZone AlertKind = "target_zone"
)
type ReviewDecision string
const (
	ReviewHold ReviewDecision = "hold"
	ReviewTrim ReviewDecision = "trim"
	ReviewSell ReviewDecision = "sell"
	ReviewWait ReviewDecision = "wait"
)
type MonitorSettings struct { Interval string `json:"interval"` }
type PreTradeConfirmation struct { ID, PlanID string; PlanSnapshotJSON string; StartedAt, ConfirmedAt time.Time }
type PriceAlertEvent struct { ID, PlanID, InstrumentID string; Kind AlertKind; TriggerPriceMinor, ThresholdMinor int64; PlanSnapshotJSON, Source string; SourceTime, TriggeredAt time.Time; NotifiedAt *time.Time }
type PositionReviewEvent struct { ID, AlertID string; Decision ReviewDecision; Reason string; CreatedAt time.Time }
```

Append migration statements for the three tables and indexes `pre_trade_plan_time`, `price_alert_open`, and `price_alert_instrument_kind`. Include foreign keys to `trade_plans`, `instruments`, and `price_alert_events`; use `CHECK` constraints for the two alert kinds and four review decisions. Record migration version 3 in `Store.Migrate` and set `CurrentSchemaVersion = 3`.

Use these table shapes exactly:

```sql
CREATE TABLE IF NOT EXISTS pre_trade_confirmations (
  id TEXT PRIMARY KEY, plan_id TEXT NOT NULL REFERENCES trade_plans(id),
  plan_snapshot_json TEXT NOT NULL CHECK(json_valid(plan_snapshot_json)),
  started_at TEXT NOT NULL, confirmed_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS price_alert_events (
  id TEXT PRIMARY KEY, plan_id TEXT NOT NULL REFERENCES trade_plans(id),
  instrument_id TEXT NOT NULL REFERENCES instruments(id),
  kind TEXT NOT NULL CHECK(kind IN ('risk_exit','target_zone')),
  trigger_price_minor INTEGER NOT NULL, threshold_minor INTEGER NOT NULL,
  plan_snapshot_json TEXT NOT NULL CHECK(json_valid(plan_snapshot_json)),
  source TEXT NOT NULL, source_time TEXT NOT NULL, triggered_at TEXT NOT NULL,
  notified_at TEXT
);
CREATE TABLE IF NOT EXISTS position_review_events (
  id TEXT PRIMARY KEY, alert_id TEXT NOT NULL UNIQUE REFERENCES price_alert_events(id),
  decision TEXT NOT NULL CHECK(decision IN ('hold','trim','sell','wait')),
  reason TEXT NOT NULL, created_at TEXT NOT NULL
);
```

- [ ] **Step 4: Run migration and domain tests**

Run: `go test ./internal/store ./internal/domain -count=1`

Expected: PASS, with both new empty-plan compatibility and all v3 tables confirmed.

- [ ] **Step 5: Review checkpoint**

Run: `git diff --check -- backend/internal/domain backend/internal/store`

Expected: no whitespace errors. Do not commit because the worktree contains user-owned changes.

## Task 2: 保存、读取和去重纪律事件

**Files:**
- Create: `backend/internal/store/discipline.go`
- Test: `backend/internal/store/discipline_test.go`

**Interfaces:**
- Consumes the types created in Task 1 and the existing `Store`, `NewID`, `audit_events` table.
- Produces `CreatePreTradeConfirmation`, `LatestPreTradeConfirmation`, `CreatePriceAlertIfCrossed`, `ListPendingAlerts`, `MarkAlertNotified`, `CreatePositionReview`, `MonitorSettings`, and `SaveMonitorSettings` store methods.

- [ ] **Step 1: Write failing store tests for append-only records**

Cover these exact cases using the seeded plan/instrument fixtures from `openTestStore`:

```go
func TestCreatePreTradeConfirmationKeepsPlanSnapshot(t *testing.T)
func TestCreatePriceAlertIfCrossedSuppressesContinuousTrigger(t *testing.T)
func TestCreatePositionReviewClosesOnlyThatAlert(t *testing.T)
func TestMonitorSettingsDefaultsToTenMinutesAndRejectsUnknownInterval(t *testing.T)
```

For the alert test: create `risk_exit` at 38_000, call again with the same kind and active condition, assert the second call returns `(created=false, nil)`; insert a non-triggered state through `RecordAlertState`, then assert a later crossed call creates a new alert.

- [ ] **Step 2: Run store tests to verify they fail**

Run: `go test ./internal/store -run 'Test(CreatePreTradeConfirmation|CreatePriceAlertIfCrossed|CreatePositionReview|MonitorSettings)' -count=1`

Expected: FAIL because the store methods are undefined.

- [ ] **Step 3: Implement transactional append-only store methods**

Use `Store` transactions for each create operation. Every insert must also append an `audit_events` row with actions `pre_trade_confirmed`, `price_alert_triggered`, `price_alert_notified`, `position_reviewed`, or `monitor_settings_changed`.

Use these signatures:

```go
func (s *Store) CreatePreTradeConfirmation(ctx context.Context, planID, snapshotJSON string, startedAt, confirmedAt time.Time) (domain.PreTradeConfirmation, error)
func (s *Store) LatestPreTradeConfirmation(ctx context.Context, planID string) (*domain.PreTradeConfirmation, error)
func (s *Store) CreatePriceAlertIfCrossed(ctx context.Context, alert domain.PriceAlertEvent) (created bool, saved domain.PriceAlertEvent, err error)
func (s *Store) RecordAlertState(ctx context.Context, planID string, kind domain.AlertKind, triggered bool, now time.Time) error
func (s *Store) ListPendingAlerts(ctx context.Context) ([]domain.PriceAlertEvent, error)
func (s *Store) ListUnnotifiedAlerts(ctx context.Context) ([]domain.PriceAlertEvent, error)
func (s *Store) MarkAlertNotified(ctx context.Context, id string, now time.Time) error
func (s *Store) CreatePositionReview(ctx context.Context, alertID string, decision domain.ReviewDecision, reason string, now time.Time) (domain.PositionReviewEvent, error)
func (s *Store) MonitorSettings(ctx context.Context) (domain.MonitorSettings, error)
func (s *Store) SaveMonitorSettings(ctx context.Context, settings domain.MonitorSettings, now time.Time) error
```

Store alert state in `app_settings` under `monitor-alert-state:<planID>:<kind>` as `{"triggered":true}`. This allows a new alert only after a later non-triggering quote flips the state to false, without deleting prior alerts.

- [ ] **Step 4: Run focused store tests**

Run: `go test ./internal/store -count=1`

Expected: PASS. Confirm that `ListPendingAlerts` excludes alerts that have a `position_review_events` row.

- [ ] **Step 5: Review checkpoint**

Run: `git diff --check -- backend/internal/store`

Expected: no whitespace errors.

## Task 3: 计划目标区间校验与开仓前确认服务/API

**Files:**
- Modify: `backend/internal/rules/engine.go`
- Modify: `backend/internal/rules/engine_test.go`
- Modify: `backend/internal/service/plans.go`
- Create: `backend/internal/service/discipline.go`
- Create: `backend/internal/service/discipline_test.go`
- Modify: `backend/internal/api/router.go`
- Modify: `backend/internal/api/router_test.go`

**Interfaces:**
- Consumes Task 1 plan fields and Task 2 store methods.
- Produces `Service.ConfirmPreTrade`, `Service.PreTradeStatus`, and HTTP endpoints `POST /api/plans/{id}/pre-trade-confirmations` and `GET /api/plans/{id}/pre-trade-confirmations/latest`.

- [ ] **Step 1: Write failing rule and service tests**

Add a rule test for incomplete target prices and an out-of-order range:

```go
func TestEvaluatePlanRejectsIncompleteTargetZone(t *testing.T) {
	p := validAlibabaPlan(); p.TargetExitLowMinor = 15_000
	got := EvaluatePlan(fixedNow, initialRule(), emptyPortfolio(), p)
	assertFinding(t, got, "TARGET_EXIT_RANGE_INVALID", SeverityHard)
}
```

Add service tests that assert a confirmation fails for rejected or expired plans, fails before 30 seconds, fails with any false attestation, and stores a plan JSON snapshot when all requirements are met.

- [ ] **Step 2: Run focused tests to verify they fail**

Run: `go test ./internal/rules ./internal/service -run 'Test(EvaluatePlanRejectsIncompleteTargetZone|ConfirmPreTrade)' -count=1`

Expected: FAIL because target validation and confirmation service do not exist.

- [ ] **Step 3: Implement target validation and confirmation service**

In `EvaluatePlan`, add one hard finding when exactly one target field is nonzero, either is negative, or `TargetExitHighMinor < TargetExitLowMinor`:

```go
if (plan.TargetExitLowMinor == 0) != (plan.TargetExitHighMinor == 0) ||
	plan.TargetExitLowMinor < 0 || plan.TargetExitHighMinor < 0 ||
	(plan.TargetExitLowMinor > 0 && plan.TargetExitHighMinor < plan.TargetExitLowMinor) {
	add("TARGET_EXIT_RANGE_INVALID", "targetExitLowMinor", "目标/估值退出区间必须同时填写，且上限不得低于下限")
}
```

Define:

```go
type PreTradeConfirmationInput struct {
	StartedAt time.Time `json:"startedAt"`
	NoFOMO bool `json:"noFomo"`
	NoLossRecovery bool `json:"noLossRecovery"`
	NoAveragingDown bool `json:"noAveragingDown"`
}
func (s *Service) ConfirmPreTrade(ctx context.Context, planID string, input PreTradeConfirmationInput) (domain.PreTradeConfirmation, error)
```

Load the plan, require `Status == "qualified"`, require `s.now().Before(plan.Draft.ValidUntil)`, require all three booleans, and require `s.now().Sub(input.StartedAt) >= 30*time.Second`. Marshal the current draft and validation into the confirmation snapshot before calling the store. Never update `trade_plans.status` for this display-only readiness state; derive readiness by querying the latest confirmation.

- [ ] **Step 4: Add router handlers and API tests**

Decode `PreTradeConfirmationInput` with `decodeJSON`; map invalid data to a normal 400 response using `writeResult`. Assert a complete request returns 201 and a request with `noFomo:false` returns an error response.

- [ ] **Step 5: Run backend tests**

Run: `go test ./internal/rules ./internal/service ./internal/api -count=1`

Expected: PASS.

## Task 4: 实现报价交叉评估、监控状态与 sidecar 监控循环

**Files:**
- Create: `backend/internal/service/monitor.go`
- Create: `backend/internal/service/monitor_test.go`
- Modify: `backend/internal/service/service.go`
- Modify: `backend/internal/store/queries.go`
- Modify: `backend/cmd/discipline-server/main.go`
- Modify: `backend/internal/api/router.go`
- Modify: `backend/internal/api/router_test.go`

**Interfaces:**
- Consumes `market.Provider.FetchQuotes`, Task 2 alert state methods, and Task 3 plan target fields.
- Produces `Service.RunMonitorOnce`, `Service.StartMonitor`, `Service.MonitorStatus`, `Service.CompletePositionReview`, and endpoints for monitor status, settings, pending alerts, and reviews.

- [ ] **Step 1: Write failing monitor tests**

Use a fake provider and fixed clock. Add exact tests:

```go
func TestRunMonitorOnceCreatesRiskExitAlertOnlyOnCrossing(t *testing.T)
func TestRunMonitorOnceCreatesTargetZoneAlertWhenPriceEntersRange(t *testing.T)
func TestRunMonitorOnceSkipsStaleQuoteAndRecordsStatus(t *testing.T)
func TestRunMonitorOnceSkipsUnplannedPosition(t *testing.T)
func TestCompletePositionReviewRequiresReasonAndClosesAlert(t *testing.T)
```

Seed a buy execution associated with a qualified plan. Test risk crossing at `CloseMinor <= RiskExitMinor`, target crossing at inclusive lower/upper bounds, and use `SourceTime = now.Add(-21*time.Minute)` for the stale case. Assert a non-triggered quote resets state so a later new crossing creates another alert.

- [ ] **Step 2: Run the monitor tests to verify they fail**

Run: `go test ./internal/service -run 'Test(RunMonitorOnce|CompletePositionReview)' -count=1`

Expected: FAIL because monitoring types and functions are not implemented.

- [ ] **Step 3: Implement the one-shot monitor evaluator**

Define these result types in `service/monitor.go`:

```go
type MonitorStatus struct {
	Enabled bool `json:"enabled"`
	Interval string `json:"interval"`
	LastAttemptAt *time.Time `json:"lastAttemptAt,omitempty"`
	LastSuccessfulAt *time.Time `json:"lastSuccessfulAt,omitempty"`
	LastError string `json:"lastError,omitempty"`
}
type MonitorResult struct { Checked int `json:"checked"`; Triggered int `json:"triggered"`; Status MonitorStatus `json:"status"` }
func (s *Service) RunMonitorOnce(ctx context.Context) (MonitorResult, error)
```

Create a store query that returns active positions together with the most recent qualified buy plan referenced by a non-reversed buy execution. Fetch all keys in one `FetchQuotes` call. Reject source times older than 20 minutes relative to `s.now()`. For each fresh quote, write the quote via `UpdateQuotes`, evaluate risk and optional target conditions, call `RecordAlertState`, and create an alert only on false-to-true transition. Persist `MonitorStatus` in `app_settings` under `monitor-status` on both success and failure.

- [ ] **Step 4: Implement lifecycle and API surface**

Add a cancellable monitor loop:

```go
func (s *Service) StartMonitor(ctx context.Context) {
	go func() {
		for { interval, err := s.monitorInterval(ctx); if err != nil { interval = 10*time.Minute }
			if interval > 0 { _, _ = s.RunMonitorOnce(ctx) }
			select { case <-ctx.Done(): return; case <-time.After(intervalOrMinute(interval)): }
		}
	}()
}
```

Use a context created in `main.go` and cancel it during server shutdown. Add routes:

```text
GET  /api/monitor/status
PUT  /api/monitor/settings
GET  /api/monitor/alerts
POST /api/monitor/alerts/{id}/reviews
GET  /api/monitor/alerts/unnotified
POST /api/monitor/alerts/{id}/notified
```

The settings handler accepts exactly `{ "interval": "off|10m|15m|30m" }`; the review handler accepts `{ "decision": "hold|trim|sell|wait", "reason": "..." }` and rejects blank reasons.

- [ ] **Step 5: Run service/API tests**

Run: `go test ./internal/service ./internal/api -count=1`

Expected: PASS, including stale quote, deduplication, settings validation, and review closure behavior.

## Task 5: Electron 本地通知及点击导航

**Files:**
- Modify: `src/main/index.ts`
- Modify: `src/main/sidecar.spec.ts`
- Modify: `src/preload/index.ts`
- Modify: `src/preload/index.d.ts`
- Modify: `src/renderer/App.vue`

**Interfaces:**
- Consumes `GET /api/monitor/alerts/unnotified` and `POST /api/monitor/alerts/{id}/notified`.
- Produces `window.discipline.onMonitorAlert(callback)` and an app-level route to `/positions?alert=<id>`.

- [ ] **Step 1: Write failing Electron/main-process tests**

Mock Electron `Notification` and the sidecar fetch. Assert that `pollMonitorNotifications()` sends one notification per unnotified alert, posts the delivered mark only after construction succeeds, and uses this exact body format:

```text
0700.HK 已触及风险退出线；报价时间 2026-08-19 10:20，请核对计划并记录决定。
```

Also assert a notification click restores/focuses the window and sends `monitor-alert-opened` with the alert ID to the renderer.

- [ ] **Step 2: Run focused Electron tests to verify failure**

Run: `pnpm exec vitest run src/main/sidecar.spec.ts`

Expected: FAIL because notification polling and bridge events are absent.

- [ ] **Step 3: Add safe notification polling**

Import `Notification` from Electron. Start a single 60-second timer after `startBackend()` and clear it before sidecar shutdown. The timer only calls the local authenticated sidecar API; it must catch errors and log to stderr rather than crash the app. Do not put broker, remote URL, or user data in a notification beyond code, alert kind, price time, and fixed review copy.

Expose this preload bridge:

```ts
onMonitorAlert: (callback: (alertID: string) => void) => {
  const listener = (_event: Electron.IpcRendererEvent, alertID: string) => callback(alertID)
  ipcRenderer.on('monitor-alert-opened', listener)
  return () => ipcRenderer.removeListener('monitor-alert-opened', listener)
},
```

In `App.vue`, subscribe once on mount and navigate to `{ path: '/positions', query: { alert: alertID } }`; unsubscribe on unmount.

- [ ] **Step 4: Run main-process and renderer shell tests**

Run: `pnpm exec vitest run src/main/sidecar.spec.ts src/renderer/App.spec.ts`

Expected: PASS.

## Task 6: 计划表单目标区间与 30 秒开仓前确认 UI

**Files:**
- Create: `src/renderer/components/plans/PreTradeConfirmation.vue`
- Create: `src/renderer/components/plans/PreTradeConfirmation.spec.ts`
- Modify: `src/renderer/components/plans/PlanForm.vue`
- Modify: `src/renderer/views/PlansView.vue`
- Modify: `src/renderer/views/PlansView.spec.ts`
- Modify: `src/renderer/types.ts`

**Interfaces:**
- Consumes Task 3 confirmation API and `PlanRecord` target fields.
- Produces a reusable confirmation component emitting `{ startedAt, noFomo, noLossRecovery, noAveragingDown }` only after all requirements are met.

- [ ] **Step 1: Write failing Vue tests**

Test PlanForm submits zero target fields when blank and minor-unit values when both target inputs contain `480` and `520`. Test the confirmation component starts disabled, remains disabled at 29 seconds, becomes eligible at 30 seconds only when all three checkboxes are checked, and emits the exact input payload.

```ts
expect(emitted('confirm')?.[0]).toEqual([{
  startedAt: '2026-08-19T10:00:00.000Z', noFomo: true,
  noLossRecovery: true, noAveragingDown: true,
}])
```

- [ ] **Step 2: Run focused Vue tests to verify failure**

Run: `pnpm exec vitest run src/renderer/components/plans/PreTradeConfirmation.spec.ts src/renderer/views/PlansView.spec.ts`

Expected: FAIL because neither UI nor target fields exist.

- [ ] **Step 3: Implement form fields and confirmation component**

Add `targetExitLow` and `targetExitHigh` to PlanForm reactive state; initialize blank/zero, hydrate old drafts as zero, and emit `targetExitLowMinor`/`targetExitHighMinor` in minor units.

`PreTradeConfirmation.vue` receives `plan: PlanRecord`, shows the fixed risk facts, creates `startedAt` on opening, uses a one-second interval cleaned in `onUnmounted`, and does not expose a submit button until `elapsedSeconds >= 30`. Use `defineEmits` only; do not call the API inside the component.

In PlansView, show the confirmation card only for qualified non-expired plans. On confirm, `POST /api/plans/{id}/pre-trade-confirmations`, reload plans/status, and display “已准备去券商执行（确认于 …）”. Keep any API error in the existing `ErrorNotice`.

- [ ] **Step 4: Run focused Vue tests**

Run: `pnpm exec vitest run src/renderer/components/plans/PreTradeConfirmation.spec.ts src/renderer/views/PlansView.spec.ts && pnpm typecheck`

Expected: PASS.

## Task 7: 持仓监控、复核与设置 UI

**Files:**
- Create: `src/renderer/components/positions/MonitorReviewPanel.vue`
- Create: `src/renderer/components/positions/MonitorReviewPanel.spec.ts`
- Modify: `src/renderer/views/PositionsView.vue`
- Modify: `src/renderer/views/SettingsView.vue`
- Modify: `src/renderer/views/SettingsView.spec.ts`
- Modify: `src/renderer/types.ts`

**Interfaces:**
- Consumes `MonitorStatus`, `PriceAlertEvent`, `PositionReviewEvent`, and the Task 4 API routes.
- Produces a one-alert-at-a-time review form with a required reason and a settings selector for valid monitor intervals.

- [ ] **Step 1: Write failing UI tests**

Add component tests that show source timestamp and “行情未更新” when `lastError` exists; assert a blank review reason cannot submit; assert the `hold` decision and a nonempty reason emits `{ decision: 'hold', reason: '继续观察财报证据' }`.

In settings tests, assert loading `interval: '15m'` selects 15 minutes and saving `off` uses `PUT /api/monitor/settings` with `{"interval":"off"}`.

- [ ] **Step 2: Run focused UI tests to verify failure**

Run: `pnpm exec vitest run src/renderer/components/positions/MonitorReviewPanel.spec.ts src/renderer/views/SettingsView.spec.ts`

Expected: FAIL because monitor APIs and UI controls are absent.

- [ ] **Step 3: Implement status, review, and interval selector**

Create `MonitorReviewPanel` with props `{ status: MonitorStatus | undefined, alerts: PriceAlertEvent[] }` and an `@review` event. Render risk/target labels from alert kind, trigger price, source time, and a `<select>` only in Settings with options `off`, `10m`, `15m`, `30m`.

In PositionsView load portfolio, monitor status, and alerts in a single `Promise.all`. If the router query contains `alert`, scroll the matching alert card into view after mount. On review success, reload monitor data; do not mutate the alert list optimistically.

In SettingsView, save monitor interval immediately through the settings endpoint; distinguish its request state from rule-version `busy` so an unavailable monitor does not disable rule editing.

- [ ] **Step 4: Run focused UI tests and type checking**

Run: `pnpm exec vitest run src/renderer/components/positions/MonitorReviewPanel.spec.ts src/renderer/views/SettingsView.spec.ts src/renderer/views/PlansView.spec.ts && pnpm typecheck`

Expected: PASS.

## Task 8: 全量验证、说明与 macOS 交付

**Files:**
- Modify: `README.md`
- Modify: `package.json`
- Modify: `resources/bin/discipline-server`
- Create: `outputs/Plain Rule-0.1.3-arm64.dmg`

**Interfaces:**
- Consumes all prior completed APIs and UI.
- Produces version 0.1.3 application documentation and a verified Apple Silicon DMG.

- [ ] **Step 1: Add release notes before building**

Raise the app version from `0.1.2` to `0.1.3`. In README, document that monitoring only runs while Plain Rule remains open, uses low-frequency public quotes, treats stale/failed quotes as informational rather than triggers, and sends a review reminder rather than trading instruction. Update the installer filename to `Plain Rule-0.1.3-arm64.dmg`.

- [ ] **Step 2: Run complete automated verification**

Run: `pnpm verify`

Expected: Go API/service/store/rules tests pass, all Vitest suites pass, `vue-tsc --noEmit` passes, and production frontend/Electron/sidecar builds succeed.

- [ ] **Step 3: Build and smoke-test the macOS artifact**

Run: `pnpm build:mac && pnpm smoke:mac`

Expected: arm64 DMG generated in `release/` and app opens with the sidecar available.

- [ ] **Step 4: Copy and validate the delivery artifact**

Run:

```bash
cp 'release/Plain Rule-0.1.3-arm64.dmg' '../outputs/Plain Rule-0.1.3-arm64.dmg'
hdiutil verify '../outputs/Plain Rule-0.1.3-arm64.dmg'
shasum -a 256 '../outputs/Plain Rule-0.1.3-arm64.dmg'
```

Expected: `hdiutil` reports `VALID`; report the resulting SHA-256 with the artifact link.

- [ ] **Step 5: Final source verification**

Run: `git diff --check && git status --short`

Expected: no diff-check errors. Preserve unrelated changes and do not commit or push.
