<script setup lang="ts">
import logoUrl from '@/assets/logo.svg'

const navigation = [
  { to: '/', label: '今日', index: '01' },
  { to: '/watchlist', label: '观察名单', index: '02' },
  { to: '/plans', label: '交易计划', index: '03' },
  { to: '/executions', label: '成交补录', index: '04' },
  { to: '/positions', label: '持仓', index: '05' },
  { to: '/market', label: '市场榜单', index: '06' },
  { to: '/reviews', label: '每周复盘', index: '07' },
  { to: '/settings', label: '规则与设置', index: '08' },
]
</script>

<template>
  <div class="app-shell">
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
