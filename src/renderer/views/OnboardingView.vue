<script setup lang="ts">
import { computed, ref } from 'vue'

import { APIError, api } from '@/renderer/lib/api'
import type { CompleteOnboardingInput, HoldingHorizon, MarketScope, UserProfile } from '@/renderer/types'

const emit = defineEmits<{ completed: [profile: UserProfile] }>()

const step = ref(1)
const capitalYuan = ref('')
const maxLossYuan = ref('')
const holdingHorizon = ref<Exclude<HoldingHorizon, 'legacy_unspecified'> | ''>('')
const enabledMarkets = ref<MarketScope[]>([])
const confirmations = ref([false, false, false])
const submitting = ref(false)
const errorMessage = ref('')

const capital = computed(() => Number(capitalYuan.value))
const maxLoss = computed(() => Number(maxLossYuan.value))
const lossRatio = computed(() => capital.value > 0 && maxLoss.value > 0 ? maxLoss.value / capital.value * 100 : 0)
const moneyStepValid = computed(() => Number.isFinite(capital.value) && Number.isFinite(maxLoss.value) && capital.value > 0 && maxLoss.value > 0 && maxLoss.value < capital.value)
const scopeStepValid = computed(() => holdingHorizon.value !== '' && enabledMarkets.value.length > 0)
const privacyStepValid = computed(() => confirmations.value.every(Boolean))

function next() {
  errorMessage.value = ''
  if (step.value === 1 && moneyStepValid.value)
    step.value = 2
  else if (step.value === 2 && scopeStepValid.value)
    step.value = 3
}

function back() {
  errorMessage.value = ''
  if (step.value > 1)
    step.value--
}

async function reloadCompletedProfile() {
  const current = await api.request<UserProfile>('/api/onboarding')
  if (current.onboardingStatus === 'completed')
    emit('completed', current)
}

async function complete() {
  if (!privacyStepValid.value || !holdingHorizon.value)
    return
  submitting.value = true
  errorMessage.value = ''
  const payload: CompleteOnboardingInput = {
    investableCapitalFen: Math.round(capital.value * 100),
    maxLossFen: Math.round(maxLoss.value * 100),
    holdingHorizon: holdingHorizon.value,
    enabledMarkets: enabledMarkets.value,
  }
  try {
    const profile = await api.request<UserProfile>('/api/onboarding/complete', {
      method: 'POST',
      body: JSON.stringify(payload),
    })
    emit('completed', profile)
  }
  catch (error) {
    if (error instanceof APIError && error.code === 'ONBOARDING_ALREADY_COMPLETED') {
      await reloadCompletedProfile()
      return
    }
    if (error instanceof APIError && Object.keys(error.fields).some(field => ['investableCapitalFen', 'maxLossFen'].includes(field)))
      step.value = 1
    else if (error instanceof APIError && Object.keys(error.fields).some(field => ['holdingHorizon', 'enabledMarkets'].includes(field)))
      step.value = 2
    errorMessage.value = error instanceof Error ? error.message : '创建本地工作区失败，请重试'
  }
  finally {
    submitting.value = false
  }
}
</script>

<template>
  <main class="onboarding">
    <header class="onboarding__brand">
      <p class="onboarding__kicker">PLAIN RULE · LOCAL FIRST</p>
      <h1>先建立你的纪律底线</h1>
      <p>这里只设置你的资金边界和使用范围。不会预填任何股票、持仓或资产配置。</p>
    </header>

    <ol class="onboarding__progress" aria-label="设置进度">
      <li :class="{ active: step === 1 }">01 资金边界</li>
      <li :class="{ active: step === 2 }">02 使用范围</li>
      <li :class="{ active: step === 3 }">03 本地确认</li>
    </ol>

    <section v-if="step === 1" class="onboarding__panel">
      <div>
        <p class="onboarding__section-label">第一步</p>
        <h2>先决定最多能失去多少</h2>
        <p>总资金用于计算仓位，最大损失用于纪律红线；两者都可以之后留下修改记录。</p>
      </div>
      <div class="onboarding__fields">
        <label class="field">
          <span>可投资总资金（元）</span>
          <input v-model="capitalYuan" inputmode="decimal" autocomplete="off" placeholder="例如 200000" />
        </label>
        <label class="field">
          <span>最大可承受损失（元）</span>
          <input v-model="maxLossYuan" inputmode="decimal" autocomplete="off" placeholder="例如 20000" />
        </label>
        <p v-if="lossRatio > 0" class="onboarding__ratio">占总资金 {{ lossRatio.toFixed(1) }}%</p>
        <p v-if="capitalYuan && maxLossYuan && !moneyStepValid" class="onboarding__error">最大损失必须大于 0 且小于总资金。</p>
      </div>
    </section>

    <section v-else-if="step === 2" class="onboarding__panel">
      <div>
        <p class="onboarding__section-label">第二步</p>
        <h2>选择你的持有期限和市场</h2>
        <p>这里只控制界面范围，不会替你选择证券，也不会自动建立计划。</p>
      </div>
      <div class="onboarding__fields">
        <fieldset class="onboarding__choices">
          <legend>预期持有期限</legend>
          <label><input v-model="holdingHorizon" type="radio" value="under_6m" /> 半年以内</label>
          <label><input v-model="holdingHorizon" type="radio" value="6_to_12m" /> 半年到一年</label>
          <label><input v-model="holdingHorizon" type="radio" value="1_to_3y" /> 一到三年</label>
          <label><input v-model="holdingHorizon" type="radio" value="over_3y" /> 三年以上</label>
        </fieldset>
        <fieldset class="onboarding__choices">
          <legend>使用市场（可多选）</legend>
          <label><input v-model="enabledMarkets" type="checkbox" value="ashare_stock" /> A 股股票</label>
          <label><input v-model="enabledMarkets" type="checkbox" value="ashare_etf" /> A 股 ETF</label>
          <label><input v-model="enabledMarkets" type="checkbox" value="hk" /> 港股</label>
        </fieldset>
      </div>
    </section>

    <section v-else class="onboarding__panel">
      <div>
        <p class="onboarding__section-label">最后确认</p>
        <h2>这是一个完全空白的本地工作区</h2>
        <p>确认后只建立你的纪律账户。你可以随后自行添加证券、成交、规则和资产配置。</p>
      </div>
      <fieldset class="onboarding__confirmations">
        <legend class="sr-only">本地与隐私确认</legend>
        <label><input v-model="confirmations[0]" type="checkbox" /> 数据只保存在本机</label>
        <label><input v-model="confirmations[1]" type="checkbox" /> 不会连接券商或自动下单</label>
        <label><input v-model="confirmations[2]" type="checkbox" /> 新工作区没有任何预设持仓和资产配置</label>
      </fieldset>
    </section>

    <p v-if="errorMessage" role="alert" class="onboarding__error">{{ errorMessage }}</p>
    <footer class="onboarding__actions">
      <button v-if="step > 1" type="button" class="button" :disabled="submitting" @click="back">返回</button>
      <button v-if="step < 3" type="button" class="button button--primary" :disabled="step === 1 ? !moneyStepValid : !scopeStepValid" @click="next">下一步</button>
      <button v-else type="button" class="button button--primary" :disabled="!privacyStepValid || submitting" @click="complete">
        {{ submitting ? '正在创建…' : '创建空白工作区' }}
      </button>
    </footer>
  </main>
</template>

<style scoped>
.onboarding { width: min(920px, calc(100% - 64px)); margin: 0 auto; padding: 56px 0 72px; }
.onboarding__brand { max-width: 720px; }
.onboarding__kicker, .onboarding__section-label { margin: 0 0 14px; color: var(--accent); font-size: 11px; font-weight: 700; letter-spacing: .16em; }
.onboarding h1 { margin: 0; font-family: var(--font-serif); font-size: clamp(42px, 6vw, 72px); font-weight: 500; letter-spacing: -.04em; }
.onboarding__brand > p:last-child, .onboarding__panel > div > p:last-child { color: var(--ink-muted); line-height: 1.8; }
.onboarding__progress { display: grid; grid-template-columns: repeat(3, 1fr); padding: 0; margin: 48px 0 20px; list-style: none; border-bottom: 1px solid var(--line); }
.onboarding__progress li { padding: 14px 2px; color: var(--ink-faint); font-size: 12px; }
.onboarding__progress li.active { color: var(--ink); border-bottom: 2px solid var(--accent); font-weight: 700; }
.onboarding__panel { display: grid; grid-template-columns: minmax(240px, .8fr) minmax(340px, 1.2fr); gap: 56px; min-height: 300px; padding: 38px; border: 1px solid var(--line); background: var(--paper); }
.onboarding__panel h2 { margin: 0; font-family: var(--font-serif); font-size: 30px; font-weight: 500; }
.onboarding__fields { display: grid; align-content: start; gap: 18px; }
.onboarding__fields .field { font-size: 13px; }
.onboarding__fields input:not([type='radio']):not([type='checkbox']) { min-height: 54px; font-size: 20px; font-variant-numeric: tabular-nums; }
.onboarding__ratio { margin: -4px 0 0; color: var(--success); font-weight: 700; }
.onboarding__choices, .onboarding__confirmations { display: grid; gap: 12px; padding: 0; border: 0; }
.onboarding__choices legend { margin-bottom: 12px; color: var(--ink-muted); font-size: 12px; }
.onboarding__choices label, .onboarding__confirmations label { display: flex; align-items: center; gap: 10px; min-height: 42px; padding: 0 12px; border: 1px solid var(--line); cursor: pointer; }
.onboarding__confirmations { align-content: start; }
.onboarding__error { margin: 14px 0 0; color: var(--accent); font-size: 12px; }
.onboarding__actions { display: flex; justify-content: flex-end; gap: 10px; margin-top: 20px; }
.onboarding__actions .button { min-width: 140px; min-height: 48px; }
@media (max-width: 820px) {
  .onboarding__panel { grid-template-columns: 1fr; gap: 24px; }
}
</style>
