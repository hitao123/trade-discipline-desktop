<script setup lang="ts">
import { computed, reactive, type DeepReadonly, watch } from 'vue'
import type { AllocationDraft, AllocationItem, AllocationOverview, Instrument } from '@/renderer/types'

type EditableItem = AllocationItem & { targetWeightPercent: number }
const props = defineProps<{ overview?: DeepReadonly<AllocationOverview>; suggestedCapitalFen?: number; instruments: readonly Instrument[]; busy: boolean }>()
const emit = defineEmits<{ save: [payload: { draft: AllocationDraft; reason: string }] }>()
const form = reactive({ initialCapitalYuan: 0, targetReturnPercent: 0, targetDeadline: '', reason: '', items: [] as EditableItem[] })
const finite = (value: unknown) => Number.isFinite(Number(value)) ? Number(value) : 0
const totalWeight = computed(() => form.items.reduce((sum, item) => sum + finite(item.targetWeightPercent), 0))
const hasCash = computed(() => form.items.some(item => item.role === 'cash' || item.key === 'cash-fund'))
const isNew = computed(() => !props.overview)

function copyDraft(draft?: DeepReadonly<AllocationDraft>) {
  form.initialCapitalYuan = draft ? finite(draft.initialCapitalFen) / 100 : finite(props.suggestedCapitalFen) / 100
  form.targetReturnPercent = draft ? finite(draft.targetReturnBP) / 100 : 0
  form.targetDeadline = draft?.targetDeadline ?? ''
  form.items = (draft?.items ?? []).map(item => ({ ...item, role: item.role ?? (item.key === 'cash-fund' ? 'cash' : 'holding'), instrumentCodes: [...item.instrumentCodes], targetWeightPercent: finite(item.targetWeightBP) / 100 }))
}
watch(() => props.overview?.version.id ?? props.suggestedCapitalFen, () => copyDraft(props.overview?.version.draft), { immediate: true })
const newKey = (prefix: string) => `${prefix}-${globalThis.crypto?.randomUUID?.() ?? Date.now().toString(36)}`
function addItem(role: 'holding' | 'cash') {
  if (role === 'cash' && hasCash.value) return
  form.items.push({ key: newKey(role), name: role === 'cash' ? '现金' : '', role, targetWeightBP: 0, targetShares: 0, instrumentCodes: [], buyRule: '', sellRule: '', note: '', targetWeightPercent: 0 })
}
function move(index: number, delta: number) {
  const next = index + delta
  if (next < 0 || next >= form.items.length) return
  const [item] = form.items.splice(index, 1)
  if (item) form.items.splice(next, 0, item)
}
function submit() {
  if (!form.reason.trim() || totalWeight.value !== 100 || !form.items.length) return
  emit('save', { reason: form.reason.trim(), draft: { initialCapitalFen: Math.round(finite(form.initialCapitalYuan) * 100), targetReturnBP: Math.round(finite(form.targetReturnPercent) * 100), targetDeadline: form.targetDeadline, items: form.items.map(({ targetWeightPercent, ...item }) => ({ ...item, instrumentCodes: item.role === 'cash' ? [] : item.instrumentCodes, targetWeightBP: Math.round(finite(targetWeightPercent) * 100), targetShares: Math.round(finite(item.targetShares)) })) } })
}
</script>

<template>
  <section class="editor-section">
    <div class="section-heading"><p>{{ isNew ? '新配置' : `配置版本 v${overview?.version.version}` }}</p><h2>{{ isNew ? '建立你的资产配置' : '调整配置计划' }}</h2><span>资产名称和关联证券由你决定；保存会留下版本与原因。</span></div>
    <form class="editor-form" @submit.prevent="submit">
      <div class="editor-fields"><label class="field"><span>基准资金（元）</span><input v-model.number="form.initialCapitalYuan" type="number" min="1" /></label><label class="field"><span>目标收益（%，可选）</span><input v-model.number="form.targetReturnPercent" type="number" min="0" step="0.01" /></label><label class="field"><span>目标截止日（可选）</span><input v-model="form.targetDeadline" type="date" /></label></div>
      <div class="item-actions"><button class="button" type="button" @click="addItem('holding')">添加资产</button><button class="button" type="button" :disabled="hasCash" @click="addItem('cash')">添加现金</button></div>
      <div class="table-frame"><table class="editor-table"><thead><tr><th>资产名称</th><th>目标权重（%）</th><th>目标股数</th><th>关联持仓</th><th>操作</th></tr></thead><tbody><tr v-for="(item, index) in form.items" :key="item.key"><td><input v-model="item.name" :aria-label="`资产名称 ${index + 1}`" /></td><td><input v-model.number="item.targetWeightPercent" :aria-label="`${item.name || `资产 ${index + 1}`}目标权重`" type="number" min="0" max="100" step="0.1" /></td><td><input v-model.number="item.targetShares" type="number" min="0" /></td><td><select v-if="item.role !== 'cash'" v-model="item.instrumentCodes" multiple aria-label="关联持仓"><option v-for="instrument in instruments" :key="instrument.id" :value="instrument.code">{{ instrument.code }} · {{ instrument.name }}</option></select><span v-else class="auto-source">自动读取现金</span></td><td><span class="row-actions"><button type="button" class="text-button" @click="move(index, -1)">上移</button><button type="button" class="text-button" @click="move(index, 1)">下移</button><button type="button" class="text-button" @click="form.items.splice(index, 1)">删除</button></span></td></tr></tbody></table></div>
      <p v-if="!form.items.length" class="empty-hint">先添加资产。系统不会替你预设腾讯、ETF 或现金比例。</p>
      <p class="weight-total" :class="{ 'weight-total--invalid': totalWeight !== 100 }">目标权重合计：{{ totalWeight.toFixed(2) }}%{{ totalWeight === 100 ? '' : '（须为 100%）' }}</p>
      <details v-for="item in form.items" :key="`${item.key}-rules`" class="rule-detail"><summary>{{ item.name || '未命名资产' }} 的规则与备注（可选）</summary><div class="rule-detail__fields"><label class="field"><span>买入规则</span><textarea v-model="item.buyRule" rows="3" /></label><label class="field"><span>卖出规则</span><textarea v-model="item.sellRule" rows="3" /></label><label class="field"><span>备注</span><textarea v-model="item.note" rows="2" /></label></div></details>
      <label class="field"><span>{{ isNew ? '建立原因' : '修改原因' }}</span><textarea v-model="form.reason" rows="3" /></label>
      <button class="button button--primary" type="submit" :disabled="busy || !form.reason.trim() || totalWeight !== 100 || !form.items.length">{{ busy ? '正在保存…' : isNew ? '建立资产配置' : '创建配置新版本' }}</button>
    </form>
  </section>
</template>

<style scoped>
.editor-section{display:grid;grid-template-columns:210px minmax(0,1fr);gap:28px;padding:28px 0;border-top:1px solid var(--line)}.section-heading p{margin:0 0 6px;color:var(--accent);font-size:10px}.section-heading h2{margin:0;font-family:var(--font-serif);font-size:22px;font-weight:500}.section-heading span,.auto-source,.empty-hint{color:var(--ink-faint);font-size:11px}.editor-form{display:grid;gap:14px}.editor-fields{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px}.item-actions,.row-actions{display:flex;gap:8px}.table-frame{overflow:auto;border:1px solid var(--line)}.editor-table{width:100%;min-width:840px;border-collapse:collapse;font-size:12px}.editor-table th{padding:10px;color:var(--ink-faint);background:var(--paper-deep);text-align:left}.editor-table td{padding:8px 10px;border-top:1px solid var(--line)}.editor-table input,.editor-table select{width:100%;min-height:34px;padding:7px;border:1px solid var(--line);background:var(--paper)}.weight-total{margin:0;color:var(--success);font-size:12px}.weight-total--invalid{color:var(--accent)}.rule-detail{padding:12px;border:1px solid var(--line)}.rule-detail__fields{display:grid;grid-template-columns:1fr 1fr;gap:12px;padding-top:12px}
</style>
