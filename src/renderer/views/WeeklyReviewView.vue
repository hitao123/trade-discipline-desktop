<script setup lang="ts">
import { computed, onMounted, reactive, shallowRef, watch } from 'vue'

import ErrorNotice from '@/renderer/components/ErrorNotice.vue'
import PageHeader from '@/renderer/components/PageHeader.vue'
import PostTradeReviewQueue from '@/renderer/components/reviews/PostTradeReviewQueue.vue'
import { api } from '@/renderer/lib/api'
import { dateLocal, formatCNY } from '@/renderer/lib/format'
import type { ExecutionRecord, Instrument, PostTradeReview } from '@/renderer/types'

interface Review {
  id: string
  periodStart: string
  periodEnd: string
  disciplineScoreBP: number
  metrics: { cashFen: number; chinaTechExposureFen: number; cumulativeLossFen: number; violationCount: number }
  userContent: { impulseNotes: string; nextAllowedAction: string }
}

const today = new Date()
const monday = new Date(today)
monday.setDate(today.getDate() - ((today.getDay() + 6) % 7))
const sunday = new Date(monday)
sunday.setDate(monday.getDate() + 6)
const form = reactive({ periodStart: dateLocal(monday), periodEnd: dateLocal(sunday), impulseNotes: '', nextAllowedAction: '' })
const review = shallowRef<Review>()
const error = shallowRef('')
const busy = shallowRef(false)
const pendingReviews = shallowRef<PostTradeReview[]>([])
const instruments = shallowRef<Instrument[]>([])
const busyExecutionId = shallowRef('')
const allPendingReviews = shallowRef<PostTradeReview[]>([])
const executions = shallowRef<ExecutionRecord[]>([])
const showAllPending = shallowRef(false)
const draftPeriod = shallowRef({ periodStart: form.periodStart, periodEnd: form.periodEnd })
const switchingPeriod = shallowRef(false)
const instrumentNames = computed(() => Object.fromEntries(instruments.value.map(instrument => [instrument.id, `${instrument.code} · ${instrument.name}`])))
const isDraftPeriodCurrent = computed(() => form.periodStart === draftPeriod.value.periodStart && form.periodEnd === draftPeriod.value.periodEnd)
const reviewDraftKey = computed(() => draftKey(draftPeriod.value))
const periodExecutions = computed(() => executions.value.filter((execution) => {
  const day = execution.executedAt.slice(0, 10)
  return day >= form.periodStart && day <= form.periodEnd
}))
const periodViolations = computed(() => pendingReviews.value.length)
const displayedPendingReviews = computed(() => showAllPending.value ? allPendingReviews.value : pendingReviews.value)

function draftKey(period: { periodStart: string; periodEnd: string }) {
  return `plain-rule:weekly-review-draft:${period.periodStart}:${period.periodEnd}`
}

function persistDraft() {
  if (review.value) return
  try { window.localStorage.setItem(reviewDraftKey.value, JSON.stringify({ impulseNotes: form.impulseNotes, nextAllowedAction: form.nextAllowedAction })) }
  catch { /* Draft persistence is local convenience only. */ }
}

function restoreDraft(period: { periodStart: string; periodEnd: string }) {
  if (review.value) return
  try {
    const raw = window.localStorage.getItem(draftKey(period))
    if (!raw) return
    const saved = JSON.parse(raw) as { impulseNotes?: string; nextAllowedAction?: string }
    form.impulseNotes = saved.impulseNotes ?? ''
    form.nextAllowedAction = saved.nextAllowedAction ?? ''
  }
  catch { /* A damaged local draft should not hide real review facts. */ }
}

async function loadPeriod(period: { periodStart: string; periodEnd: string }) {
  try {
    const loaded = await api.request<Review | null>(`/api/reviews?periodStart=${encodeURIComponent(period.periodStart)}&periodEnd=${encodeURIComponent(period.periodEnd)}`)
    if (form.periodStart !== period.periodStart || form.periodEnd !== period.periodEnd) return
    review.value = loaded ?? undefined
    if (loaded) {
      form.impulseNotes = loaded.userContent.impulseNotes
      form.nextAllowedAction = loaded.userContent.nextAllowedAction
    }
    else {
      form.impulseNotes = ''
      form.nextAllowedAction = ''
      restoreDraft(period)
    }
    return true
  }
  catch (cause) {
    error.value = cause instanceof Error ? cause.message : '复盘加载失败'
    return false
  }
}

async function loadPending(period = { periodStart: form.periodStart, periodEnd: form.periodEnd }) {
  try {
    const loaded = await api.request<PostTradeReview[]>(`/api/post-trade-reviews?periodStart=${encodeURIComponent(period.periodStart)}&periodEnd=${encodeURIComponent(period.periodEnd)}`)
    if (form.periodStart !== period.periodStart || form.periodEnd !== period.periodEnd) return
    pendingReviews.value = Array.isArray(loaded) ? loaded : []
  }
  catch (cause) {
    error.value = cause instanceof Error ? cause.message : '待复盘成交加载失败'
  }
}

async function loadAllPending() {
  try {
    const loaded = await api.request<PostTradeReview[]>('/api/post-trade-reviews')
    allPendingReviews.value = Array.isArray(loaded) ? loaded : []
  }
  catch { allPendingReviews.value = [] }
}

async function loadExecutions() {
  try {
    const loaded = await api.request<ExecutionRecord[]>('/api/executions')
    executions.value = Array.isArray(loaded) ? loaded : []
  }
  catch { executions.value = [] }
}

async function loadInstruments() {
  try {
    instruments.value = await api.request<Instrument[]>('/api/instruments')
  }
  catch (cause) {
    error.value = cause instanceof Error ? cause.message : '证券资料加载失败'
  }
}

async function onPeriodChange() {
  error.value = ''
  persistDraft()
  const period = { periodStart: form.periodStart, periodEnd: form.periodEnd }
  switchingPeriod.value = true
  try {
    const [periodLoaded] = await Promise.all([loadPeriod(period), loadPending(period)])
    if (periodLoaded && form.periodStart === period.periodStart && form.periodEnd === period.periodEnd)
      draftPeriod.value = period
    else if (!periodLoaded && form.periodStart === period.periodStart && form.periodEnd === period.periodEnd) {
      form.periodStart = draftPeriod.value.periodStart
      form.periodEnd = draftPeriod.value.periodEnd
      void loadPending(draftPeriod.value)
    }
  }
  finally {
    if ((form.periodStart === period.periodStart && form.periodEnd === period.periodEnd) || isDraftPeriodCurrent.value)
      switchingPeriod.value = false
  }
}

function chooseWeek(offset: number) {
  const start = offset === 0
    ? new Date(`${dateLocal(monday)}T12:00:00`)
    : new Date(`${form.periodStart}T12:00:00`)
  if (offset !== 0) start.setDate(start.getDate() + offset * 7)
  const end = new Date(start)
  end.setDate(end.getDate() + 6)
  form.periodStart = dateLocal(start)
  form.periodEnd = dateLocal(end)
  void onPeriodChange()
}

async function completePostTradeReview(executionId: string, note: string) {
  busyExecutionId.value = executionId
  error.value = ''
  try {
    await api.request(`/api/post-trade-reviews/${executionId}/complete`, { method: 'POST', body: JSON.stringify({ note }) })
    await Promise.all([loadPending(), loadAllPending()])
  }
  catch (cause) {
    error.value = cause instanceof Error ? cause.message : '成交复盘保存失败'
  }
  finally {
    busyExecutionId.value = ''
  }
}

async function submit() {
  if (!isDraftPeriodCurrent.value || switchingPeriod.value) {
    error.value = '当前周期尚未成功读取，不能提交其他周期的草稿。'
    return
  }
  const submittedPeriod = { periodStart: form.periodStart, periodEnd: form.periodEnd }
  const submittedDraftKey = draftKey(submittedPeriod)
  const submittedContent = {
    periodStart: submittedPeriod.periodStart,
    periodEnd: submittedPeriod.periodEnd,
    impulseNotes: form.impulseNotes,
    nextAllowedAction: form.nextAllowedAction,
  }
  busy.value = true
  try {
    const saved = await api.request<Review>('/api/reviews', { method: 'POST', body: JSON.stringify(submittedContent) })
    window.localStorage.removeItem(submittedDraftKey)
    if (form.periodStart === submittedPeriod.periodStart && form.periodEnd === submittedPeriod.periodEnd)
      review.value = saved
  }
  catch (cause) {
    error.value = cause instanceof Error ? cause.message : '复盘保存失败'
  }
  finally {
    busy.value = false
  }
}

watch(() => [form.impulseNotes, form.nextAllowedAction], () => {
  if (!switchingPeriod.value) persistDraft()
})

onMounted(() => Promise.all([loadPeriod(draftPeriod.value), loadPending(draftPeriod.value), loadAllPending(), loadInstruments(), loadExecutions()]))
</script>

<template>
  <div>
    <PageHeader eyebrow="REVIEW" title="每周复盘" description="评价过程，不奖励侥幸。事实来自本地成交、待复盘和持仓记录；草稿只保存在本机。" />
    <ErrorNotice :message="error" />
    <div class="review-queue-heading"><span>{{ showAllPending ? '全部未完成成交复盘' : '本周期成交复盘' }}</span><button class="text-button" type="button" @click="showAllPending = !showAllPending">{{ showAllPending ? '只看本周期' : `查看跨周全部 ${allPendingReviews.length} 笔` }}</button></div>
    <PostTradeReviewQueue :reviews="displayedPendingReviews" :instrument-names="instrumentNames" :busy-execution-id="busyExecutionId" @complete="completePostTradeReview" />
    <div class="review-layout">
      <form class="form-stack" @submit.prevent="submit">
        <div class="week-switch"><button class="button" type="button" :disabled="busy || switchingPeriod" @click="chooseWeek(-1)">上周</button><button class="button" type="button" :disabled="busy || switchingPeriod" @click="chooseWeek(0)">本周</button><span>切换周期不会覆盖未提交草稿</span></div>
        <div class="field-grid">
          <label class="field"><span>周期开始</span><input v-model="form.periodStart" aria-label="周期开始" type="date" required :disabled="busy || switchingPeriod" @change="onPeriodChange" /></label>
          <label class="field"><span>周期结束</span><input v-model="form.periodEnd" aria-label="周期结束" type="date" required :disabled="busy || switchingPeriod" @change="onPeriodChange" /></label>
        </div>
        <section class="period-facts" aria-label="本周期事实摘要"><strong>本周期事实</strong><span>{{ periodExecutions.length }} 笔成交 · {{ periodViolations }} 笔待成交复盘 · {{ allPendingReviews.length }} 笔跨周未完成</span><span v-if="periodExecutions.length">最早成交：{{ new Date(periodExecutions.at(-1)?.executedAt ?? '').toLocaleString('zh-CN') }}</span><span v-else>该周期没有本地成交记录。</span></section>
        <label class="field"><span>冲动与纪律记录</span><textarea v-model="form.impulseNotes" rows="7" placeholder="写事实：当时想做什么，最后按什么规则处理？" :disabled="busy || switchingPeriod" /></label>
        <label class="field"><span>下周唯一允许动作</span><textarea v-model="form.nextAllowedAction" rows="3" required placeholder="例如：只跟踪一项可验证证据，不因短期涨跌临时加仓" :disabled="busy || switchingPeriod" /></label>
        <p v-if="pendingReviews.length" class="pending-gate">请先完成上方 {{ pendingReviews.length }} 笔成交复盘，再提交本周复盘。</p>
        <button class="button button--primary" type="submit" :disabled="busy || switchingPeriod || !isDraftPeriodCurrent || pendingReviews.length > 0">{{ busy ? '提交中…' : switchingPeriod ? '正在读取周期…' : pendingReviews.length ? '先完成成交复盘' : '提交每周复盘' }}</button>
      </form>
      <aside class="score-panel">
        <p>{{ review ? `${review.periodStart} 至 ${review.periodEnd}` : '当前周期尚未提交' }}</p>
        <template v-if="review">
          <strong>{{ (review.disciplineScoreBP / 100).toFixed(0) }}</strong>
          <span>纪律得分</span>
          <dl>
            <div><dt>现金</dt><dd>{{ formatCNY(review.metrics.cashFen) }}</dd></div>
            <div><dt>累计亏损</dt><dd>{{ formatCNY(review.metrics.cumulativeLossFen) }}</dd></div>
            <div><dt>违规次数</dt><dd>{{ review.metrics.violationCount }}</dd></div>
          </dl>
          <blockquote>{{ review.userContent.nextAllowedAction }}</blockquote>
        </template>
        <span v-else>尚未正式提交；本机草稿会在返回此周期时恢复。</span>
        <button v-if="allPendingReviews.length" class="text-button" type="button" @click="showAllPending = true">查看全部 {{ allPendingReviews.length }} 笔跨周待复盘</button>
      </aside>
    </div>
  </div>
</template>

<style scoped>
.review-layout { display: grid; grid-template-columns: minmax(0, 1fr) 300px; gap: 28px; }.form-stack { display: grid; gap: 16px; }.field-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }.week-switch { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }.week-switch span { margin-left: 6px; color: var(--ink-muted); font-size: 12px; }.period-facts { display: grid; gap: 6px; padding: 15px; border-left: 3px solid var(--ink); background: var(--paper-deep); }.period-facts strong { font-size: 13px; }.period-facts span { color: var(--ink-muted); font-size: 12px; }.score-panel { display: grid; gap: 10px; padding: 22px; border-top: 3px solid var(--ink); background: var(--paper-deep); }.score-panel > p { margin: 0; color: var(--ink-faint); font-size: 11px; }.score-panel > strong { display: block; margin-top: 15px; font-family: var(--font-serif); font-size: 64px; font-weight: 500; line-height: 1; }.score-panel > span { color: var(--ink-muted); font-size: 12px; }.score-panel dl { margin: 14px 0; }.score-panel dl div { display: flex; justify-content: space-between; padding: 10px 0; border-top: 1px solid var(--line); font-size: 11px; }.score-panel dd { margin: 0; }.score-panel blockquote { margin: 0; padding: 14px; color: var(--ink-muted); border-left: 2px solid var(--accent); background: var(--paper); font-size: 12px; line-height: 1.7; }
.review-queue-heading { display: flex; align-items: center; justify-content: space-between; gap: 14px; margin: 16px 0 -10px; color: var(--ink-muted); font-size: 13px; }
.pending-gate { margin: 0; color: var(--accent); font-size: 12px; }
</style>
