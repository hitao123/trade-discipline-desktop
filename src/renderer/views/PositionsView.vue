<script setup lang="ts">
import { computed, onMounted, shallowRef } from 'vue'

import ErrorNotice from '@/renderer/components/ErrorNotice.vue'
import PageHeader from '@/renderer/components/PageHeader.vue'
import PositionReviewPanel from '@/renderer/components/positions/PositionReviewPanel.vue'
import { useRemoteData } from '@/renderer/composables/useRemoteData'
import { api } from '@/renderer/lib/api'
import { formatCNY, formatPrice } from '@/renderer/lib/format'
import type { MonitorStatus, Portfolio, PriceAlert } from '@/renderer/types'

const { data, loading, error, refresh } = useRemoteData(() => api.request<Portfolio>('/api/portfolio'))
const positions = computed(() => Object.values(data.value?.positions ?? {}))
const priceCurrency = (code: string) => code === '0700' || code === '9988' ? 'HKD' : 'CNY'
const monitorStatus = shallowRef<MonitorStatus>()
const alerts = shallowRef<PriceAlert[]>([])
const monitorError = shallowRef('')
const reviewingAlertID = shallowRef('')
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

onMounted(reload)
</script>

<template>
  <div>
    <PageHeader eyebrow="POSITIONS" title="持仓由成交重放，不可直接改写" description="数量、人民币成本和已实现盈亏来自不可变事件。录错时追加冲正，旧记录仍留在审计中。"><button class="button" type="button" :disabled="loading" @click="reload">重新计算</button></PageHeader>
    <ErrorNotice :message="error" />
    <section class="monitor-status"><strong>价格提醒</strong><span>{{ monitorSummary }}</span><time v-if="monitorStatus?.lastSuccessfulAt">上次成功：{{ new Date(monitorStatus.lastSuccessfulAt).toLocaleString('zh-CN') }}</time><span v-if="monitorStatus?.lastError" class="negative">{{ monitorStatus.lastError }}</span></section>
    <p v-if="monitorError" class="monitor-error" role="alert">{{ monitorError }}</p>
    <section v-if="alerts.length" class="review-list" aria-label="待完成持仓复核"><PositionReviewPanel v-for="alert in alerts" :key="alert.id" :alert="alert" :busy="reviewingAlertID === alert.id" :error="reviewingAlertID === alert.id ? monitorError : ''" @submit="submitReview" /></section>
    <div v-if="data" class="portfolio-summary"><div><span>可用现金</span><strong>{{ formatCNY(data.availableCashFen) }}</strong></div><div><span>中国科技敞口（市值）</span><strong>{{ formatCNY(data.chinaTechExposureFen) }}</strong></div><div><span>累计亏损（含浮亏）</span><strong>{{ formatCNY(data.cumulativeLossFen) }}</strong></div></div>
    <div class="table-frame">
      <table><thead><tr><th>证券</th><th>数量</th><th>最新收盘</th><th>人民币成本</th><th>人民币市值</th><th>浮动盈亏</th><th>已实现盈亏</th><th>风险标签</th></tr></thead><tbody>
        <tr v-for="position in positions" :key="position.instrumentId">
          <td><strong>{{ position.code }}</strong></td>
          <td>{{ position.quantity }} 股 · {{ Math.floor(position.quantity / 100) }} 手</td>
          <td><template v-if="position.referencePriceMinor">{{ formatPrice(position.referencePriceMinor, priceCurrency(position.code)) }}</template><span v-else class="muted">待同步</span></td>
          <td>{{ formatCNY(position.costFen) }}</td>
          <td>{{ formatCNY(position.marketValueFen) }}</td>
          <td :class="{ negative: position.unrealizedPnLFen < 0, positive: position.unrealizedPnLFen > 0 }">{{ formatCNY(position.unrealizedPnLFen) }}</td>
          <td :class="{ negative: position.realizedPnLFen < 0, positive: position.realizedPnLFen > 0 }">{{ formatCNY(position.realizedPnLFen) }}</td>
          <td>{{ position.isChinaTech ? '中国科技穿透敞口' : '普通持仓' }}</td>
        </tr>
        <tr v-if="positions.length === 0"><td colspan="8" class="empty-cell">尚无成交事件。这里不会手工创建持仓。</td></tr>
      </tbody></table>
    </div>
  </div>
</template>

<style scoped>
.portfolio-summary { display: grid; grid-template-columns: repeat(3, 1fr); margin-bottom: 24px; border: 1px solid var(--line); }.portfolio-summary div { display: grid; gap: 8px; padding: 18px; border-right: 1px solid var(--line); }.portfolio-summary div:last-child { border: 0; }.portfolio-summary span { color: var(--ink-faint); font-size: 11px; }.portfolio-summary strong { font-family: var(--font-serif); font-size: 22px; font-weight: 500; }
.monitor-status { display: flex; flex-wrap: wrap; align-items: center; gap: 9px 14px; margin: 0 0 16px; padding: 12px 14px; border: 1px solid var(--line); background: var(--paper-deep); font-size: 12px; }.monitor-status strong { color: var(--ink); }.monitor-status span, .monitor-status time { color: var(--ink-muted); }.monitor-error { margin: 0 0 16px; color: var(--accent); font-size: 12px; }.review-list { display: grid; gap: 12px; margin: 0 0 20px; }.table-frame { overflow: auto; border: 1px solid var(--line); } table { width: 100%; min-width: 980px; border-collapse: collapse; font-size: 13px; } th { padding: 12px 14px; color: var(--ink-faint); background: var(--paper-deep); font-size: 10px; text-align: left; } td { padding: 15px 14px; border-top: 1px solid var(--line); }.negative { color: var(--accent); }.positive { color: var(--success); }.muted { color: var(--ink-faint); }.empty-cell { padding: 44px; color: var(--ink-faint); text-align: center; }
</style>
