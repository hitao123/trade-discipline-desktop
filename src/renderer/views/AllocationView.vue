<script setup lang="ts">
import { onMounted, shallowRef } from 'vue'

import AllocationEditor from '@/renderer/components/allocation/AllocationEditor.vue'
import AllocationHistory from '@/renderer/components/allocation/AllocationHistory.vue'
import AllocationSummary from '@/renderer/components/allocation/AllocationSummary.vue'
import AllocationValueForm from '@/renderer/components/allocation/AllocationValueForm.vue'
import ErrorNotice from '@/renderer/components/ErrorNotice.vue'
import PageHeader from '@/renderer/components/PageHeader.vue'
import { useRemoteData } from '@/renderer/composables/useRemoteData'
import { api } from '@/renderer/lib/api'
import type { AllocationDraft, AllocationOverview, AllocationVersion } from '@/renderer/types'

const { data: overview, loading, error: loadError, refresh } = useRemoteData(() => api.request<AllocationOverview>('/api/allocation'))
const versions = shallowRef<AllocationVersion[]>([])
const actionError = shallowRef('')
const notice = shallowRef('')
const revisionBusy = shallowRef(false)
const valueBusy = shallowRef(false)

async function load() {
  actionError.value = ''
  await refresh()
  if (!overview.value) return
  try {
    versions.value = await api.request<AllocationVersion[]>('/api/allocation/versions')
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
    await load()
  }
  catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : '配置版本保存失败'
  }
  finally {
    revisionBusy.value = false
  }
}

async function recordValue(payload: { itemKey: string; valueFen: number; observedAt: string }) {
  valueBusy.value = true
  actionError.value = ''
  notice.value = ''
  try {
    await api.request(`/api/allocation/items/${payload.itemKey}/value-events`, { method: 'POST', body: JSON.stringify({ valueFen: payload.valueFen, observedAt: payload.observedAt }) })
    notice.value = '市值记录已追加，当前进度已重新计算。'
    await load()
  }
  catch (cause) {
    actionError.value = cause instanceof Error ? cause.message : '市值记录保存失败'
  }
  finally {
    valueBusy.value = false
  }
}

onMounted(load)
</script>

<template>
  <div>
    <PageHeader eyebrow="ASSET ALLOCATION" title="资产配置与年末目标" description="以 20 万元基准、截至 2026 年末 10% 目标收益跟踪。这里记录计划与市值事实，不连接券商，也不会生成交易指令。"><button class="button" type="button" :disabled="loading" @click="load">刷新本地进度</button></PageHeader>
    <ErrorNotice :message="loadError || actionError" />
    <p v-if="notice" class="success-notice" role="status">{{ notice }}</p>
    <template v-if="overview">
      <AllocationSummary :overview="overview" />
      <AllocationValueForm :items="overview.items" :busy="valueBusy" @record-value="recordValue" />
      <AllocationEditor :overview="overview" :busy="revisionBusy" @save="saveRevision" />
      <AllocationHistory :versions="versions" />
    </template>
  </div>
</template>
