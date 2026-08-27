<script setup lang="ts">
import type { ExecutionRecord } from '@/renderer/types'

interface Props {
  records: readonly ExecutionRecord[]
  busy: boolean
}

interface Emits {
  select: [record: ExecutionRecord]
}

defineProps<Props>()
const emit = defineEmits<Emits>()
</script>

<template>
  <section class="history" aria-label="可修正成交">
    <div class="history__heading">
      <strong>选择录错的成交</strong>
      <span>只显示尚未被冲正的记录。</span>
    </div>
    <div v-if="records.length" class="history__list">
      <article v-for="record in records" :key="record.id" class="history__item">
        <div>
          <strong>{{ record.name || record.code }}</strong>
          <span>{{ record.code }} · {{ record.side === 'buy' ? '买入' : '卖出' }} {{ record.quantity }} 股</span>
          <time>{{ new Date(record.executedAt).toLocaleString('zh-CN') }}</time>
        </div>
        <button class="button" type="button" :disabled="busy" @click="emit('select', record)">修正这条</button>
      </article>
    </div>
    <p v-else class="history__empty">没有可以修正的成交。</p>
  </section>
</template>

<style scoped>
.history { display: grid; gap: 12px; padding: 18px; border: 1px solid var(--line); background: var(--paper-deep); }
.history__heading { display: grid; gap: 4px; }
.history__heading strong { font-size: 13px; }
.history__heading span, .history__item span, .history__item time, .history__empty { color: var(--ink-faint); font-size: 11px; }
.history__list { display: grid; gap: 8px; }
.history__item { display: flex; align-items: center; justify-content: space-between; gap: 18px; padding: 12px; border-top: 1px solid var(--line); }
.history__item div { display: grid; gap: 3px; }
.history__empty { margin: 0; }
</style>
