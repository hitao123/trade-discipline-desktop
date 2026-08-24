<script setup lang="ts">
import { computed, reactive, watch } from 'vue'

import { dateTimeLocal, formatPrice } from '@/renderer/lib/format'
import type { ExecutionScreenshotPrefill } from '@/renderer/lib/execution-screenshot'
import type { Instrument, PlanRecord } from '@/renderer/types'

interface Props {
  instruments: Instrument[]
  plans: PlanRecord[]
  busy: boolean
  prefill?: ExecutionScreenshotPrefill | undefined
}

interface Emits {
  submit: [payload: Record<string, unknown>]
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const form = reactive({
  instrumentId: '',
  side: 'buy' as 'buy' | 'sell',
  quantity: 100,
  localPrice: 0,
  settlementYuan: null as number | null,
  planId: '',
  executedAt: dateTimeLocal(),
  brokerReference: '',
  exitCode: '',
  evidence: '',
  fearScore: 0,
  greedScore: 0,
  revengeScore: 0,
})

const selected = computed(() => props.instruments.find(item => item.id === form.instrumentId))
const localPriceMinor = computed(() => Math.round(Number(form.localPrice || 0) * 100))
const localPriceTenThousandth = computed(() => Math.round(Number(form.localPrice || 0) * 10_000))
const localAmountMinor = computed(() => Math.round(Number(form.localPrice || 0) * Number(form.quantity || 0) * 100))
const localAmountLabel = computed(() => formatPrice(localAmountMinor.value, selected.value?.currency ?? 'CNY'))
const priceStep = computed(() => selected.value?.assetType === 'etf' ? 0.001 : 0.01)

watch(() => props.instruments, (items) => {
  const preferred = items.find(item => item.code === '9988.HK') ?? items[0]
  if (!form.instrumentId && preferred) form.instrumentId = preferred.id
}, { immediate: true })

watch(selected, (instrument) => {
  if (!instrument) return
  form.quantity = instrument.lotSize
  form.settlementYuan = instrument.currency === 'CNY' ? localAmountMinor.value / 100 : null
}, { immediate: true, flush: 'sync' })

watch([localAmountMinor, () => selected.value?.currency, () => form.side], ([amount, currency]) => {
  form.settlementYuan = currency === 'CNY' ? Number(amount) / 100 : null
}, { flush: 'sync' })

watch(() => props.prefill, (draft) => {
  if (!draft) return
  if (draft.instrumentId) form.instrumentId = draft.instrumentId
  if (draft.side) form.side = draft.side
  if (draft.quantity !== undefined) form.quantity = draft.quantity
  if (draft.localPrice !== undefined) form.localPrice = draft.localPrice
  if (draft.settlementYuan !== undefined) form.settlementYuan = draft.settlementYuan
  if (draft.executedAt) form.executedAt = draft.executedAt
}, { immediate: true })

function submit() {
  const settlementYuan = Math.abs(Number(form.settlementYuan || 0))
  emit('submit', {
    instrumentId: form.instrumentId,
    side: form.side,
    quantity: Number(form.quantity),
    localPriceMinor: localPriceMinor.value,
    localPriceTenThousandth: localPriceTenThousandth.value,
    settlementFen: Math.round(settlementYuan * 100) * (form.side === 'buy' ? -1 : 1),
    planId: form.planId || undefined,
    executedAt: form.executedAt ? new Date(form.executedAt).toISOString() : undefined,
    brokerReference: form.brokerReference || undefined,
    exitCode: form.exitCode || undefined,
    evidence: form.evidence || undefined,
    emotion: {
      fearScore: Number(form.fearScore),
      greedScore: Number(form.greedScore),
      revengeScore: Number(form.revengeScore),
    },
  })
}
</script>

<template>
  <form class="quick-form" @submit.prevent="submit">
    <div class="boundary-note">
      <strong>券商成交后补录</strong>
      <span>只写事实，不连接券商，也不会下单。</span>
    </div>

    <div class="quick-grid quick-grid--primary">
      <label class="field">
        <span>证券</span>
        <select v-model="form.instrumentId" required>
          <option v-for="instrument in instruments" :key="instrument.id" :value="instrument.id">{{ instrument.code }} · {{ instrument.name }}</option>
        </select>
      </label>
      <label class="field">
        <span>买卖方向</span>
        <select v-model="form.side" required><option value="buy">买入</option><option value="sell">卖出</option></select>
      </label>
      <label class="field">
        <span>数量</span>
        <input v-model.number="form.quantity" type="number" :min="selected?.lotSize ?? 1" :step="selected?.lotSize ?? 1" required />
        <small>一手 {{ selected?.lotSize ?? 100 }} 股</small>
      </label>
      <label class="field">
        <span>成交均价</span>
        <input v-model.number="form.localPrice" aria-label="成交均价" type="number" :min="priceStep" :step="priceStep" required />
        <small>{{ selected?.currency ?? '本币' }}</small>
      </label>
    </div>

    <div class="amount-row">
      <div><span>本币成交额</span><strong>{{ localAmountLabel }}</strong></div>
      <label class="field settlement-field">
        <span>券商实际人民币扣款/到账（元）</span>
        <input v-model.number="form.settlementYuan" aria-label="券商实际人民币扣款/到账（元）" type="number" min="0.01" step="0.01" required />
        <small v-if="selected?.currency === 'HKD'">必须照券商实际成交填写，不使用估算汇率入账</small>
        <small v-else>已按本币金额带出，可按券商费用修正</small>
      </label>
    </div>

    <p class="discipline-note">无计划也可以如实保存，系统会记入纪律记录。</p>

    <section class="emotion-panel" aria-labelledby="execution-emotion-title">
      <div>
        <strong id="execution-emotion-title">当时情绪</strong>
        <span>只填下单那一刻的真实程度，0 表示没有，10 表示非常强。</span>
      </div>
      <div class="emotion-grid">
        <label class="field"><span>恐惧 0–10</span><input v-model.number="form.fearScore" aria-label="恐惧 0–10" type="number" min="0" max="10" step="1" required /></label>
        <label class="field"><span>贪婪 0–10</span><input v-model.number="form.greedScore" aria-label="贪婪 0–10" type="number" min="0" max="10" step="1" required /></label>
        <label class="field"><span>回本/报复性冲动 0–10</span><input v-model.number="form.revengeScore" aria-label="回本/报复性冲动 0–10" type="number" min="0" max="10" step="1" required /></label>
      </div>
    </section>

    <details class="optional-details">
      <summary>稍后补充计划、时间和凭证</summary>
      <div class="quick-grid optional-grid">
        <label class="field"><span>对应计划（可不选）</span><select v-model="form.planId"><option value="">无计划成交</option><option v-for="plan in plans" :key="plan.id" :value="plan.id">{{ plan.draft.code }} · {{ plan.status }}</option></select></label>
        <label class="field"><span>成交时间</span><input v-model="form.executedAt" type="datetime-local" step="1" /></label>
        <label class="field"><span>券商成交编号（可选）</span><input v-model="form.brokerReference" /></label>
        <label v-if="form.side === 'sell'" class="field"><span>卖出代码</span><select v-model="form.exitCode"><option value="">暂未补充</option><option value="T">T · 目标/估值退出</option><option value="B">B · 逻辑破坏</option><option value="R">R · 风险退出</option><option value="C">C · 组合约束</option></select></label>
        <label v-if="form.side === 'sell'" class="field field--wide"><span>卖出证据（可稍后在复盘补充）</span><textarea v-model="form.evidence" rows="2" /></label>
      </div>
    </details>

    <button class="button button--primary quick-submit" type="submit" :disabled="busy">{{ busy ? '正在入账…' : '立即如实入账' }}</button>
  </form>
</template>

<style scoped>
.quick-form { display: grid; gap: 16px; max-width: 900px; }
.boundary-note { display: flex; align-items: baseline; gap: 14px; padding: 13px 15px; color: var(--ink-muted); border: 1px solid var(--line); background: var(--paper-deep); font-size: 12px; }
.boundary-note strong { color: var(--ink); }
.quick-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
.quick-grid--primary { grid-template-columns: 1.35fr .8fr .65fr .8fr; }
.amount-row { display: grid; grid-template-columns: minmax(180px, .75fr) minmax(260px, 1.25fr); gap: 14px; align-items: stretch; }
.amount-row > div { display: grid; align-content: center; gap: 7px; padding: 15px; border: 1px solid var(--line); background: var(--paper-deep); }
.amount-row span { color: var(--ink-muted); font-size: 11px; }
.amount-row strong { font-family: var(--font-serif); font-size: 24px; font-weight: 500; }
.settlement-field { margin: 0; }
.discipline-note { margin: 0; color: var(--accent); font-size: 12px; }
.emotion-panel { display: grid; gap: 12px; padding: 14px; border-left: 3px solid var(--accent); background: var(--paper-deep); }
.emotion-panel > div:first-child { display: grid; gap: 3px; }
.emotion-panel strong { font-size: 12px; }
.emotion-panel span { color: var(--ink-muted); font-size: 11px; }
.emotion-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.optional-details { padding: 12px 14px; border: 1px solid var(--line); color: var(--ink-muted); font-size: 12px; }
.optional-details summary { cursor: pointer; font-weight: 650; }
.optional-grid { margin-top: 14px; }
.field--wide { grid-column: 1 / -1; }
.quick-submit { min-height: 48px; font-size: 15px; }
@media (max-width: 900px) { .quick-grid--primary { grid-template-columns: repeat(2, minmax(0, 1fr)); }.amount-row { grid-template-columns: 1fr; }.emotion-grid { grid-template-columns: 1fr; } }
</style>
