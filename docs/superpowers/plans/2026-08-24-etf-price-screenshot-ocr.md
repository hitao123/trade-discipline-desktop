# ETF Price and Screenshot OCR Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Filter fixed-income ETFs, correct A-share reference prices and ETF trade precision, and locally recognize a broker screenshot into an emotion-aware quick execution draft.

**Architecture:** Market-source normalization shares one ETF visibility predicate and Eastmoney requests decimal values explicitly. Executions retain the existing cents fields for compatibility while adding a ten-thousandth-yuan unit price, and the existing transaction stores three user-entered emotion scores. A sandboxed renderer asks Electron to run a bundled Swift Vision helper; a pure TypeScript parser turns recognized lines into a draft, but only the existing submit action writes it.

**Tech Stack:** Go 1.25, SQLite, Vue 3 Composition API, TypeScript, Electron IPC, Swift Vision, Vitest.

**Spec:** `docs/superpowers/specs/2026-08-24-etf-price-screenshot-ocr-design.md`

## Global Constraints

- The app remains a single-user local macOS Apple Silicon application and never connects to a broker or submits an order.
- Selected screenshots stay local, are not copied into application data, and are not stored in SQLite.
- OCR only pre-fills; portfolio and cash change only after the user clicks `立即如实入账`.
- OCR never infers emotion; fear, greed, and break-even/revenge impulse are user-entered integers from 0 through 10.
- Add only three focused regression cases; extend existing tests for persistence and UI behavior instead of adding broad new suites.

---

### Task 1: ETF ranking and reference-price correction

**Files:**
- Create: `backend/internal/market/etf_filter.go`
- Modify: `backend/internal/market/eastmoney.go`
- Modify: `backend/internal/market/sina.go`
- Modify: `backend/internal/market/csv.go`
- Modify: `backend/internal/market/market_test.go`
- Modify: `backend/internal/store/store.go`

**Interfaces:**
- Produces: `func visibleETF(name string) bool` for every ranking source.
- Produces: Eastmoney `FetchQuotes` responses with decimal query parameters and `CloseMinor = round(priceYuan * 100)`.
- Produces: schema version 8 cleanup of only stale Eastmoney A-share instrument reference prices.

- [ ] **Step 1: Replace the existing ETF truncation test with the filtering regression**

Use one mixed list containing high-turnover `短融ETF`, `科创债`, `货币ETF`, and lower-turnover equity/gold ETFs. Assert excluded names never appear, the remaining rows stay turnover-sorted, and the result contains at most ten entries.

- [ ] **Step 2: Update the quote regression to require decimal query parameters**

The HTTP fixture must assert:

```go
if query.Get("fltt") != "2" || query.Get("invt") != "2" {
    t.Fatalf("decimal quote parameters missing: %s", query.Encode())
}
```

Return `f2: 0.643` for `1.515880` and assert `CloseMinor == 64`.

- [ ] **Step 3: Run the two market regressions and confirm they fail**

Run: `go test ./internal/market -run 'TestNormalizeETF|TestFetchQuotes' -count=1`

Expected: ETF fixed-income names leak and/or the A-share quote uses the old scale.

- [ ] **Step 4: Implement one shared ETF predicate and decimal quote parsing**

Implement:

```go
func visibleETF(name string) bool {
    for _, marker := range []string{"债", "短融", "同业存单", "货币", "现金", "保证金", "理财金"} {
        if strings.Contains(strings.TrimSpace(name), marker) { return false }
    }
    return true
}
```

Call it before sorting/slicing in Eastmoney, Sina, and CSV paths. In `FetchQuotes`, set `fltt=2` and `invt=2`, then use `round(row.Price * 100)` for HK, SH, and SZ.

- [ ] **Step 5: Add schema-8 reference-cache cleanup**

On the one-time version-8 transition, clear only SH/SZ `latest_price_minor/latest_price_at` values whose source is the old `eastmoney-public-close` quote cache; keep executions, settlement costs, market snapshots, and historical bars unchanged. Record schema version 8 in the same migration transaction.

- [ ] **Step 6: Run market and store tests**

Run: `go test ./internal/market ./internal/store -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/market backend/internal/store/store.go
git commit -m "fix: correct ETF rankings and reference prices"
```

### Task 2: Precise ETF execution and emotion persistence

**Files:**
- Modify: `backend/internal/store/migrations.go`
- Modify: `backend/internal/store/store.go`
- Modify: `backend/internal/domain/execution.go`
- Modify: `backend/internal/store/executions.go`
- Modify: `backend/internal/store/post_trade_reviews.go`
- Modify: `backend/internal/service/executions.go`
- Modify: `backend/internal/service/post_trade_reviews.go`
- Modify: `backend/internal/service/post_trade_reviews_test.go`
- Modify: `src/shared/contracts.ts`
- Modify: `src/renderer/types.ts`
- Modify: `src/renderer/components/reviews/PostTradeReviewQueue.vue`

**Interfaces:**
- Consumes: existing `localPriceMinor` for backward-compatible clients.
- Produces: `localPriceTenThousandth: number` representing yuan times 10,000.
- Produces: `emotion: { fearScore: number; greedScore: number; revengeScore: number }`.
- Produces: `local_amount_minor = round(localPriceTenThousandth * quantity / 100)`.

- [ ] **Step 1: Extend the existing quick-execution service test**

Record a CNY ETF with `LocalPriceTenThousandth: 6520`, quantity `8000`, and emotions `3/1/0`. Assert settlement defaults to `-521600`, the stored precise price equals `6520`, and `emotion_json` plus the execution audit fact contain all three scores.

- [ ] **Step 2: Run that existing test and confirm it fails**

Run: `go test ./internal/service -run TestRecordQuickExecutionRequiresActualHKSettlementAndPrefillsCNYSettlement -count=1`

Expected: compile failure because the precise price and emotion fields do not exist.

- [ ] **Step 3: Add compatible schema and domain fields**

Add nullable/compatible `local_price_ten_thousandth INTEGER` to `execution_events`. During migration, backfill old rows with `local_price_minor * 100`. Add the field to insert, reversal, load, pending-review queries, and JSON types while continuing to populate `local_price_minor`.

- [ ] **Step 4: Validate and persist emotion atomically**

Define:

```go
type ExecutionEmotion struct {
    FearScore int `json:"fearScore"`
    GreedScore int `json:"greedScore"`
    RevengeScore int `json:"revengeScore"`
}
```

Reject values outside 0–10. Marshal emotion once, pass it to `AppendExecution`, include it in the execution audit `after_json`, and expose it on pending reviews.

- [ ] **Step 5: Compute the local amount from the precise price**

Prefer `LocalPriceTenThousandth` when positive. Guard multiplication overflow, then round to cents. If only `LocalPriceMinor` is present, derive the precise value as `minor * 100` to preserve old API behavior.

- [ ] **Step 6: Run service, store, and domain tests**

Run: `go test ./internal/service ./internal/store ./internal/domain -count=1`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend src/shared/contracts.ts src/renderer/types.ts src/renderer/components/reviews/PostTradeReviewQueue.vue
git commit -m "feat: preserve ETF execution precision and emotion"
```

### Task 3: Local screenshot recognition pipeline

**Files:**
- Create: `native/ocr/main.swift`
- Create: `scripts/build-ocr.mjs`
- Create: `src/renderer/lib/execution-screenshot.ts`
- Create: `src/renderer/lib/execution-screenshot.spec.ts`
- Modify: `src/main/index.ts`
- Modify: `src/preload/index.ts`
- Modify: `src/shared/global.d.ts`
- Modify: `package.json`
- Modify: `electron-builder.yml`

**Interfaces:**
- Produces: helper JSON `{ lines: Array<{ text: string; confidence: number }> }`.
- Produces: `window.discipline.recognizeExecutionScreenshot(): Promise<OCRSelection | null>`.
- Produces: `parseExecutionScreenshot(lines: OCRLine[]): ExecutionScreenshotDraft`.

- [ ] **Step 1: Write the one TypeScript OCR parser regression**

Use recognized lines representing the supplied screenshot and assert:

```ts
expect(parseExecutionScreenshot(lines)).toMatchObject({
  code: '515880', side: 'buy', quantity: 8000,
  localPrice: 0.652, settlementYuan: 5216,
  executedAt: '2026-08-24T10:34:21',
})
```

Also assert its computed gross amount is `5216`, not `5200`.

- [ ] **Step 2: Run the parser test and confirm it fails**

Run: `pnpm vitest run src/renderer/lib/execution-screenshot.spec.ts`

Expected: module not found.

- [ ] **Step 3: Implement labeled, defensive text parsing**

Prefer lines labeled `委托数量/已成交`, `成交价格`, `元`, and an exact timestamp; do not select the amount as quantity. Return `warnings: string[]` for missing fields or amount/gross differences above `max(1 yuan, gross * 0.005)`.

- [ ] **Step 4: Implement the Swift Vision helper**

Load the single path argument through `NSImage`, run `VNRecognizeTextRequest` with `.accurate`, `zh-Hans/en-US`, sort observations top-to-bottom and left-to-right, and encode only text/confidence JSON to stdout. Errors go to stderr with a nonzero exit status.

- [ ] **Step 5: Add the bounded Electron IPC bridge**

Use `dialog.showOpenDialog` for PNG/JPG/JPEG/HEIC. Invoke the helper with `execFile` (15-second timeout, 2 MiB output cap), parse JSON, and return the original basename plus lines. Never use `shell`, never copy the image, and return `null` on cancel.

- [ ] **Step 6: Build and package the helper**

Compile with `xcrun swiftc -O -framework Vision -framework AppKit`, put the binary at `resources/bin/plain-rule-ocr`, include it in `electron-builder.yml`, and add `build:ocr` before the Electron packaging build.

- [ ] **Step 7: Run parser, type, and helper checks**

Run: `pnpm vitest run src/renderer/lib/execution-screenshot.spec.ts && pnpm typecheck && pnpm build:ocr`

Expected: PASS and an executable `resources/bin/plain-rule-ocr`.

- [ ] **Step 8: Commit**

```bash
git add native scripts src/main src/preload src/shared package.json electron-builder.yml src/renderer/lib
git commit -m "feat: add local execution screenshot OCR"
```

### Task 4: Screenshot prefill and emotion-aware quick-entry UI

**Files:**
- Create: `src/renderer/components/executions/ExecutionScreenshotImport.vue`
- Modify: `src/renderer/components/executions/QuickExecutionForm.vue`
- Modify: `src/renderer/components/executions/QuickExecutionForm.spec.ts`
- Modify: `src/renderer/views/ExecutionsView.vue`
- Modify: `src/renderer/views/ExecutionsView.spec.ts`

**Interfaces:**
- Consumes: `ExecutionScreenshotDraft` from Task 3.
- Produces: `prefill` emission with matched `instrumentId` and reliable OCR fields.
- Produces: quick API payload with `localPriceTenThousandth` and `emotion`.

- [ ] **Step 1: Extend existing UI tests without adding a new suite**

Update `QuickExecutionForm.spec.ts` to apply a 515880 prefill, enter fear/greed/revenge `3/1/0`, and assert the emitted payload contains exact price `6520`, settlement `-521600`, and emotion. In `ExecutionsView.spec.ts`, stub `recognizeExecutionScreenshot` to return the supplied screenshot lines, click `从成交截图识别`, and assert `/api/executions/quick` has not been requested before `立即如实入账` is clicked.

- [ ] **Step 2: Run the two existing UI suites and confirm they fail**

Run: `pnpm vitest run src/renderer/components/executions/QuickExecutionForm.spec.ts src/renderer/views/ExecutionsView.spec.ts`

Expected: new prefill/emotion assertions fail.

- [ ] **Step 3: Build the screenshot import component**

Show a `从成交截图识别` button, local-processing copy, extracted summary, and warnings. Match instruments by normalized six-digit code. Emit only recognized values; unknown instruments remain unselected and visibly require manual choice.

- [ ] **Step 4: Make the quick form accept one-shot prefill**

Watch a replaced `prefill` object and assign code-matched instrument, side, quantity, `localPrice`, settlement, and local `datetime-local` value. Use price step `0.001` for ETFs and `0.01` for stocks. Compute displayed/default amount as `round(localPrice * quantity * 100)`.

- [ ] **Step 5: Add compact user-entered emotion controls**

Add three numeric inputs labeled `恐惧 0–10`, `贪婪 0–10`, and `回本/报复性冲动 0–10`, defaulting to zero. Submit them in `emotion`; do not populate them from OCR.

- [ ] **Step 6: Wire orchestration and verify UI tests**

Place screenshot import above the quick form in `ExecutionsView`, pass its prefill to the form, and keep save handling unchanged. Run: `pnpm vitest run src/renderer/components/executions/QuickExecutionForm.spec.ts src/renderer/views/ExecutionsView.spec.ts`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add src/renderer/components/executions src/renderer/views/ExecutionsView.vue src/renderer/views/ExecutionsView.spec.ts
git commit -m "feat: prefill quick records from broker screenshots"
```

### Task 5: Focused release verification

**Files:**
- Modify: `docs/验收清单.md`
- Modify: `README.md`

**Interfaces:**
- Consumes: all prior tasks.
- Produces: user-facing local OCR/privacy and ETF precision instructions.

- [ ] **Step 1: Run formatting and full verification**

Run: `gofmt -w` only on changed Go files, then `pnpm verify`.

Expected: all existing and three focused regressions pass; renderer, backend, and Electron builds succeed.

- [ ] **Step 2: Test the supplied screenshot through the real helper**

Run the built helper against `/var/folders/ng/x9xtjk252m104rhvj94rcrxh0000gn/T/codex-clipboard-80b97f20-c1ae-4dc7-b6a3-c325f97e58a6.png`, feed its lines to the parser, and confirm the six expected fields. No image or OCR output is copied into the repository.

- [ ] **Step 3: Package and smoke the macOS app**

Run: `pnpm build:mac && pnpm smoke:mac`

Expected: arm64 DMG builds, packaged OCR helper exists under `Resources/bin`, and the application reaches its ready endpoint without sidecar failure.

- [ ] **Step 4: Update concise user documentation**

Document that OCR is local and prefill-only, emotions are manually entered, CNY ETF prices accept three decimals, and bond/money ETFs are excluded from top-ten rankings.

- [ ] **Step 5: Review the final diff and commit**

Run: `git diff --check && git status --short`.

```bash
git add README.md docs/验收清单.md
git commit -m "docs: document local screenshot quick records"
```
