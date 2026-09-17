<script setup lang="ts">
import { computed, reactive, watch } from 'vue'

import { dateTimeLocal } from '@/renderer/lib/format'
import type { ExecutionRecord, Instrument } from '@/renderer/types'

interface Props {
  record?: ExecutionRecord | undefined
  instruments: readonly Instrument[]
  busy: boolean
}

interface Emits {
  submit: [payload: { originalId: string; reason: string; draft: Record<string, unknown> }]
  cancel: []
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()
const form = reactive({
  instrumentId: '', side: 'buy' as 'buy' | 'sell', quantity: 1, localPrice: 0,
  settlementYuan: 0, executedAt: dateTimeLocal(), reason: '', planId: '', exitCode: '', evidence: '', brokerReference: '',
  fearScore: 0, greedScore: 0, revengeScore: 0,
  emotionState: 'unfilled' as 'recorded' | 'unfilled' | 'unknown',
})
const selected = computed(() => props.instruments.find(item => item.id === form.instrumentId))
const priceStep = computed(() => selected.value?.assetType === 'etf' ? 0.001 : 0.01)

watch(() => props.record, (record) => {
  if (!record) return
  form.instrumentId = record.instrumentId
  form.side = record.side
  form.quantity = record.quantity
  form.localPrice = record.localPriceTenThousandth / 10_000
  form.settlementYuan = Math.abs(record.settlementFen) / 100
  form.executedAt = dateTimeLocal(new Date(record.executedAt))
  form.reason = ''
  form.planId = record.planId ?? ''
  form.exitCode = record.exitCode ?? ''
  form.evidence = record.evidence ?? ''
  form.brokerReference = record.brokerReference ?? ''
  form.fearScore = record.emotion.fearScore
  form.greedScore = record.emotion.greedScore
  form.revengeScore = record.emotion.revengeScore
  form.emotionState = record.emotion.state ?? 'recorded'
}, { immediate: true })

function submit() {
  if (!props.record || !form.reason.trim()) return
  const localPrice = Number(form.localPrice)
  const quantity = Number(form.quantity)
  const settlementFen = Math.round(Math.abs(Number(form.settlementYuan)) * 100) * (form.side === 'buy' ? -1 : 1)
  emit('submit', {
    originalId: props.record.id,
    reason: form.reason.trim(),
    draft: {
      instrumentId: form.instrumentId, side: form.side, quantity,
      localPriceMinor: Math.round(localPrice * 100), localPriceTenThousandth: Math.round(localPrice * 10_000),
      localAmountMinor: Math.round(localPrice * quantity * 100), settlementFen,
      executedAt: new Date(form.executedAt).toISOString(), planId: form.planId || undefined,
      exitCode: form.exitCode, evidence: form.evidence, brokerReference: form.brokerReference,
      emotion: form.emotionState === 'recorded'
        ? { state: 'recorded', fearScore: Number(form.fearScore), greedScore: Number(form.greedScore), revengeScore: Number(form.revengeScore) }
        : { state: form.emotionState, fearScore: 0, greedScore: 0, revengeScore: 0 },
    },
  })
}
</script>

<template>
  <form v-if="record" class="correction" @submit.prevent="submit">
    <div class="correction__heading">
      <div><p>CORRECTION</p><h2>修正 {{ record.name || record.code }} 的成交</h2></div>
      <button class="text-button" type="button" @click="emit('cancel')">取消</button>
    </div>
    <p class="correction__note">保存时会自动冲正原记录并写入正确记录，现金和持仓一次更新。</p>
    <div class="correction__grid">
      <label class="field"><span>证券</span><select v-model="form.instrumentId" required><option v-for="instrument in instruments" :key="instrument.id" :value="instrument.id">{{ instrument.code }} · {{ instrument.name }}</option></select></label>
      <label class="field"><span>买卖方向</span><select v-model="form.side"><option value="buy">买入</option><option value="sell">卖出</option></select></label>
      <label class="field"><span>修正后数量</span><input v-model.number="form.quantity" aria-label="修正后数量" type="number" min="1" step="1" required /><small>一手 {{ selected?.lotSize ?? 100 }} 股，仅作参考</small></label>
      <label class="field"><span>修正后成交均价</span><input v-model.number="form.localPrice" type="number" :min="priceStep" :step="priceStep" required /><small>{{ selected?.currency ?? '本币' }}</small></label>
      <label class="field"><span>券商实际人民币扣款/到账（元）</span><input v-model.number="form.settlementYuan" type="number" min="0.01" step="0.01" required /></label>
      <label class="field"><span>成交时间</span><input v-model="form.executedAt" type="datetime-local" step="1" required /></label>
    </div>
    <div class="emotion-state" role="radiogroup" aria-label="情绪记录状态"><label><input v-model="form.emotionState" type="radio" value="unfilled" />未填写</label><label><input v-model="form.emotionState" type="radio" value="recorded" />如实评分</label><label><input v-model="form.emotionState" type="radio" value="unknown" />记不清</label></div>
    <div v-if="form.emotionState === 'recorded'" class="emotion-grid">
      <label class="field"><span>恐惧 0–10</span><input v-model.number="form.fearScore" type="number" min="0" max="10" step="1" /></label>
      <label class="field"><span>贪婪 0–10</span><input v-model.number="form.greedScore" type="number" min="0" max="10" step="1" /></label>
      <label class="field"><span>回本/报复性冲动 0–10</span><input v-model.number="form.revengeScore" type="number" min="0" max="10" step="1" /></label>
    </div>
    <label class="field"><span>修正原因</span><input v-model="form.reason" aria-label="修正原因" placeholder="例如：截图识别数量录错" required /></label>
    <button class="button button--primary" type="submit" :disabled="busy || !form.reason.trim()">{{ busy ? '正在修正…' : '保存修正' }}</button>
  </form>
</template>

<style scoped>
.correction { display: grid; gap: 15px; padding: 20px; border: 1px solid var(--line); border-top: 3px solid var(--accent); background: var(--paper-deep); }
.correction__heading { display: flex; align-items: start; justify-content: space-between; gap: 16px; }
.correction__heading p { margin: 0 0 5px; color: var(--accent); font-size: 10px; letter-spacing: .14em; }
.correction__heading h2 { margin: 0; font-family: var(--font-serif); font-size: 22px; font-weight: 500; }
.correction__note { margin: 0; color: var(--ink-muted); font-size: 11px; }
.correction__grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.emotion-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.emotion-state { display: flex; flex-wrap: wrap; gap: 14px; }.emotion-state label { display: flex; align-items: center; gap: 6px; font-size: 12px; }.emotion-state input { width: auto; }
@media (max-width: 900px) { .correction__grid, .emotion-grid { grid-template-columns: 1fr; } }
</style>
