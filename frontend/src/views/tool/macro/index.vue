<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { NButton, useMessage } from 'naive-ui'
import { useAuth } from '@/hooks/business/auth'
import { $t } from '@/locales'
import {
  fetchSyncStatus,
  getDepositRates,
  getLoanRates,
  getLPR,
  getMoneySupplyMonth,
  getMoneySupplyYear,
  getReserveRatios,
  syncMacroData,
  getGDP,
  getCPI,
  getPMI,
  getPPI
} from '@/service/api'
import DepositRatePanel from './modules/deposit-rate.vue'
import LoanRatePanel from './modules/loan-rate.vue'
import LprPanel from './modules/lpr.vue'
import MoneySupplyMonthPanel from './modules/money-supply-month.vue'
import MoneySupplyYearPanel from './modules/money-supply-year.vue'
import ReserveRatioPanel from './modules/reserve-ratio.vue'
import GDPPanel from './modules/gdp.vue'
import CPIPanel from './modules/cpi.vue'
import PMIPanel from './modules/pmi.vue'
import PPIPanel from './modules/ppi.vue'

defineOptions({ name: 'ToolMacro' })

const message = useMessage()
const { hasAuth } = useAuth()
const canSync = computed(() => hasAuth('stock:sync:execute'))

const loading = ref(false)
const depositRates = ref<Api.Macro.DepositRate[]>([])
const loanRates = ref<Api.Macro.LoanRate[]>([])
const reserveRatios = ref<Api.Macro.ReserveRatio[]>([])
const moneySupplyMonth = ref<Api.Macro.MoneySupplyMonth[]>([])
const moneySupplyYear = ref<Api.Macro.MoneySupplyYear[]>([])
const lprData = ref<Api.Macro.LPR[]>([])
const gdpData = ref<Api.Macro.GDP[]>([])
const cpiData = ref<Api.Macro.CPI[]>([])
const pmiData = ref<Api.Macro.PMI[]>([])
const ppiData = ref<Api.Macro.PPI[]>([])

async function loadAll() {
  loading.value = true
  try {
    const [drRes, lrRes, rrRes, msmRes, msyRes, lprRes, gdpRes, cpiRes, pmiRes, ppiRes] = await Promise.all([
      getDepositRates(),
      getLoanRates(),
      getReserveRatios(),
      getMoneySupplyMonth(),
      getMoneySupplyYear(),
      getLPR(),
      getGDP(),
      getCPI(),
      getPMI(),
      getPPI()
    ])
    if (drRes.data) depositRates.value = drRes.data
    if (lrRes.data) loanRates.value = lrRes.data
    if (rrRes.data) reserveRatios.value = rrRes.data
    if (msmRes.data) moneySupplyMonth.value = msmRes.data
    if (msyRes.data) moneySupplyYear.value = msyRes.data
    if (lprRes.data) lprData.value = lprRes.data
    if (gdpRes.data) gdpData.value = gdpRes.data
    if (cpiRes.data) cpiData.value = cpiRes.data
    if (pmiRes.data) pmiData.value = pmiRes.data
    if (ppiRes.data) ppiData.value = ppiRes.data
  } catch (e: any) {
    message.error(e.message || $t('page.tool.macro.loadFailed'))
  } finally {
    loading.value = false
  }
}

// 同步任务状态: 页面触发同步后轮询状态, 完成自动刷新数据
const syncStatus = ref<Api.Stock.SyncStatus | null>(null)
const syncRunning = ref(false)
const SYNC_POLL_RUNNING_MS = 3000
const SYNC_POLL_IDLE_MS = 60000
let syncPollTimer: ReturnType<typeof setTimeout> | null = null
let syncPollDisposed = false

const syncFinishedText = computed(() => {
  const t = syncStatus.value?.finishedAt
  if (!t) return ''
  const d = new Date(t)
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return ''
  return d.toLocaleString('zh-CN', { hour12: false })
})

function stopSyncPolling() {
  syncPollDisposed = true
  if (syncPollTimer) {
    clearTimeout(syncPollTimer)
    syncPollTimer = null
  }
}

async function refreshSyncStatus() {
  try {
    const { data } = await fetchSyncStatus()
    if (data) {
      const wasRunning = syncRunning.value
      syncStatus.value = data
      syncRunning.value = data.running
      if (wasRunning && !data.running) {
        if (data.lastError) {
          message.error($t('page.tool.macro.syncFailed', { error: data.lastError }))
        } else {
          message.success($t('page.tool.macro.syncDone'))
        }
        loadAll()
      }
    }
  } catch {
    // ignore
  }
  if (!syncPollDisposed) {
    syncPollTimer = setTimeout(refreshSyncStatus, syncRunning.value ? SYNC_POLL_RUNNING_MS : SYNC_POLL_IDLE_MS)
  }
}

async function handleSync() {
  if (syncRunning.value) return
  try {
    await syncMacroData()
    syncRunning.value = true
    message.success($t('page.tool.macro.syncStarted'))
    if (syncPollTimer) {
      clearTimeout(syncPollTimer)
      syncPollTimer = null
    }
    refreshSyncStatus()
  } catch (e: any) {
    message.error(e.message || $t('page.tool.macro.syncStartFailed'))
  }
}

onMounted(() => {
  loadAll()
  refreshSyncStatus()
})

onUnmounted(() => {
  stopSyncPolling()
})
</script>

<template>
  <div class="h-full flex flex-col gap-4 p-4 overflow-auto">
    <div class="flex items-center justify-between">
      <div>
        <h2 class="text-lg font-bold">{{ $t('page.tool.macro.title') }}</h2>
      </div>
      <div class="flex items-center gap-3">
        <span v-if="syncRunning" class="text-sm text-blue-500">{{ $t('page.tool.macro.syncing') }}</span>
        <span v-else-if="syncFinishedText" class="text-sm text-gray-500">
          {{ $t('page.tool.macro.lastSync', { time: syncFinishedText }) }}
        </span>
        <NButton v-if="canSync" type="primary" size="small" :loading="syncRunning" @click="handleSync">
          {{ $t('page.tool.macro.sync') }}
        </NButton>
      </div>
    </div>

    <NCard :bordered="false" size="small">
      <NTabs type="line" animated>
        <NTabPane name="deposit-rate" :tab="$t('page.tool.macro.depositRate')">
          <DepositRatePanel :data="depositRates" :loading="loading" />
        </NTabPane>
        <NTabPane name="loan-rate" :tab="$t('page.tool.macro.loanRate')">
          <LoanRatePanel :data="loanRates" :loading="loading" />
        </NTabPane>
        <NTabPane name="lpr" :tab="$t('page.tool.macro.lpr')">
          <LprPanel :data="lprData" :loading="loading" />
        </NTabPane>
        <NTabPane name="reserve" :tab="$t('page.tool.macro.reserveRatio')">
          <ReserveRatioPanel :data="reserveRatios" :loading="loading" />
        </NTabPane>
        <NTabPane name="month" :tab="$t('page.tool.macro.moneySupplyMonth')">
          <MoneySupplyMonthPanel :data="moneySupplyMonth" :loading="loading" />
        </NTabPane>
        <NTabPane name="year" :tab="$t('page.tool.macro.moneySupplyYear')">
          <MoneySupplyYearPanel :data="moneySupplyYear" :loading="loading" />
        </NTabPane>
        <NTabPane name="gdp" :tab="$t('page.tool.macro.gdp')">
          <GDPPanel :data="gdpData" :loading="loading" />
        </NTabPane>
        <NTabPane name="cpi" :tab="$t('page.tool.macro.cpi')">
          <CPIPanel :data="cpiData" :loading="loading" />
        </NTabPane>
        <NTabPane name="pmi" :tab="$t('page.tool.macro.pmi')">
          <PMIPanel :data="pmiData" :loading="loading" />
        </NTabPane>
        <NTabPane name="ppi" :tab="$t('page.tool.macro.ppi')">
          <PPIPanel :data="ppiData" :loading="loading" />
        </NTabPane>
      </NTabs>
    </NCard>
  </div>
</template>
