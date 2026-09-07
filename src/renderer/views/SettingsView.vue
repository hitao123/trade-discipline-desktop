<script setup lang="ts">
import { computed, onMounted, reactive, shallowRef } from 'vue'

import ErrorNotice from '@/renderer/components/ErrorNotice.vue'
import PageHeader from '@/renderer/components/PageHeader.vue'
import { useUserProfile } from '@/renderer/composables/useUserProfile'
import { api } from '@/renderer/lib/api'
import { dateTimeLocal, formatCNY } from '@/renderer/lib/format'
import type { CashEvent, CashEventType, Dashboard, HoldingHorizon, MarketScope, MonitorInterval, MonitorSettings, RuleVersion, UserProfile } from '@/renderer/types'

interface AuditRow { id: string; entityType: string; action: string; createdAt: string }
interface CSVPreview { valid: unknown[]; errors: unknown[]; duplicates: unknown[]; stockTop20: unknown[]; etfTop20: unknown[] }
interface RateRow { currency: string; yuan: number }

const rules = shallowRef<RuleVersion[]>([])
const { profile, replaceProfile } = useUserProfile()
const audit = shallowRef<AuditRow[]>([])
const error = shallowRef('')
const notice = shallowRef('')
const busy = shallowRef(false)
const monitorBusy = shallowRef(false)
const monitorInterval = shallowRef<MonitorInterval>('10m')
const edit = reactive({
  reason: '',
  lossCaution: 15000,
  lossRedLine: 20000,
  noAddToLosingInstrument: false,
})
const rateRows = shallowRef<RateRow[]>([{ currency: 'CNY', yuan: 1 }, { currency: 'HKD', yuan: 0.95 }])
const newRate = reactive({ currency: '', yuan: 1 })
const profileEdit = reactive({
  maxLoss: (profile.value?.maxLossFen ?? 0) / 100,
  holdingHorizon: (profile.value?.holdingHorizon ?? '6_to_12m') as Exclude<HoldingHorizon, 'legacy_unspecified'>,
  enabledMarkets: [...(profile.value?.enabledMarkets ?? [])] as MarketScope[],
  reason: '',
})
const cashEvents = shallowRef<CashEvent[]>([])
const fundedCapitalFen = shallowRef(profile.value?.investableCapitalFen ?? 0)
const cashEdit = reactive({
  eventType: 'deposit' as Exclude<CashEventType, 'reversal'>,
  amountYuan: 0,
  reason: '',
  occurredAt: dateTimeLocal(),
})
const cashBusy = shallowRef(false)
const csv = shallowRef<{ preview: CSVPreview; digest: string; name: string }>()
const current = computed(() => rules.value[0])
const cashTypeLabel: Record<CashEventType, string> = { deposit: '入金', withdrawal: '出金', dividend: '分红', reversal: '冲正' }

function snapshotRates(snapshot: RuleVersion['snapshot']): RateRow[] {
  const source = snapshot.currencyRatesBP ?? (snapshot.hkdCnyRateBP ? { CNY: 10_000, HKD: snapshot.hkdCnyRateBP } : { CNY: 10_000, HKD: 9_500 })
  const rows = Object.entries(source).map(([currency, bp]) => ({ currency, yuan: Number(bp) / 10_000 }))
  if (!rows.some(row => row.currency === 'CNY'))
    rows.unshift({ currency: 'CNY', yuan: 1 })
  return rows.sort((left, right) => left.currency.localeCompare(right.currency))
}

function currencyRatesBP() {
  return Object.fromEntries(rateRows.value
    .filter(row => row.currency.trim() && row.yuan > 0)
    .map(row => [row.currency.trim().toUpperCase(), Math.round(row.yuan * 10_000)]))
}

async function load() {
  try {
    const [loadedRules, loadedAudit, monitorSettings, loadedCash, dashboard] = await Promise.all([
      api.request<RuleVersion[]>('/api/rules'),
      api.request<AuditRow[]>('/api/audit'),
      api.request<MonitorSettings>('/api/monitor/settings'),
      api.request<CashEvent[]>('/api/cash-events').catch(() => [] as CashEvent[]),
      api.request<Dashboard>('/api/dashboard').catch(() => undefined),
    ])
    rules.value = loadedRules
    audit.value = loadedAudit
    cashEvents.value = loadedCash
    if (dashboard)
      fundedCapitalFen.value = dashboard.initialCapitalFen
    monitorInterval.value = monitorSettings.interval
    if (current.value) {
      edit.lossCaution = current.value.snapshot.lossCautionFen / 100
      edit.lossRedLine = current.value.snapshot.lossRedLineFen / 100
      edit.noAddToLosingInstrument = current.value.snapshot.noAddToLosingInstrument === true
      rateRows.value = snapshotRates(current.value.snapshot)
    }
  }
  catch (cause) { error.value = cause instanceof Error ? cause.message : '设置加载失败' }
}

async function saveMonitorSettings() {
  monitorBusy.value = true
  error.value = ''
  try {
    await api.request<MonitorSettings>('/api/monitor/settings', { method: 'PUT', body: JSON.stringify({ interval: monitorInterval.value }) })
    notice.value = '价格提醒设置已保存；仅在应用运行时检查公开报价。'
  }
  catch (cause) { error.value = cause instanceof Error ? cause.message : '价格提醒设置保存失败' }
  finally { monitorBusy.value = false }
}

async function recordCashEvent() {
  if (cashEdit.amountYuan <= 0 || !cashEdit.reason.trim())
    return
  cashBusy.value = true
  error.value = ''
  try {
    await api.request('/api/cash-events', { method: 'POST', body: JSON.stringify({
      eventType: cashEdit.eventType,
      amountFen: Math.round(cashEdit.amountYuan * 100),
      reason: cashEdit.reason.trim(),
      occurredAt: new Date(cashEdit.occurredAt).toISOString(),
    }) })
    cashEdit.amountYuan = 0
    cashEdit.reason = ''
    notice.value = '资金变动已入账，可用现金已更新。'
    await load()
  }
  catch (cause) { error.value = cause instanceof Error ? cause.message : '资金变动保存失败' }
  finally { cashBusy.value = false }
}

async function createRule() {
  if (!current.value)
    return
  busy.value = true
  error.value = ''
  try {
    const rates = currencyRatesBP()
    await api.request('/api/rules/versions', { method: 'POST', body: JSON.stringify({
      reason: edit.reason,
      snapshot: {
        ...current.value.snapshot,
        lossCautionFen: Math.round(edit.lossCaution * 100),
        lossRedLineFen: Math.round(edit.lossRedLine * 100),
        noAddToLosingInstrument: edit.noAddToLosingInstrument,
        currencyRatesBP: rates,
        hkdCnyRateBP: rates.HKD ?? current.value.snapshot.hkdCnyRateBP,
      },
    }) })
    edit.reason = ''
    notice.value = '规则新版本已创建，历史计划仍引用旧版本'
    await load()
  }
  catch (cause) { error.value = cause instanceof Error ? cause.message : '规则修改失败' }
  finally { busy.value = false }
}

async function saveProfile() {
  busy.value = true
  error.value = ''
  try {
    const updated = await api.request<UserProfile>('/api/profile', { method: 'PUT', body: JSON.stringify({
      investableCapitalFen: fundedCapitalFen.value || profile.value?.investableCapitalFen || 0,
      maxLossFen: Math.round(profileEdit.maxLoss * 100),
      holdingHorizon: profileEdit.holdingHorizon,
      enabledMarkets: profileEdit.enabledMarkets,
      reason: profileEdit.reason,
    }) })
    replaceProfile(updated)
    profileEdit.reason = ''
    notice.value = '基础资料和风险规则已创建新版本，账户现金只通过资金变动改写。'
    await load()
  }
  catch (cause) { error.value = cause instanceof Error ? cause.message : '基础资料修改失败' }
  finally { busy.value = false }
}

function addRate() {
  const currency = newRate.currency.trim().toUpperCase()
  if (!currency || newRate.yuan <= 0 || rateRows.value.some(row => row.currency === currency))
    return
  rateRows.value = [...rateRows.value, { currency, yuan: newRate.yuan }]
  newRate.currency = ''
  newRate.yuan = 1
}

function removeRate(currency: string) {
  if (currency === 'CNY')
    return
  rateRows.value = rateRows.value.filter(row => row.currency !== currency)
}

async function chooseCSV() {
  if (!window.discipline) {
    error.value = '请在 macOS 桌面应用中选择 CSV 文件'
    return
  }
  const selected = await window.discipline.selectCSV()
  if (!selected)
    return
  try {
    const result = await api.request<{ preview: CSVPreview; digest: string }>('/api/market/csv/preview', { method: 'POST', body: JSON.stringify({ content: selected.content }) })
    csv.value = { ...result, name: selected.name }
  }
  catch (cause) { error.value = cause instanceof Error ? cause.message : 'CSV 预览失败' }
}

async function confirmCSV() {
  if (!csv.value)
    return
  try {
    await api.request('/api/market/csv/confirm', { method: 'POST', body: JSON.stringify({ preview: csv.value.preview, digest: csv.value.digest }) })
    notice.value = 'CSV 收盘榜单已导入'
    csv.value = undefined
  }
  catch (cause) { error.value = cause instanceof Error ? cause.message : 'CSV 导入失败' }
}

async function exportBackup() { try { const path = await window.discipline?.exportBackup(); if (path) notice.value = `备份已导出：${path}` } catch (cause) { error.value = cause instanceof Error ? cause.message : '备份导出失败' } }
async function restoreBackup() { try { const ok = await window.discipline?.restoreBackup(); if (ok) notice.value = '备份已恢复，本地后端已经重启' } catch (cause) { error.value = cause instanceof Error ? cause.message : '备份恢复失败' } }
async function openLogs() { await window.discipline?.openLogs() }

onMounted(load)
</script>

<template>
  <div>
    <PageHeader eyebrow="RULES & DATA" title="规则只能向前修改" description="修改会创建新版本并写入原因与审计。恢复备份之前，应用会先保护当前数据库。" />
    <ErrorNotice :message="error" /><p v-if="notice" class="success-notice" role="status">{{ notice }}</p>
    <section class="settings-section">
      <div class="section-title"><p>通用工作区</p><h2>基础资料与风险边界</h2></div>
      <form class="rule-form" @submit.prevent="saveProfile">
        <p class="funded-capital">当前资金池 {{ formatCNY(fundedCapitalFen) }}。初始资金是向导写入的基线，之后只通过下方资金变动改写现金。</p>
        <label class="field"><span>最大可承受损失（元）</span><input v-model.number="profileEdit.maxLoss" aria-label="最大可承受损失（元）" type="number" min="1" /></label>
        <label class="field"><span>预期持有期限</span><select v-model="profileEdit.holdingHorizon"><option value="under_6m">半年以内</option><option value="6_to_12m">半年到一年</option><option value="1_to_3y">一到三年</option><option value="over_3y">三年以上</option></select></label>
        <fieldset class="sequence-gate"><legend>使用市场</legend><div class="field-grid field-grid--three"><label class="check-field"><input v-model="profileEdit.enabledMarkets" type="checkbox" value="ashare_stock" /><span>A 股股票</span></label><label class="check-field"><input v-model="profileEdit.enabledMarkets" type="checkbox" value="ashare_etf" /><span>A 股 ETF</span></label><label class="check-field"><input v-model="profileEdit.enabledMarkets" type="checkbox" value="hk" /><span>港股</span></label></div></fieldset>
        <label class="field"><span>资料修改原因</span><textarea v-model="profileEdit.reason" aria-label="资料修改原因" rows="3" placeholder="例如：调整持有期限或风险边界" /></label>
        <button class="button button--primary" type="submit" :disabled="busy || !profileEdit.reason.trim() || profileEdit.enabledMarkets.length === 0">保存并创建记录</button>
      </form>
      <form class="rule-form cash-form" @submit.prevent="recordCashEvent">
        <h3>记一笔资金变动</h3>
        <div class="field-grid field-grid--three">
          <label class="field"><span>类型</span><select v-model="cashEdit.eventType" aria-label="资金变动类型"><option value="deposit">入金</option><option value="withdrawal">出金</option><option value="dividend">分红</option></select></label>
          <label class="field"><span>金额（元）</span><input v-model.number="cashEdit.amountYuan" aria-label="资金变动金额（元）" type="number" min="0.01" step="0.01" required /></label>
          <label class="field"><span>时间</span><input v-model="cashEdit.occurredAt" type="datetime-local" required /></label>
        </div>
        <label class="field"><span>原因</span><input v-model="cashEdit.reason" aria-label="资金变动原因" required placeholder="例如：券商转入、转出生活费、ETF 分红" /></label>
        <button class="button button--primary" type="submit" :disabled="cashBusy || cashEdit.amountYuan <= 0 || !cashEdit.reason.trim()">{{ cashBusy ? '正在入账…' : '保存资金变动' }}</button>
      </form>
      <ol class="cash-history"><li v-for="event in cashEvents" :key="event.id"><strong>{{ cashTypeLabel[event.eventType] }}</strong><span>{{ formatCNY(event.amountFen) }}</span><p>{{ event.reason }}</p><time>{{ new Date(event.occurredAt).toLocaleString('zh-CN') }}</time></li><li v-if="cashEvents.length === 0">尚无入金、出金或分红记录</li></ol>
    </section>

    <section v-if="current" class="settings-section">
      <div class="section-title"><p>当前规则 v{{ current.version }}</p><h2>资金与汇率</h2></div>
      <form class="rule-form" @submit.prevent="createRule">
        <div class="field-grid field-grid--two">
          <label class="field"><span>损失警戒线（元）</span><input v-model.number="edit.lossCaution" aria-label="损失警戒线（元）" type="number" min="1" /></label>
          <label class="field"><span>损失红线（元）</span><input v-model.number="edit.lossRedLine" aria-label="损失红线（元）" type="number" min="1" /></label>
        </div>
        <label class="check-field"><input v-model="edit.noAddToLosingInstrument" type="checkbox" /><span>同一标的浮亏时禁止加仓</span></label>
        <fieldset class="sequence-gate">
          <legend>对人民币汇率（1 外币 = 多少人民币）</legend>
          <div v-for="row in rateRows" :key="row.currency" class="rate-row">
            <label class="field"><span>币种</span><input :value="row.currency" aria-label="汇率币种" readonly /></label>
            <label class="field"><span>人民币</span><input v-model.number="row.yuan" :aria-label="`${row.currency}汇率`" type="number" min="0.0001" step="0.0001" :disabled="row.currency === 'CNY'" /></label>
            <button v-if="row.currency !== 'CNY'" class="text-button" type="button" @click="removeRate(row.currency)">移除</button>
          </div>
          <div class="rate-row">
            <label class="field"><span>新增币种</span><input v-model="newRate.currency" aria-label="新增汇率币种" placeholder="USD" /></label>
            <label class="field"><span>人民币</span><input v-model.number="newRate.yuan" aria-label="新增汇率" type="number" min="0.0001" step="0.0001" /></label>
            <button class="text-button" type="button" @click="addRate">添加</button>
          </div>
        </fieldset>
        <label class="field"><span>修改原因</span><textarea v-model="edit.reason" aria-label="修改原因" rows="3" placeholder="为什么未来需要改？不能写成给历史交易找理由。" /></label>
        <button class="button button--primary" type="submit" :disabled="busy || !edit.reason.trim()">创建规则新版本</button>
      </form>
    </section>

    <section class="settings-section">
      <div class="section-title"><p>价格提醒</p><h2>应用运行时检查</h2></div>
      <div class="reminder-settings"><label class="field"><span>提醒检查间隔</span><select v-model="monitorInterval" aria-label="提醒检查间隔"><option value="off">关闭</option><option value="10m">每 10 分钟</option><option value="15m">每 15 分钟</option><option value="30m">每 30 分钟</option></select></label><p>仅在 Plain Rule 打开期间查询公开报价；触及风险退出线或目标区间时，系统会通知并要求你完成复核记录。</p><button class="button button--primary" type="button" :disabled="monitorBusy" @click="saveMonitorSettings">{{ monitorBusy ? '正在保存…' : '保存价格提醒设置' }}</button></div>
    </section>

    <section class="settings-section">
      <div class="section-title"><p>收盘数据</p><h2>CSV 备用导入</h2></div>
      <div class="data-actions"><button class="button" type="button" @click="chooseCSV">选择 CSV 并预览</button><p>UTF-8/BOM；股票前 20、ETF 前 20 会分别生成快照。</p></div>
      <div v-if="csv" class="csv-preview"><strong>{{ csv.name }}</strong><span>有效 {{ csv.preview.valid.length }} 行</span><span>错误 {{ csv.preview.errors.length }} 行</span><span>重复 {{ csv.preview.duplicates.length }} 行</span><button class="button button--primary" type="button" @click="confirmCSV">确认导入预览内容</button></div>
    </section>

    <section class="settings-section">
      <div class="section-title"><p>本地数据</p><h2>备份、恢复与日志</h2></div>
      <div class="data-actions"><button class="button" type="button" @click="exportBackup">导出完整备份</button><button class="button" type="button" @click="restoreBackup">校验并恢复备份</button><button class="text-button" type="button" @click="openLogs">打开日志目录</button></div>
    </section>

    <section class="settings-section">
      <div class="section-title"><p>版本历史</p><h2>修改原因与关键差异</h2></div>
      <ol class="rule-history"><li v-for="rule in rules" :key="rule.id"><div><strong>v{{ rule.version }}</strong><time>{{ new Date(rule.createdAt).toLocaleString('zh-CN') }}</time></div><p>{{ rule.reason }}</p><span>警戒 {{ formatCNY(rule.snapshot.lossCautionFen) }} · 红线 {{ formatCNY(rule.snapshot.lossRedLineFen) }}</span></li></ol>
    </section>

    <section class="settings-section">
      <div class="section-title"><p>审计</p><h2>最近关键修改</h2></div>
      <ol class="audit-list"><li v-for="event in audit.slice(0, 12)" :key="event.id"><time>{{ new Date(event.createdAt).toLocaleString('zh-CN') }}</time><strong>{{ event.entityType }}</strong><span>{{ event.action }}</span></li><li v-if="audit.length === 0">尚无修改记录</li></ol>
    </section>
  </div>
</template>

<style scoped>
.settings-section { display: grid; grid-template-columns: 210px minmax(0, 1fr); gap: 28px; padding: 28px 0; border-bottom: 1px solid var(--line); }.section-title p { margin: 0 0 6px; color: var(--accent); font-size: 10px; letter-spacing: .12em; }.section-title h2 { margin: 0; font-family: var(--font-serif); font-size: 22px; font-weight: 500; }.rule-form, .reminder-settings { display: grid; gap: 15px; }.funded-capital, .cash-form h3 { margin: 0; color: var(--ink-muted); font-size: 13px; }.cash-form { margin-top: 22px; padding-top: 18px; border-top: 1px solid var(--line); }.cash-history { display: grid; gap: 1px; margin: 16px 0 0; padding: 0; background: var(--line); list-style: none; }.cash-history li { display: grid; grid-template-columns: 64px 120px 1fr auto; gap: 12px; align-items: center; padding: 12px 14px; background: var(--paper); font-size: 12px; }.cash-history time, .cash-history span { color: var(--ink-faint); }.cash-history p { margin: 0; }.reminder-settings p { max-width: 620px; margin: 0; color: var(--ink-muted); font-size: 12px; line-height: 1.65; }.reminder-settings .field { max-width: 260px; }.field-grid { display: grid; gap: 14px; }.field-grid--three { grid-template-columns: repeat(3, minmax(0, 1fr)); }.field-grid--two { grid-template-columns: repeat(2, minmax(0, 1fr)); }.sequence-gate { display: grid; gap: 12px; margin: 0; padding: 14px; border: 1px solid var(--line); }.sequence-gate legend { padding: 0 6px; color: var(--ink-muted); font-size: 11px; }.sequence-gate p { margin: 0; color: var(--ink-faint); font-size: 11px; }.check-field { display: flex; align-items: center; gap: 8px; color: var(--ink); font-size: 13px; }.check-field input { width: auto; }.rate-row { display: grid; grid-template-columns: 120px 1fr auto; gap: 12px; align-items: end; }.data-actions { display: flex; align-items: center; gap: 12px; }.data-actions p { color: var(--ink-faint); font-size: 11px; }.csv-preview { grid-column: 2; display: flex; align-items: center; gap: 14px; margin-top: 12px; padding: 12px; background: var(--paper-deep); font-size: 11px; }.audit-list { margin: 0; padding: 0; list-style: none; }.audit-list li { display: grid; grid-template-columns: 150px 120px 1fr; gap: 12px; padding: 10px 0; border-top: 1px solid var(--line); font-size: 11px; }.audit-list time { color: var(--ink-faint); }.success-notice { color: #506448; font-size: 13px; }
.rule-history { display: grid; gap: 1px; margin: 0; padding: 0; background: var(--line); list-style: none; }.rule-history li { display: grid; grid-template-columns: 120px 1fr; gap: 5px 16px; padding: 14px; background: var(--paper); }.rule-history li div { grid-row: span 2; display: grid; gap: 4px; }.rule-history strong { font-family: var(--font-serif); font-size: 20px; font-weight: 500; }.rule-history time, .rule-history span { color: var(--ink-faint); font-size: 10px; }.rule-history p { margin: 0; font-size: 12px; }
</style>
