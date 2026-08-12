<script setup lang="ts">
import { onMounted, reactive, shallowRef } from 'vue'

import ErrorNotice from '@/renderer/components/ErrorNotice.vue'
import PageHeader from '@/renderer/components/PageHeader.vue'
import { api } from '@/renderer/lib/api'
import { dateLocal, formatCNY } from '@/renderer/lib/format'

interface Review { id: string; periodStart: string; periodEnd: string; disciplineScoreBP: number; metrics: { cashFen: number; chinaTechExposureFen: number; cumulativeLossFen: number; violationCount: number }; userContent: { impulseNotes: string; nextAllowedAction: string } }
const today = new Date(); const monday = new Date(today); monday.setDate(today.getDate() - ((today.getDay() + 6) % 7)); const sunday = new Date(monday); sunday.setDate(monday.getDate() + 6)
const form = reactive({ periodStart: dateLocal(monday), periodEnd: dateLocal(sunday), impulseNotes: '', nextAllowedAction: '' })
const review = shallowRef<Review>(); const error = shallowRef(''); const busy = shallowRef(false)
async function load() { try { review.value = await api.request<Review | null>('/api/reviews/current') ?? undefined } catch (cause) { error.value = cause instanceof Error ? cause.message : '复盘加载失败' } }
async function submit() { busy.value = true; try { review.value = await api.request<Review>('/api/reviews', { method: 'POST', body: JSON.stringify(form) }) } catch (cause) { error.value = cause instanceof Error ? cause.message : '复盘保存失败' } finally { busy.value = false } }
onMounted(load)
</script>

<template>
  <div>
    <PageHeader eyebrow="REVIEW" title="评价过程，不奖励侥幸" description="合规亏损不会扣结果分，违规盈利不会得到奖励。每周只承诺下一件允许做的事。" />
    <ErrorNotice :message="error" />
    <div class="review-layout">
      <form class="form-stack" @submit.prevent="submit"><div class="field-grid"><label class="field"><span>周期开始</span><input v-model="form.periodStart" type="date" required /></label><label class="field"><span>周期结束</span><input v-model="form.periodEnd" type="date" required /></label></div><label class="field"><span>冲动与纪律记录</span><textarea v-model="form.impulseNotes" rows="7" placeholder="写事实：当时想做什么，最后按什么规则处理？" /></label><label class="field"><span>下周唯一允许动作</span><textarea v-model="form.nextAllowedAction" rows="3" required placeholder="例如：只跟踪阿里云收入证据，不新增中国科技仓位" /></label><button class="button button--primary" type="submit" :disabled="busy">{{ busy ? '提交中…' : '提交每周复盘' }}</button></form>
      <aside class="score-panel"><p>最近一次复盘</p><template v-if="review"><strong>{{ (review.disciplineScoreBP / 100).toFixed(0) }}</strong><span>纪律得分</span><dl><div><dt>现金</dt><dd>{{ formatCNY(review.metrics.cashFen) }}</dd></div><div><dt>中国科技敞口</dt><dd>{{ formatCNY(review.metrics.chinaTechExposureFen) }}</dd></div><div><dt>违规次数</dt><dd>{{ review.metrics.violationCount }}</dd></div></dl><blockquote>{{ review.userContent.nextAllowedAction }}</blockquote></template><span v-else>尚未提交复盘</span></aside>
    </div>
  </div>
</template>

<style scoped>
.review-layout { display: grid; grid-template-columns: minmax(0, 1fr) 300px; gap: 28px; }.form-stack { display: grid; gap: 16px; }.field-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }.score-panel { padding: 22px; border-top: 3px solid var(--ink); background: var(--paper-deep); }.score-panel > p { margin: 0; color: var(--ink-faint); font-size: 11px; }.score-panel > strong { display: block; margin-top: 15px; font-family: var(--font-serif); font-size: 64px; font-weight: 500; line-height: 1; }.score-panel > span { color: var(--ink-muted); font-size: 12px; }.score-panel dl { margin: 24px 0; }.score-panel dl div { display: flex; justify-content: space-between; padding: 10px 0; border-top: 1px solid var(--line); font-size: 11px; }.score-panel dd { margin: 0; }.score-panel blockquote { margin: 0; padding: 14px; color: var(--ink-muted); border-left: 2px solid var(--accent); background: var(--paper); font-size: 12px; line-height: 1.7; }
</style>
