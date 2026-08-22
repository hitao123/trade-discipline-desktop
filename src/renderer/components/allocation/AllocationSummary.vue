<script setup lang="ts">
import { computed, type DeepReadonly } from 'vue'

import { formatCNY, formatPercentBP } from '@/renderer/lib/format'
import type { AllocationOverview } from '@/renderer/types'

const props = defineProps<{ overview: DeepReadonly<AllocationOverview> }>()
const targetReturn = computed(() => `${(props.overview.version.draft.targetReturnBP / 100).toFixed(2)}%`)
const deadline = computed(() => props.overview.version.draft.targetDeadline.replaceAll('-', ' 年 ').replace(/ 年 (\d{2})$/, ' 月'))
</script>

<template>
  <section class="summary" aria-label="资产配置目标摘要">
    <div class="summary__metric"><span>当前总资产</span><strong>{{ formatCNY(overview.currentTotalFen) }}</strong></div>
    <div class="summary__metric"><span>年末目标</span><strong>{{ formatCNY(overview.targetTotalFen) }}</strong><small>截至 {{ deadline }} · 目标收益 {{ targetReturn }}</small></div>
    <div class="summary__metric"><span>当前收益</span><strong :class="{ positive: overview.returnBP > 0, negative: overview.returnBP < 0 }">{{ formatPercentBP(overview.returnBP) }}</strong></div>
    <div class="summary__metric"><span>距目标差额</span><strong>{{ formatCNY(overview.goalGapFen) }}</strong><small>正数表示尚未达到目标</small></div>
  </section>

  <section class="table-frame" aria-label="资产配置进度">
    <table class="allocation-table">
      <thead><tr><th>资产</th><th>目标</th><th>当前市值</th><th>当前占比</th><th>建仓偏离</th><th>再平衡偏离</th><th>最近记录</th></tr></thead>
      <tbody>
        <tr v-for="progress in overview.items" :key="progress.item.key">
          <td><strong>{{ progress.item.name }}</strong><small v-if="progress.item.targetShares">目标约 {{ progress.item.targetShares }} 股</small></td>
          <td>{{ (progress.item.targetWeightBP / 100).toFixed(1) }}%</td>
          <td>{{ formatCNY(progress.currentValueFen) }}</td>
          <td>{{ (progress.currentWeightBP / 100).toFixed(1) }}%</td>
          <td>{{ formatCNY(progress.buildGapFen) }}</td>
          <td>{{ formatCNY(progress.rebalanceGapFen) }}</td>
          <td><time v-if="progress.latestValueRecorded">{{ new Date(progress.latestValueRecorded.observedAt).toLocaleString('zh-CN') }}</time><span v-else>尚未记录</span></td>
        </tr>
      </tbody>
    </table>
  </section>
</template>

<style scoped>
.summary { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); margin-bottom: 24px; border: 1px solid var(--line); }.summary__metric { display: grid; gap: 7px; min-height: 122px; padding: 18px; border-right: 1px solid var(--line); }.summary__metric:last-child { border-right: 0; }.summary__metric span, .summary__metric small { color: var(--ink-faint); font-size: 10px; }.summary__metric strong { font-family: var(--font-serif); font-size: 23px; font-weight: 500; }.positive { color: var(--success); }.negative { color: var(--accent); }.table-frame { overflow: auto; margin-bottom: 32px; border: 1px solid var(--line); }.allocation-table { width: 100%; min-width: 920px; border-collapse: collapse; font-size: 12px; }.allocation-table th { padding: 11px 13px; color: var(--ink-faint); background: var(--paper-deep); font-size: 10px; text-align: left; }.allocation-table td { padding: 13px; border-top: 1px solid var(--line); vertical-align: top; }.allocation-table td small, .allocation-table td time, .allocation-table td span { display: block; margin-top: 4px; color: var(--ink-faint); font-size: 10px; }
</style>
