<script setup lang="ts">
import { computed, reactive, watch } from 'vue'

import { preferredInstrumentId } from '@/renderer/lib/plan-options'
import { dateTimeLocal } from '@/renderer/lib/format'
import type { Instrument } from '@/renderer/types'

const props = defineProps<{ instruments: Instrument[]; busy: boolean; submitDisabled?: boolean; fieldErrors: Record<string, string>; initialDraft: Record<string, unknown> | undefined; submitLabel?: string; preferredInstrumentId?: string }>()
const emit = defineEmits<{ submit: [payload: Record<string, unknown>] }>()

const now = new Date()
const nextWeek = new Date(now.getTime() + 7 * 24 * 60 * 60 * 1000)
const form = reactive({
  instrumentId: '', thesis: '', falsification: '', pricedExpectation: '', evidence1: '', evidence2: '',
  breakCondition: '', exitCondition: '', entryLow: 0, entryHigh: 0, riskExit: 0, quantity: 100,
  targetExitLow: 0, targetExitHigh: 0,
  estimatedCost: 0, maxPlanLoss: 0, stressDrop: 30, fearScore: 0, greedScore: 0, revengeScore: 0,
  referencePriceAt: dateTimeLocal(now), validUntil: dateTimeLocal(nextWeek),
})

const selected = computed(() => props.instruments.find(item => item.id === form.instrumentId))
const quantityError = computed(() => {
  const lotSize = selected.value?.lotSize ?? 100
  const quantity = Number(form.quantity)
  return Number.isInteger(quantity) && quantity > 0 && quantity % lotSize === 0
    ? ''
    : `必须是 ${lotSize} 的整数倍`
})

watch(() => props.instruments, (instruments) => {
  if (!form.instrumentId)
    form.instrumentId = preferredInstrumentId(instruments, props.preferredInstrumentId)
}, { immediate: true })

watch(() => form.instrumentId, (id, previous) => {
  const instrument = props.instruments.find(item => item.id === id)
  if (instrument)
    form.quantity = instrument.lotSize
  if (previous && previous !== id) {
    form.entryLow = 0
    form.entryHigh = 0
    form.riskExit = 0
    form.targetExitLow = 0
    form.targetExitHigh = 0
    form.estimatedCost = 0
    form.maxPlanLoss = 0
  }
})

watch(() => props.initialDraft, (draft) => {
  if (!draft) return
  form.instrumentId = String(draft.instrumentId ?? '')
  form.thesis = String(draft.thesis ?? '')
  form.falsification = String(draft.falsification ?? '')
  form.pricedExpectation = String(draft.pricedExpectation ?? '')
  const evidence = Array.isArray(draft.evidence) ? draft.evidence : []
  form.evidence1 = String(evidence[0] ?? '')
  form.evidence2 = String(evidence[1] ?? '')
  form.breakCondition = String(draft.breakCondition ?? '')
  form.exitCondition = String(draft.exitCondition ?? '')
  form.entryLow = Number(draft.entryLowMinor ?? 0) / 100
  form.entryHigh = Number(draft.entryHighMinor ?? 0) / 100
  form.riskExit = Number(draft.riskExitMinor ?? 0) / 100
  form.targetExitLow = Number(draft.targetExitLowMinor ?? 0) / 100
  form.targetExitHigh = Number(draft.targetExitHighMinor ?? 0) / 100
  form.quantity = Number(draft.quantity ?? 100)
  form.estimatedCost = Number(draft.estimatedCostFen ?? 0) / 100
  form.maxPlanLoss = Number(draft.maxPlanLossFen ?? 0) / 100
  form.stressDrop = Number(draft.stressDropBP ?? 0) / 100
  form.fearScore = Number(draft.fearScore ?? 0)
  form.greedScore = Number(draft.greedScore ?? 0)
  form.revengeScore = Number(draft.revengeScore ?? 0)
  if (draft.referencePriceAt) form.referencePriceAt = dateTimeLocal(new Date(String(draft.referencePriceAt)))
  if (draft.validUntil) form.validUntil = dateTimeLocal(new Date(String(draft.validUntil)))
}, { immediate: true })

function submit() {
  emit('submit', {
    instrumentId: form.instrumentId,
    thesis: form.thesis,
    falsification: form.falsification,
    pricedExpectation: form.pricedExpectation,
    evidence: [form.evidence1, form.evidence2].map(item => item.trim()).filter(Boolean),
    breakCondition: form.breakCondition,
    exitCondition: form.exitCondition,
    entryLowMinor: Math.round(form.entryLow * 100),
    entryHighMinor: Math.round(form.entryHigh * 100),
    riskExitMinor: Math.round(form.riskExit * 100),
    targetExitLowMinor: Math.round(form.targetExitLow * 100),
    targetExitHighMinor: Math.round(form.targetExitHigh * 100),
    quantity: Number(form.quantity),
    estimatedCostFen: Math.round(form.estimatedCost * 100),
    maxPlanLossFen: Math.round(form.maxPlanLoss * 100),
    stressDropBP: Math.round(form.stressDrop * 100),
    fearScore: Number(form.fearScore),
    greedScore: Number(form.greedScore),
    revengeScore: Number(form.revengeScore),
    referencePriceAt: new Date(form.referencePriceAt).toISOString(),
    validUntil: new Date(form.validUntil).toISOString(),
  })
}
</script>

<template>
  <form class="form-stack" novalidate @submit.prevent="submit">
    <fieldset>
      <legend>01 · 判断</legend>
      <label class="field"><span>证券</span><select v-model="form.instrumentId" aria-label="证券"><option v-for="instrument in instruments" :key="instrument.id" :value="instrument.id">{{ instrument.code }} · {{ instrument.name }} · 每手 {{ instrument.lotSize }}</option></select></label>
      <label class="field"><span>一句话买入逻辑</span><textarea v-model="form.thesis" rows="2" required /><small>{{ fieldErrors.thesis }}</small></label>
      <div class="field-grid">
        <label class="field"><span>市场可能错在哪里</span><textarea v-model="form.falsification" rows="3" required /></label>
        <label class="field"><span>当前价格已反映什么</span><textarea v-model="form.pricedExpectation" rows="3" required /></label>
      </div>
      <div class="field-grid">
        <label class="field"><span>未来证据 1</span><input v-model="form.evidence1" required /></label>
        <label class="field"><span>未来证据 2（可选）</span><input v-model="form.evidence2" /></label>
      </div>
      <label class="field"><span>逻辑破坏条件</span><textarea v-model="form.breakCondition" rows="2" required /></label>
      <label class="field"><span>目标或估值退出条件</span><textarea v-model="form.exitCondition" rows="2" required /></label>
    </fieldset>

    <fieldset>
      <legend>02 · 仓位与风险</legend>
      <div class="field-grid field-grid--three">
        <label class="field"><span>买入下限（本币）</span><input v-model.number="form.entryLow" aria-label="买入下限（本币）" type="number" min="0" step="0.01" required /></label>
        <label class="field"><span>买入上限（本币）</span><input v-model.number="form.entryHigh" aria-label="买入上限（本币）" type="number" min="0" step="0.01" required /></label>
        <label class="field"><span>风险退出价（本币）</span><input v-model.number="form.riskExit" type="number" min="0" step="0.01" required /></label>
      </div>
      <div class="field-grid">
        <label class="field"><span>目标退出下限（本币）</span><input v-model.number="form.targetExitLow" type="number" min="0" step="0.01" /><small class="field__hint">可选；填写后需同时填写上限</small></label>
        <label class="field"><span>目标退出上限（本币）</span><input v-model.number="form.targetExitHigh" type="number" min="0" step="0.01" /><small class="field__hint">进入该区间时提醒你复核</small></label>
      </div>
      <div class="field-grid field-grid--three">
        <label class="field"><span>计划股数</span><input v-model.number="form.quantity" aria-label="计划股数" :aria-invalid="quantityError ? 'true' : 'false'" type="number" min="1" step="1" required /><small v-if="quantityError" role="alert">{{ quantityError }}</small><small v-else class="field__hint">每手 {{ selected?.lotSize ?? 100 }} 股</small></label>
        <label class="field"><span>预计人民币资金（元）</span><input v-model.number="form.estimatedCost" type="number" min="0" step="0.01" required /></label>
        <label class="field"><span>最大计划损失（元）</span><input v-model.number="form.maxPlanLoss" type="number" min="0" step="0.01" required /></label>
      </div>
      <div class="field-grid field-grid--three">
        <label class="field"><span>压力跌幅（%）</span><input v-model.number="form.stressDrop" type="number" min="0.01" max="100" step="0.01" required /></label>
        <label class="field"><span>参考价时间</span><input v-model="form.referencePriceAt" type="datetime-local" required /></label>
        <label class="field"><span>计划有效期</span><input v-model="form.validUntil" type="datetime-local" required /></label>
      </div>
    </fieldset>

    <fieldset>
      <legend>03 · 当时的情绪</legend>
      <div class="field-grid field-grid--three">
        <label class="field"><span>害怕 0–10</span><input v-model.number="form.fearScore" type="number" min="0" max="10" /></label>
        <label class="field"><span>贪婪 0–10</span><input v-model.number="form.greedScore" type="number" min="0" max="10" /></label>
        <label class="field"><span>扳本冲动 0–10</span><input v-model.number="form.revengeScore" type="number" min="0" max="10" /></label>
      </div>
    </fieldset>

    <button class="button button--primary" type="submit" :disabled="busy || submitDisabled">{{ busy ? '正在校验…' : (submitLabel ?? '保存并校验') }}</button>
  </form>
</template>

<style scoped>
.form-stack { display: grid; gap: 24px; }
fieldset { display: grid; gap: 15px; margin: 0; padding: 20px; border: 1px solid var(--line); }
legend { padding: 0 8px; color: var(--ink-muted); font-size: 12px; font-weight: 750; letter-spacing: .08em; }
.field-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
.field-grid--three { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.field small { min-height: 14px; color: var(--accent); }
.field .field__hint { color: var(--ink-faint); }
</style>
