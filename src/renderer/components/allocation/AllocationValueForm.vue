<script setup lang="ts">
import { computed, reactive, type DeepReadonly, watch } from 'vue'

import { dateTimeLocal } from '@/renderer/lib/format'
import type { AllocationItemProgress } from '@/renderer/types'

const props = defineProps<{ items: readonly DeepReadonly<AllocationItemProgress>[]; busy: boolean }>()
const emit = defineEmits<{ recordValue: [payload: { itemKey: string; valueFen: number; observedAt: string }] }>()

const form = reactive({ itemKey: '', yuan: '', observedAt: dateTimeLocal() })
const selected = computed(() => props.items.find(item => item.item.key === form.itemKey))
const localError = computed(() => {
  if (form.yuan === '') return ''
  const value = Number(form.yuan)
  return Number.isFinite(value) && value >= 0 ? '' : '请输入大于或等于 0 的人民币市值'
})

watch(() => props.items, (items) => {
  if (!items.some(item => item.item.key === form.itemKey)) form.itemKey = items[0]?.item.key ?? ''
}, { immediate: true })

function submit() {
  if (!form.itemKey || form.yuan === '' || localError.value || !form.observedAt) return
  emit('recordValue', { itemKey: form.itemKey, valueFen: Math.round(Number(form.yuan) * 100), observedAt: new Date(form.observedAt).toISOString() })
}
</script>

<template>
  <section class="value-form-section">
    <div class="section-heading"><p>手工市值</p><h2>追加一条当前事实</h2><span>不会连券商，也不会下单；旧记录会保留。</span></div>
    <form class="value-form" @submit.prevent="submit">
      <label class="field"><span>资产项目</span><select v-model="form.itemKey" aria-label="资产项目"><option v-for="progress in items" :key="progress.item.key" :value="progress.item.key">{{ progress.item.name }}</option></select></label>
      <label class="field"><span>当前市值（元）</span><input v-model="form.yuan" aria-label="当前市值（元）" type="number" min="0" step="0.01" placeholder="例如 12500" /></label>
      <label class="field"><span>记录时间</span><input v-model="form.observedAt" type="datetime-local" /></label>
      <button class="button button--primary" type="submit" :disabled="busy || !form.itemKey || form.yuan === '' || Boolean(localError)">{{ busy ? '正在保存…' : '保存市值记录' }}</button>
    </form>
    <p v-if="selected" class="value-form__hint">将记录 {{ selected.item.name }} 的独立市值事实，而非生成交易指令。</p>
    <p v-if="localError" class="value-form__error" role="alert">{{ localError }}</p>
  </section>
</template>

<style scoped>
.value-form-section { display: grid; grid-template-columns: 210px minmax(0, 1fr); gap: 28px; padding: 28px 0; border-top: 1px solid var(--line); }.section-heading p { margin: 0 0 6px; color: var(--accent); font-size: 10px; letter-spacing: .12em; }.section-heading h2 { margin: 0; font-family: var(--font-serif); font-size: 22px; font-weight: 500; }.section-heading span { display: block; margin-top: 8px; color: var(--ink-faint); font-size: 11px; line-height: 1.6; }.value-form { display: grid; grid-template-columns: 1.4fr 1fr 1.15fr auto; align-items: end; gap: 12px; }.value-form__hint, .value-form__error { grid-column: 2; margin: -14px 0 0; font-size: 11px; }.value-form__hint { color: var(--ink-faint); }.value-form__error { color: var(--accent); }
</style>
