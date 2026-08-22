# Optional Tencent Sequence Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Tencent sequence gate optional and disabled by default, while allowing the user to configure it in a new immutable rule version.

**Architecture:** Add one persisted boolean to the rule snapshot. The rules engine evaluates the existing 20-day / 90-score condition only when that boolean is enabled. The existing settings view owns a small reactive draft, copies the current snapshot when saving, and exposes the toggle plus its two thresholds.

**Tech Stack:** Go, SQLite JSON rule snapshots, Vue 3 `<script setup>`, TypeScript, Vitest.

## Global Constraints

- Existing plans and executions remain unchanged; only future validation uses the latest rule snapshot.
- A missing boolean in an existing local `rule-1` JSON snapshot means the gate is disabled.
- Rules remain append-only: settings create a new version with a non-empty reason.
- No network, broker, or data-source behavior changes.

---

### Task 1: Default rule does not block Tencent

**Files:**
- Modify: `backend/internal/rules/engine_test.go`
- Modify: `backend/internal/rules/snapshot.go`
- Modify: `backend/internal/rules/engine.go`

**Interfaces:**
- Produces: `Snapshot.EnforceTencentSequenceGate bool`
- Consumes: `EvaluatePlan(now, rule, portfolio, draft) Decision`

- [ ] **Step 1: Write the failing test**

```go
func TestDefaultRuleDoesNotBlockTencentSequence(t *testing.T) {
	state := emptyPortfolio()
	state.AlibabaObservationTradingDays = 0
	state.DisciplineScoreBP = 0
	got := EvaluatePlan(fixedNow, initialRule(), state, validTencentPlan())
	if !got.Qualified { t.Fatalf("unexpected decision: %#v", got) }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/rules -run TestDefaultRuleDoesNotBlockTencentSequence -count=1`

Expected: fails because the current default snapshot always emits `TENCENT_SEQUENCE_GATE`.

- [ ] **Step 3: Write minimal implementation**

```go
EnforceTencentSequenceGate bool `json:"enforceTencentSequenceGate"`

if rule.EnforceTencentSequenceGate && plan.Code == "0700.HK" && (...) {
	add("TENCENT_SEQUENCE_GATE", "instrumentId", "...")
}
```

Keep `InitialSnapshot().EnforceTencentSequenceGate` false. Set the field true in the existing gate-specific test.

- [ ] **Step 4: Run focused Go tests**

Run: `go test ./internal/rules -count=1`

Expected: default Tencent plan is qualified; enabled-gate test still rejects incomplete progress.

### Task 2: Expose and persist the optional setting

**Files:**
- Modify: `src/renderer/types.ts`
- Modify: `src/renderer/views/SettingsView.vue`
- Modify: `src/renderer/views/SettingsView.spec.ts`

**Interfaces:**
- Consumes: `RuleVersion.snapshot.enforceTencentSequenceGate`, `tencentObservationDays`, `minimumDisciplineScoreBP`
- Produces: `POST /api/rules/versions` body containing the edited snapshot and the existing non-empty reason.

- [ ] **Step 1: Write the failing user-facing test**

```ts
it('creates a rule version with the Tencent gate disabled by default', async () => {
	render(SettingsView)
	await fireEvent.update(screen.getByLabelText('修改原因'), '腾讯顺序门槛改为可选提醒')
	await fireEvent.click(screen.getByRole('button', { name: '创建规则新版本' }))
	const call = request.mock.calls.find(([path]) => path === '/api/rules/versions')
	expect(JSON.parse(String(call?.[1]?.body)).snapshot.enforceTencentSequenceGate).toBe(false)
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm exec vitest run src/renderer/views/SettingsView.spec.ts`

Expected: the setting is absent from the saved snapshot.

- [ ] **Step 3: Write minimal implementation**

Add `enforceTencentSequenceGate`, `tencentObservationDays`, and `minimumDisciplineScore` to the local settings draft. Load them from the current snapshot with defaults `false`, `20`, and `90`; render a checkbox plus two numeric inputs only when enabled. On submit, persist the boolean and convert displayed discipline score to basis points.

- [ ] **Step 4: Run focused Vue tests**

Run: `pnpm exec vitest run src/renderer/views/SettingsView.spec.ts`

Expected: the user can create a new rule version with the gate disabled, with a required reason.

### Task 3: Integrate and package

**Files:**
- Modify: `README.md`
- Modify: `package.json`
- Generated: `resources/bin/discipline-server`
- Generated: `release/Plain Rule-0.1.2-arm64.dmg`

- [ ] **Step 1: Document the change and increment the patch version**

Describe the Tencent sequence gate as optional and reference the 0.1.2 installation artifact.

- [ ] **Step 2: Run full verification**

Run: `pnpm verify`

Expected: Go tests, Vitest, TypeScript checking, and production build exit 0.

- [ ] **Step 3: Package and smoke-test macOS build**

Run: `pnpm build:mac`, `pnpm smoke:mac`, and `hdiutil verify release/Plain\ Rule-0.1.2-arm64.dmg`

Expected: a valid arm64 DMG that starts with isolated local data.
