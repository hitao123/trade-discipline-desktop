<script setup lang="ts">
import { onMounted, shallowRef } from 'vue'

import ErrorNotice from '@/renderer/components/ErrorNotice.vue'
import ExecutionCorrectionPanel from '@/renderer/components/executions/ExecutionCorrectionPanel.vue'
import ExecutionHistory from '@/renderer/components/executions/ExecutionHistory.vue'
import ExecutionForm from '@/renderer/components/executions/ExecutionForm.vue'
import ExecutionScreenshotImport from '@/renderer/components/executions/ExecutionScreenshotImport.vue'
import QuickExecutionForm from '@/renderer/components/executions/QuickExecutionForm.vue'
import PageHeader from '@/renderer/components/PageHeader.vue'
import { api } from '@/renderer/lib/api'
import { formatCNY } from '@/renderer/lib/format'
import type { ExecutionScreenshotPrefill } from '@/renderer/lib/execution-screenshot'
import type { ExecutionRecord, Instrument, PlanRecord, Position } from '@/renderer/types'

interface Receipt { id: string; classification: string; violationCode?: string; position: Position; cashFen: number; pendingReview?: boolean; cooldown?: { expectedEndsAt: string } }

const instruments = shallowRef<Instrument[]>([])
const plans = shallowRef<PlanRecord[]>([])
const receipt = shallowRef<Receipt>()
const busy = shallowRef(false)
const error = shallowRef('')
const reversalReason = shallowRef('')
const entryMode = shallowRef<'quick' | 'full'>('quick')
const screenshotPrefill = shallowRef<ExecutionScreenshotPrefill>()
const executionRecords = shallowRef<ExecutionRecord[]>([])
const selectedExecution = shallowRef<ExecutionRecord>()

async function loadActiveExecutions() {
  try {
    return await api.request<ExecutionRecord[]>('/api/executions')
  }
  catch {
    return []
  }
}

async function load() {
  try {
    const [loadedInstruments, loadedPlans, loadedExecutions] = await Promise.all([
      api.request<Instrument[]>('/api/instruments'),
      api.request<PlanRecord[]>('/api/plans'),
      loadActiveExecutions(),
    ])
    instruments.value = loadedInstruments
    plans.value = loadedPlans
    executionRecords.value = loadedExecutions
    void repairCorruptedInstrumentNames(loadedInstruments)
  }
  catch (cause) { error.value = cause instanceof Error ? cause.message : '成交页加载失败' }
}

function includeResolvedInstrument(instrument: Instrument) {
  const exists = instruments.value.some(item => item.id === instrument.id)
  instruments.value = exists
    ? instruments.value.map(item => item.id === instrument.id ? instrument : item)
    : [...instruments.value, instrument]
}

async function repairCorruptedInstrumentNames(items: Instrument[]) {
  for (const instrument of items) {
    if (!instrument.name.includes('\uFFFD')) continue
    try {
      const repaired = await api.request<Instrument>('/api/instruments/resolve', { method: 'POST', body: JSON.stringify({ code: instrument.code }) })
      includeResolvedInstrument(repaired)
    }
    catch {
      // Keep the existing instrument available when public quote sources are temporarily unavailable.
    }
  }
}

async function record(payload: Record<string, unknown>) {
  busy.value = true; error.value = ''
  try {
    receipt.value = await api.request<Receipt>('/api/executions', { method: 'POST', body: JSON.stringify(payload) })
    executionRecords.value = await loadActiveExecutions()
  }
  catch (cause) { error.value = cause instanceof Error ? cause.message : '成交记录失败' }
  finally { busy.value = false }
}

async function recordQuick(payload: Record<string, unknown>) {
  busy.value = true; error.value = ''
  try {
    receipt.value = await api.request<Receipt>('/api/executions/quick', { method: 'POST', body: JSON.stringify(payload) })
    executionRecords.value = await loadActiveExecutions()
  }
  catch (cause) { error.value = cause instanceof Error ? cause.message : '极速补录失败' }
  finally { busy.value = false }
}

async function reverse() {
  if (!receipt.value || !reversalReason.value.trim()) return
  busy.value = true; error.value = ''
  try {
    receipt.value = await api.request<Receipt>(`/api/executions/${receipt.value.id}/reverse`, { method: 'POST', body: JSON.stringify({ reason: reversalReason.value }) })
    reversalReason.value = ''
  }
  catch (cause) { error.value = cause instanceof Error ? cause.message : '冲正失败' }
  finally { busy.value = false }
}

async function correctExecution(payload: { originalId: string; reason: string; draft: Record<string, unknown> }) {
  busy.value = true
  error.value = ''
  try {
    receipt.value = await api.request<Receipt>(`/api/executions/${payload.originalId}/correct`, {
      method: 'POST',
      body: JSON.stringify({ reason: payload.reason, draft: payload.draft }),
    })
    executionRecords.value = await loadActiveExecutions()
    selectedExecution.value = undefined
  }
  catch (cause) {
    error.value = cause instanceof Error ? cause.message : '成交修正失败'
  }
  finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <div>
    <PageHeader eyebrow="RECORD" title="记录事实，不替过去找理由" description="违规成交也必须完整保存。系统会更新真实持仓，并同时生成违规、冷静期和审计记录。" />
    <ErrorNotice :message="error" />
	<div class="entry-tabs" role="tablist" aria-label="补录方式">
	  <button type="button" :class="{ active: entryMode === 'quick' }" @click="entryMode = 'quick'">极速留痕</button>
	  <button type="button" :class="{ active: entryMode === 'full' }" @click="entryMode = 'full'">完整补录</button>
	</div>
    <div class="execution-layout">
	  <div v-if="entryMode === 'quick'" class="quick-entry">
        <ExecutionScreenshotImport :instruments="instruments" @instrument-resolved="includeResolvedInstrument" @prefill="screenshotPrefill = $event" />
        <QuickExecutionForm :instruments="instruments" :plans="plans" :busy="busy" :prefill="screenshotPrefill" @submit="recordQuick" />
      </div>
      <ExecutionForm v-else :instruments="instruments" :plans="plans" :busy="busy" @submit="record" />
      <aside v-if="receipt" class="receipt" :class="{ 'receipt--violation': receipt.classification === 'serious_violation' }">
        <p>记录完成</p>
        <h2>{{ receipt.classification === 'reversed' ? '原成交已追加冲正' : receipt.classification === 'serious_violation' ? '严重违规已如实入账' : '成交与计划一致' }}</h2>
        <dl><div><dt>剩余现金</dt><dd>{{ formatCNY(receipt.cashFen) }}</dd></div><div><dt>当前数量</dt><dd>{{ receipt.position.quantity }} 股</dd></div><div v-if="receipt.cooldown"><dt>冷静期至</dt><dd>{{ new Date(receipt.cooldown.expectedEndsAt).toLocaleDateString('zh-CN') }}</dd></div></dl>
        <p v-if="receipt.violationCode" class="violation-code">违规代码：{{ receipt.violationCode }}</p>
		<p v-if="receipt.pendingReview" class="pending-review">已加入待复盘</p>
        <div v-if="receipt.classification !== 'reversed'" class="reversal-box"><label class="field"><span>若本次录错，填写冲正原因</span><input v-model="reversalReason" placeholder="不会删除原记录" /></label><button class="button" type="button" :disabled="busy || !reversalReason.trim()" @click="reverse">追加冲正</button></div>
      </aside>
    </div>
    <section class="correction-section" aria-label="近期成交修正">
      <ExecutionHistory :records="executionRecords" :busy="busy" @select="selectedExecution = $event" />
      <ExecutionCorrectionPanel :record="selectedExecution" :instruments="instruments" :busy="busy" @submit="correctExecution" @cancel="selectedExecution = undefined" />
    </section>
  </div>
</template>

<style scoped>
.execution-layout { display: grid; grid-template-columns: minmax(0, 1fr) 300px; gap: 28px; align-items: start; }
.quick-entry { display: grid; gap: 16px; min-width: 0; }
.entry-tabs { display: flex; gap: 18px; margin: 0 0 20px; border-bottom: 1px solid var(--line); }.entry-tabs button { padding: 9px 1px; color: var(--ink-faint); border: 0; border-bottom: 2px solid transparent; background: transparent; cursor: pointer; }.entry-tabs button.active { color: var(--ink); border-bottom-color: var(--accent); font-weight: 700; }
.receipt { padding: 22px; border-top: 3px solid #617158; background: var(--paper-deep); }
.receipt--violation { border-top-color: var(--accent); }
.receipt > p { margin: 0 0 8px; color: var(--ink-faint); font-size: 11px; letter-spacing: .12em; }
.receipt h2 { margin: 0 0 20px; font-family: var(--font-serif); font-size: 22px; font-weight: 500; }
.receipt--violation h2 { color: var(--accent); }
.receipt dl { display: grid; gap: 10px; margin: 0; }
.receipt dl div { display: flex; justify-content: space-between; gap: 12px; padding-top: 10px; border-top: 1px solid var(--line); font-size: 12px; }
.receipt dt { color: var(--ink-muted); }.receipt dd { margin: 0; font-weight: 650; }
.violation-code { color: var(--accent) !important; font-size: 11px; letter-spacing: .04em !important; }.reversal-box { display: grid; gap: 10px; margin-top: 18px; padding-top: 16px; border-top: 1px solid var(--line); }.reversal-box .field { margin: 0; }
.pending-review { margin-top: 12px !important; color: #8a6a32 !important; letter-spacing: 0 !important; }
.correction-section { display: grid; gap: 14px; margin-top: 28px; padding-top: 24px; border-top: 1px solid var(--line); }
</style>
