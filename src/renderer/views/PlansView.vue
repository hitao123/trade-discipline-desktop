<script setup lang="ts">
import { computed, onMounted, shallowRef } from 'vue'

import ErrorNotice from '@/renderer/components/ErrorNotice.vue'
import PageHeader from '@/renderer/components/PageHeader.vue'
import PlanForm from '@/renderer/components/plans/PlanForm.vue'
import PreTradeConfirmation from '@/renderer/components/plans/PreTradeConfirmation.vue'
import { APIError, api } from '@/renderer/lib/api'
import type { Instrument, PlanRecord } from '@/renderer/types'

const instruments = shallowRef<Instrument[]>([])
const plans = shallowRef<PlanRecord[]>([])
const selectedPlan = shallowRef<PlanRecord>()
const busy = shallowRef(false)
const error = shallowRef('')
const fieldErrors = shallowRef<Record<string, string>>({})
const editingPlan = shallowRef<PlanRecord>()
const revisionReason = shallowRef('')
const confirmationPlan = shallowRef<PlanRecord>()
const confirmationStartedAt = shallowRef('')
const confirmationBusy = shallowRef(false)
const confirmationError = shallowRef('')
const confirmationNotice = shallowRef('')
const editorMode = shallowRef<'list' | 'create' | 'revise'>('list')
const draftPreview = shallowRef<Record<string, unknown>>()
const dashboard = shallowRef<{ cooldown?: { reason: string; expectedEndsAt: string } }>()
const planDraftKey = 'plain-rule:plan-draft:v1'
const editorVisible = computed(() => editorMode.value !== 'list')

async function load() {
  try {
    [instruments.value, plans.value] = await Promise.all([
      api.request<Instrument[]>('/api/instruments'),
      api.request<PlanRecord[]>('/api/plans'),
    ])
  }
  catch (cause) {
    error.value = cause instanceof Error ? cause.message : '计划数据加载失败'
  }
  try { dashboard.value = await api.request('/api/dashboard') }
  catch { /* The persisted plan still renders when the optional current-status summary is unavailable. */ }
}

async function savePlan(payload: Record<string, unknown>) {
  busy.value = true
  error.value = ''
  fieldErrors.value = {}
  try {
    const plan = editingPlan.value
      ? await api.request<PlanRecord>(`/api/plans/${editingPlan.value.id}`, { method: 'PUT', body: JSON.stringify({ reason: revisionReason.value, draft: payload }) })
      : await api.request<PlanRecord>('/api/plans', { method: 'POST', body: JSON.stringify(payload) })
    selectedPlan.value = plan
    plans.value = editingPlan.value ? plans.value.map(item => item.id === plan.id ? plan : item) : [plan, ...plans.value]
    const wasNewPlan = !editingPlan.value
    editingPlan.value = undefined
    editorMode.value = 'list'
    if (wasNewPlan) {
      try { window.localStorage.removeItem(planDraftKey) }
      catch { /* Draft cleanup must not turn a saved, audited plan into an error. */ }
    }
    revisionReason.value = ''
    confirmationPlan.value = undefined
  }
  catch (cause) {
    error.value = cause instanceof Error ? cause.message : '计划保存失败'
    if (cause instanceof APIError) fieldErrors.value = cause.fields
  }
  finally {
    busy.value = false
  }
}

function startPreTradeConfirmation(plan: PlanRecord) {
  confirmationPlan.value = plan
  confirmationStartedAt.value = new Date().toISOString()
  confirmationError.value = ''
  confirmationNotice.value = ''
}

async function confirmPreTrade(payload: { planId: string; startedAt: string; noFomo: boolean; noLossRecovery: boolean; noAveragingDown: boolean }) {
  confirmationBusy.value = true
  confirmationError.value = ''
  try {
    await api.request(`/api/plans/${payload.planId}/pre-trade-confirmations`, { method: 'POST', body: JSON.stringify(payload) })
    confirmationPlan.value = undefined
    confirmationNotice.value = '已准备去券商执行；开仓前确认已写入本地记录。'
    error.value = ''
  }
  catch (cause) {
    confirmationError.value = cause instanceof Error ? cause.message : '开仓前确认记录失败'
  }
  finally {
    confirmationBusy.value = false
  }
}

function startRevision(plan: PlanRecord) {
  editingPlan.value = plan
  selectedPlan.value = plan
  revisionReason.value = ''
  editorMode.value = 'revise'
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

function startCreate() {
  editingPlan.value = undefined
  selectedPlan.value = undefined
  revisionReason.value = ''
  editorMode.value = 'create'
}

function closeEditor() {
  editingPlan.value = undefined
  revisionReason.value = ''
  editorMode.value = 'list'
}

function executionState(plan: PlanRecord) {
  if (plan.status !== 'qualified') return '创建时未通过校验'
  if (dashboard.value?.cooldown && new Date(dashboard.value.cooldown.expectedEndsAt).getTime() > Date.now()) return `当前受冷静期限制：${dashboard.value.cooldown.reason}`
  if (plan.draft.validUntil && new Date(String(plan.draft.validUntil)).getTime() <= Date.now()) return '当前已过有效期'
  return '当前可进入开仓前确认'
}

onMounted(load)
</script>

<template>
  <div>
    <PageHeader eyebrow="PLANS" title="交易计划" description="先写计划，再谈买入。计划可以被拒绝，但不能被无痕覆盖。系统不向券商发送指令。"><button class="button button--primary" type="button" @click="startCreate">新建计划</button></PageHeader>
    <ErrorNotice :message="error" />
    <p v-if="confirmationNotice" class="confirmation-notice" role="status">{{ confirmationNotice }}</p>
    <div v-if="editorVisible" class="plan-layout">
      <div class="plan-editor">
        <div v-if="editingPlan" class="revision-bar">
          <div><strong>正在修订 {{ editingPlan.draft.code }}</strong><span>旧内容会保留，必须写修改原因</span></div>
          <label class="field"><span>修改原因</span><input v-model="revisionReason" placeholder="例如：补充最新财报证据" required /></label>
          <button class="button" type="button" @click="closeEditor">返回计划列表</button>
        </div>
        <div v-else class="editor-heading"><div><p class="kicker">NEW PLAN</p><h2>按步骤填写计划</h2><span>输入会保存为本地草稿；只有最后保存并校验才会创建正式记录。</span></div><button class="button" type="button" @click="closeEditor">返回计划列表</button></div>
        <PlanForm :key="editingPlan?.id ?? 'new'" :instruments="instruments" :busy="busy" :submit-disabled="!!editingPlan && !revisionReason.trim()" :field-errors="fieldErrors" :initial-draft="editingPlan?.draft" :draft-storage-key="editingPlan ? undefined : planDraftKey" :submit-label="editingPlan ? '保存修订并重新校验' : '保存并校验'" @change="draftPreview = $event" @submit="savePlan" />
      </div>
      <aside class="decision-panel">
        <template v-if="selectedPlan || draftPreview">
          <p class="kicker">动态校验摘要</p>
          <h2 :class="selectedPlan?.status === 'qualified' ? 'qualified' : 'rejected'">{{ selectedPlan ? (selectedPlan.status === 'qualified' ? '创建时校验通过' : '创建时校验未通过') : '填写中，尚未校验' }}</h2>
          <p v-if="selectedPlan" class="decision-panel__meta">规则版本 {{ selectedPlan.ruleVersionId }} · {{ executionState(selectedPlan) }}</p>
          <p v-else class="decision-panel__meta">证券：{{ draftPreview?.instrumentId ? '已选择' : '待选择' }} · 有效期：{{ draftPreview?.validUntil ? new Date(String(draftPreview.validUntil)).toLocaleString('zh-CN') : '待填写' }}</p>
          <ul v-if="selectedPlan?.validation.findings.length">
            <li v-for="finding in selectedPlan?.validation.findings ?? []" :key="finding.code"><strong>{{ finding.code }}</strong><span>{{ finding.message }}</span></li>
          </ul>
          <p v-else class="calm-note">填写完成后才会按当前规则校验。通过不代表应立即买入，仍需进行开仓前确认。</p>
        </template>
        <template v-else>
          <p class="kicker">校验记录</p>
          <h2>拒绝也是有效记录</h2>
          <p class="calm-note">非整手、资金超限、浮亏加仓和跨标的摊平都会保留明确原因。</p>
          <p class="decision-panel__count">已有 {{ plans.length }} 条计划</p>
        </template>
      </aside>
    </div>
    <section v-if="selectedPlan && !editorVisible" class="saved-decision" aria-live="polite">
      <div><p class="kicker">刚刚完成的校验</p><h2 :class="selectedPlan.status === 'qualified' ? 'qualified' : 'rejected'">{{ selectedPlan.status === 'qualified' ? '创建时校验通过' : '创建时校验未通过' }}</h2><span>规则版本 {{ selectedPlan.ruleVersionId }} · {{ executionState(selectedPlan) }}</span></div>
      <ul v-if="selectedPlan.validation.findings.length"><li v-for="finding in selectedPlan.validation.findings" :key="finding.code"><strong>{{ finding.code }}</strong><span>{{ finding.message }}</span></li></ul>
      <p v-else>资格通过不代表应立即买入；仍需完成开仓前确认。</p>
    </section>
    <PreTradeConfirmation v-if="confirmationPlan" :plan="confirmationPlan" :started-at="confirmationStartedAt" :busy="confirmationBusy" :error="confirmationError" @confirm="confirmPreTrade" />
    <section class="plan-history">
      <div class="section-title"><div><p class="kicker">PLAN HISTORY</p><h2>计划记录</h2></div><span>{{ plans.length }} 条</span></div>
      <div v-if="plans.length" class="plan-list">
        <article v-for="plan in plans" :key="plan.id">
          <div><strong>{{ plan.draft.code }}</strong><span :class="plan.status">{{ plan.status === 'qualified' ? '合格' : '拒绝' }}</span></div>
          <p>{{ plan.draft.thesis || '未填写买入逻辑' }}</p>
          <small>{{ plan.draft.quantity }} 股 · 规则 {{ plan.ruleVersionId }} · 有效至 {{ plan.draft.validUntil ? new Date(String(plan.draft.validUntil)).toLocaleString('zh-CN') : '未填写' }}</small>
          <small class="execution-state">{{ executionState(plan) }}</small>
          <button class="button" type="button" @click="startRevision(plan)">修订（保留原记录）</button>
          <button v-if="plan.status === 'qualified'" class="button button--primary" type="button" @click="startPreTradeConfirmation(plan)">开始开仓前确认</button>
        </article>
      </div>
      <p v-else class="calm-note">尚无计划记录。</p>
    </section>
  </div>
</template>

<style scoped>
.plan-layout { display: grid; grid-template-columns: minmax(0, 1fr) 300px; gap: 28px; align-items: start; margin-bottom: 32px; }.plan-editor { min-width: 0; }
.decision-panel { position: sticky; top: 28px; padding: 22px; border-top: 3px solid var(--ink); background: var(--paper-deep); }
.kicker { margin: 0 0 12px; color: var(--ink-faint); font-size: 11px; letter-spacing: .12em; }
.decision-panel h2 { margin: 0; font-family: var(--font-serif); font-size: 25px; font-weight: 500; }
.decision-panel h2.rejected { color: var(--accent); }
.decision-panel h2.qualified { color: #55644d; }
.decision-panel__meta, .decision-panel__count { color: var(--ink-faint); font-size: 12px; }
.decision-panel ul { display: grid; gap: 12px; padding: 0; list-style: none; }
.decision-panel li { display: grid; gap: 4px; padding-top: 12px; border-top: 1px solid var(--line); }
.decision-panel li strong { color: var(--accent); font-size: 10px; }
.decision-panel li span, .calm-note { color: var(--ink-muted); font-size: 13px; line-height: 1.65; }
.revision-bar { display: grid; grid-template-columns: minmax(180px, .8fr) minmax(260px, 1.2fr) auto; gap: 14px; align-items: end; margin-bottom: 18px; padding: 16px; border-left: 3px solid var(--accent); background: var(--paper-deep); }
.editor-heading { display: flex; align-items: start; justify-content: space-between; gap: 18px; margin-bottom: 18px; padding: 16px; border-top: 3px solid var(--ink); background: var(--paper-deep); }.editor-heading h2 { margin: 0; font-family: var(--font-serif); font-size: 24px; font-weight: 500; }.editor-heading span { display: block; margin-top: 5px; color: var(--ink-muted); font-size: 13px; }
.revision-bar > div { display: grid; gap: 4px; }.revision-bar span { color: var(--ink-muted); font-size: 12px; }.revision-bar .field { margin: 0; }
.plan-history { margin-top: 36px; padding-top: 24px; border-top: 1px solid var(--line); }.section-title { display: flex; align-items: end; justify-content: space-between; }.section-title h2 { margin: 0; font-family: var(--font-serif); font-size: 25px; font-weight: 500; }.section-title > span { color: var(--ink-faint); font-size: 12px; }
.plan-list { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 12px; margin-top: 16px; }.plan-list article { display: grid; gap: 10px; padding: 16px; border: 1px solid var(--line); }.plan-list article > div { display: flex; justify-content: space-between; }.plan-list article p { min-height: 42px; margin: 0; color: var(--ink-muted); font-size: 13px; line-height: 1.6; }.plan-list article small { color: var(--ink-faint); }.plan-list .qualified { color: #55644d; }.plan-list .rejected { color: var(--accent); }
.execution-state { color: var(--ink-muted) !important; }
.saved-decision { display: grid; grid-template-columns: minmax(200px, .7fr) 1.3fr; gap: 18px; margin: 0 0 28px; padding: 18px; border-top: 3px solid var(--ink); background: var(--paper-deep); }.saved-decision h2 { margin: 0; font-family: var(--font-serif); font-size: 22px; font-weight: 500; }.saved-decision h2.rejected { color: var(--accent); }.saved-decision h2.qualified { color: var(--success); }.saved-decision span, .saved-decision p { color: var(--ink-muted); font-size: 13px; }.saved-decision ul { display: grid; gap: 8px; margin: 0; padding: 0; list-style: none; }.saved-decision li { display: grid; gap: 2px; padding-top: 8px; border-top: 1px solid var(--line); font-size: 13px; }.saved-decision li strong { color: var(--accent); font-size: 11px; }
.confirmation-notice { margin: 0 0 16px; color: var(--success); font-size: 13px; }
@media (max-width: 900px) { .revision-bar { grid-template-columns: 1fr; } }
</style>
