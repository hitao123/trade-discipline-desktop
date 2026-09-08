<script setup lang="ts">
import { computed, onMounted, shallowRef } from 'vue'
import { RouterLink } from 'vue-router'

import ErrorNotice from '@/renderer/components/ErrorNotice.vue'
import PageHeader from '@/renderer/components/PageHeader.vue'
import RiskMeter from '@/renderer/components/RiskMeter.vue'
import { useRemoteData } from '@/renderer/composables/useRemoteData'
import { api } from '@/renderer/lib/api'
import { formatCNY } from '@/renderer/lib/format'
import type { Dashboard } from '@/renderer/types'

const { data, loading, error, refresh } = useRemoteData((signal) => api.request<Dashboard>('/api/dashboard', { signal }))
const equity = computed(() => data.value ? data.value.portfolio.availableCashFen + Object.values(data.value.portfolio.positions).reduce((sum, item) => sum + item.costFen + item.unrealizedPnLFen, 0) : 0)
const pendingReviewCount = shallowRef(0)
const pendingPositionReviewCount = shallowRef(0)
const positionCount = computed(() => data.value ? Object.values(data.value.portfolio.positions).filter(position => position.quantity > 0).length : 0)

async function loadTasks() {
  try {
    const [reviews, alerts] = await Promise.all([
      api.request<unknown[]>('/api/post-trade-reviews'),
      api.request<unknown[]>('/api/monitor/alerts'),
    ])
    pendingReviewCount.value = reviews.length
    pendingPositionReviewCount.value = alerts.length
  }
  catch { /* Tasks are omitted rather than inferred when a local endpoint is unavailable. */ }
}

async function refreshAll() {
  await refresh()
  await loadTasks()
}

onMounted(refreshAll)
</script>

<template>
  <div>
    <PageHeader eyebrow="TODAY" title="今日" description="先看纪律，再看盈亏。这里仅显示已有记录产生的限制和待办，不给出交易指令。"><button class="button" type="button" :disabled="loading" @click="refreshAll">更新摘要</button></PageHeader>
    <ErrorNotice :message="error" />
    <template v-if="data">
      <section class="discipline-limit" :class="{ 'discipline-limit--cooldown': data.cooldown }">
        <p>当前纪律限制</p><h2>{{ data.allowedAction }}</h2>
        <span v-if="data.cooldown">触发原因：{{ data.cooldown.reason }} · 预计至 {{ new Date(data.cooldown.expectedEndsAt).toLocaleDateString('zh-CN') }}</span>
        <RouterLink v-if="data.cooldown" class="text-button" to="/executions">查看触发记录与冲正流程</RouterLink>
      </section>
      <section class="next-steps" aria-label="下一步">
        <div><p>下一步</p><h2>来自真实记录的待办</h2></div>
        <RouterLink v-if="pendingReviewCount" class="next-step" to="/reviews">完成 {{ pendingReviewCount }} 笔成交复盘</RouterLink>
        <RouterLink v-if="pendingPositionReviewCount" class="next-step" to="/positions">复核 {{ pendingPositionReviewCount }} 项持仓提醒</RouterLink>
        <RouterLink v-if="positionCount" class="next-step" to="/positions">查看 {{ positionCount }} 个当前持仓</RouterLink>
        <p v-if="!pendingReviewCount && !pendingPositionReviewCount && !positionCount" class="next-steps__empty">目前没有由本地记录生成的待办。</p>
      </section>
      <div class="metric-grid">
        <article><span>账户净值（参考市值）</span><strong>{{ formatCNY(equity) }}</strong><small>统一资金池 {{ formatCNY(data.initialCapitalFen) }}</small></article>
        <article><span>可用现金</span><strong>{{ formatCNY(data.portfolio.availableCashFen) }}</strong><small>港股按实际人民币结算</small></article>
        <article><span>累计违规</span><strong>{{ data.violationCount }}</strong><small>违规盈利不加纪律分</small></article>
        <article><span>损失红线已使用</span><strong>{{ formatCNY(data.lossUsedFen) }}</strong><small>上限 {{ formatCNY(data.lossRedLineFen) }}</small></article>
      </div>
      <section class="risk-section"><RiskMeter label="组合损失红线使用" :used="data.lossUsedFen" :caution="data.lossCautionFen" :limit="data.lossRedLineFen" /></section>
      <section class="freshness"><strong>数据新鲜度</strong><span>{{ data.lastMarketFetch ? `最近收盘快照：${new Date(data.lastMarketFetch).toLocaleString('zh-CN')}` : '尚未获取收盘榜单，可去“市场榜单”刷新或导入 CSV' }}</span></section>
    </template>
  </div>
</template>

<style scoped>
.discipline-limit { display: grid; gap: 7px; padding: 20px 24px; border-left: 4px solid #5f6d58; background: #e9ede4; }.discipline-limit--cooldown { border-left-color: var(--accent); background: #f2e3df; }.discipline-limit p, .next-steps p { margin: 0; color: var(--ink-faint); font-size: 11px; letter-spacing: .1em; }.discipline-limit h2, .next-steps h2 { margin: 0; font-family: var(--font-serif); font-size: 22px; font-weight: 500; }.discipline-limit span { color: var(--accent); font-size: 13px; }
.next-steps { display: grid; grid-template-columns: minmax(220px, 1fr) repeat(3, minmax(0, 1fr)); gap: 1px; margin-top: 18px; border: 1px solid var(--line); background: var(--line); }.next-steps > div, .next-step, .next-steps__empty { padding: 16px; background: var(--paper); }.next-step { color: var(--ink); font-size: 13px; line-height: 1.5; text-decoration: none; }.next-step:hover { background: var(--paper-deep); }.next-steps__empty { grid-column: 2 / -1; color: var(--ink-muted) !important; font-size: 13px !important; letter-spacing: 0 !important; }
.metric-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); margin-top: 26px; border: 1px solid var(--line); }
.metric-grid article { display: grid; gap: 8px; min-height: 130px; padding: 20px; border-right: 1px solid var(--line); }.metric-grid article:last-child { border: 0; }.metric-grid span, .metric-grid small { color: var(--ink-faint); font-size: 11px; }.metric-grid strong { font-family: var(--font-serif); font-size: 24px; font-weight: 500; font-variant-numeric: tabular-nums; }
.risk-section { margin-top: 26px; padding: 22px; border: 1px solid var(--line); }.freshness { display: flex; gap: 20px; margin-top: 18px; color: var(--ink-muted); font-size: 12px; }.freshness strong { color: var(--ink); }
@media (max-width: 900px) { .next-steps { grid-template-columns: 1fr; }.next-steps__empty { grid-column: auto; } }
</style>
