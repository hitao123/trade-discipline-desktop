<script setup lang="ts">
import { computed, reactive, watch } from 'vue'

import { activeQualifiedPlans, preferredInstrumentId } from '@/renderer/lib/plan-options'
import { dateTimeLocal } from '@/renderer/lib/format'
import type { Instrument, PlanRecord } from '@/renderer/types'

const props = defineProps<{ instruments: Instrument[]; plans: PlanRecord[]; busy: boolean; preferredInstrumentId?: string }>()
const emit = defineEmits<{ submit: [payload: Record<string, unknown>] }>()

const form = reactive({
  planId: '', instrumentId: '', side: 'buy', executedAt: dateTimeLocal(), quantity: 100,
  localPrice: 0, localAmount: 0, settlementYuan: 0, exitCode: '', evidence: '', brokerReference: '',
  fearScore: 0, greedScore: 0, revengeScore: 0,
})
const selected = computed(() => props.instruments.find(item => item.id === form.instrumentId))
const qualifiedPlans = computed(() => activeQualifiedPlans(props.plans))
const noPlan = computed(() => form.planId === '')

watch(() => props.instruments, (items) => {
  if (!form.instrumentId)
    form.instrumentId = preferredInstrumentId(items, props.preferredInstrumentId)
}, { immediate: true })
watch(selected, (item) => { if (item) form.quantity = item.lotSize }, { immediate: true })

function submit() {
  const settlement = Math.round(Math.abs(form.settlementYuan) * 100) * (form.side === 'buy' ? -1 : 1)
  emit('submit', {
    planId: form.planId || undefined, instrumentId: form.instrumentId, side: form.side,
    executedAt: new Date(form.executedAt).toISOString(), quantity: Number(form.quantity),
    localPriceMinor: Math.round(form.localPrice * 100), localAmountMinor: Math.round(form.localAmount * 100),
    settlementFen: settlement, exitCode: form.exitCode || undefined, evidence: form.evidence || undefined,
    brokerReference: form.brokerReference || undefined,
    emotion: { fearScore: Number(form.fearScore), greedScore: Number(form.greedScore), revengeScore: Number(form.revengeScore) },
  })
}
</script>

<template>
  <form class="form-stack" @submit.prevent="submit">
    <div class="boundary-note"><strong>这里只补录，不会向券商发送订单</strong><span>请先在券商 App 完成真实交易，再把实际结果写进来。</span></div>
    <label class="field"><span>对应计划（可不选）</span><select v-model="form.planId" aria-label="对应计划（可不选）"><option value="">无计划成交</option><option v-for="plan in qualifiedPlans" :key="plan.id" :value="plan.id">{{ plan.draft.code }} · {{ plan.status }} · {{ plan.draft.quantity }} 股</option></select></label>
    <p v-if="noPlan" class="violation-warning">无计划成交将记为严重违规，但不会阻止保存。</p>
    <div class="field-grid">
      <label class="field"><span>证券</span><select v-model="form.instrumentId"><option v-for="instrument in instruments" :key="instrument.id" :value="instrument.id">{{ instrument.code }} · {{ instrument.name }}</option></select></label>
      <label class="field"><span>买卖方向</span><select v-model="form.side"><option value="buy">买入</option><option value="sell">卖出</option></select></label>
    </div>
    <div class="field-grid field-grid--three">
      <label class="field"><span>成交时间</span><input v-model="form.executedAt" type="datetime-local" required /></label>
      <label class="field"><span>数量</span><input v-model.number="form.quantity" type="number" min="1" step="1" required /><small>当前交易单位 {{ selected?.lotSize ?? 100 }}</small></label>
      <label class="field"><span>本币成交价</span><input v-model.number="form.localPrice" type="number" min="0" step="0.01" required /></label>
    </div>
    <div class="field-grid field-grid--three">
      <label class="field"><span>本币成交金额</span><input v-model.number="form.localAmount" type="number" min="0" step="0.01" required /></label>
      <label class="field"><span>实际人民币扣款/到账（元）</span><input v-model.number="form.settlementYuan" type="number" min="0" step="0.01" required /><small>只填正数，系统按买卖方向记账</small></label>
      <label class="field"><span>券商成交编号（可选）</span><input v-model="form.brokerReference" /></label>
    </div>
    <div v-if="form.side === 'sell'" class="field-grid">
      <label class="field"><span>卖出代码 T/B/R/C</span><select v-model="form.exitCode" required><option value="">请选择</option><option value="T">T · 达到目标/估值退出</option><option value="B">B · 逻辑破坏</option><option value="R">R · 风险退出</option><option value="C">C · 组合约束</option></select></label>
      <label class="field"><span>卖出证据</span><textarea v-model="form.evidence" rows="3" required /></label>
    </div>
    <div class="field-grid field-grid--three">
      <label class="field"><span>害怕 0–10</span><input v-model.number="form.fearScore" aria-label="害怕 0–10" type="number" min="0" max="10" /></label>
      <label class="field"><span>贪婪 0–10</span><input v-model.number="form.greedScore" aria-label="贪婪 0–10" type="number" min="0" max="10" /></label>
      <label class="field"><span>扳本冲动 0–10</span><input v-model.number="form.revengeScore" aria-label="扳本冲动 0–10" type="number" min="0" max="10" /></label>
    </div>
    <button class="button button--primary" type="submit" :disabled="busy">{{ busy ? '正在记录…' : '确认如实记录' }}</button>
  </form>
</template>

<style scoped>
.form-stack { display: grid; gap: 17px; max-width: 900px; }
.field-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
.field-grid--three { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.boundary-note { display: flex; align-items: baseline; gap: 16px; padding: 14px 16px; color: var(--ink-muted); border: 1px solid var(--line); background: var(--paper-deep); font-size: 12px; }
.boundary-note strong { color: var(--ink); }
.violation-warning { margin: 0; padding: 12px 14px; color: #812b20; border-left: 3px solid var(--accent); background: #f2dfda; font-size: 13px; }
</style>
