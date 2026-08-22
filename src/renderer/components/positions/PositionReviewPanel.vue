<script setup lang="ts">
import { computed, reactive } from 'vue'

import type { PriceAlert } from '@/renderer/types'

interface ReviewPayload {
  alertId: string
  decision: 'hold' | 'trim' | 'sell' | 'wait'
  reason: string
}

const props = defineProps<{ alert: PriceAlert; busy: boolean; error?: string }>()
const emit = defineEmits<{ submit: [payload: ReviewPayload] }>()

const form = reactive<{ decision: ReviewPayload['decision'] | ''; reason: string }>({ decision: '', reason: '' })
const title = computed(() => props.alert.kind === 'risk_exit' ? '已触及风险退出线' : '已进入目标退出区间')
const description = computed(() => props.alert.kind === 'risk_exit'
  ? '不要把提醒当作卖出指令。请对照原计划，说明为何持有、减仓、卖出或等待。'
  : '目标价格已触及。请记录是否分批兑现、继续持有，或先等待新的证据。')
const canSubmit = computed(() => form.decision !== '' && form.reason.trim().length > 0 && !props.busy)

function submit() {
  if (!canSubmit.value || form.decision === '') return
  emit('submit', { alertId: props.alert.id, decision: form.decision, reason: form.reason.trim() })
}
</script>

<template>
  <section class="review" aria-label="持仓复核">
    <div class="review__heading">
      <p class="kicker">POSITION REVIEW</p>
      <h3>{{ title }}</h3>
      <p>{{ description }}</p>
    </div>
    <p class="review__meta">触发价 {{ alert.triggerPriceMinor / 100 }} · 触发于 {{ new Date(alert.triggeredAt).toLocaleString('zh-CN') }}</p>
    <div class="review__options" role="radiogroup" aria-label="复核决定">
      <label><input v-model="form.decision" type="radio" value="hold" />继续持有</label>
      <label><input v-model="form.decision" type="radio" value="trim" />减仓</label>
      <label><input v-model="form.decision" type="radio" value="sell" />卖出</label>
      <label><input v-model="form.decision" type="radio" value="wait" />等待新证据</label>
    </div>
    <label class="field"><span>本次决定理由</span><textarea v-model="form.reason" rows="3" placeholder="写下依据，不写情绪化结论。" /></label>
    <p v-if="error" class="review__error" role="alert">{{ error }}</p>
    <button class="button button--primary" type="button" :disabled="!canSubmit" @click="submit">{{ busy ? '正在保存…' : '保存复核记录' }}</button>
  </section>
</template>

<style scoped>
.review { display: grid; gap: 14px; padding: 18px; border: 1px solid var(--line); border-left: 3px solid var(--accent); background: var(--paper); }.review__heading { display: grid; gap: 5px; }.review__heading h3 { margin: 0; color: var(--accent); font-family: var(--font-serif); font-size: 22px; font-weight: 500; }.review__heading p:not(.kicker) { margin: 0; color: var(--ink-muted); font-size: 12px; line-height: 1.6; }.kicker { margin: 0; color: var(--ink-faint); font-size: 10px; letter-spacing: .12em; }.review__meta { margin: 0; color: var(--ink-faint); font-size: 11px; }.review__options { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }.review__options label { display: flex; align-items: center; gap: 8px; padding: 9px; border: 1px solid var(--line); color: var(--ink); font-size: 12px; }.review__options input { width: auto; }.review__error { margin: 0; color: var(--accent); font-size: 12px; }
</style>
