<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted, h } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage, NButton, NTag } from 'naive-ui'
import { useAuth } from '@/hooks/business/auth'
import {
  stockScreen,
  stockIndustries,
  stockConcepts,
  saveFilterCondition,
  getFilterConditions,
  deleteFilterCondition,
  fetchSyncStatus,
  getWatchlistCodes
} from '@/service/api'
import WatchlistAddDialog from '@/components/custom/watchlist-add-dialog.vue'

defineOptions({ name: 'ToolStockscreen' })

const router = useRouter()
const message = useMessage()
const { hasAuth } = useAuth()

const loading = ref(false)
const results = ref<Api.Stock.ScreenResult[]>([])
const total = ref(0)
const currentPage = ref(1)
const pageSize = ref(20)

const industryOptions = ref<string[]>([])
const conceptOptions = ref<{ label: string; value: string }[]>([])
const savedFilters = ref<Api.Stock.FilterConditionSave[]>([])
const watchlistCodes = ref<Set<string>>(new Set())

function screenRowKey(row: Api.Stock.ScreenResult) {
  return row.code
}

function displayCode(code: string) {
  return code.includes('.') ? code.split('.')[1] : code
}

const filterForm = reactive({
  conditions: [] as Api.Stock.FilterCondition[],
  conceptNames: [] as string[],
  industries: [] as string[],
  markets: [] as string[],
  securityTypes: [] as number[],
  excludeSt: true,
  sortBy: 'code',
  sortOrder: 'asc' as 'asc' | 'desc' | null,
  keyword: ''
})

// 筛选条件模板
const filterTemplates = [
  { label: 'PE(TTM)', field: 'peTtm', type: 'number' },
  { label: 'PB', field: 'pb', type: 'number' },
  { label: 'ROE(%)', field: 'roe', type: 'number' },
  { label: '营收增长率(%)', field: 'revenueYoy', type: 'number' },
  { label: '净利润增长率(%)', field: 'netProfitYoy', type: 'number' },
  { label: '涨跌幅(%)', field: 'changePct', type: 'number' },
  { label: '换手率(%)', field: 'turnoverRate', type: 'number' },
  { label: '总市值(亿)', field: 'marketCap', type: 'number' }
]

const operatorOptions = [
  { label: '>', value: 'gt' },
  { label: '>=', value: 'gte' },
  { label: '<', value: 'lt' },
  { label: '<=', value: 'lte' },
  { label: '=', value: 'eq' },
  { label: '区间', value: 'between' }
]

const marketOptions = [
  { label: '沪市', value: 'SH' },
  { label: '深市', value: 'SZ' },
  { label: '北交所', value: 'BJ' }
]

const securityTypeOptions = [
  { label: '股票', value: 1 },
  { label: '指数', value: 2 },
  { label: 'ETF', value: 5 }
]

const columns = [
  { title: '代码', key: 'code', width: 80, fixed: 'left' as const,
    render: (row: Api.Stock.ScreenResult) => h('a', {
      class: 'text-blue-500 cursor-pointer hover:underline',
      onClick: () => goToDetail(row.code)
    }, displayCode(row.code))
  },
  { title: '名称', key: 'name', width: 100, fixed: 'left' as const },
  { title: '类型', key: 'type', width: 60,
    render: (row: Api.Stock.ScreenResult) => {
      if (row.type === 2) return h(NTag, { size: 'small', type: 'info', bordered: false }, { default: () => '指数' })
      if (row.type === 5) return h(NTag, { size: 'small', type: 'warning', bordered: false }, { default: () => 'ETF' })
      return '股票'
    }
  },
  { title: '现价', key: 'price', width: 80, render: (row: Api.Stock.ScreenResult) => row.price?.toFixed(2) || '-' },
  { title: '涨跌%', key: 'changePct', width: 80, sorter: true,
    render: (row: Api.Stock.ScreenResult) => {
      if (row.changePct === null) return '-'
      const color = row.changePct >= 0 ? 'text-red-500' : 'text-green-500'
      return h('span', { class: color }, `${row.changePct >= 0 ? '+' : ''}${row.changePct.toFixed(2)}%`)
    }
  },
  { title: 'PE(TTM)', key: 'peTtm', width: 80, sorter: true,
    render: (row: Api.Stock.ScreenResult) => row.peTtm?.toFixed(2) || '-'
  },
  { title: 'PB', key: 'pb', width: 60, sorter: true,
    render: (row: Api.Stock.ScreenResult) => row.pb?.toFixed(2) || '-'
  },
  { title: 'ROE%', key: 'roe', width: 70, sorter: true,
    render: (row: Api.Stock.ScreenResult) => row.roe?.toFixed(2) || '-'
  },
  { title: '换手率%', key: 'turnoverRate', width: 80, sorter: true,
    render: (row: Api.Stock.ScreenResult) => row.turnoverRate?.toFixed(2) || '-'
  },
  { title: '市值(亿)', key: 'marketCap', width: 90, sorter: true,
    render: (row: Api.Stock.ScreenResult) => row.marketCap?.toFixed(2) || '-'
  },
  { title: '营收增长%', key: 'revenueYoy', width: 90, sorter: true,
    render: (row: Api.Stock.ScreenResult) => row.revenueYoy?.toFixed(2) || '-'
  },
  { title: '行业', key: 'industry', width: 80 },
  { title: '操作', key: 'actions', width: 100, fixed: 'right' as const,
    render: (row: Api.Stock.ScreenResult) => h('div', { class: 'flex gap-1' }, [
      h(NButton, { 
        size: 'tiny', 
        text: true, 
        disabled: !hasAuth('stock:menu:view'),
        onClick: () => goToDetail(row.code) 
      }, { default: () => '详情' }),
      !watchlistCodes.value.has(row.code) && hasAuth('stock:watchlist:edit')
        ? h(NButton, { 
            size: 'tiny', 
            text: true, 
            onClick: () => handleAddWatchlist(row) 
          }, { default: () => '加自选' })
        : null
    ])
  }
]

// 动态筛选条件
const dynamicFilters = ref<Array<{
  field: string
  label: string
  operator: string
  value: number | null
  value2: number | null
}>>([])

function addFilter() {
  dynamicFilters.value.push({
    field: 'peTtm',
    label: 'PE(TTM)',
    operator: 'lt',
    value: null,
    value2: null
  })
}

function removeFilter(index: number) {
  dynamicFilters.value.splice(index, 1)
}

function buildConditions(): Api.Stock.FilterCondition[] {
  const conditions: Api.Stock.FilterCondition[] = []
  for (const f of dynamicFilters.value) {
    if (f.operator === 'between') {
      if (f.value !== null && f.value2 !== null) {
        conditions.push({
          field: f.field,
          operator: 'between',
          value: [f.value, f.value2]
        })
      }
    } else if (f.value !== null) {
      conditions.push({
        field: f.field,
        operator: f.operator as any,
        value: f.value
      })
    }
  }
  return conditions
}

// 请求序号守卫: 防止旧响应覆盖新结果
let reqSeq = 0

async function doScreen() {
  const seq = ++reqSeq
  loading.value = true
  try {
    const conditions = buildConditions()
    const { data: res } = await stockScreen({
      conditions,
      conceptNames: filterForm.conceptNames,
      industries: filterForm.industries,
      markets: filterForm.markets,
      securityTypes: filterForm.securityTypes,
      excludeSt: filterForm.excludeSt,
      sortBy: filterForm.sortOrder ? filterForm.sortBy : undefined,
      sortOrder: filterForm.sortOrder || undefined,
      page: currentPage.value,
      pageSize: pageSize.value,
      keyword: filterForm.keyword || undefined
    })
    if (seq !== reqSeq) return
    if (res) {
      results.value = res.list
      total.value = res.total
    }
  } catch (e: any) {
    if (seq !== reqSeq) return
    message.error(e.message || '筛选失败')
  } finally {
    if (seq === reqSeq) {
      loading.value = false
    }
  }
}

// 点击"开始筛选"/回车触发: 重置到第一页
function handleScreen() {
  currentPage.value = 1
  doScreen()
}

function handleSorter(options: any) {
  if (options.columnKey) {
    filterForm.sortBy = options.columnKey
    // 取消排序(order 为 falsy)时不带排序参数
    filterForm.sortOrder = options.order === 'ascend' ? 'asc' : options.order === 'descend' ? 'desc' : null
    doScreen()
  }
}

function handlePageChange(page: number) {
  currentPage.value = page
  doScreen()
}

function handlePageSizeChange(size: number) {
  pageSize.value = size
  currentPage.value = 1
  doScreen()
}

function goToDetail(code: string) {
  router.push({ name: 'tool_stockdetail', query: { code } })
}

const showAddDialog = ref(false)
const addCode = ref('')
const addName = ref('')

async function loadWatchlistCodes() {
  try {
    const { data } = await getWatchlistCodes()
    watchlistCodes.value = new Set(data || [])
  } catch {
    // ignore
  }
}

function handleAddWatchlist(row: Api.Stock.ScreenResult) {
  addCode.value = row.code
  addName.value = row.name
  showAddDialog.value = true
}

function handleWatchlistAdded() {
  watchlistCodes.value = new Set([...watchlistCodes.value, addCode.value])
}

async function handleSaveFilter() {
  const conditions = buildConditions()
  if (conditions.length === 0) {
    message.warning('请先添加筛选条件')
    return
  }
  try {
    await saveFilterCondition({
      name: `筛选条件 ${new Date().toLocaleString()}`,
      conditions: JSON.stringify(conditions)
    })
    message.success('保存成功')
    loadSavedFilters()
  } catch (e: any) {
    message.error(e.message || '保存失败')
  }
}

async function loadSavedFilters() {
  try {
    const { data } = await getFilterConditions()
    if (data) {
      savedFilters.value = data
    }
  } catch {
    // ignore
  }
}

function loadFilter(filter: Api.Stock.FilterConditionSave) {
  try {
    const conditions = JSON.parse(filter.conditions) as Api.Stock.FilterCondition[]
    dynamicFilters.value = conditions.map(c => ({
      field: c.field,
      label: filterTemplates.find(t => t.field === c.field)?.label || c.field,
      operator: c.operator,
      value: Array.isArray(c.value) ? c.value[0] as number : c.value as number,
      value2: Array.isArray(c.value) ? c.value[1] as number : null
    }))
    doScreen()
  } catch {
    message.error('加载筛选条件失败')
  }
}

async function handleDeleteFilter(id: number) {
  try {
    await deleteFilterCondition(id)
    message.success('已删除')
    loadSavedFilters()
  } catch (e: any) {
    message.error(e.message || '删除失败')
  }
}

// 数据同步状态: 同步由后台定时任务驱动, 页面仅展示状态(运行中进度/上次完成时间/失败原因)
const syncStatus = ref<Api.Stock.SyncStatus | null>(null)
const syncRunning = ref(false)
// 轮询频率: 同步任务运行中 3 秒, 空闲 1 分钟
const SYNC_POLL_RUNNING_MS = 3000
const SYNC_POLL_IDLE_MS = 60000
let syncPollTimer: ReturnType<typeof setTimeout> | null = null
let syncPollDisposed = false

const syncProgressText = computed(() => {
  const s = syncStatus.value
  if (!s || s.total <= 0) return ''
  return ` (${s.progress}/${s.total})`
})

function formatSyncTime(t: string) {
  return new Date(t).toLocaleString('zh-CN', { hour12: false })
}

const syncFinishedText = computed(() => {
  const t = syncStatus.value?.finishedAt
  if (!t) return ''
  const d = new Date(t)
  // Go 零值时间(0001-01-01)表示从未同步过
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return ''
  return formatSyncTime(t)
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
        // 同步任务刚结束: 提示结果并刷新筛选数据
        if (data.lastError) {
          message.error(`同步失败: ${data.lastError}`)
        } else {
          message.success('数据同步完成，结果已刷新')
          doScreen()
        }
      }
    }
  } catch {
    // ignore
  }
  // 按当前状态调度下次轮询
  if (!syncPollDisposed) {
    syncPollTimer = setTimeout(refreshSyncStatus, syncRunning.value ? SYNC_POLL_RUNNING_MS : SYNC_POLL_IDLE_MS)
  }
}

onMounted(async () => {
  try {
    const [industriesRes, conceptsRes] = await Promise.all([stockIndustries(), stockConcepts()])
    if (industriesRes.data) {
      industryOptions.value = industriesRes.data
    }
    if (conceptsRes.data) {
      conceptOptions.value = conceptsRes.data.map((c: { name: string; code: string }) => ({ label: c.name, value: c.name }))
    }
  } catch {
    // ignore
  }
  loadSavedFilters()
  loadWatchlistCodes()
  doScreen()
  refreshSyncStatus()
})

onUnmounted(() => {
  stopSyncPolling()
})
</script>

<template>
  <div class="flex h-full">
    <!-- 左侧筛选面板 -->
    <div class="w-72 border-r border-gray-200 p-4 overflow-y-auto flex-shrink-0">
      <div class="mb-4">
        <h3 class="text-lg font-bold mb-2">筛选条件</h3>
        <div class="space-y-3">
          <!-- 名称/代码搜索 -->
          <div>
            <NInput
              v-model:value="filterForm.keyword"
              placeholder="输入股票代码或名称"
              clearable
              @keyup.enter="handleScreen"
            />
          </div>

          <!-- 行业筛选 -->
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">行业</label>
            <NSelect
              v-model:value="filterForm.industries"
              :options="industryOptions.map(i => ({ label: i, value: i }))"
              multiple
              placeholder="选择行业"
              clearable
            />
          </div>

          <!-- 概念筛选 -->
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">概念板块</label>
            <NSelect
              v-model:value="filterForm.conceptNames"
              :options="conceptOptions"
              multiple
              placeholder="选择概念"
              clearable
              filterable
            />
          </div>

          <!-- 证券类型筛选 -->
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">证券类型</label>
            <NSelect
              v-model:value="filterForm.securityTypes"
              :options="securityTypeOptions"
              multiple
              placeholder="不选=全部"
              clearable
            />
          </div>

          <!-- 市场筛选 -->
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">上市板块</label>
            <NSelect
              v-model:value="filterForm.markets"
              :options="marketOptions"
              multiple
              placeholder="选择板块"
              clearable
            />
          </div>

          <!-- 排除ST -->
          <div class="flex items-center">
            <NCheckbox v-model:checked="filterForm.excludeSt">排除ST</NCheckbox>
          </div>
        </div>
      </div>

      <!-- 动态筛选条件 -->
      <div class="mb-4">
        <div class="flex items-center justify-between mb-2">
          <h3 class="text-sm font-bold">指标筛选</h3>
          <NButton size="small" @click="addFilter">+ 添加</NButton>
        </div>
        <div class="space-y-2">
          <div v-for="(filter, index) in dynamicFilters" :key="index" class="flex items-center gap-1">
            <NSelect
              v-model:value="filter.field"
              :options="filterTemplates.map(t => ({ label: t.label, value: t.field }))"
              size="small"
              class="w-24"
              @update:value="(val: string) => {
                filter.label = filterTemplates.find(t => t.field === val)?.label || val
              }"
            />
            <NSelect
              v-model:value="filter.operator"
              :options="operatorOptions"
              size="small"
              class="w-16"
            />
            <NInputNumber
              v-model:value="filter.value"
              size="small"
              class="w-20"
              placeholder="值"
            />
            <NInputNumber
              v-if="filter.operator === 'between'"
              v-model:value="filter.value2"
              size="small"
              class="w-20"
              placeholder="至"
            />
            <NButton size="small" text @click="removeFilter(index)">
              <template #icon><span class="i-material-icons-close text-red-500" /></template>
            </NButton>
          </div>
        </div>
      </div>

      <!-- 操作按钮 -->
      <div class="space-y-2">
        <NButton type="primary" block :loading="loading" @click="handleScreen">开始筛选</NButton>
        <NButton v-if="hasAuth('stock:screen:save')" block @click="handleSaveFilter">保存筛选条件</NButton>
      </div>

      <!-- 已保存的筛选条件 -->
      <div v-if="savedFilters.length > 0" class="mt-4">
        <h3 class="text-sm font-bold mb-2">已保存的条件</h3>
        <div class="space-y-1">
          <div
            v-for="filter in savedFilters"
            :key="filter.id"
            class="flex items-center justify-between p-2 bg-gray-50 rounded cursor-pointer hover:bg-gray-100"
            @click="loadFilter(filter)"
          >
            <span class="text-sm truncate">{{ filter.name }}</span>
            <NButton size="tiny" text @click.stop="handleDeleteFilter(filter.id)">
              <template #icon><span class="i-material-icons-delete text-gray-400" /></template>
            </NButton>
          </div>
        </div>
      </div>
    </div>

    <!-- 右侧结果区域 -->
    <div class="flex-1 flex flex-col overflow-hidden">
      <!-- 结果统计 -->
      <div class="p-3 border-b border-gray-200 flex items-center justify-between">
        <span class="text-sm text-gray-600">
          共筛选出 <strong>{{ total }}</strong> 条证券
        </span>
      </div>

      <!-- 结果表格 -->
      <div class="flex-1 overflow-auto">
        <NDataTable
          :columns="columns"
          :data="results"
          :loading="loading"
          :row-key="screenRowKey"
          :scroll-x="1200"
          size="small"
          striped
          remote
          @update:sorter="handleSorter"
        />
      </div>

      <!-- 底部: 数据同步状态(左) + 分页(右) -->
      <div class="p-3 border-t border-gray-200 flex items-center justify-between gap-4">
        <div
          v-if="hasAuth('stock:sync:execute')"
          class="min-w-0 text-12px leading-5 text-gray-400"
        >
          <template v-if="syncStatus">
            <div v-if="syncRunning" class="flex items-center gap-1 text-blue-500">
              <span class="i-mdi-loading animate-spin flex-shrink-0" />
              <span>{{ syncStatus.task }}同步中{{ syncProgressText }}</span>
            </div>
            <template v-else>
              <div v-if="syncStatus.lastError" class="text-red-500 truncate" :title="syncStatus.lastError">
                上次同步失败: {{ syncStatus.lastError }}
              </div>
              <div v-else-if="syncFinishedText">上次同步完成: {{ syncFinishedText }}</div>
              <div v-else>暂无同步记录</div>
            </template>
          </template>
          <template v-else>状态加载中...</template>
        </div>
        <NPagination
          class="flex-shrink-0"
          :page="currentPage"
          :page-size="pageSize"
          :item-count="total"
          :page-sizes="[20, 50, 100]"
          show-size-picker
          @update:page="handlePageChange"
          @update:page-size="handlePageSizeChange"
        />
      </div>
    </div>

    <WatchlistAddDialog v-model:show="showAddDialog" :code="addCode" :name="addName" @added="handleWatchlistAdded" />
  </div>
</template>
