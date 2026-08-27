<script setup lang="ts">
import { computed, onMounted, ref, shallowRef } from 'vue'

import AllocationEditor from '@/renderer/components/allocation/AllocationEditor.vue'
import AllocationHistory from '@/renderer/components/allocation/AllocationHistory.vue'
import AllocationSummary from '@/renderer/components/allocation/AllocationSummary.vue'
import AllocationValueForm from '@/renderer/components/allocation/AllocationValueForm.vue'
import ErrorNotice from '@/renderer/components/ErrorNotice.vue'
import PageHeader from '@/renderer/components/PageHeader.vue'
import { useRemoteData } from '@/renderer/composables/useRemoteData'
import { api } from '@/renderer/lib/api'
import type { AllocationDraft, AllocationState, AllocationVersion, Instrument } from '@/renderer/types'

const { data: state, loading, error: loadError, refresh } = useRemoteData(() => api.request<AllocationState>('/api/allocation'))
const overview = computed(() => state.value?.overview)
const editingBlank = ref(false)
const versions = shallowRef<AllocationVersion[]>([])
const instruments = shallowRef<Instrument[]>([])
const actionError = shallowRef('')
const notice = shallowRef('')
const revisionBusy = shallowRef(false)
const valueBusy = shallowRef(false)

async function load() {
  actionError.value = ''
  await refresh()
  if (!state.value) return
  try {
    instruments.value = await api.request<Instrument[]>('/api/instruments')
    versions.value = state.value.configured ? await api.request<AllocationVersion[]>('/api/allocation/versions') : []
  }
  catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : '配置版本加载失败'
  }
}

async function saveRevision(payload: { draft: AllocationDraft; reason: string }) {
  revisionBusy.value = true
  actionError.value = ''
  notice.value = ''
  try {
    await api.request('/api/allocation', { method: 'PUT', body: JSON.stringify(payload) })
    notice.value = '配置新版本已保存，旧版本和市值记录均已保留。'
    editingBlank.value = false
    await load()
  }
  catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : '配置版本保存失败'
  }
  finally {
    revisionBusy.value = false
  }
}

async function recordAdjustment(payload: { itemKey: string; adjustmentFen: number; observedAt: string }) {
  valueBusy.value = true
  actionError.value = ''
  notice.value = ''
  try {
    await api.request(`/api/allocation/items/${payload.itemKey}/adjustments`, { method: 'POST', body: JSON.stringify({ adjustmentFen: payload.adjustmentFen, observedAt: payload.observedAt }) })
    notice.value = '外部调整已保存，持仓与配置进度已重新计算。'
    await load()
  }
  catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : '外部调整保存失败'
  }
  finally {
    valueBusy.value = false
  }
}

onMounted(load)
</script>

<template>
  <div>
    <PageHeader eyebrow="ASSET ALLOCATION" title="资产配置与年末目标" description="真实持仓和现金会自动汇总到配置项目；未归类持仓仍计入总资产。这里不连接券商，也不会生成交易指令。"><button class="button" type="button" :disabled="loading" @click="load">刷新本地进度</button></PageHeader>
    <ErrorNotice :message="loadError || actionError" />
    <p v-if="notice" class="success-notice" role="status">{{ notice }}</p>
    <template v-if="overview">
      <AllocationSummary :overview="overview" />
      <AllocationValueForm :items="overview.items" :busy="valueBusy" @record-adjustment="recordAdjustment" />
      <AllocationEditor :overview="overview" :instruments="instruments" :busy="revisionBusy" @save="saveRevision" />
      <AllocationHistory :versions="versions" />
    </template>
    <section v-else-if="state && !editingBlank" class="allocation-empty">
      <p>EMPTY ALLOCATION</p><h2>尚未建立资产配置</h2><span>这里不会出现任何默认股票或比例。你可以从自己的资产分类开始。</span><button class="button button--primary" type="button" @click="editingBlank = true">开始配置</button>
    </section>
    <AllocationEditor v-else-if="state" :suggested-capital-fen="state.suggestedCapitalFen" :instruments="instruments" :busy="revisionBusy" @save="saveRevision" />
  </div>
</template>

<style scoped>.allocation-empty{display:grid;justify-items:start;gap:12px;padding:48px;border:1px solid var(--line)}.allocation-empty p{margin:0;color:var(--accent);font-size:10px;letter-spacing:.14em}.allocation-empty h2{margin:0;font-family:var(--font-serif);font-size:30px;font-weight:500}.allocation-empty span{color:var(--ink-muted)}</style>
