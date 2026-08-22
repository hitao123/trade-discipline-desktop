<script setup lang="ts">
import { computed, reactive, type DeepReadonly, watch } from 'vue'

import type { AllocationDraft, AllocationItem, AllocationOverview } from '@/renderer/types'

type EditableItem = AllocationItem & { targetWeightPercent: number }

const props = defineProps<{ overview: DeepReadonly<AllocationOverview>; busy: boolean }>()
const emit = defineEmits<{ save: [payload: { draft: AllocationDraft; reason: string }] }>()

const form = reactive({ initialCapitalYuan: 0, targetReturnPercent: 0, targetDeadline: '', reason: '', items: [] as EditableItem[] })
const totalWeight = computed(() => form.items.reduce((sum, item) => sum + finiteNumber(item.targetWeightPercent), 0))

function finiteNumber(value: unknown): number {
  const numberValue = Number(value)
  return Number.isFinite(numberValue) ? numberValue : 0
}

function copyDraft(draft: DeepReadonly<AllocationDraft>) {
	form.initialCapitalYuan = finiteNumber(draft.initialCapitalFen) / 100
	form.targetReturnPercent = finiteNumber(draft.targetReturnBP) / 100
  form.targetDeadline = draft.targetDeadline
	form.items = draft.items.map(item => ({
		key: item.key,
		name: item.name,
		targetWeightBP: finiteNumber(item.targetWeightBP),
		targetShares: finiteNumber(item.targetShares),
		buyRule: item.buyRule,
		sellRule: item.sellRule,
		note: item.note,
		targetWeightPercent: finiteNumber(item.targetWeightBP) / 100,
	}))
}

watch(() => props.overview.version.id, () => copyDraft(props.overview.version.draft), { immediate: true })

function submit() {
  if (!form.reason.trim() || totalWeight.value !== 100) return
  emit('save', {
    reason: form.reason.trim(),
    draft: {
		initialCapitalFen: Math.round(finiteNumber(form.initialCapitalYuan) * 100),
		targetReturnBP: Math.round(finiteNumber(form.targetReturnPercent) * 100),
		targetDeadline: form.targetDeadline,
		items: form.items.map(({ targetWeightPercent, ...item }) => ({ ...item, targetWeightBP: Math.round(finiteNumber(targetWeightPercent) * 100), targetShares: Math.round(finiteNumber(item.targetShares)) })),
    },
  })
}
</script>

<template>
  <section class="editor-section">
    <div class="section-heading"><p>配置版本 v{{ overview.version.version }}</p><h2>设定你的年末计划</h2><span>保存会创建一个新版本，已有市值记录不被覆盖。</span></div>
    <form class="editor-form" @submit.prevent="submit">
      <div class="editor-fields"><label class="field"><span>基准资金（元）</span><input v-model.number="form.initialCapitalYuan" type="number" min="1" step="1" /></label><label class="field"><span>目标收益（%）</span><input v-model.number="form.targetReturnPercent" type="number" min="0" step="0.01" /></label><label class="field"><span>目标截止日</span><input v-model="form.targetDeadline" type="date" /></label></div>
      <div class="table-frame"><table class="editor-table"><thead><tr><th>资产</th><th>目标权重（%）</th><th>目标股数</th><th>备注</th></tr></thead><tbody><tr v-for="item in form.items" :key="item.key"><td><strong>{{ item.name }}</strong></td><td><input v-model.number="item.targetWeightPercent" type="number" min="0" max="100" step="0.1" /></td><td><input v-model.number="item.targetShares" type="number" min="0" step="1" /></td><td><input v-model="item.note" type="text" /></td></tr></tbody></table></div>
      <p class="weight-total" :class="{ 'weight-total--invalid': totalWeight !== 100 }">目标权重合计：{{ totalWeight.toFixed(2) }}%{{ totalWeight === 100 ? '' : '（须为 100%）' }}</p>
      <details v-for="item in form.items" :key="`${item.key}-rules`" class="rule-detail"><summary>{{ item.name }} 的买入与卖出规则</summary><div class="rule-detail__fields"><label class="field"><span>买入规则</span><textarea v-model="item.buyRule" rows="3" /></label><label class="field"><span>卖出规则</span><textarea v-model="item.sellRule" rows="3" /></label></div></details>
      <label class="field"><span>修改原因</span><textarea v-model="form.reason" rows="3" placeholder="例如：调整年末目标，或补充已确认的配置原则。" /></label>
      <button class="button button--primary" type="submit" :disabled="busy || !form.reason.trim() || totalWeight !== 100">{{ busy ? '正在保存…' : '创建配置新版本' }}</button>
    </form>
  </section>
</template>

<style scoped>
.editor-section { display: grid; grid-template-columns: 210px minmax(0, 1fr); gap: 28px; padding: 28px 0; border-top: 1px solid var(--line); }.section-heading p { margin: 0 0 6px; color: var(--accent); font-size: 10px; letter-spacing: .12em; }.section-heading h2 { margin: 0; font-family: var(--font-serif); font-size: 22px; font-weight: 500; }.section-heading span { display: block; margin-top: 8px; color: var(--ink-faint); font-size: 11px; line-height: 1.6; }.editor-form { display: grid; gap: 14px; }.editor-fields { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }.table-frame { overflow: auto; border: 1px solid var(--line); }.editor-table { width: 100%; min-width: 670px; border-collapse: collapse; font-size: 12px; }.editor-table th { padding: 10px; color: var(--ink-faint); background: var(--paper-deep); font-size: 10px; text-align: left; }.editor-table td { padding: 8px 10px; border-top: 1px solid var(--line); }.editor-table input { width: 100%; min-height: 34px; padding: 7px; color: var(--ink); border: 1px solid var(--line); background: var(--paper); }.weight-total { margin: 0; color: var(--success); font-size: 12px; }.weight-total--invalid { color: var(--accent); }.rule-detail { padding: 12px; border: 1px solid var(--line); }.rule-detail summary { cursor: pointer; font-size: 12px; }.rule-detail__fields { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; padding-top: 12px; }
</style>
