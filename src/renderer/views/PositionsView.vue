<script setup lang="ts">
import { computed, onMounted, shallowRef } from 'vue'

import ErrorNotice from '@/renderer/components/ErrorNotice.vue'
import ExecutionCorrectionPanel from '@/renderer/components/executions/ExecutionCorrectionPanel.vue'
import ExecutionHistory from '@/renderer/components/executions/ExecutionHistory.vue'
import PageHeader from '@/renderer/components/PageHeader.vue'
import PositionReviewPanel from '@/renderer/components/positions/PositionReviewPanel.vue'
import { useOptionalUserProfile } from '@/renderer/composables/useUserProfile'
import { useRemoteData } from '@/renderer/composables/useRemoteData'
import { api } from '@/renderer/lib/api'
import { formatCNY, formatPrice } from '@/renderer/lib/format'
import type { ExecutionRecord, Instrument, MonitorStatus, Portfolio, PriceAlert, Position } from '@/renderer/types'

const { data, loading, error, refresh } = useRemoteData(() => api.request<Portfolio>('/api/portfolio'))
const userProfile = useOptionalUserProfile()
const genericMode = computed(() => userProfile?.profile.value?.mode === 'generic')
const positions = computed(() => Object.values(data.value?.positions ?? {}))
const priceCurrency = (code: string) => code === '0700' || code === '9988' ? 'HKD' : 'CNY'
const monitorStatus = shallowRef<MonitorStatus>()
const alerts = shallowRef<PriceAlert[]>([])
const monitorError = shallowRef('')
const reviewingAlertID = shallowRef('')
const correctingPositionID = shallowRef('')
const executionRecords = shallowRef<ExecutionRecord[]>([])
const instruments = shallowRef<Instrument[]>([])
const selectedExecution = shallowRef<ExecutionRecord>()
const correctionBusy = shallowRef(false)
const correctionError = shallowRef('')
const monitorSummary = computed(() => {
  if (!monitorStatus.value || monitorStatus.value.interval === 'off') return '价格提醒已关闭'
  return `每 ${monitorStatus.value.interval.replace('m', ' 分钟')}检查一次`
})

async function loadMonitor() {
  try {
    const [status, pendingAlerts] = await Promise.all([
      api.request<MonitorStatus>('/api/monitor/status'),
      api.request<PriceAlert[]>('/api/monitor/alerts'),
    ])
    monitorStatus.value = status
    alerts.value = pendingAlerts
    monitorError.value = ''
  }
  catch (cause) {
    monitorError.value = cause instanceof Error ? cause.message : '价格提醒状态加载失败'
  }
}

async function reload() {
  await Promise.all([refresh(), loadMonitor()])
}

async function submitReview(payload: { alertId: string; decision: 'hold' | 'trim' | 'sell' | 'wait'; reason: string }) {
  reviewingAlertID.value = payload.alertId
  monitorError.value = ''
  try {
    await api.request(`/api/monitor/alerts/${payload.alertId}/reviews`, { method: 'POST', body: JSON.stringify({ decision: payload.decision, reason: payload.reason }) })
    alerts.value = alerts.value.filter(alert => alert.id !== payload.alertId)
  }
  catch (cause) {
    monitorError.value = cause instanceof Error ? cause.message : '持仓复核记录失败'
  }
  finally {
    reviewingAlertID.value = ''
  }
}

async function openCorrection(position: Position) {
  correctingPositionID.value = position.instrumentId
  selectedExecution.value = undefined
  correctionError.value = ''
  correctionBusy.value = true
  try {
    const [records, loadedInstruments] = await Promise.all([
      api.request<ExecutionRecord[]>(`/api/executions?instrumentId=${encodeURIComponent(position.instrumentId)}`),
      api.request<Instrument[]>('/api/instruments'),
    ])
    executionRecords.value = records
    instruments.value = loadedInstruments
  }
  catch (cause) {
    correctionError.value = cause instanceof Error ? cause.message : '成交记录加载失败'
  }
  finally {
    correctionBusy.value = false
  }
}

function closeCorrection() {
  correctingPositionID.value = ''
  executionRecords.value = []
  selectedExecution.value = undefined
  correctionError.value = ''
}

async function correctExecution(payload: { originalId: string; reason: string; draft: Record<string, unknown> }) {
  correctionBusy.value = true
  correctionError.value = ''
  try {
    await api.request(`/api/executions/${payload.originalId}/correct`, {
      method: 'POST',
      body: JSON.stringify({ reason: payload.reason, draft: payload.draft }),
    })
    closeCorrection()
    await refresh()
  }
  catch (cause) {
    correctionError.value = cause instanceof Error ? cause.message : '成交修正失败'
  }
  finally {
    correctionBusy.value = false
  }
}

onMounted(reload)
</script>

<template>
  <div>
    <PageHeader eyebrow="POSITIONS" title="持仓由成交记录自动计算" description="录错时可以一步修正。系统会保留原记录和修正原因，同时更新持仓与现金。"><button class="button" type="button" :disabled="loading" @click="reload">重新计算</button></PageHeader>
    <ErrorNotice :message="error" />
    <section class="monitor-status"><strong>价格提醒</strong><span>{{ monitorSummary }}</span><time v-if="monitorStatus?.lastSuccessfulAt">上次成功：{{ new Date(monitorStatus.lastSuccessfulAt).toLocaleString('zh-CN') }}</time><span v-if="monitorStatus?.lastError" class="negative">{{ monitorStatus.lastError }}</span></section>
    <p v-if="monitorError" class="monitor-error" role="alert">{{ monitorError }}</p>
    <section v-if="alerts.length" class="review-list" aria-label="待完成持仓复核"><PositionReviewPanel v-for="alert in alerts" :key="alert.id" :alert="alert" :busy="reviewingAlertID === alert.id" :error="reviewingAlertID === alert.id ? monitorError : ''" @submit="submitReview" /></section>
    <div v-if="data" class="portfolio-summary"><div><span>可用现金</span><strong>{{ formatCNY(data.availableCashFen) }}</strong></div><div v-if="!genericMode"><span>中国科技敞口（市值）</span><strong>{{ formatCNY(data.chinaTechExposureFen) }}</strong></div><div><span>累计亏损（含浮亏）</span><strong>{{ formatCNY(data.cumulativeLossFen) }}</strong></div></div>
    <div class="table-frame">
      <table><thead><tr><th>证券</th><th>数量</th><th>最新收盘</th><th>人民币成本</th><th>人民币市值</th><th>浮动盈亏</th><th>已实现盈亏</th><th>风险标签</th><th>操作</th></tr></thead><tbody>
        <tr v-for="position in positions" :key="position.instrumentId">
          <td><span class="security"><strong>{{ position.name || position.code }}</strong><small v-if="position.name">{{ position.code }}</small></span></td>
          <td>{{ position.quantity }} 股 · {{ Math.floor(position.quantity / (position.lotSize || 100)) }} 手</td>
          <td><template v-if="position.referencePriceMinor">{{ formatPrice(position.referencePriceMinor, position.currency || priceCurrency(position.code)) }}</template><span v-else class="muted">待同步</span></td>
          <td>{{ formatCNY(position.costFen) }}</td>
          <td>{{ formatCNY(position.marketValueFen) }}</td>
          <td :class="{ negative: position.unrealizedPnLFen < 0, positive: position.unrealizedPnLFen > 0 }">{{ formatCNY(position.unrealizedPnLFen) }}</td>
          <td :class="{ negative: position.realizedPnLFen < 0, positive: position.realizedPnLFen > 0 }">{{ formatCNY(position.realizedPnLFen) }}</td>
          <td>{{ position.isChinaTech ? '中国科技穿透敞口' : '普通持仓' }}</td>
          <td><button class="text-button" type="button" @click="openCorrection(position)">修正成交</button></td>
        </tr>
        <tr v-if="positions.length === 0"><td colspan="9" class="empty-cell">尚无成交事件。这里不会手工创建持仓。</td></tr>
      </tbody></table>
    </div>
    <section v-if="correctingPositionID" class="correction-workspace" aria-label="修正成交">
      <div class="correction-workspace__heading"><div><strong>修正成交记录</strong><span>选择录错的一条，保存后现金与持仓会同步更新。</span></div><button class="text-button" type="button" @click="closeCorrection">关闭</button></div>
      <ErrorNotice :message="correctionError" />
      <ExecutionHistory :records="executionRecords" :busy="correctionBusy" @select="selectedExecution = $event" />
      <ExecutionCorrectionPanel :record="selectedExecution" :instruments="instruments" :busy="correctionBusy" @submit="correctExecution" @cancel="selectedExecution = undefined" />
    </section>
  </div>
</template>

<style scoped>
.portfolio-summary { display: grid; grid-template-columns: repeat(3, 1fr); margin-bottom: 24px; border: 1px solid var(--line); }.portfolio-summary div { display: grid; gap: 8px; padding: 18px; border-right: 1px solid var(--line); }.portfolio-summary div:last-child { border: 0; }.portfolio-summary span { color: var(--ink-faint); font-size: 11px; }.portfolio-summary strong { font-family: var(--font-serif); font-size: 22px; font-weight: 500; }
.monitor-status { display: flex; flex-wrap: wrap; align-items: center; gap: 9px 14px; margin: 0 0 16px; padding: 12px 14px; border: 1px solid var(--line); background: var(--paper-deep); font-size: 12px; }.monitor-status strong { color: var(--ink); }.monitor-status span, .monitor-status time { color: var(--ink-muted); }.monitor-error { margin: 0 0 16px; color: var(--accent); font-size: 12px; }.review-list { display: grid; gap: 12px; margin: 0 0 20px; }.table-frame { overflow: auto; border: 1px solid var(--line); } table { width: 100%; min-width: 980px; border-collapse: collapse; font-size: 13px; } th { padding: 12px 14px; color: var(--ink-faint); background: var(--paper-deep); font-size: 10px; text-align: left; } td { padding: 15px 14px; border-top: 1px solid var(--line); }.negative { color: var(--accent); }.positive { color: var(--success); }.muted { color: var(--ink-faint); }.empty-cell { padding: 44px; color: var(--ink-faint); text-align: center; }
.security { display: grid; gap: 3px; min-width: 110px; }.security small { color: var(--ink-faint); font-size: 10px; }.correction-workspace { display: grid; gap: 14px; margin-top: 22px; padding-top: 22px; border-top: 1px solid var(--line); }.correction-workspace__heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; }.correction-workspace__heading > div { display: grid; gap: 4px; }.correction-workspace__heading span { color: var(--ink-muted); font-size: 11px; }
</style>
