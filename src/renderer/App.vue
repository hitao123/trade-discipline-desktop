<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, provide, ref } from 'vue'
import { useRouter } from 'vue-router'

import logoUrl from '@/assets/logo.svg'
import { userProfileKey } from '@/renderer/composables/useUserProfile'
import { api } from '@/renderer/lib/api'
import type { UserProfile } from '@/renderer/types'
import OnboardingView from '@/renderer/views/OnboardingView.vue'

const allNavigation = [
  { to: '/', label: '今日', index: '01' },
  { to: '/watchlist', label: '观察名单', index: '02' },
  { to: '/plans', label: '交易计划', index: '03' },
  { to: '/executions', label: '成交补录', index: '04' },
  { to: '/positions', label: '持仓', index: '05' },
  { to: '/allocation', label: '资产配置', index: '06' },
  { to: '/market', label: '市场榜单', index: '07' },
  { to: '/reviews', label: '每周复盘', index: '08' },
  { to: '/settings', label: '规则与设置', index: '09' },
]

const router = useRouter()
const appState = ref<'loading' | 'error' | 'pending' | 'completed'>('loading')
const profile = ref<UserProfile | null>(null)
const loadError = ref('')
const navigation = computed(() => allNavigation)
let removeMonitorAlertListener: (() => void) | undefined

function replaceProfile(next: UserProfile) {
  profile.value = next
}

provide(userProfileKey, { profile: computed(() => profile.value), replaceProfile })

function startMonitorAlertListener() {
  if (!removeMonitorAlertListener)
    removeMonitorAlertListener = window.discipline?.onMonitorAlert((alertID) => {
      void router.push({ path: '/positions', query: alertID ? { alert: alertID } : {} })
    })
}

async function loadProfile() {
  appState.value = 'loading'
  loadError.value = ''
  try {
    const current = await api.request<UserProfile>('/api/onboarding')
    replaceProfile(current)
    appState.value = current.onboardingStatus === 'completed' ? 'completed' : 'pending'
    if (appState.value === 'completed')
      startMonitorAlertListener()
  }
  catch (error) {
    appState.value = 'error'
    loadError.value = error instanceof Error ? error.message : '无法检查本地工作区状态'
  }
}

async function handleOnboardingCompleted(next: UserProfile) {
  replaceProfile(next)
  appState.value = 'completed'
  await router.replace('/')
  startMonitorAlertListener()
}

onMounted(() => { void loadProfile() })

onBeforeUnmount(() => removeMonitorAlertListener?.())
</script>

<template>
  <div v-if="appState === 'loading'" class="app-gate">
    <img :src="logoUrl" class="app-gate__logo" alt="" />
    <p class="app-gate__name">Plain Rule</p>
    <p>正在打开本地工作区…</p>
  </div>

  <div v-else-if="appState === 'error'" class="app-gate">
    <img :src="logoUrl" class="app-gate__logo" alt="" />
    <p class="app-gate__name">暂时无法打开本地工作区</p>
    <p>{{ loadError }}</p>
    <button type="button" class="button button--primary" @click="loadProfile">重新检查本地服务</button>
  </div>

  <OnboardingView v-else-if="appState === 'pending'" @completed="handleOnboardingCompleted" />

  <div v-else class="app-shell">
    <aside class="sidebar">
      <header class="brand">
        <img :src="logoUrl" class="brand__mark brand__mark--logo" alt="" />
        <div>
          <p class="brand__name">Plain Rule</p>
          <p class="brand__caption">Trading Discipline Workspace</p>
        </div>
      </header>

      <nav class="primary-nav" aria-label="主要功能">
        <RouterLink v-for="item in navigation" :key="item.to" class="primary-nav__item" :to="item.to">
          <span class="primary-nav__index" aria-hidden="true">{{ item.index }}</span>
          <span>{{ item.label }}</span>
        </RouterLink>
      </nav>

      <footer class="sidebar__footer">
        <p>本地记录 · 不连接券商</p>
        <span class="status-dot" aria-hidden="true" />
        <span>数据仅保存在这台 Mac</span>
      </footer>
    </aside>

    <main class="workspace">
      <RouterView />
    </main>
  </div>
</template>
