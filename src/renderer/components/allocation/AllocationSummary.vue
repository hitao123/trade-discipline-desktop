<script setup lang="ts">
import { computed, type DeepReadonly } from 'vue'

import { formatCNY, formatPercentBP } from '@/renderer/lib/format'
import type { AllocationOverview } from '@/renderer/types'

const props = defineProps<{ overview: DeepReadonly<AllocationOverview> }>()
const targetReturn = computed(() => props.overview.version.draft.targetReturnBP ? `${(props.overview.version.draft.targetReturnBP / 100).toFixed(2)}%` : '未设置目标收益')
const deadline = computed(() => props.overview.version.draft.targetDeadline ? props.overview.version.draft.targetDeadline.replaceAll('-', ' 年 ').replace(/ 年 (\d{2})$/, ' 月') : '')
</script>

<template>
  <section class="summary" aria-label="资产配置目标摘要">
    <div class="summary__metric"><span>当前总资产</span><strong>{{ formatCNY(overview.currentTotalFen) }}</strong></div>
    <div class="summary__metric"><span>配置目标</span><strong>{{ formatCNY(overview.targetTotalFen) }}</strong><small><template v-if="deadline">截至 {{ deadline }} · </template>{{ targetReturn }}</small></div>
    <div class="summary__metric"><span>当前收益</span><strong :class="{ positive: overview.returnBP > 0, negative: overview.returnBP < 0 }">{{ formatPercentBP(overview.returnBP) }}</strong></div>
    <div class="summary__metric"><span>距目标差额</span><strong>{{ formatCNY(overview.goalGapFen) }}</strong><small>正数表示尚未达到目标</small></div>
  </section>

  <section class="table-frame" aria-label="资产配置进度">
    <table class="allocation-table">
      <thead><tr><th>资产</th><th>目标</th><th>当前市值</th><th>当前占比</th><th>建仓偏离</th><th>再平衡偏离</th><th>数据来源</th></tr></thead>
      <tbody>
        <tr v-for="progress in overview.items" :key="progress.item.key">
          <td><strong>{{ progress.item.name }}</strong><small v-if="progress.item.targetShares">目标约 {{ progress.item.targetShares }} 股</small></td>
          <td>{{ (progress.item.targetWeightBP / 100).toFixed(1) }}%</td>
          <td>{{ formatCNY(progress.currentValueFen) }}<small v-if="progress.item.role === 'cash' || progress.item.key === 'cash-fund'">现金自动同步</small><small v-else-if="progress.linkedValueFen || progress.manualAdjustmentFen" class="value-source">持仓 {{ formatCNY(progress.linkedValueFen) }} + 调整 {{ formatCNY(progress.manualAdjustmentFen) }}</small></td>
          <td>{{ (progress.currentWeightBP / 100).toFixed(1) }}%</td>
          <td>{{ formatCNY(progress.buildGapFen) }}</td>
          <td>{{ formatCNY(progress.rebalanceGapFen) }}</td>
          <td><span v-if="progress.linkedPositions.length">{{ progress.linkedPositions.map(position => `${position.name || position.code} · ${position.code}`).join('、') }}</span><span v-else-if="progress.item.instrumentCodes.length">等待对应持仓</span><span v-else>未关联证券</span><time v-if="progress.latestAdjustment">调整于 {{ new Date(progress.latestAdjustment.observedAt).toLocaleString('zh-CN') }}</time></td>
        </tr>
      </tbody>
    </table>
  </section>
  <section v-if="overview.unassignedPositions.length" class="unassigned" aria-label="尚未归类持仓">
    <div><strong>尚未归类的持仓</strong><span>已计入总资产，但还没有归入某个配置项目。</span></div>
    <ul><li v-for="position in overview.unassignedPositions" :key="position.instrumentId"><strong>{{ position.name || position.code }} · {{ position.code }}</strong><span>{{ formatCNY(position.marketValueFen) }}</span></li></ul>
  </section>
</template>

<style scoped>
.summary { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); margin-bottom: 24px; border: 1px solid var(--line); }.summary__metric { display: grid; gap: 7px; min-height: 122px; padding: 18px; border-right: 1px solid var(--line); }.summary__metric:last-child { border-right: 0; }.summary__metric span, .summary__metric small { color: var(--ink-faint); font-size: 10px; }.summary__metric strong { font-family: var(--font-serif); font-size: 23px; font-weight: 500; }.positive { color: var(--success); }.negative { color: var(--accent); }.table-frame { overflow: auto; margin-bottom: 32px; border: 1px solid var(--line); }.allocation-table { width: 100%; min-width: 920px; border-collapse: collapse; font-size: 12px; }.allocation-table th { padding: 11px 13px; color: var(--ink-faint); background: var(--paper-deep); font-size: 10px; text-align: left; }.allocation-table td { padding: 13px; border-top: 1px solid var(--line); vertical-align: top; }.allocation-table td small, .allocation-table td time, .allocation-table td span { display: block; margin-top: 4px; color: var(--ink-faint); font-size: 10px; }
.value-source { white-space: nowrap; }.unassigned { display: grid; grid-template-columns: 210px minmax(0, 1fr); gap: 28px; margin: -12px 0 32px; padding: 18px; border-left: 3px solid #8a6a32; background: var(--paper-deep); }.unassigned > div { display: grid; align-content: start; gap: 5px; }.unassigned span { color: var(--ink-muted); font-size: 11px; }.unassigned ul { display: grid; gap: 8px; margin: 0; padding: 0; list-style: none; }.unassigned li { display: flex; justify-content: space-between; gap: 16px; padding-bottom: 8px; border-bottom: 1px solid var(--line); font-size: 12px; }
</style>
