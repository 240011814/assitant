<script setup lang="ts">
import { computed, h, ref } from 'vue'
import { useMessage } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { runBacktest } from '@/service/api'
import { useEcharts } from '@/hooks/common/echarts'
import { $t } from '@/locales'

defineOptions({ name: 'ToolBacktest' })

const message = useMessage()

// 条件构建器
interface ConditionRow {
  field: Api.Stock.BacktestField
  operator: Api.Stock.BacktestOperator
  value: number | null
  value2: number | null
}

const fieldOptions = computed(() => [
  { label: $t('page.tool.backtest.field.price'), value: 'price' },
  { label: $t('page.tool.backtest.field.changePct'), value: 'changePct' },
  { label: $t('page.tool.backtest.field.turnoverRate'), value: 'turnoverRate' },
  { label: $t('page.tool.backtest.field.amount'), value: 'amount' },
  { label: $t('page.tool.backtest.field.peTtm'), value: 'peTtm' },
  { label: $t('page.tool.backtest.field.pb'), value: 'pb' }
])

const operatorOptions = computed(() => [
  { label: '>', value: 'gt' },
  { label: '>=', value: 'gte' },
  { label: '<', value: 'lt' },
  { label: '<=', value: 'lte' },
  { label: $t('page.tool.backtest.between'), value: 'between' }
])

const conditions = ref<ConditionRow[]>([])

function addCondition() {
  conditions.value.push({ field: 'peTtm', operator: 'lt', value: null, value2: null })
}

function removeCondition(index: number) {
  conditions.value.splice(index, 1)
}

function buildConditions(): Api.Stock.BacktestCondition[] {
  const list: Api.Stock.BacktestCondition[] = []
  for (const c of conditions.value) {
    if (c.value === null) continue
    if (c.operator === 'between') {
      if (c.value2 === null) continue
      list.push({ field: c.field, operator: 'between', value: [c.value, c.value2] })
    } else {
      list.push({ field: c.field, operator: c.operator, value: c.value })
    }
  }
  return list
}

// 回测参数
const nowYear = new Date().getFullYear()
const params = ref({
  startYear: nowYear - 5,
  endYear: nowYear,
  holdDays: 20,
  maxStocks: 30
})

const running = ref(false)
const result = ref<Api.Stock.BacktestResponse | null>(null)

async function handleRun() {
  if (params.value.startYear > params.value.endYear) {
    message.warning($t('page.tool.backtest.yearInvalid'))
    return
  }
  running.value = true
  const { data, error } = await runBacktest({
    conditions: buildConditions(),
    startYear: params.value.startYear,
    endYear: params.value.endYear,
    holdDays: params.value.holdDays,
    maxStocks: params.value.maxStocks
  })
  running.value = false
  if (error) {
    message.error(error.message || $t('page.tool.backtest.runFailed'))
    return
  }
  if (data) {
    result.value = data
    updateChart(data)
  }
}

// 汇总统计卡片
const summaryCards = computed(() => {
  const s = result.value?.summary
  if (!s) return []
  return [
    { label: $t('page.tool.backtest.periodCount'), value: String(s.periodCount), raw: null },
    { label: $t('page.tool.backtest.meanReturn'), value: pct(s.meanReturn), raw: s.meanReturn },
    { label: $t('page.tool.backtest.medianReturn'), value: pct(s.medianReturn), raw: s.medianReturn },
    { label: $t('page.tool.backtest.winRate'), value: pct(s.winRate), raw: null },
    { label: $t('page.tool.backtest.bestReturn'), value: pct(s.bestReturn), raw: s.bestReturn },
    { label: $t('page.tool.backtest.worstReturn'), value: pct(s.worstReturn), raw: s.worstReturn },
    { label: $t('page.tool.backtest.cumulativeRet'), value: pct(s.cumulativeRet), raw: s.cumulativeRet }
  ]
})

// 小数转百分比: ×100 保留2位
function pct(v: number | null | undefined) {
  if (v === null || v === undefined) return '-'
  return `${(v * 100).toFixed(2)}%`
}

// 收益着色: 正收益绿 / 负收益红
function returnClass(v: number | null | undefined) {
  if (v === null || v === undefined) return ''
  return v >= 0 ? 'text-green-500' : 'text-red-500'
}

// 各期收益柱状图
const { domRef, updateOptions } = useEcharts(() => ({
  tooltip: { trigger: 'axis' },
  grid: { left: '3%', right: '4%', bottom: '3%', top: '12%', containLabel: true },
  xAxis: { type: 'category', data: [] as string[] },
  yAxis: { type: 'value', name: '%' },
  series: [
    {
      name: $t('page.tool.backtest.return'),
      type: 'bar',
      data: [] as { value: number; itemStyle: { color: string } }[]
    }
  ]
}))

function updateChart(res: Api.Stock.BacktestResponse) {
  updateOptions(opts => {
    opts.xAxis.data = res.periods.map(p => p.rebalanceDate)
    opts.series[0].data = res.periods.map(p => ({
      value: p.return === null ? Number.NaN : Number((p.return * 100).toFixed(2)),
      itemStyle: { color: (p.return ?? 0) >= 0 ? '#18a058' : '#d03050' }
    }))
    return opts
  })
}

// 各期明细表
const periodColumns = computed<DataTableColumns<Api.Stock.BacktestPeriod>>(() => [
  { title: $t('page.tool.backtest.rebalanceDate'), key: 'rebalanceDate', width: 120 },
  { title: $t('page.tool.backtest.sellDate'), key: 'sellDate', width: 120 },
  { title: $t('page.tool.backtest.stockCount'), key: 'stockCount', width: 90 },
  {
    title: $t('page.tool.backtest.return'),
    key: 'return',
    width: 110,
    render: row => {
      if (row.return === null) return '-'
      return h('span', { class: returnClass(row.return) }, pct(row.return))
    }
  }
])
</script>

<template>
  <div class="flex h-full">
    <!-- 左侧: 条件构建器 + 参数 -->
    <div class="w-76 border-r border-gray-200 p-4 overflow-y-auto flex-shrink-0">
      <div class="mb-4">
        <h3 class="text-lg font-bold mb-2">{{ $t('page.tool.backtest.conditions') }}</h3>
        <div class="space-y-2">
          <div v-for="(c, index) in conditions" :key="index" class="flex items-center gap-1">
            <NSelect v-model:value="c.field" :options="fieldOptions" size="small" class="w-28" />
            <NSelect v-model:value="c.operator" :options="operatorOptions" size="small" class="w-16" />
            <NInputNumber v-model:value="c.value" size="small" class="w-20" placeholder="min" />
            <NInputNumber v-if="c.operator === 'between'" v-model:value="c.value2" size="small" class="w-20" placeholder="max" />
            <NButton size="small" text @click="removeCondition(index)">
              <template #icon><span class="i-material-icons-close text-red-500" /></template>
            </NButton>
          </div>
        </div>
        <NButton size="small" class="mt-2" @click="addCondition">{{ $t('page.tool.backtest.addCondition') }}</NButton>
        <div class="mt-2 text-xs text-gray-400">{{ $t('page.tool.backtest.conditionsTip') }}</div>
      </div>

      <div class="mb-4 space-y-3">
        <h3 class="text-sm font-bold">{{ $t('page.tool.backtest.params') }}</h3>
        <div>
          <div class="text-sm mb-1">{{ $t('page.tool.backtest.startYear') }}</div>
          <NInputNumber v-model:value="params.startYear" size="small" :precision="0" class="w-full" />
        </div>
        <div>
          <div class="text-sm mb-1">{{ $t('page.tool.backtest.endYear') }}</div>
          <NInputNumber v-model:value="params.endYear" size="small" :precision="0" class="w-full" />
        </div>
        <div>
          <div class="text-sm mb-1">{{ $t('page.tool.backtest.holdDays') }}</div>
          <NInputNumber v-model:value="params.holdDays" size="small" :min="1" :precision="0" class="w-full" />
        </div>
        <div>
          <div class="text-sm mb-1">{{ $t('page.tool.backtest.maxStocks') }}</div>
          <NInputNumber v-model:value="params.maxStocks" size="small" :min="1" :precision="0" class="w-full" />
        </div>
      </div>

      <NButton type="primary" block :loading="running" @click="handleRun">
        {{ running ? $t('page.tool.backtest.running') : $t('page.tool.backtest.run') }}
      </NButton>
    </div>

    <!-- 右侧: 结果 -->
    <div class="flex-1 overflow-auto p-4">
      <NEmpty v-if="!result" class="py-24" :description="$t('page.tool.backtest.emptyTip')" />

      <template v-else>
        <!-- 汇总统计卡片组 -->
        <div class="grid grid-cols-2 md:grid-cols-4 gap-3 mb-4">
          <div
            v-for="card in summaryCards"
            :key="card.label"
            class="rounded-lg bg-gray-50 dark:bg-gray-800 p-3 flex flex-col gap-1"
          >
            <span class="text-xs text-gray-500">{{ card.label }}</span>
            <span class="text-lg font-bold" :class="returnClass(card.raw)">
              {{ card.value }}
            </span>
          </div>
        </div>

        <div class="mb-2 text-sm text-gray-400">
          {{ $t('page.tool.backtest.skipped') }}: {{ result.skipped }}
        </div>

        <!-- 每期收益柱状图 -->
        <NCard :bordered="false" size="small" :title="$t('page.tool.backtest.periodChart')" class="mb-4">
          <div ref="domRef" class="h-360px overflow-hidden" />
        </NCard>

        <!-- 各期明细 -->
        <NCard :bordered="false" size="small" :title="$t('page.tool.backtest.periodTable')">
          <NDataTable
            :columns="periodColumns"
            :data="result.periods"
            :row-key="(row: Api.Stock.BacktestPeriod) => row.rebalanceDate"
            :pagination="{ pageSize: 20 }"
            size="small"
            striped
          />
        </NCard>
      </template>
    </div>
  </div>
</template>
