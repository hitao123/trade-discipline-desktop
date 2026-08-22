# 市场榜单备用源与实时成交标签 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让收盘榜单在主源中断时自动使用明确标注的备用源，并提供独立、手动刷新的实时成交观察标签。

**Architecture:** Go 市场服务为收盘与实时快照分配不同的持久化模式；东方财富仍为收盘主源，新浪作为排名备用源与实时来源。Vue 读取模式化快照和持久化刷新状态，清楚展示来源、时间与缓存原因。

**Tech Stack:** Go、SQLite、net/http、Vue 3、Vitest。

**Spec:** `docs/superpowers/specs/2026-08-22-market-fallback-live-tab-design.md`

## Global Constraints

- 手动刷新，不新增后台高频轮询。
- 公开数据始终展示来源和时间；实时数据不影响交易纪律、资产配置或交易动作。
- 收盘和实时快照不可相互覆盖；失败保留旧缓存并持久化错误状态。

### Task 1: 数据源备用与 v5 快照/状态持久化

**Files:**
- Create: `backend/internal/market/sina.go`, `backend/internal/market/sina_test.go`
- Modify: `backend/internal/market/provider.go`, `backend/internal/store/migrations.go`, `backend/internal/store/market.go`, `backend/internal/store/store.go`
- Test: `backend/internal/store/store_test.go`

- [ ] **Step 1:** 为新浪股票/ETF榜单和上证指数交易日写失败测试。
- [ ] **Step 2:** 运行测试确认失败。
- [ ] **Step 3:** 实现解析、来源时间和交易时段判定。
- [ ] **Step 4:** 增加 v5 的模式快照及刷新状态读取/写入。
- [ ] **Step 5:** 运行 market/store 定向测试。

### Task 2: 服务与 API

**Files:**
- Modify: `backend/internal/service/market.go`, `backend/internal/service/market_test.go`, `backend/internal/api/router.go`, `backend/internal/api/router_test.go`

- [ ] **Step 1:** 为主源失败后的备用榜单、模式隔离、状态持久化与实时刷新写失败测试。
- [ ] **Step 2:** 运行测试确认失败。
- [ ] **Step 3:** 实现备用源、收盘状态和实时服务方法。
- [ ] **Step 4:** 注册实时/状态 API 并添加处理器。
- [ ] **Step 5:** 运行 service/api 定向测试。

### Task 3: 收盘状态与实时成交界面

**Files:**
- Create: `src/renderer/components/market/MarketRefreshStatus.vue`
- Modify: `src/renderer/composables/useMarketHistory.ts`, `src/renderer/composables/useMarketHistory.spec.ts`, `src/renderer/views/MarketView.vue`, `src/renderer/views/MarketView.spec.ts`, `src/renderer/types.ts`

- [ ] **Step 1:** 为两个模式、备用源/缓存状态、实时按钮写失败界面测试。
- [ ] **Step 2:** 运行测试确认失败。
- [ ] **Step 3:** 实现只读状态组件与 composable API 流程。
- [ ] **Step 4:** 实现界面标签与文案；确认无交易按钮。
- [ ] **Step 5:** 运行前端定向测试与类型检查。

### Task 4: 回归和交付

**Files:**
- Modify: `README.md`, `scripts/smoke-macos.sh`

- [ ] **Step 1:** 更新公开数据与实时观察边界说明，v5 首启表检查。
- [ ] **Step 2:** 运行 `pnpm verify`、`pnpm build:mac` 和 `pnpm smoke:mac`。
