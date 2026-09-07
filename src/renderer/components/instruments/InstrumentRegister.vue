<script setup lang="ts">
import { reactive, shallowRef } from 'vue'

import { api } from '@/renderer/lib/api'
import type { Instrument } from '@/renderer/types'

const emit = defineEmits<{ registered: [instrument: Instrument] }>()
const busy = shallowRef(false)
const error = shallowRef('')
const form = reactive({
  market: 'HK' as Instrument['market'],
  code: '',
  name: '',
  lotSize: 100,
  localOnly: false,
})

async function submit() {
  if (!form.code.trim()) return
  busy.value = true
  error.value = ''
  try {
    const instrument = await api.request<Instrument>('/api/instruments', {
      method: 'POST',
      body: JSON.stringify({
        market: form.market,
        code: form.code.trim(),
        name: form.name.trim(),
        lotSize: Number(form.lotSize) || undefined,
        localOnly: form.localOnly,
      }),
    })
    form.code = ''
    form.name = ''
    emit('registered', instrument)
  }
  catch (cause) {
    error.value = cause instanceof Error ? cause.message : '证券登记失败'
  }
  finally {
    busy.value = false
  }
}
</script>

<template>
  <form class="instrument-register" @submit.prevent="submit">
    <div class="instrument-register__copy">
      <p>ADD SECURITY</p>
      <h2>手动添加证券</h2>
    </div>
    <div class="instrument-register__grid">
      <label class="field"><span>市场</span><select v-model="form.market" aria-label="证券市场"><option value="HK">港股</option><option value="SH">上海</option><option value="SZ">深圳</option></select></label>
      <label class="field"><span>代码</span><input v-model="form.code" aria-label="证券代码" required placeholder="0700 或 0700.HK" /></label>
      <label class="field"><span>名称（行情失败时必填）</span><input v-model="form.name" aria-label="证券名称" placeholder="腾讯控股" /></label>
      <label class="field"><span>每手</span><input v-model.number="form.lotSize" aria-label="每手股数" type="number" min="1" step="1" /></label>
    </div>
    <label class="check-field"><input v-model="form.localOnly" type="checkbox" /><span>仅本地登记，不查询公开行情</span></label>
    <button class="button button--primary" type="submit" :disabled="busy || !form.code.trim()">{{ busy ? '正在登记…' : '添加证券' }}</button>
    <p v-if="error" class="instrument-register__error">{{ error }}</p>
  </form>
</template>

<style scoped>
.instrument-register { display: grid; gap: 12px; max-width: 900px; padding: 15px; border: 1px solid var(--line); background: var(--paper-deep); }
.instrument-register__copy p { margin: 0 0 4px; color: var(--accent); font-size: 9px; letter-spacing: .14em; }
.instrument-register__copy h2 { margin: 0; font-size: 14px; }
.instrument-register__grid { display: grid; grid-template-columns: 100px 1fr 1fr 90px; gap: 10px; }
.check-field { display: flex; align-items: center; gap: 8px; font-size: 12px; }
.check-field input { width: auto; }
.instrument-register__error { margin: 0; color: var(--accent); font-size: 12px; }
@media (max-width: 800px) { .instrument-register__grid { grid-template-columns: 1fr 1fr; } }
</style>
