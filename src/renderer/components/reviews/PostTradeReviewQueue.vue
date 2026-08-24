<script setup lang="ts">
import { reactive } from 'vue'

import { formatCNY } from '@/renderer/lib/format'
import type { PostTradeReview } from '@/renderer/types'

interface Props {
  reviews: PostTradeReview[]
  instrumentNames: Record<string, string>
  busyExecutionId: string
}

interface Emits {
  complete: [executionId: string, note: string]
}

defineProps<Props>()
const emit = defineEmits<Emits>()
const notes = reactive<Record<string, string>>({})

function signedCNY(fen: number) {
  return formatCNY(fen).replace('-', '−')
}

function executionPrice(review: PostTradeReview) {
  const value = review.localPriceTenThousandth
    ? review.localPriceTenThousandth / 10_000
    : review.localPriceMinor / 100
  return value.toFixed(4).replace(/0+$/, '').replace(/\.$/, '')
}

function complete(executionId: string) {
  const note = notes[executionId]?.trim() ?? ''
  if (note) emit('complete', executionId, note)
}
</script>

<template>
  <section class="queue" aria-labelledby="post-trade-review-title">
    <header>
      <div>
        <p>POST-TRADE</p>
        <h2 id="post-trade-review-title">本周成交待复盘</h2>
      </div>
      <strong>{{ reviews.length }}</strong>
    </header>

    <p v-if="reviews.length === 0" class="empty">本周没有待补的成交复盘</p>
    <article v-for="review in reviews" v-else :key="review.id" class="review-card">
      <div class="review-card__facts">
        <div>
          <strong>{{ instrumentNames[review.instrumentId] ?? review.instrumentId }}</strong>
          <span>{{ review.side === 'buy' ? '买入' : '卖出' }} {{ review.quantity }} 股</span>
        </div>
        <dl>
          <div><dt>成交均价</dt><dd>{{ executionPrice(review) }}</dd></div>
          <div><dt>实际结算</dt><dd>{{ signedCNY(review.settlementFen) }}</dd></div>
          <div><dt>成交时间</dt><dd>{{ new Date(review.executedAt).toLocaleString('zh-CN', { hour12: false }) }}</dd></div>
          <div><dt>当时情绪</dt><dd>恐惧 {{ review.emotion?.fearScore ?? 0 }} · 贪婪 {{ review.emotion?.greedScore ?? 0 }} · 回本冲动 {{ review.emotion?.revengeScore ?? 0 }}</dd></div>
        </dl>
      </div>
      <label class="field">
        <span>这笔成交当时发生了什么？</span>
        <textarea
          v-model="notes[review.executionId]"
          :aria-label="'这笔成交当时发生了什么？'"
          rows="3"
          placeholder="写事实：触发点、当时情绪、是否违背计划，以及下次如何阻断。"
        />
      </label>
      <button
        class="button button--secondary"
        type="button"
        :disabled="!notes[review.executionId]?.trim() || busyExecutionId === review.executionId"
        @click="complete(review.executionId)"
      >
        {{ busyExecutionId === review.executionId ? '保存中…' : '完成这笔复盘' }}
      </button>
    </article>
  </section>
</template>

<style scoped>
.queue { display: grid; gap: 14px; margin-bottom: 28px; }
.queue > header { display: flex; align-items: end; justify-content: space-between; padding-bottom: 12px; border-bottom: 2px solid var(--ink); }
.queue header p { margin: 0 0 5px; color: var(--accent); font-size: 10px; letter-spacing: .14em; }
.queue h2 { margin: 0; font-family: var(--font-serif); font-size: 25px; font-weight: 500; }
.queue > header > strong { color: var(--accent); font-family: var(--font-serif); font-size: 32px; font-weight: 500; }
.empty { margin: 0; padding: 18px; color: var(--ink-muted); border: 1px solid var(--line); background: var(--paper-deep); font-size: 12px; }
.review-card { display: grid; grid-template-columns: minmax(0, 1fr) 1.1fr auto; gap: 16px; align-items: end; padding: 16px; border: 1px solid var(--line); background: var(--paper); }
.review-card__facts { display: grid; gap: 12px; }
.review-card__facts > div { display: grid; gap: 4px; }
.review-card__facts span { color: var(--ink-muted); font-size: 12px; }
.review-card dl { display: flex; flex-wrap: wrap; gap: 8px 16px; margin: 0; }
.review-card dl div { display: grid; gap: 2px; }
.review-card dt { color: var(--ink-faint); font-size: 9px; }
.review-card dd { margin: 0; font-size: 11px; }
.review-card .field { margin: 0; }
.review-card button { min-width: 128px; }
@media (max-width: 1000px) { .review-card { grid-template-columns: 1fr; align-items: stretch; }.review-card button { justify-self: start; } }
</style>
