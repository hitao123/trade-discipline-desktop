<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, shallowRef } from 'vue'

import { formatCNY, formatPrice } from '@/renderer/lib/format'
import type { PlanRecord } from '@/renderer/types'

interface ConfirmationPayload {
  planId: string
  startedAt: string
  noFomo: boolean
  noLossRecovery: boolean
  noAveragingDown: boolean
}

const props = defineProps<{
  plan: PlanRecord
  startedAt: string
  busy: boolean
  error?: string
}>()

const emit = defineEmits<{
  confirm: [payload: ConfirmationPayload]
}>()

const acknowledgements = reactive({ noFomo: false, noLossRecovery: false, noAveragingDown: false })
const nowMilliseconds = shallowRef(Date.now())
let ticker: number | undefined

const startedMilliseconds = computed(() => new Date(props.startedAt).getTime())
const remainingSeconds = computed(() => Math.max(0, Math.ceil((startedMilliseconds.value + 30_000 - nowMilliseconds.value) / 1_000)))
const canConfirm = computed(() => remainingSeconds.value === 0 && acknowledgements.noFomo && acknowledgements.noLossRecovery && acknowledgements.noAveragingDown && !props.busy)
const currency = computed(() => String(props.plan.draft.code ?? '').endsWith('.HK') ? 'HKD' : 'CNY')
const targetRange = computed(() => {
  const low = Number(props.plan.draft.targetExitLowMinor ?? 0)
  const high = Number(props.plan.draft.targetExitHighMinor ?? 0)
  if (low <= 0 || high <= 0) return '未设置价格目标区间'
  return `${formatPrice(low, currency.value)} 至 ${formatPrice(high, currency.value)}`
})
const breakCondition = computed(() => String(props.plan.draft.breakCondition ?? '未填写'))
const validUntilLabel = computed(() => {
  const raw = props.plan.draft.validUntil
  return raw ? new Date(String(raw)).toLocaleString('zh-CN') : '未填写'
})

function submit() {
  if (!canConfirm.value) return
  emit('confirm', { planId: props.plan.id, startedAt: props.startedAt, ...acknowledgements })
}

onMounted(() => {
  ticker = window.setInterval(() => { nowMilliseconds.value = Date.now() }, 1_000)
})

onBeforeUnmount(() => {
  if (ticker !== undefined) window.clearInterval(ticker)
})
</script>

<template>
  <section class="confirmation" aria-label="开仓前确认">
    <div class="confirmation__heading">
      <p class="kicker">PRE-TRADE PAUSE</p>
      <h3>下单前，先停 30 秒</h3>
      <p>这不是券商指令。确认会写入本地记录，然后请你自行到券商 App 下单。</p>
    </div>
    <div class="confirmation__facts">
      <span>{{ plan.draft.code }} · {{ plan.draft.quantity }} 股</span>
      <span>预计人民币资金：{{ formatCNY(Number(plan.draft.estimatedCostFen ?? 0)) }}</span>
      <span>最大计划损失：{{ formatCNY(Number(plan.draft.maxPlanLossFen ?? 0)) }}</span>
      <span>风险退出：{{ formatPrice(Number(plan.draft.riskExitMinor ?? 0), currency) }}</span>
      <span>目标区间：{{ targetRange }}</span>
      <span>逻辑破坏：{{ breakCondition }}</span>
      <span>计划有效至：{{ validUntilLabel }}</span>
    </div>
    <p v-if="remainingSeconds > 0" class="confirmation__wait" role="status">请再等待 {{ remainingSeconds }} 秒</p>
    <p v-else class="confirmation__ready" role="status">30 秒已到，请逐项确认后自行下单。</p>
    <div class="confirmation__checks">
      <label><input v-model="acknowledgements.noFomo" type="checkbox" />我不是因为害怕错过而买入</label>
      <label><input v-model="acknowledgements.noLossRecovery" type="checkbox" />我不是为了扳回亏损而买入</label>
      <label><input v-model="acknowledgements.noAveragingDown" type="checkbox" />我不会在浮亏时继续加仓</label>
    </div>
    <p v-if="error" class="confirmation__error" role="alert">{{ error }}</p>
    <button class="button button--primary" type="button" :disabled="!canConfirm" @click="submit">{{ busy ? '正在记录…' : '确认后去券商下单' }}</button>
  </section>
</template>

<style scoped>
.confirmation { display: grid; gap: 14px; margin-top: 18px; padding: 18px; border: 1px solid var(--line); border-top: 3px solid var(--ink); background: var(--paper); }.confirmation__heading { display: grid; gap: 5px; }.confirmation__heading h3 { margin: 0; font-family: var(--font-serif); font-size: 22px; font-weight: 500; }.confirmation__heading p:not(.kicker) { margin: 0; color: var(--ink-muted); font-size: 12px; line-height: 1.6; }.kicker { margin: 0; color: var(--ink-faint); font-size: 10px; letter-spacing: .12em; }.confirmation__facts { display: flex; flex-wrap: wrap; gap: 8px; }.confirmation__facts span { padding: 6px 8px; border: 1px solid var(--line); color: var(--ink-muted); font-size: 11px; }.confirmation__wait { margin: 0; color: var(--accent); font-size: 13px; }.confirmation__ready { margin: 0; color: var(--success); font-size: 13px; }.confirmation__checks { display: grid; gap: 9px; padding: 14px; background: var(--paper-deep); }.confirmation__checks label { display: flex; align-items: center; gap: 8px; color: var(--ink); font-size: 13px; }.confirmation__checks input { width: auto; }.confirmation__error { margin: 0; color: var(--accent); font-size: 12px; }
</style>
