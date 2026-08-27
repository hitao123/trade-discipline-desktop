<script setup lang="ts">
import { computed, reactive, type DeepReadonly, watch } from 'vue'

import { dateTimeLocal } from '@/renderer/lib/format'
import type { AllocationItemProgress } from '@/renderer/types'

const props = defineProps<{ items: readonly DeepReadonly<AllocationItemProgress>[]; busy: boolean }>()
const emit = defineEmits<{ recordAdjustment: [payload: { itemKey: string; adjustmentFen: number; observedAt: string }] }>()

const form = reactive({ itemKey: '', yuan: '', observedAt: dateTimeLocal() })
const selected = computed(() => props.items.find(item => item.item.key === form.itemKey))
const localError = computed(() => {
  if (form.yuan === '') return ''
  const value = Number(form.yuan)
  return Number.isFinite(value) ? '' : '请输入有效的人民币调整金额'
})

watch(() => props.items, (items) => {
  if (!items.some(item => item.item.key === form.itemKey)) form.itemKey = items[0]?.item.key ?? ''
}, { immediate: true })

function submit() {
  if (!form.itemKey || form.yuan === '' || localError.value || !form.observedAt) return
  emit('recordAdjustment', { itemKey: form.itemKey, adjustmentFen: Math.round(Number(form.yuan) * 100), observedAt: new Date(form.observedAt).toISOString() })
}
</script>

<template>
  <section class="value-form-section">
    <div class="section-heading"><p>外部调整</p><h2>补充持仓之外的金额</h2><span>持仓和现金已自动同步。这里只记录费用、场外资产等差额，可填正数或负数。</span></div>
    <form class="value-form" @submit.prevent="submit">
      <label class="field"><span>资产项目</span><select v-model="form.itemKey" aria-label="资产项目"><option v-for="progress in items" :key="progress.item.key" :value="progress.item.key">{{ progress.item.name }}</option></select></label>
      <label class="field"><span>外部调整（元）</span><input v-model="form.yuan" aria-label="外部调整（元）" type="number" step="0.01" placeholder="例如 200 或 -50" /></label>
      <label class="field"><span>记录时间</span><input v-model="form.observedAt" type="datetime-local" /></label>
      <button class="button button--primary" type="submit" :disabled="busy || !form.itemKey || form.yuan === '' || Boolean(localError)">{{ busy ? '正在保存…' : '保存调整' }}</button>
    </form>
    <p v-if="selected" class="value-form__hint">{{ selected.item.name }} 当前自动值 {{ (selected.linkedValueFen / 100).toLocaleString('zh-CN') }} 元；新调整会替代上一条调整，不会生成交易指令。</p>
    <p v-if="localError" class="value-form__error" role="alert">{{ localError }}</p>
  </section>
</template>

<style scoped>
.value-form-section { display: grid; grid-template-columns: 210px minmax(0, 1fr); gap: 28px; padding: 28px 0; border-top: 1px solid var(--line); }.section-heading p { margin: 0 0 6px; color: var(--accent); font-size: 10px; letter-spacing: .12em; }.section-heading h2 { margin: 0; font-family: var(--font-serif); font-size: 22px; font-weight: 500; }.section-heading span { display: block; margin-top: 8px; color: var(--ink-faint); font-size: 11px; line-height: 1.6; }.value-form { display: grid; grid-template-columns: 1.4fr 1fr 1.15fr auto; align-items: end; gap: 12px; }.value-form__hint, .value-form__error { grid-column: 2; margin: -14px 0 0; font-size: 11px; }.value-form__hint { color: var(--ink-faint); }.value-form__error { color: var(--accent); }
</style>
