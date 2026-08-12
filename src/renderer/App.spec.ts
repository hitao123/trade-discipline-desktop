import { render, screen } from '@testing-library/vue'
import { describe, expect, it } from 'vitest'

import App from './App.vue'
import { router } from './router'

describe('App', () => {
  it('renders the eight primary navigation entries', async () => {
    await router.push('/')
    await router.isReady()
    render(App, { global: { plugins: [router] } })

    expect(screen.getByRole('link', { name: '今日' })).toBeTruthy()
    expect(screen.getByRole('link', { name: '观察名单' })).toBeTruthy()
    expect(screen.getByRole('link', { name: '交易计划' })).toBeTruthy()
    expect(screen.getByRole('link', { name: '成交补录' })).toBeTruthy()
    expect(screen.getByRole('link', { name: '持仓' })).toBeTruthy()
    expect(screen.getByRole('link', { name: '市场榜单' })).toBeTruthy()
    expect(screen.getByRole('link', { name: '每周复盘' })).toBeTruthy()
    expect(screen.getByRole('link', { name: '规则与设置' })).toBeTruthy()
  })
})
