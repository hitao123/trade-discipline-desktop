<script setup lang="ts">
import { onMounted, reactive, shallowRef } from 'vue'

import ErrorNotice from '@/renderer/components/ErrorNotice.vue'
import InstrumentRegister from '@/renderer/components/instruments/InstrumentRegister.vue'
import PageHeader from '@/renderer/components/PageHeader.vue'
import { api } from '@/renderer/lib/api'
import type { Instrument } from '@/renderer/types'

interface WatchItem { id: string; instrument: Instrument; sourceType: string; reason: string; addedAt: string }
const items = shallowRef<WatchItem[]>([])
const instruments = shallowRef<Instrument[]>([])
const error = shallowRef('')
const form = reactive({ instrumentId: '', reason: '' })

async function load() {
  try { [items.value, instruments.value] = await Promise.all([api.request('/api/watchlist'), api.request('/api/instruments')]) as [WatchItem[], Instrument[]]; if (!form.instrumentId && instruments.value[0]) form.instrumentId = instruments.value[0].id }
  catch (cause) { error.value = cause instanceof Error ? cause.message : '观察名单加载失败' }
}
function includeInstrument(instrument: Instrument) {
  const exists = instruments.value.some(item => item.id === instrument.id)
  instruments.value = exists ? instruments.value.map(item => item.id === instrument.id ? instrument : item) : [...instruments.value, instrument]
  form.instrumentId = instrument.id
}

async function add() {
  const instrument = instruments.value.find(item => item.id === form.instrumentId)
  if (!instrument) return
  try {
    const item = await api.request<WatchItem>('/api/watchlist', { method: 'POST', body: JSON.stringify({ market: instrument.market, code: instrument.code, reason: form.reason, sourceType: 'manual' }) })
    items.value = [item, ...items.value]; form.reason = ''
  }
  catch (cause) { error.value = cause instanceof Error ? cause.message : '加入观察失败' }
}
onMounted(load)
</script>

<template>
  <div>
    <PageHeader eyebrow="WATCHLIST" title="观察，不急着行动" description="观察项只能继续写交易计划。榜单热度本身不是买入理由。" />
    <ErrorNotice :message="error" />
    <InstrumentRegister @registered="includeInstrument" />
    <form class="watch-form" @submit.prevent="add"><label class="field"><span>证券</span><select v-model="form.instrumentId"><option v-for="instrument in instruments" :key="instrument.id" :value="instrument.id">{{ instrument.code }} · {{ instrument.name }}</option></select></label><label class="field field--grow"><span>观察理由</span><input v-model="form.reason" required placeholder="要等待哪一条证据？" /></label><button class="button button--primary" type="submit">加入观察</button></form>
    <div class="watch-list"><article v-for="item in items" :key="item.id"><div><span>{{ item.instrument.market }}</span><h2>{{ item.instrument.name }}</h2><p>{{ item.instrument.code }} · 每手 {{ item.instrument.lotSize }}</p></div><blockquote>{{ item.reason }}</blockquote><RouterLink class="text-button" to="/plans">创建计划 →</RouterLink></article><p v-if="items.length === 0" class="empty-state">观察名单为空。可从市场榜单加入，或在上方选择已有证券。</p></div>
  </div>
</template>

<style scoped>
.watch-form { display: flex; align-items: end; gap: 12px; margin-bottom: 24px; padding: 16px; border: 1px solid var(--line); background: var(--paper-deep); }.field--grow { flex: 1; }.watch-list { display: grid; gap: 1px; background: var(--line); border: 1px solid var(--line); }.watch-list article { display: grid; grid-template-columns: 220px 1fr auto; align-items: center; gap: 24px; padding: 20px; background: var(--paper); }.watch-list span { color: var(--accent); font-size: 10px; }.watch-list h2 { margin: 5px 0; font-family: var(--font-serif); font-size: 20px; font-weight: 500; }.watch-list p, blockquote { margin: 0; color: var(--ink-muted); font-size: 12px; }.watch-list blockquote { line-height: 1.7; }.empty-state { padding: 44px !important; color: var(--ink-faint) !important; text-align: center; background: var(--paper); }
</style>
