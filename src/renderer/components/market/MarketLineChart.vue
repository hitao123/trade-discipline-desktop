<script setup lang="ts">
import { computed, useId } from 'vue'

import { buildLineGeometry, type ChartDatum, type ChartPoint } from '@/renderer/lib/chart'

interface Props {
  label: string
  points: ChartDatum[]
  formatValue: (value: number) => string
  emptyText?: string
  loading?: boolean
  lowThreshold?: number
}

const props = withDefaults(defineProps<Props>(), {
  emptyText: '暂无可用数据',
  loading: false,
})

const chartWidth = 600
const chartHeight = 220
const chartPadding = 28
const viewBox = '0 0 ' + chartWidth + ' ' + chartHeight
const thresholdGradientId = `chart-threshold-${useId()}`
const geometry = computed(() => buildLineGeometry(props.points, chartWidth, chartHeight, chartPadding))
const firstPoint = computed(() => props.points[0])
const lastPoint = computed(() => props.points.at(-1))
const chartLabel = computed(() => {
  if (!firstPoint.value || !lastPoint.value) return props.label
  return props.label + '，' + firstPoint.value.date + ' 至 ' + lastPoint.value.date + '，最新 ' + props.formatValue(lastPoint.value.value)
})
const thresholdY = computed(() => {
  if (props.lowThreshold === undefined || geometry.value.points.length === 0) return undefined
  const { min, max } = geometry.value
  const plotHeight = chartHeight - chartPadding * 2
  return chartPadding + ((max - props.lowThreshold) / (max - min)) * plotHeight
})
const thresholdOffset = computed(() => {
  if (thresholdY.value === undefined) return undefined
  return `${Math.max(0, Math.min(100, (thresholdY.value / chartHeight) * 100))}%`
})
const visibleThresholdY = computed(() => {
  if (thresholdY.value === undefined || thresholdY.value < chartPadding || thresholdY.value > chartHeight - chartPadding) return undefined
  return thresholdY.value
})

function pointStyle(point: ChartPoint) {
  return {
    left: String((point.x / chartWidth) * 100) + '%',
    top: String((point.y / chartHeight) * 100) + '%',
  }
}

function pointLabel(point: ChartPoint) {
  return point.date + ' ' + props.formatValue(point.value)
}

function shortDate(date: string) {
  return date.slice(5)
}
</script>

<template>
  <div v-if="loading" class="chart-empty" role="status">历史数据加载中…</div>
  <div v-else-if="points.length === 0" class="chart-empty">{{ emptyText }}</div>
  <div v-else class="chart">
    <div class="chart-plot">
      <svg
        class="chart-svg"
        :viewBox="viewBox"
        role="img"
        :aria-label="chartLabel"
        preserveAspectRatio="none"
      >
        <defs v-if="thresholdOffset">
          <linearGradient :id="thresholdGradientId" x1="0" x2="0" y1="0" :y2="chartHeight" gradientUnits="userSpaceOnUse">
            <stop :offset="thresholdOffset" stop-color="var(--accent)" />
            <stop :offset="thresholdOffset" stop-color="#55715c" />
          </linearGradient>
        </defs>
        <line
          v-if="geometry.zeroY !== undefined"
          class="chart-zero"
          :x1="chartPadding"
          :x2="chartWidth - chartPadding"
          :y1="geometry.zeroY"
          :y2="geometry.zeroY"
          aria-label="零轴"
        />
        <line
          v-if="visibleThresholdY !== undefined"
          class="chart-threshold"
          :x1="chartPadding"
          :x2="chartWidth - chartPadding"
          :y1="visibleThresholdY"
          :y2="visibleThresholdY"
          aria-hidden="true"
        />
        <path
          class="chart-line"
          :d="geometry.path"
          :style="thresholdOffset ? { stroke: `url(#${thresholdGradientId})` } : undefined"
        />
      </svg>
      <button
        v-for="point in geometry.points"
        :key="point.date"
        class="chart-point"
        :class="{
          'chart-point--negative': point.value < 0,
          'chart-point--low': lowThreshold !== undefined && point.value < lowThreshold,
        }"
        type="button"
        :style="pointStyle(point)"
        :aria-label="pointLabel(point)"
      >
        <span class="chart-tooltip">{{ pointLabel(point) }}</span>
      </button>
    </div>
    <div class="chart-axis" aria-hidden="true">
      <span>{{ shortDate(firstPoint!.date) }}</span>
      <span>{{ shortDate(lastPoint!.date) }}</span>
    </div>
  </div>
</template>

<style scoped>
.chart { min-width: 0; }
.chart-plot { position: relative; height: 220px; }
.chart-svg { display: block; width: 100%; height: 100%; overflow: visible; }
.chart-line { fill: none; stroke: var(--accent); stroke-width: 2.2; vector-effect: non-scaling-stroke; animation: line-in 260ms ease-out; }
.chart-zero { stroke: var(--line); stroke-width: 1; stroke-dasharray: 5 5; vector-effect: non-scaling-stroke; }
.chart-threshold { stroke: #55715c; stroke-width: 1; stroke-dasharray: 3 5; opacity: .45; vector-effect: non-scaling-stroke; }
.chart-point {
  position: absolute;
  width: 10px;
  height: 10px;
  padding: 0;
  border: 2px solid var(--paper);
  border-radius: 50%;
  background: var(--accent);
  box-shadow: 0 0 0 1px var(--accent);
  cursor: crosshair;
  transform: translate(-50%, -50%);
  transition: transform 140ms ease, box-shadow 140ms ease;
}
.chart-point--negative,
.chart-point--low { background: #55715c; box-shadow: 0 0 0 1px #55715c; }
.chart-point:hover, .chart-point:focus-visible { z-index: 2; transform: translate(-50%, -50%) scale(1.35); }
.chart-tooltip {
  position: absolute;
  left: 50%;
  bottom: 16px;
  width: max-content;
  max-width: 180px;
  padding: 6px 8px;
  color: var(--paper);
  background: var(--ink);
  font-size: 10px;
  font-weight: 600;
  opacity: 0;
  pointer-events: none;
  transform: translateX(-50%);
  transition: opacity 120ms ease;
}
.chart-point:hover .chart-tooltip, .chart-point:focus-visible .chart-tooltip { opacity: 1; }
.chart-axis { display: flex; justify-content: space-between; color: var(--ink-faint); font-size: 10px; font-variant-numeric: tabular-nums; }
.chart-empty { display: grid; min-height: 220px; place-items: center; color: var(--ink-faint); border-block: 1px solid var(--line); font-size: 12px; }

@keyframes line-in {
  from { opacity: 0; transform: translateY(4px); }
  to { opacity: 1; transform: translateY(0); }
}

@media (prefers-reduced-motion: reduce) {
  .chart-line { animation: none; }
  .chart-point, .chart-tooltip { transition: none; }
}
</style>
