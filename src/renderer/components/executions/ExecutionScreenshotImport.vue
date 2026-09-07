<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, shallowRef } from 'vue'

import { api } from '@/renderer/lib/api'
import { parseExecutionScreenshot, type ExecutionScreenshotPrefill } from '@/renderer/lib/execution-screenshot'
import type { Instrument } from '@/renderer/types'

interface Props {
  instruments: Instrument[]
}

interface Emits {
  prefill: [draft: ExecutionScreenshotPrefill]
  instrumentResolved: [instrument: Instrument]
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()
const busy = shallowRef(false)
const error = shallowRef('')
const result = shallowRef<ExecutionScreenshotPrefill>()
const fileName = shallowRef('')

const summary = computed(() => {
  const draft = result.value
  if (!draft) return ''
  return `已识别：${draft.code ?? '代码待确认'} · ${draft.side === 'buy' ? '买入' : draft.side === 'sell' ? '卖出' : '方向待确认'} · ${draft.quantity ?? '数量待确认'} 股 · ${draft.localPrice ?? '价格待确认'} 元`
})

type ScreenshotSelection = { name: string; lines: Array<{ text: string; confidence: number }> }

async function recognizeWith(loader: (() => Promise<ScreenshotSelection | null>) | undefined, unavailableMessage: string) {
  if (busy.value) return
  error.value = ''
  if (!loader) {
    error.value = unavailableMessage
    return
  }
  busy.value = true
  try {
    const selection = await loader()
    if (!selection) return
    fileName.value = selection.name
    const parsed = parseExecutionScreenshot(selection.lines)
    let instrument = parsed.code
      ? props.instruments.find(item => codesMatch(item.code, parsed.code))
      : undefined
    const warnings = [...parsed.warnings]
    if (parsed.code && (!instrument || instrument.name.includes('\uFFFD'))) {
      try {
        instrument = await api.request<Instrument>('/api/instruments/resolve', { method: 'POST', body: JSON.stringify({ code: parsed.code, market: parsed.market }) })
        emit('instrumentResolved', instrument)
      }
      catch (cause) {
        const message = cause instanceof Error ? cause.message : '公开行情查询失败'
        warnings.push(`未能自动录入 ${parsed.code}：${message}`)
      }
    }
    const draft: ExecutionScreenshotPrefill = { ...parsed, instrumentId: instrument?.id, warnings }
    result.value = draft
    emit('prefill', draft)
  }
  catch (cause) {
    error.value = cause instanceof Error ? cause.message : '截图识别失败，仍可继续手工填写。'
  }
  finally {
    busy.value = false
  }
}

function recognizeFile() {
  return recognizeWith(window.discipline?.recognizeExecutionScreenshot, '当前环境没有本地截图识别能力，仍可继续手工填写。')
}

function recognizeClipboard() {
  return recognizeWith(window.discipline?.recognizeExecutionClipboard, '当前环境不能读取图片剪贴板，仍可选择截图文件。')
}

function handlePaste(event: ClipboardEvent) {
  const containsImage = Array.from(event.clipboardData?.items ?? []).some(item => item.type.startsWith('image/'))
  if (!containsImage || busy.value) return
  event.preventDefault()
  void recognizeClipboard()
}

function codesMatch(left?: string, right?: string) {
  if (!left || !right) return false
  const normalize = (value: string) => value.replace(/\.HK$/i, '').replace(/^0+/, '')
  return left === right || normalize(left) === normalize(right)
}

onMounted(() => window.addEventListener('paste', handlePaste))
onBeforeUnmount(() => window.removeEventListener('paste', handlePaste))
</script>

<template>
  <section class="screenshot-import" aria-labelledby="screenshot-import-title">
    <div class="screenshot-import__copy">
      <p>LOCAL OCR</p>
      <div>
        <h2 id="screenshot-import-title">用券商成交截图预填</h2>
        <span>截图后直接按 ⌘V，或选择图片文件；只在本机识别，不上传。</span>
      </div>
    </div>
    <div class="screenshot-import__actions">
      <button class="button button--secondary" type="button" :disabled="busy" @click="recognizeFile">
        从成交截图识别
      </button>
      <button class="button button--primary" type="button" aria-label="粘贴截图识别" :disabled="busy" @click="recognizeClipboard">
        {{ busy ? '正在本机识别…' : '粘贴截图' }} <kbd v-if="!busy">⌘V</kbd>
      </button>
    </div>
    <div v-if="result" class="screenshot-result">
      <strong>{{ summary }}</strong>
      <span v-if="fileName">{{ fileName }}</span>
      <ul v-if="result.warnings.length">
        <li v-for="warning in result.warnings" :key="warning">{{ warning }}</li>
      </ul>
      <small v-else>字段已预填。请核对金额，并补充下单时的真实情绪。</small>
    </div>
    <p v-if="error" class="screenshot-error">{{ error }}</p>
  </section>
</template>

<style scoped>
.screenshot-import { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 12px 18px; align-items: center; max-width: 900px; padding: 15px; border: 1px solid var(--line); background: var(--paper-deep); }
.screenshot-import__copy { display: flex; gap: 14px; align-items: baseline; }
.screenshot-import__copy p { margin: 0; color: var(--accent); font-size: 9px; letter-spacing: .14em; }
.screenshot-import__copy h2 { margin: 0 0 4px; font-size: 14px; }
.screenshot-import__copy span { color: var(--ink-muted); font-size: 11px; }
.screenshot-import__actions { display: flex; gap: 8px; }
.screenshot-import__actions kbd { margin-left: 5px; color: inherit; font: inherit; opacity: .7; }
.screenshot-result { grid-column: 1 / -1; display: grid; gap: 5px; padding-top: 11px; border-top: 1px solid var(--line); font-size: 11px; }
.screenshot-result strong { font-weight: 650; }
.screenshot-result span, .screenshot-result small { color: var(--ink-muted); }
.screenshot-result ul { display: grid; gap: 3px; margin: 2px 0 0; padding-left: 18px; color: var(--accent); }
.screenshot-error { grid-column: 1 / -1; margin: 0; color: var(--accent); font-size: 11px; }
@media (max-width: 700px) { .screenshot-import { grid-template-columns: 1fr; }.screenshot-import__actions { flex-wrap: wrap; }.screenshot-result, .screenshot-error { grid-column: 1; } }
</style>
