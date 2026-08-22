<script setup lang="ts">
import { onMounted, shallowRef } from 'vue'

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
    editingPlan.value = undefined
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
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

onMounted(load)
</script>

<template>
  <div>
    <PageHeader eyebrow="PLANS" title="先写计划，再谈买入" description="计划可以被拒绝，但不能被无痕覆盖。系统只负责纪律校验，不会向券商发送任何指令。" />
    <ErrorNotice :message="error" />
    <p v-if="confirmationNotice" class="confirmation-notice" role="status">{{ confirmationNotice }}</p>
    <div class="plan-layout">
      <div>
        <div v-if="editingPlan" class="revision-bar">
          <div><strong>正在修订 {{ editingPlan.draft.code }}</strong><span>旧内容会保留，必须写修改原因</span></div>
          <label class="field"><span>修改原因</span><input v-model="revisionReason" placeholder="例如：补充最新财报证据" required /></label>
          <button class="button" type="button" @click="editingPlan = undefined; revisionReason = ''">取消修订</button>
        </div>
        <PlanForm :key="editingPlan?.id ?? 'new'" :instruments="instruments" :busy="busy" :submit-disabled="!!editingPlan && !revisionReason.trim()" :field-errors="fieldErrors" :initial-draft="editingPlan?.draft" :submit-label="editingPlan ? '保存修订并重新校验' : '保存并校验'" @submit="savePlan" />
        <PreTradeConfirmation v-if="confirmationPlan" :plan="confirmationPlan" :started-at="confirmationStartedAt" :busy="confirmationBusy" :error="confirmationError" @confirm="confirmPreTrade" />
      </div>
      <aside class="decision-panel">
        <template v-if="selectedPlan">
          <p class="kicker">本次校验</p>
          <h2 :class="selectedPlan.status === 'qualified' ? 'qualified' : 'rejected'">{{ selectedPlan.status === 'qualified' ? '计划合格' : '计划被拒绝' }}</h2>
          <p class="decision-panel__meta">规则版本 {{ selectedPlan.ruleVersionId }}</p>
          <ul v-if="selectedPlan.validation.findings.length">
            <li v-for="finding in selectedPlan.validation.findings" :key="finding.code"><strong>{{ finding.code }}</strong><span>{{ finding.message }}</span></li>
          </ul>
          <p v-else class="calm-note">资格通过不代表应该立即买入。去券商交易前，再读一次逻辑破坏条件。</p>
        </template>
        <template v-else>
          <p class="kicker">校验记录</p>
          <h2>拒绝也是有效记录</h2>
          <p class="calm-note">非整手、资金超限、浮亏加仓和跨标的摊平都会保留明确原因。</p>
          <p class="decision-panel__count">已有 {{ plans.length }} 条计划</p>
        </template>
      </aside>
    </div>
    <section class="plan-history">
      <div class="section-title"><div><p class="kicker">PLAN HISTORY</p><h2>计划记录</h2></div><span>{{ plans.length }} 条</span></div>
      <div v-if="plans.length" class="plan-list">
        <article v-for="plan in plans" :key="plan.id">
          <div><strong>{{ plan.draft.code }}</strong><span :class="plan.status">{{ plan.status === 'qualified' ? '合格' : '拒绝' }}</span></div>
          <p>{{ plan.draft.thesis || '未填写买入逻辑' }}</p>
          <small>{{ plan.draft.quantity }} 股 · 规则 {{ plan.ruleVersionId }}</small>
          <button class="button" type="button" @click="startRevision(plan)">修订（保留原记录）</button>
          <button v-if="plan.status === 'qualified'" class="button button--primary" type="button" @click="startPreTradeConfirmation(plan)">开始开仓前确认</button>
        </article>
      </div>
      <p v-else class="calm-note">尚无计划记录。</p>
    </section>
  </div>
</template>

<style scoped>
.plan-layout { display: grid; grid-template-columns: minmax(0, 1fr) 300px; gap: 28px; align-items: start; }
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
.revision-bar > div { display: grid; gap: 4px; }.revision-bar span { color: var(--ink-muted); font-size: 12px; }.revision-bar .field { margin: 0; }
.plan-history { margin-top: 36px; padding-top: 24px; border-top: 1px solid var(--line); }.section-title { display: flex; align-items: end; justify-content: space-between; }.section-title h2 { margin: 0; font-family: var(--font-serif); font-size: 25px; font-weight: 500; }.section-title > span { color: var(--ink-faint); font-size: 12px; }
.plan-list { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 12px; margin-top: 16px; }.plan-list article { display: grid; gap: 10px; padding: 16px; border: 1px solid var(--line); }.plan-list article > div { display: flex; justify-content: space-between; }.plan-list article p { min-height: 42px; margin: 0; color: var(--ink-muted); font-size: 13px; line-height: 1.6; }.plan-list article small { color: var(--ink-faint); }.plan-list .qualified { color: #55644d; }.plan-list .rejected { color: var(--accent); }
.confirmation-notice { margin: 0 0 16px; color: var(--success); font-size: 13px; }
@media (max-width: 900px) { .revision-bar { grid-template-columns: 1fr; } }
</style>
