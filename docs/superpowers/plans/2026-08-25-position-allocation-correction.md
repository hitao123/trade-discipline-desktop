# Position, Allocation, Correction, and Bond-Fund Guard Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show named holdings, support one-step auditable execution correction, derive allocation progress from the real portfolio with optional adjustments, and permanently reject bond-like ETFs.

**Architecture:** Keep execution events immutable, but make correction a single SQLite transaction that appends a reversal and replacement and supersedes derivative discipline state. Treat portfolio replay as the source of truth for allocation; configuration stores explicit instrument-code bindings while a new signed adjustment stream represents assets outside the local ledger. Put the fixed-income ETF predicate in the market domain and enforce it again at every storage boundary.

**Tech Stack:** Go 1.25, `modernc.org/sqlite`, Vue 3.5 Composition API with `<script setup lang="ts">`, Vitest/Testing Library, Electron/Vite.

**Spec:** `docs/superpowers/specs/2026-08-25-position-allocation-correction-design.md`

## Global Constraints

- Preserve all unrelated dirty-worktree changes; do not reset, reformat, or commit them.
- Keep the app local-only and do not add brokerage connectivity or order placement.
- Do not add a new navigation route; use existing holdings, execution, and allocation screens.
- Actual broker facts require only positive quantity/price, valid settlement sign, instrument, and time; lot-size mismatch must not block recording.
- Cover only the four high-risk areas named in the spec, extending existing test cases instead of creating a large number of new test files.
- Existing allocation versions and legacy value events remain readable but do not drive linked current values.
- Bond-like ETFs must be rejected in backend domain/storage boundaries, not merely hidden by Vue.

## File and Component Map

- `backend/internal/market/etf_filter.go`: exported fixed-income/cash ETF eligibility predicate.
- `backend/internal/store/store.go`: schema version 9 migration orchestration and forbidden-instrument cleanup.
- `backend/internal/store/migrations.go`: correction relation and signed allocation-adjustment tables/columns.
- `backend/internal/store/instruments.go`, `backend/internal/store/market.go`: reject forbidden ETF upserts and list only eligible active instruments.
- `backend/internal/domain/portfolio.go`: named position contract.
- `backend/internal/store/queries.go`, `backend/internal/service/executions.go`: enrich replayed positions with instrument metadata.
- `backend/internal/domain/allocation.go`, `backend/internal/store/allocation.go`, `backend/internal/service/allocation.go`: code bindings, signed adjustments, linked portfolio calculation, and unassigned holdings.
- `backend/internal/store/executions.go`: active execution listing and atomic correction transaction.
- `backend/internal/api/router.go`: execution list/correction and allocation-adjustment HTTP seams.
- `src/renderer/components/executions/ExecutionCorrectionPanel.vue`: one responsibility—edit one active execution and emit a typed correction draft.
- `src/renderer/components/executions/ExecutionHistory.vue`: one responsibility—show active recent executions and emit the selected record.
- `src/renderer/views/PositionsView.vue`: orchestration plus holdings table/shortcut only.
- `src/renderer/views/ExecutionsView.vue`: compose entry, history, and correction components.
- `src/renderer/components/allocation/AllocationSummary.vue`: linked/current source presentation.
- `src/renderer/components/allocation/AllocationValueForm.vue`: repurposed signed “system external adjustment” form.
- `src/renderer/components/allocation/AllocationEditor.vue`: explicit instrument-code binding controls.
- `src/renderer/views/AllocationView.vue`: fetch eligible instruments and orchestrate saves.
- `src/renderer/types.ts`: shared frontend contracts.

---

### Task 1: Enforce bond-fund exclusion and migrate existing automatic rows

**Files:**
- Modify: `backend/internal/market/etf_filter.go`
- Modify: `backend/internal/market/market_test.go`
- Modify: `backend/internal/store/market.go`
- Modify: `backend/internal/store/instruments.go`
- Modify: `backend/internal/store/queries.go`
- Modify: `backend/internal/service/instruments.go`
- Modify: `backend/internal/store/store.go`
- Modify: `backend/internal/store/store_test.go`

**Interfaces:**
- Produces: `market.IsEligibleETFName(name string) bool`.
- Produces: schema migration 9 that removes forbidden market-rank entries and their unreferenced instruments.
- Consumers: all later instrument selectors and allocation binding controls receive only eligible instruments.

- [ ] **Step 1: Extend the existing ETF test and migration test first**

Add table cases to `TestNormalizeETFFiltersFixedIncomeBeforeTakingTopTen` and add one store migration test that inserts `511360 短融ETF海富通` plus a `market_rank_entries` reference, reruns migration, and expects both records removed. Also insert `515880 通信ETF国泰` and expect it preserved.

```go
if market.IsEligibleETFName("公司债ETF南方") {
	t.Fatal("bond ETF must be rejected")
}
if !market.IsEligibleETFName("通信ETF国泰") {
	t.Fatal("equity ETF must remain eligible")
}
```

- [ ] **Step 2: Run focused Go tests and verify RED**

Run: `go test ./internal/market ./internal/store -run 'TestNormalizeETFFilters|TestMigrationRemovesUnusedForbiddenETFs'`

Expected: FAIL because the exported predicate and schema-9 cleanup do not exist.

- [ ] **Step 3: Export and reuse one predicate**

Implement:

```go
func IsEligibleETFName(name string) bool {
	name = strings.TrimSpace(name)
	for _, marker := range []string{"债", "短融", "同业存单", "货币", "现金", "保证金", "理财金"} {
		if strings.Contains(name, marker) { return false }
	}
	return name != ""
}
```

Keep `visibleETF` as a small compatibility wrapper if existing providers still call it. In both `saveMarketSnapshotTx` and `UpsertResolvedInstrument`, reject or skip ETF quotes for which the predicate is false. In `ResolveInstrument`, return `不支持债券、货币或现金管理类 ETF` before upsert. In `ListInstruments`, add the same name guard so an old row cannot leak into UI while migration is pending.

Implement `purgeForbiddenETFsV9`: first fail if a forbidden instrument is referenced by user facts (`execution_events`, `trade_plans`, `watchlist_items`, `price_alert_events`), then delete its generated `market_rank_entries`, delete the instrument, and record schema version 9. Bump `CurrentSchemaVersion` to 9.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run: `go test ./internal/market ./internal/store -run 'TestNormalizeETFFilters|TestMigrationRemovesUnusedForbiddenETFs|TestMigrateIsIdempotent'`

Expected: PASS.

---

### Task 2: Return names and market metadata with portfolio positions

**Files:**
- Modify: `backend/internal/domain/portfolio.go`
- Modify: `backend/internal/store/queries.go`
- Modify: `backend/internal/service/executions.go`
- Modify: `backend/internal/service/executions_test.go`
- Modify: `src/renderer/types.ts`

**Interfaces:**
- Produces: `PositionState{Name, Market, Currency, LotSize}`.
- Consumers: holdings display, correction form, and allocation linked-source details.

- [ ] **Step 1: Add the portfolio metadata assertion to the existing service test**

Extend `TestPortfolioUsesLatestHKCloseWithConservativeRMBRate`:

```go
if position.Name != "阿里巴巴-W" || position.Market != "HK" || position.Currency != "HKD" || position.LotSize != 100 {
	t.Fatalf("missing instrument metadata: %#v", position)
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run: `go test ./internal/service -run TestPortfolioUsesLatestHKCloseWithConservativeRMBRate`

Expected: FAIL to compile because the fields are absent.

- [ ] **Step 3: Add metadata enrichment**

Add fields to `domain.PositionState`, add `Store.InstrumentsByID(ctx) (map[string]InstrumentRow, error)`, and enrich positions in `Service.Portfolio` after replay:

```go
instrument := instruments[id]
position.Name = instrument.Name
position.Market = instrument.Market
position.Currency = instrument.Currency
position.LotSize = instrument.LotSize
```

Mirror those fields in the TypeScript `Position` interface.

- [ ] **Step 4: Run the focused test and verify GREEN**

Run: `go test ./internal/service -run TestPortfolioUsesLatestHKCloseWithConservativeRMBRate`

Expected: PASS.

---

### Task 3: Add active execution history and atomic one-step correction

**Files:**
- Modify: `backend/internal/store/migrations.go`
- Modify: `backend/internal/store/executions.go`
- Modify: `backend/internal/store/queries.go`
- Modify: `backend/internal/service/executions.go`
- Modify: `backend/internal/service/executions_test.go`
- Modify: `backend/internal/api/router.go`
- Modify: `backend/internal/api/router_test.go`

**Interfaces:**
- Produces: `Service.ListActiveExecutions(ctx, instrumentID string) ([]ExecutionRecord, error)`.
- Produces: `Service.CorrectExecution(ctx, originalID, reason string, draft ExecutionDraft) (ExecutionReceipt, error)`.
- HTTP: `GET /api/executions?instrumentId=<id>` and `POST /api/executions/{id}/correct`.
- Consumers: `ExecutionHistory.vue` and `ExecutionCorrectionPanel.vue`.

- [ ] **Step 1: Write the single high-value correction service test**

The test records an unplanned 100-share buy for `-1_205_000` fen, corrects it to 80 shares and `-964_000` fen, then asserts:

```go
if receipt.Position.Quantity != 80 || receipt.CashFen != 19_036_000 {
	t.Fatalf("corrected receipt=%#v", receipt)
}
// Three events: original, reversal, replacement; one correction relation.
// Original violation/cooldown/review are superseded, replacement derivatives are current.
```

Use 80 shares deliberately to prove actual broker facts are not blocked by the 100-share lot size.

- [ ] **Step 2: Run the focused test and verify RED**

Run: `go test ./internal/service -run TestCorrectExecutionAtomicallyReplacesFactAndCurrentDisciplineState`

Expected: FAIL because `CorrectExecution` is missing and odd-lot quantity is rejected.

- [ ] **Step 3: Add correction persistence and active-history query**

Add `execution_corrections(original_execution_id UNIQUE, reversal_execution_id, replacement_execution_id, reason, created_at)` and `execution_id` on `cooldown_periods`. Refactor the existing transaction body into private `appendExecutionTx` and `appendReversalTx` helpers.

Define:

```go
type ExecutionRecord struct {
	ID string `json:"id"`
	InstrumentID string `json:"instrumentId"`
	Code string `json:"code"`
	Name string `json:"name"`
	Side string `json:"side"`
	Quantity int `json:"quantity"`
	LocalPriceTenThousandth int64 `json:"localPriceTenThousandth"`
	SettlementFen int64 `json:"settlementFen"`
	ExecutedAt time.Time `json:"executedAt"`
	Emotion domain.ExecutionEmotion `json:"emotion"`
}
```

`AppendExecutionCorrection` must begin one transaction, verify the original is active, append reversal and replacement, mark the original violation acknowledged, close its cooldown via `execution_id`, complete its pending review with a superseded note, insert the correction relation/audit, and commit only after all writes succeed.

Change `CountViolations` to count only `acknowledged_at IS NULL`; `LatestCooldown` already reads only open rows and will stop seeing the corrected original after its `actual_ends_at` is set.

- [ ] **Step 4: Refactor service validation and implement correction**

Change the universal quantity validation to `draft.Quantity <= 0`; keep lot size only as display metadata. Extract classification/input creation so normal record and correction use the same discipline rules. Before correcting, calculate portfolio state as if the original were reversed, validate a replacement sell against that state, then call the atomic store method.

Expose list/correct handlers in the router. Decode this body:

```go
var input struct {
	Reason string `json:"reason"`
	Draft service.ExecutionDraft `json:"draft"`
}
```

- [ ] **Step 5: Run focused service and API tests and verify GREEN**

Run: `go test ./internal/service ./internal/api -run 'TestCorrectExecutionAtomically|TestCorrectExecutionRoute'`

Expected: PASS.

---

### Task 4: Build holdings names, execution history, and correction UI

**Files:**
- Create: `src/renderer/components/executions/ExecutionCorrectionPanel.vue`
- Create: `src/renderer/components/executions/ExecutionHistory.vue`
- Modify: `src/renderer/views/PositionsView.vue`
- Modify: `src/renderer/views/PositionsView.spec.ts`
- Modify: `src/renderer/views/ExecutionsView.vue`
- Modify: `src/renderer/views/ExecutionsView.spec.ts`
- Modify: `src/renderer/types.ts`

**Interfaces:**
- `ExecutionHistory` props: `{ records: readonly ExecutionRecord[]; busy: boolean }`; emits `select(record)`.
- `ExecutionCorrectionPanel` props: `{ record?: ExecutionRecord; instruments: readonly Instrument[]; busy: boolean }`; emits `submit({ originalId, reason, draft })` and `cancel()`.
- `PositionsView` requests records only after a user opens correction for one holding.

- [ ] **Step 1: Extend the existing holdings UI test first**

Return a `515880` position with `name: '通信ETF国泰'` and one active execution. Assert the page shows both name and code, click `修正成交`, change quantity to `80`, fill reason, and assert `POST /api/executions/execution-1/correct` receives the typed payload.

- [ ] **Step 2: Run focused Vitest and verify RED**

Run: `pnpm vitest run src/renderer/views/PositionsView.spec.ts`

Expected: FAIL because the name and correction controls are absent.

- [ ] **Step 3: Implement focused Vue components**

Keep route views as composition surfaces. Use `shallowRef` for primitive selection/busy/error state, `reactive` for the correction form, `computed` for selected instrument and derived amount, and typed props/emits. The form accepts any positive integer quantity and displays `一手 N 股，仅作参考` rather than treating it as a blocker.

In `PositionsView`, render:

```vue
<td class="security-cell">
  <strong>{{ position.name || position.code }}</strong>
  <small>{{ position.code }}</small>
</td>
```

Add a row action that loads `GET /api/executions?instrumentId=...`. In `ExecutionsView`, load the latest active records and compose `ExecutionHistory` plus the same correction panel so closed positions remain correctable.

- [ ] **Step 4: Run the two focused view tests and verify GREEN**

Run: `pnpm vitest run src/renderer/views/PositionsView.spec.ts src/renderer/views/ExecutionsView.spec.ts`

Expected: PASS.

---

### Task 5: Link allocation calculations to portfolio and signed external adjustments

**Files:**
- Modify: `backend/internal/domain/allocation.go`
- Modify: `backend/internal/store/migrations.go`
- Modify: `backend/internal/store/allocation.go`
- Modify: `backend/internal/service/allocation.go`
- Modify: `backend/internal/store/allocation_test.go`
- Modify: `backend/internal/service/allocation_test.go`
- Modify: `backend/internal/api/router.go`

**Interfaces:**
- `AllocationItem` adds `InstrumentCodes []string`.
- `AllocationItemProgress` adds `LinkedValueFen`, `ManualAdjustmentFen`, and `LinkedPositions []PositionState`.
- `AllocationOverview` adds `UnassignedPositions []PositionState`.
- HTTP: `POST /api/allocation/items/{key}/adjustments` with `{ adjustmentFen, observedAt }`.

- [ ] **Step 1: Write the linked-allocation service test first**

Record these literal facts: Tencent market value `4_180_000`, communication ETF `520_000`, semiconductor ETF `114_000`, and available cash `12_128_550`. Append a `+10_000` external adjustment to communication. Assert Tencent, communication, semiconductor, and cash resolve from the portfolio; the communication current value is `530_000`; and an unbound position appears exactly once in `UnassignedPositions` and still contributes to total assets.

- [ ] **Step 2: Run the focused test and verify RED**

Run: `go test ./internal/service -run TestAllocationUsesPortfolioBindingsCashAndSignedAdjustments`

Expected: FAIL because binding/adjustment contracts are absent.

- [ ] **Step 3: Add binding and adjustment persistence**

Add `allocation_adjustment_events` with signed `adjustment_fen`, `source='manual_adjustment'`, timestamps, and index. Keep `allocation_value_events` unchanged/read-only for legacy history.

Seed default `InstrumentCodes` as:

```go
map[string][]string{
	"tencent": {"0700.HK"},
	"semiconductor-equipment-etf-159558": {"159558"},
	"communication-etf": {"515880"},
	"china-internet-etf": {"513050"},
}
```

When loading an older allocation JSON with no bindings, apply these defaults in memory and persist them with the next explicit revision; do not rewrite version history during read.

- [ ] **Step 4: Derive overview from one portfolio source of truth**

Call `s.Portfolio(ctx)` once. Build a `code -> itemKey` map and reject duplicate codes during allocation revision. For every position, add its market value to the bound item or to `UnassignedPositions`. Set cash from `portfolio.AvailableCashFen`. Add only the latest signed adjustment per item. Reject a new adjustment if the resulting item value would be negative.

Current total must equal:

```go
portfolio.AvailableCashFen + sum(position.MarketValueFen) + sum(latestAdjustments)
```

Legacy value events remain available from the old history API but no longer enter that formula.

- [ ] **Step 5: Run focused allocation tests and verify GREEN**

Run: `go test ./internal/store ./internal/service -run 'TestAllocationUsesPortfolioBindings|TestAllocationAdjustmentEventsAppend'`

Expected: PASS.

---

### Task 6: Present linked allocation sources and editable mappings

**Files:**
- Modify: `src/renderer/types.ts`
- Modify: `src/renderer/views/AllocationView.vue`
- Modify: `src/renderer/views/AllocationView.spec.ts`
- Modify: `src/renderer/components/allocation/AllocationSummary.vue`
- Modify: `src/renderer/components/allocation/AllocationValueForm.vue`
- Modify: `src/renderer/components/allocation/AllocationEditor.vue`

**Interfaces:**
- `AllocationView` passes eligible `Instrument[]` to `AllocationEditor`.
- `AllocationValueForm` emits `{ itemKey, adjustmentFen, observedAt }`.
- `AllocationEditor` emits the existing revision payload with `instrumentCodes` per item.

- [ ] **Step 1: Rewrite the single allocation view test around linked values**

The fixture must include `linkedValueFen`, `manualAdjustmentFen`, linked positions, and one unassigned position. Assert the page labels the sources “持仓/现金事实” and “系统外调整”, shows bound code `0700.HK`, shows `待归类持仓`, saves `+500` yuan to `/adjustments`, and saves a revised binding without any trading action.

- [ ] **Step 2: Run focused Vitest and verify RED**

Run: `pnpm vitest run src/renderer/views/AllocationView.spec.ts`

Expected: FAIL because the UI still treats manual values as complete current values.

- [ ] **Step 3: Implement the allocation UI with explicit data flow**

Load `/api/instruments` alongside allocation/versions. `AllocationSummary` renders linked fact, signed adjustment, total, bindings, and unassigned holdings. Repurpose `AllocationValueForm` copy and payload to “系统外调整”; allow signed numeric input. In `AllocationEditor`, use a compact multi-select inside each asset row/detail and prevent selecting codes already bound elsewhere by deriving available options with `computed`.

Keep props read-only and send all changes upward via typed emits; do not mutate `overview` or `instruments` props.

- [ ] **Step 4: Run focused view test and verify GREEN**

Run: `pnpm vitest run src/renderer/views/AllocationView.spec.ts`

Expected: PASS.

---

### Task 7: Integrated regression, versioning, and macOS package

**Files:**
- Modify: `package.json`
- Modify/generated by existing build: `resources/bin/discipline-server`
- Create/generated by existing package: `release/Plain Rule-0.1.14-arm64.dmg`

**Interfaces:**
- Produces: version `0.1.14` macOS Apple Silicon DMG with schema 9 backend.

- [ ] **Step 1: Inspect the combined diff and contracts**

Run: `git diff --check` and `git diff --stat`. Confirm no unrelated files were reverted and every spec acceptance criterion maps to a changed seam.

- [ ] **Step 2: Run narrow-to-broad verification**

Run:

```bash
pnpm test:go
pnpm test
pnpm typecheck
pnpm build
```

Expected: all commands exit 0 with no test failures or type errors.

- [ ] **Step 3: Bump version and build the installer**

Change `package.json` version from `0.1.13` to `0.1.14`, then run `pnpm build:mac`.

Expected: `release/Plain Rule-0.1.14-arm64.dmg` exists.

- [ ] **Step 4: Run packaged-app smoke verification**

Run: `pnpm smoke:mac`.

Expected: packaged backend health check and Electron startup smoke both pass.

- [ ] **Step 5: Report evidence without committing**

Report focused RED/GREEN results, full verification counts, DMG absolute path and SHA-256, migration safety behavior, and any manual UI state not directly exercised. Do not commit, push, sign, publish, or modify the user’s installed application.
