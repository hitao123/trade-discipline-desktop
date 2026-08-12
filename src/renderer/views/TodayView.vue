<script setup lang="ts">
import { computed, onMounted } from 'vue'

import ErrorNotice from '@/renderer/components/ErrorNotice.vue'
import PageHeader from '@/renderer/components/PageHeader.vue'
import RiskMeter from '@/renderer/components/RiskMeter.vue'
import { useRemoteData } from '@/renderer/composables/useRemoteData'
import { api } from '@/renderer/lib/api'
import { formatCNY } from '@/renderer/lib/format'
import type { Dashboard } from '@/renderer/types'

const { data, loading, error, refresh } = useRemoteData(() => api.request<Dashboard>('/api/dashboard'))
const equity = computed(() => data.value ? data.value.portfolio.availableCashFen + Object.values(data.value.portfolio.positions).reduce((sum, item) => sum + item.costFen + item.unrealizedPnLFen, 0) : 0)
onMounted(refresh)
</script>

<template>
  <div>
    <PageHeader eyebrow="TODAY" title="先看纪律，再看盈亏" description="今天不需要做很多事，只需要做当前规则允许的那一件事。"><button class="button" type="button" :disabled="loading" @click="refresh">更新摘要</button></PageHeader>
    <ErrorNotice :message="error" />
    <template v-if="data">
      <section class="action-strip" :class="{ 'action-strip--cooldown': data.cooldown }"><p>今日唯一允许动作</p><h2>{{ data.allowedAction }}</h2><span v-if="data.cooldown">冷静期预计至 {{ new Date(data.cooldown.expectedEndsAt).toLocaleDateString('zh-CN') }}</span></section>
      <div class="metric-grid">
        <article><span>账户净值（参考市值）</span><strong>{{ formatCNY(equity) }}</strong><small>统一资金池 {{ formatCNY(data.initialCapitalFen) }}</small></article>
        <article><span>可用现金</span><strong>{{ formatCNY(data.portfolio.availableCashFen) }}</strong><small>港股按实际人民币结算</small></article>
        <article><span>中国科技敞口</span><strong>{{ formatCNY(data.portfolio.chinaTechExposureFen) }}</strong><small>上限 {{ formatCNY(data.chinaTechLimitFen) }}</small></article>
        <article><span>累计违规</span><strong>{{ data.violationCount }}</strong><small>违规盈利不加纪律分</small></article>
        <article><span>腾讯顺序门槛</span><strong>{{ data.portfolio.alibabaObservationTradingDays }} / 20</strong><small>收盘观察日 · 纪律分 {{ (data.portfolio.disciplineScoreBP / 100).toFixed(0) }} / 90</small></article>
      </div>
      <section class="risk-section"><RiskMeter label="组合损失红线使用" :used="data.lossUsedFen" :caution="data.lossCautionFen" :limit="data.lossRedLineFen" /></section>
      <section class="freshness"><strong>数据新鲜度</strong><span>{{ data.lastMarketFetch ? `最近收盘快照：${new Date(data.lastMarketFetch).toLocaleString('zh-CN')}` : '尚未获取收盘榜单，可去“市场榜单”刷新或导入 CSV' }}</span></section>
    </template>
  </div>
</template>

<style scoped>
.action-strip { padding: 20px 24px; border-left: 4px solid #5f6d58; background: #e9ede4; }
.action-strip--cooldown { border-left-color: var(--accent); background: #f2e3df; }
.action-strip p { margin: 0 0 6px; color: var(--ink-faint); font-size: 10px; letter-spacing: .13em; }.action-strip h2 { margin: 0; font-family: var(--font-serif); font-size: 22px; font-weight: 500; }.action-strip span { display: block; margin-top: 8px; color: var(--accent); font-size: 12px; }
.metric-grid { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); margin-top: 26px; border: 1px solid var(--line); }
.metric-grid article { display: grid; gap: 8px; min-height: 130px; padding: 20px; border-right: 1px solid var(--line); }.metric-grid article:last-child { border: 0; }.metric-grid span, .metric-grid small { color: var(--ink-faint); font-size: 11px; }.metric-grid strong { font-family: var(--font-serif); font-size: 24px; font-weight: 500; font-variant-numeric: tabular-nums; }
.risk-section { margin-top: 26px; padding: 22px; border: 1px solid var(--line); }.freshness { display: flex; gap: 20px; margin-top: 18px; color: var(--ink-muted); font-size: 12px; }.freshness strong { color: var(--ink); }
</style>
