# 极速成交补录 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让券商成交后的极简补录立即进入正式持仓与现金账本，并把纪律说明留给可追踪的事后复盘。

**Architecture:** 新表单调用独立的快速成交 API，但该 API 必须复用现有不可变成交、违规、冷静期、审计及冲正链路。成交后复盘和港股参考汇率为独立观察记录，绝不修改实际人民币结算。

**Tech Stack:** Go 1.25、SQLite（modernc）、Vue 3、TypeScript、Vitest。

**Spec:** `docs/superpowers/specs/2026-08-23-quick-record-data-health-design.md`

## Global Constraints

- 保存即更新持仓、现金和盈亏；不得产生待确认影子账本。
- 无计划/违规成交必须如实产生既有违规与冷静期。
- 已入账成交只能追加冲正，不能编辑、删除。
- 港股现金以券商实际人民币扣款/到账为准；公开汇率仅供复盘。
- 不接入券商、不发送订单。

---

### Task 1: Schema 7 的成交后复盘和汇率观察

**Files:**
- Modify: `backend/internal/store/migrations.go`, `backend/internal/store/store.go`, `backend/internal/store/executions.go`, `backend/internal/store/store_test.go`
- Create: `backend/internal/store/post_trade_reviews.go`, `backend/internal/store/post_trade_reviews_test.go`

**Interfaces:**
- `CurrentSchemaVersion = 7`.
- `post_trade_reviews(id, execution_id UNIQUE, status CHECK(pending|completed), note, created_at, completed_at)`.
- `execution_fx_observations(id, execution_id, base_currency, quote_currency, rate_minor, source, source_time, observed_at)`.
- `store.AppendExecutionInput.QuickRecord bool` creates the pending review in the same transaction.

- [ ] **Step 1: Write failing store tests**

```go
func TestQuickExecutionCreatesPendingReviewAtomically(t *testing.T) {
    result, err := db.AppendExecution(ctx, store.AppendExecutionInput{Event: buy, QuickRecord: true})
    review, err := db.PostTradeReview(ctx, result.ExecutionID)
    if err != nil || review.Status != "pending" { t.Fatal(err) }
}
```

Create a schema-6 fixture and assert migration preserves old executions and adds both new tables.

- [ ] **Step 2: Run test to verify failure**

Run: `cd backend && go test ./internal/store -run 'TestQuickExecutionCreatesPendingReviewAtomically|TestMigrationAddsPostTradeReview' -count=1`  
Expected: FAIL because schema 7 and review APIs do not exist.

- [ ] **Step 3: Implement migration and atomic store writes**

Add foreign keys to `execution_events(id)`, UTC RFC3339 timestamps and `pending/completed` constraints. Insert the pending review in the transaction already used by `AppendExecution`; never mutate `execution_events` after save.

- [ ] **Step 4: Add query/update tests and verify**

Test period query, non-empty completion note, empty-note rejection, and an HKD/CNY observation that leaves `settlement_fen` unchanged.

Run: `cd backend && go test ./internal/store -count=1`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/store/migrations.go backend/internal/store/store.go backend/internal/store/executions.go backend/internal/store/post_trade_reviews.go backend/internal/store/store_test.go backend/internal/store/post_trade_reviews_test.go && git commit -m "feat: store post-trade reviews"
```

### Task 2: 快速成交服务、复盘 API 与周复盘拦截

**Files:**
- Modify: `backend/internal/service/executions.go`, `backend/internal/service/reviews.go`, `backend/internal/api/router.go`, `backend/internal/api/router_test.go`, `backend/internal/service/executions_test.go`
- Create: `backend/internal/service/post_trade_reviews.go`, `backend/internal/service/post_trade_reviews_test.go`

**Interfaces:**
- `QuickExecutionDraft{InstrumentID, Side, Quantity, LocalPriceMinor, SettlementFen, PlanID?, ExecutedAt?, BrokerReference?}`.
- `RecordQuickExecution(ctx, draft) (ExecutionReceipt, error)` and `CompletePostTradeReview(ctx, executionID, note)`.
- `POST /api/executions/quick`, `GET /api/post-trade-reviews`, `POST /api/post-trade-reviews/{executionID}/complete`.

- [ ] **Step 1: Write failing service/API tests**

```go
receipt, err := svc.RecordQuickExecution(ctx, QuickExecutionDraft{InstrumentID: "hk-9988", Side: "buy", Quantity: 100, LocalPriceMinor: 12_000, SettlementFen: -1_205_000})
if err != nil || receipt.Position.Quantity != 100 || receipt.CashFen != 18_795_000 { t.Fatalf("%#v %v", receipt, err) }
```

Assert no-plan quick save produces a violation, cooldown and pending review. Assert missing HK settlement returns concise Chinese copy.

- [ ] **Step 2: Run test to verify failure**

Run: `cd backend && go test ./internal/service ./internal/api -run 'TestRecordQuickExecution|TestPostTradeReview' -count=1`  
Expected: FAIL because quick APIs do not exist.

- [ ] **Step 3: Implement through the formal execution path**

Calculate `LocalAmountMinor = LocalPriceMinor * int64(Quantity)` with overflow validation; default time to `s.now()`. Delegate lot size, sell quantity, rule checks, violations, cooldown, audit and portfolio replay to the existing recording path; only add the quick-record store flag.

- [ ] **Step 4: Gate weekly review**

Before `SaveWeeklyReview`, query pending quick reviews within the submitted period. Return `PENDING_POST_TRADE_REVIEW` with a count until every item has a non-empty completion note; existing completed/manual reviews remain valid.

- [ ] **Step 5: Verify and commit**

Run: `cd backend && go test ./internal/service ./internal/api -count=1`  
Expected: PASS.

```bash
git add backend/internal/service/executions.go backend/internal/service/post_trade_reviews.go backend/internal/service/reviews.go backend/internal/api/router.go backend/internal/api/router_test.go backend/internal/service/executions_test.go backend/internal/service/post_trade_reviews_test.go && git commit -m "feat: record quick executions"
```

### Task 3: 极简成交界面与即时回执

**Files:**
- Create: `src/renderer/components/executions/QuickExecutionForm.vue`, `src/renderer/components/executions/QuickExecutionForm.spec.ts`
- Modify: `src/renderer/views/ExecutionsView.vue`, `src/renderer/views/ExecutionsView.spec.ts`, `src/renderer/types.ts`

**Interfaces:**
- `QuickExecutionForm` emits instrument, side, quantity, local price, actual settlement plus optional plan/time/reference.
- `ExecutionForm` remains the full-detail route.

- [ ] **Step 1: Write failing component tests**

```ts
await fireEvent.update(screen.getByLabelText('成交均价'), '480')
expect(screen.getByText('本币成交额 ¥48,000.00')).toBeTruthy()
await fireEvent.click(screen.getByRole('button', { name: '立即如实入账' }))
expect(request).toHaveBeenCalledWith('/api/executions/quick', expect.objectContaining({ method: 'POST' }))
```

Test CNY settlement prefill, required HK actual RMB settlement, no-plan save, and violation/cooldown receipt.

- [ ] **Step 2: Run test to verify failure**

Run: `pnpm vitest run src/renderer/components/executions/QuickExecutionForm.spec.ts src/renderer/views/ExecutionsView.spec.ts`  
Expected: FAIL because the quick form does not exist.

- [ ] **Step 3: Implement compact form**

Use buy/sell controls, lot-size default, auto-calculated read-only local amount, and default local time. Put plan association, broker number, exit evidence and explicit timestamp in collapsed “稍后补充”. Disable only while saving; no plan must not block factual recording.

- [ ] **Step 4: Reuse receipt/correction flow and verify**

Show existing cash/position/classification receipt plus “已加入待复盘”。Keep reason-required append reversal for both quick and full records.

Run: `pnpm test && pnpm typecheck`  
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/renderer/components/executions/QuickExecutionForm.vue src/renderer/components/executions/QuickExecutionForm.spec.ts src/renderer/views/ExecutionsView.vue src/renderer/views/ExecutionsView.spec.ts src/renderer/types.ts && git commit -m "feat: add quick execution record form"
```

### Task 4: 待复盘队列、参考汇率与发布验证

**Files:**
- Create: `src/renderer/components/reviews/PostTradeReviewQueue.vue`, `src/renderer/components/reviews/PostTradeReviewQueue.spec.ts`
- Modify: `src/renderer/views/WeeklyReviewView.vue`, `src/renderer/views/WeeklyReviewView.spec.ts`, `README.md`, `docs/验收清单.md`

**Interfaces:**
- `PostTradeReviewQueue` emits `complete({ executionId, note })`.
- Weekly review loads current-period pending items before submission.

- [ ] **Step 1: Write failing weekly-review tests**

```ts
expect(await screen.findByText('本周有 1 笔待纪律复盘')).toBeTruthy()
await fireEvent.update(screen.getByLabelText('事后说明'), '冲动追涨，未按计划等待')
await fireEvent.click(screen.getByRole('button', { name: '完成复盘' }))
expect(request).toHaveBeenCalledWith('/api/post-trade-reviews/execution-1/complete', expect.anything())
```

- [ ] **Step 2: Run test to verify failure**

Run: `pnpm vitest run src/renderer/components/reviews/PostTradeReviewQueue.spec.ts src/renderer/views/WeeklyReviewView.spec.ts`  
Expected: FAIL because the queue is absent.

- [ ] **Step 3: Implement queue and FX-reference display**

Render pending quick records before the weekly form. Completion sends execution ID and note. Show an available HKD/CNY observation only as “参考汇率”，never as cash or settlement.

- [ ] **Step 4: Full verification, docs and commit**

Run: `pnpm test:go && pnpm test && pnpm typecheck && pnpm build && pnpm smoke:mac`  
Expected: all commands exit 0 and no fixture opens a broker connection.

Update documentation for actual-settlement priority, pending reviews, append-only correction and schema 7.

```bash
git add src/renderer/components/reviews/PostTradeReviewQueue.vue src/renderer/components/reviews/PostTradeReviewQueue.spec.ts src/renderer/views/WeeklyReviewView.vue src/renderer/views/WeeklyReviewView.spec.ts README.md docs/验收清单.md && git commit -m "feat: review quick executions"
```
