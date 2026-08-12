<script setup lang="ts">
import { onMounted, shallowRef } from 'vue'

import ErrorNotice from '@/renderer/components/ErrorNotice.vue'
import ExecutionForm from '@/renderer/components/executions/ExecutionForm.vue'
import PageHeader from '@/renderer/components/PageHeader.vue'
import { api } from '@/renderer/lib/api'
import { formatCNY } from '@/renderer/lib/format'
import type { Instrument, PlanRecord, Position } from '@/renderer/types'

interface Receipt { id: string; classification: string; violationCode?: string; position: Position; cashFen: number; cooldown?: { expectedEndsAt: string } }

const instruments = shallowRef<Instrument[]>([])
const plans = shallowRef<PlanRecord[]>([])
const receipt = shallowRef<Receipt>()
const busy = shallowRef(false)
const error = shallowRef('')
const reversalReason = shallowRef('')

async function load() {
  try {
    [instruments.value, plans.value] = await Promise.all([api.request<Instrument[]>('/api/instruments'), api.request<PlanRecord[]>('/api/plans')])
  }
  catch (cause) { error.value = cause instanceof Error ? cause.message : '成交页加载失败' }
}

async function record(payload: Record<string, unknown>) {
  busy.value = true; error.value = ''
  try { receipt.value = await api.request<Receipt>('/api/executions', { method: 'POST', body: JSON.stringify(payload) }) }
  catch (cause) { error.value = cause instanceof Error ? cause.message : '成交记录失败' }
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

onMounted(load)
</script>

<template>
  <div>
    <PageHeader eyebrow="RECORD" title="记录事实，不替过去找理由" description="违规成交也必须完整保存。系统会更新真实持仓，并同时生成违规、冷静期和审计记录。" />
    <ErrorNotice :message="error" />
    <div class="execution-layout">
      <ExecutionForm :instruments="instruments" :plans="plans" :busy="busy" @submit="record" />
      <aside v-if="receipt" class="receipt" :class="{ 'receipt--violation': receipt.classification === 'serious_violation' }">
        <p>记录完成</p>
        <h2>{{ receipt.classification === 'reversed' ? '原成交已追加冲正' : receipt.classification === 'serious_violation' ? '严重违规已如实入账' : '成交与计划一致' }}</h2>
        <dl><div><dt>剩余现金</dt><dd>{{ formatCNY(receipt.cashFen) }}</dd></div><div><dt>当前数量</dt><dd>{{ receipt.position.quantity }} 股</dd></div><div v-if="receipt.cooldown"><dt>冷静期至</dt><dd>{{ new Date(receipt.cooldown.expectedEndsAt).toLocaleDateString('zh-CN') }}</dd></div></dl>
        <p v-if="receipt.violationCode" class="violation-code">违规代码：{{ receipt.violationCode }}</p>
        <div v-if="receipt.classification !== 'reversed'" class="reversal-box"><label class="field"><span>若本次录错，填写冲正原因</span><input v-model="reversalReason" placeholder="不会删除原记录" /></label><button class="button" type="button" :disabled="busy || !reversalReason.trim()" @click="reverse">追加冲正</button></div>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.execution-layout { display: grid; grid-template-columns: minmax(0, 1fr) 300px; gap: 28px; align-items: start; }
.receipt { padding: 22px; border-top: 3px solid #617158; background: var(--paper-deep); }
.receipt--violation { border-top-color: var(--accent); }
.receipt > p { margin: 0 0 8px; color: var(--ink-faint); font-size: 11px; letter-spacing: .12em; }
.receipt h2 { margin: 0 0 20px; font-family: var(--font-serif); font-size: 22px; font-weight: 500; }
.receipt--violation h2 { color: var(--accent); }
.receipt dl { display: grid; gap: 10px; margin: 0; }
.receipt dl div { display: flex; justify-content: space-between; gap: 12px; padding-top: 10px; border-top: 1px solid var(--line); font-size: 12px; }
.receipt dt { color: var(--ink-muted); }.receipt dd { margin: 0; font-weight: 650; }
.violation-code { color: var(--accent) !important; font-size: 11px; letter-spacing: .04em !important; }.reversal-box { display: grid; gap: 10px; margin-top: 18px; padding-top: 16px; border-top: 1px solid var(--line); }.reversal-box .field { margin: 0; }
</style>
