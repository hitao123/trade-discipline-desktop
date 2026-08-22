# 资产配置菜单 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Plain Rule 增加独立、可版本化的资产配置菜单，追踪 200,000 元起始资金在 2026-12-31 前实现 10% 目标收益的进度。

**Architecture:** Go sidecar 以 SQLite 的配置版本和追加式市值事件作为事实来源，并在读取时计算建仓偏离、再平衡偏离和收益目标进度。Vue 新页面只调用本地 API、呈现计算结果并提交带原因的配置修订或市值事件；不连接券商，不生成交易指令。

**Tech Stack:** Go 1.22、SQLite（modernc）、Go `net/http`、Vue 3 `<script setup>`、Vue Router、Vitest、Vue Testing Library。

**Spec:** `docs/superpowers/specs/2026-08-22-asset-allocation-menu-design.md`

## Global Constraints

- 初始资金固定为 20,000,000 分（200,000 元），初始目标收益固定预填为 1,000 BP（10%），截止日期固定预填为 `2026-12-31`，但后续修订允许通过带原因的新版本改变目标。
- 初始目标权重必须是 10,000 BP（100%）；所有资产最新市值相加才是组合当前总资产，现金/货基不得自动反推。
- 初始快照必须含七类资产和半导体设备 ETF 159558 的 1,000,000 分、现金/货基的 19,000,000 分市值记录。
- 全部配置版本和市值事件追加保存并审计；不得改写旧版本或旧市值事件。
- 偏离和 10% 目标只作记录和进度展示；文案不得构成买卖指令。
- 不新增外部网络请求、券商连接或 Excel 运行时读取；不得提交、重置或清理现有工作区改动。

---

### Task 1: 领域模型、SQLite v4 与配置事实存储

**Files:**
- Create: `backend/internal/domain/allocation.go`
- Create: `backend/internal/domain/allocation_test.go`
- Create: `backend/internal/store/allocation.go`
- Create: `backend/internal/store/allocation_test.go`
- Modify: `backend/internal/store/migrations.go`
- Modify: `backend/internal/store/store.go`
- Modify: `backend/internal/store/store_test.go`

**Interfaces:**
- Produces `domain.AllocationDraft`, `domain.AllocationItem`, `domain.AllocationVersion`, `domain.AllocationValueEvent`, `domain.AllocationSnapshot`.
- Produces `Store.EnsureInitialAllocation(ctx, now)`, `Store.CurrentAllocation(ctx)`, `Store.CreateAllocationVersion(ctx, draft, reason, now)`, `Store.AppendAllocationValueEvent(ctx, profileID, event, now)`, `Store.ListAllocationVersions(ctx)`, and `Store.ListAllocationValueEvents(ctx, profileID, itemKey)`.
- Later tasks use `TargetWeightBP`, `TargetReturnBP`, and all fen amounts as integers.

- [ ] **Step 1: Write failing domain and store tests**

```go
var fixedNow = time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC)

func TestInitialAllocationMatchesCheckedWorkbook(t *testing.T) {
    db := openTestStore(t)
    snapshot, err := db.EnsureInitialAllocation(context.Background(), fixedNow)
    if err != nil { t.Fatal(err) }
    if snapshot.Version.Draft.InitialCapitalFen != 20_000_000 || snapshot.Version.Draft.TargetReturnBP != 1_000 || snapshot.Version.Draft.TargetDeadline != "2026-12-31" {
        t.Fatalf("draft=%#v", snapshot.Version.Draft)
    }
    if got := snapshot.Values["semiconductor-equipment-etf-159558"].ValueFen; got != 1_000_000 { t.Fatalf("semiconductor=%d", got) }
    if got := snapshot.Values["cash-fund"].ValueFen; got != 19_000_000 { t.Fatalf("cash=%d", got) }
}

func TestAllocationVersionAndValueEventsAreAppendOnly(t *testing.T) {
    db := openTestStore(t)
    initial, _ := db.EnsureInitialAllocation(context.Background(), fixedNow)
    revised := initial.Version.Draft
    revised.Items[0].TargetWeightBP = 3_800
    revised.Items[1].TargetWeightBP = 1_850
    _, err := db.CreateAllocationVersion(context.Background(), revised, "降低腾讯目标权重", fixedNow.Add(time.Hour))
    if err != nil { t.Fatal(err) }
    _, err = db.AppendAllocationValueEvent(context.Background(), initial.ProfileID, domain.AllocationValueEvent{ItemKey: "tencent", ValueFen: 7_800_000, Source: "manual", ObservedAt: fixedNow}, fixedNow)
    if err != nil { t.Fatal(err) }
    versions, _ := db.ListAllocationVersions(context.Background())
    events, _ := db.ListAllocationValueEvents(context.Background(), initial.ProfileID, "tencent")
    if len(versions) != 2 || versions[1].Version != 1 || len(events) != 2 || events[0].ValueFen != 7_800_000 || events[1].ValueFen != 0 { t.Fatalf("versions=%#v events=%#v", versions, events) }
}
```

- [ ] **Step 2: Run focused tests and verify they fail**

Run: `go test ./internal/domain ./internal/store -run 'Test(InitialAllocationMatchesCheckedWorkbook|AllocationVersionAndValueEventsAreAppendOnly)' -count=1`

Expected: FAIL because allocation types and store methods do not exist.

- [ ] **Step 3: Add allocation types and migration version 4**

```go
type AllocationItem struct {
    Key string `json:"key"`
    Name string `json:"name"`
    TargetWeightBP int `json:"targetWeightBP"`
    TargetShares int `json:"targetShares,omitempty"`
    BuyRule string `json:"buyRule"`
    SellRule string `json:"sellRule"`
    Note string `json:"note"`
}

type AllocationDraft struct {
    InitialCapitalFen int64 `json:"initialCapitalFen"`
    TargetReturnBP int `json:"targetReturnBP"`
    TargetDeadline string `json:"targetDeadline"`
    Items []AllocationItem `json:"items"`
}
```

Append these tables to `migrationStatements`, set `CurrentSchemaVersion = 4`, and record migration 4 in `Store.Migrate`:

```sql
CREATE TABLE IF NOT EXISTS allocation_profiles (
  id TEXT PRIMARY KEY, created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS allocation_versions (
  id TEXT PRIMARY KEY, profile_id TEXT NOT NULL REFERENCES allocation_profiles(id),
  version INTEGER NOT NULL, draft_json TEXT NOT NULL CHECK(json_valid(draft_json)),
  change_reason TEXT NOT NULL, created_at TEXT NOT NULL, previous_id TEXT REFERENCES allocation_versions(id),
  UNIQUE(profile_id, version)
);
CREATE TABLE IF NOT EXISTS allocation_value_events (
  id TEXT PRIMARY KEY, profile_id TEXT NOT NULL REFERENCES allocation_profiles(id),
  item_key TEXT NOT NULL, value_fen INTEGER NOT NULL CHECK(value_fen >= 0),
  source TEXT NOT NULL CHECK(source IN ('initial_import','manual')),
  observed_at TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS allocation_value_latest ON allocation_value_events(profile_id, item_key, observed_at DESC, created_at DESC);
```

Seed only when no `allocation_profiles` row exists. Use seven stable keys, retain the full user rules in the seeded items, write one `initial_import` event per asset, and write `allocation_profile / initial_import` audit data in the same transaction.

- [ ] **Step 4: Implement immutable query and write methods**

`EnsureInitialAllocation` must return the one profile, version 1, and latest value map. `CreateAllocationVersion` loads the previous version, calculates `version + 1`, inserts the new JSON and an `allocation_profile / revised` audit event. `AppendAllocationValueEvent` inserts a new row and an `allocation_value / recorded` audit event. Never issue `UPDATE` or `DELETE` against either allocation history table.

- [ ] **Step 5: Run focused store tests and migration regression tests**

Run: `go test ./internal/domain ./internal/store -run 'Test(InitialAllocationMatchesCheckedWorkbook|AllocationVersionAndValueEventsAreAppendOnly|Migration)' -count=1`

Expected: PASS; assert migration version 4 and all three allocation tables exist.

### Task 2: 服务层计算、输入校验与本地 API

**Files:**
- Create: `backend/internal/service/allocation.go`
- Create: `backend/internal/service/allocation_test.go`
- Modify: `backend/internal/api/router.go`
- Modify: `backend/internal/api/router_test.go`

**Interfaces:**
- Consumes `Store` allocation methods from Task 1.
- Produces `Service.Allocation(ctx) (domain.AllocationSnapshot, error)`, `Service.ReviseAllocation(ctx, draft, reason)`, `Service.RecordAllocationValue(ctx, itemKey, valueFen, observedAt)`, `Service.AllocationVersions(ctx)`, and `Service.AllocationValueEvents(ctx, itemKey)`.
- Extends router with `/api/allocation`, `/api/allocation/versions`, and `/api/allocation/items/{key}/value-events`.

- [ ] **Step 1: Write failing service tests for calculations and validation**

```go
func TestAllocationCalculatesReturnAndTwoKindsOfDeviation(t *testing.T) {
    svc := openExecutionService(t)
    snapshot, err := svc.Allocation(context.Background())
    if err != nil { t.Fatal(err) }
    if snapshot.CurrentTotalFen != 20_000_000 || snapshot.TargetTotalFen != 22_000_000 || snapshot.ReturnBP != 0 { t.Fatalf("snapshot=%#v", snapshot) }
    if _, err := svc.RecordAllocationValue(context.Background(), "semiconductor-equipment-etf-159558", 1_250_000, svc.now()); err != nil { t.Fatal(err) }
    snapshot, _ = svc.Allocation(context.Background())
    if snapshot.CurrentTotalFen != 20_250_000 || snapshot.ReturnBP != 125 { t.Fatalf("snapshot=%#v", snapshot) }
    semiconductor := snapshot.Items["semiconductor-equipment-etf-159558"]
    if semiconductor.BuildGapFen != 1_250_000 || semiconductor.RebalanceGapFen != 1_281_250 { t.Fatalf("item=%#v", semiconductor) }
}

func TestAllocationRevisionRequiresReasonAndExactWeights(t *testing.T) {
    svc := openExecutionService(t)
    initial, _ := svc.Allocation(context.Background())
    if _, err := svc.ReviseAllocation(context.Background(), initial.Version.Draft, ""); err == nil { t.Fatal("expected reason error") }
    draft := initial.Version.Draft
    draft.Items[0].TargetWeightBP--
    if _, err := svc.ReviseAllocation(context.Background(), draft, "测试不完整权重"); err == nil { t.Fatal("expected weight error") }
}
```

- [ ] **Step 2: Run the focused service tests and verify they fail**

Run: `go test ./internal/service -run 'TestAllocation(CalculatesReturnAndTwoKindsOfDeviation|RevisionRequiresReasonAndExactWeights)' -count=1`

Expected: FAIL because service allocation methods do not exist.

- [ ] **Step 3: Implement calculation and validation boundary**

Implement a read model with these exact calculations:

```go
targetTotalFen := draft.InitialCapitalFen * int64(10_000 + draft.TargetReturnBP) / 10_000
currentTotalFen := sum(latestValueFenByItem)
returnBP := int((currentTotalFen - draft.InitialCapitalFen) * 10_000 / draft.InitialCapitalFen)
buildTargetFen := draft.InitialCapitalFen * int64(item.TargetWeightBP) / 10_000
rebalanceTargetFen := currentTotalFen * int64(item.TargetWeightBP) / 10_000
buildGapFen := buildTargetFen - currentValueFen
rebalanceGapFen := rebalanceTargetFen - currentValueFen
```

Reject a configuration unless: initial capital is positive, `TargetReturnBP >= 0`, deadline parses as `2006-01-02`, exactly seven unique nonblank keys/names exist, all rules are nonblank except an optional note, Tencent target shares are nonnegative, each weight is between 0 and 10,000, and total weight is exactly 10,000. Reject a value event when key is absent, value is negative, or observation time is zero. Trim reasons and free text before saving.

- [ ] **Step 4: Add API routes and handler tests**

Register and test:

```go
mux.HandleFunc("GET /api/allocation", router.allocation)
mux.HandleFunc("PUT /api/allocation", router.reviseAllocation)
mux.HandleFunc("GET /api/allocation/versions", router.allocationVersions)
mux.HandleFunc("GET /api/allocation/items/{key}/value-events", router.allocationValueEvents)
mux.HandleFunc("POST /api/allocation/items/{key}/value-events", router.recordAllocationValue)
```

Tests must prove unauthenticated access is rejected by existing middleware, initial `GET` returns 220,000 yuan target total and a 10% target, a bad `PUT` returns 422, a valid `PUT` creates version 2, and a `POST` value event changes current total without changing configuration version.

- [ ] **Step 5: Run service and API tests**

Run: `go test ./internal/service ./internal/api -run Allocation -count=1`

Expected: PASS.

### Task 3: 独立 Vue 菜单、编辑器和市值记录界面

**Files:**
- Create: `src/renderer/views/AllocationView.vue`
- Create: `src/renderer/views/AllocationView.spec.ts`
- Create: `src/renderer/components/allocation/AllocationSummary.vue`
- Create: `src/renderer/components/allocation/AllocationEditor.vue`
- Create: `src/renderer/components/allocation/AllocationValueForm.vue`
- Create: `src/renderer/components/allocation/AllocationSummary.spec.ts`
- Create: `src/renderer/components/allocation/AllocationEditor.spec.ts`
- Modify: `src/renderer/types.ts`
- Modify: `src/renderer/router.ts`
- Modify: `src/renderer/App.vue`

**Interfaces:**
- Consumes `GET /api/allocation`, `PUT /api/allocation`, `POST /api/allocation/items/{key}/value-events`, and version/event list endpoints from Task 2.
- Produces a route `/allocation` and a navigation item between `/positions` and `/market`.
- Component data flows down through props; actions flow up through `@save`, `@record-value`, and `@edit` events.

- [ ] **Step 1: Add frontend types and failing component tests**

Define `AllocationItem`, `AllocationDraft`, `AllocationValueEvent`, `AllocationItemProgress`, and `AllocationSnapshot` in `src/renderer/types.ts`. Write tests that expect the summary to render “目标总资产 ¥220,000.00”, “目标收益 10.00%”, and neutral wording “距离目标差额”; write editor tests that require a revision reason and emit values in fen/BP; write view tests that load `/api/allocation`, save `PUT /api/allocation`, and post an item value event.

- [ ] **Step 2: Run focused Vitest tests and verify they fail**

Run: `pnpm vitest run src/renderer/components/allocation/AllocationSummary.spec.ts src/renderer/components/allocation/AllocationEditor.spec.ts src/renderer/views/AllocationView.spec.ts`

Expected: FAIL because allocation components, types, and route do not exist.

- [ ] **Step 3: Build presentational components**

`AllocationSummary.vue` receives a snapshot and displays current total, cumulative return, 220,000 yuan goal total, deadline, target gap, and a progress bar. Its copy must say “收益目标是跟踪标尺，不代表建议采取交易动作。”

`AllocationEditor.vue` receives the editable draft and emits a full `AllocationDraft` plus `reason`. It uses `<input type="number">` for target percentage and Tencent target shares, textareas for buy/sell rules, and disables save until reason is nonblank. It must show both “建仓偏离（按 20 万基准）” and “再平衡偏离（按当前总资产）”.

`AllocationValueForm.vue` receives one item and emits `{ itemKey, valueFen, observedAt }`; it allows cash/货基 and labels the field “当前市值（人民币）”.

- [ ] **Step 4: Implement view orchestration and navigation**

`AllocationView.vue` owns API effects with `shallowRef` state and keeps the latest snapshot after a successful save or value event. It must surface `APIError.fields` with existing `ErrorNotice`, render version history as read-only, and never label a positive gap as “买入” or a negative gap as “卖出”. Add `{ to: '/allocation', label: '资产配置', index: '06' }` to `App.vue`, renumber following navigation entries, and register the route in `router.ts`.

- [ ] **Step 5: Run focused frontend tests and typecheck**

Run: `pnpm vitest run src/renderer/components/allocation/AllocationSummary.spec.ts src/renderer/components/allocation/AllocationEditor.spec.ts src/renderer/views/AllocationView.spec.ts && pnpm typecheck`

Expected: PASS.

### Task 4: 全量回归与交付说明

**Files:**
- Modify: `README.md`
- Modify: `scripts/smoke-macos.sh`

**Interfaces:**
- Consumes all completed API and UI work from Tasks 1–3.
- Produces updated local-use documentation and a startup smoke test asserting schema version 4.

- [ ] **Step 1: Add documentation and failing smoke assertion**

Document the separate asset-allocation menu, 10% by 2026-12-31 tracking target, manual/append-only valuation records, lack of broker integration, and the fact that the target does not generate trading instructions. Change the smoke script’s expected schema value from `3` to `4`, adding all allocation tables to its SQLite table check.

- [ ] **Step 2: Build and run fresh full verification**

Run: `pnpm verify`

Expected: all Go packages, all Vitest files, Vue typecheck, and production build pass.

- [ ] **Step 3: Run packaged-app startup smoke test**

Run: `pnpm build:mac && pnpm smoke:mac`

Expected: the temporary first-run database passes SQLite integrity, schema v4, original ledger/history/discipline tables, and allocation tables.

- [ ] **Step 4: Inspect final diff without altering existing work**

Run: `git diff --check && git status --short`

Expected: no whitespace errors; preserve all unrelated pre-existing changes and do not commit.
