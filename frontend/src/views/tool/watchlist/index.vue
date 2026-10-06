<script setup lang="ts">
import { computed, h, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NPopconfirm, NTag, useMessage } from 'naive-ui'
import { useAuth } from '@/hooks/business/auth'
import { deleteWatchlist, getWatchlist, stockConcepts, stockIndustries } from '@/service/api'
import { $t } from '@/locales'

defineOptions({ name: 'ToolWatchlist' })

const router = useRouter()
const message = useMessage()
const { hasAuth } = useAuth()

const canEdit = computed(() => hasAuth('stock:watchlist:edit'))

const loading = ref(false)
const list = ref<Api.Stock.WatchlistItem[]>([])
const currentPage = ref(1)
const pageSize = ref(20)

const industryOptions = ref<string[]>([])
const conceptOptions = ref<{ label: string; value: string }[]>([])

function displayCode(code: string) {
  return code.includes('.') ? code.split('.')[1] : code
}

// 筛选条件(与筛选页对齐)
const filterForm = reactive({
  keyword: '',
  groupNames: [] as string[],
  industries: [] as string[],
  conceptNames: [] as string[],
  markets: [] as string[],
  securityTypes: [] as number[],
  excludeSt: true
})

const groupOptions = computed(() =>
  Array.from(new Set(list.value.map(item => item.groupName || $t('page.tool.watchlist.defaultGroup'))))
)

const filterTemplates = [
  { label: $t('page.tool.watchlist.field.peTtm'), field: 'peTtm', type: 'number' },
  { label: $t('page.tool.watchlist.field.pb'), field: 'pb', type: 'number' },
  { label: $t('page.tool.watchlist.field.roe'), field: 'roe', type: 'number' },
  { label: $t('page.tool.watchlist.field.revenueYoy'), field: 'revenueYoy', type: 'number' },
  { label: $t('page.tool.watchlist.field.netProfitYoy'), field: 'netProfitYoy', type: 'number' },
  { label: $t('page.tool.watchlist.field.changePct'), field: 'changePct', type: 'number' },
  { label: $t('page.tool.watchlist.field.turnoverRate'), field: 'turnoverRate', type: 'number' },
  { label: $t('page.tool.watchlist.field.marketCap'), field: 'marketCap', type: 'number' }
]

const operatorOptions = [
  { label: '>', value: 'gt' },
  { label: '>=', value: 'gte' },
  { label: '<', value: 'lt' },
  { label: '<=', value: 'lte' },
  { label: '=', value: 'eq' },
  { label: $t('page.tool.watchlist.between'), value: 'between' }
]

const marketOptions = [
  { label: $t('page.tool.watchlist.marketSh'), value: 'SH' },
  { label: $t('page.tool.watchlist.marketSz'), value: 'SZ' },
  { label: $t('page.tool.watchlist.marketBj'), value: 'BJ' }
]

const securityTypeOptions = [
  { label: $t('page.tool.watchlist.typeStock'), value: 1 },
  { label: $t('page.tool.watchlist.typeIndex'), value: 2 },
  { label: $t('page.tool.watchlist.typeEtf'), value: 5 }
]

const dynamicFilters = ref<
  Array<{ field: string; label: string; operator: string; value: number | null; value2: number | null }>
>([])

function addFilter() {
  dynamicFilters.value.push({ field: 'peTtm', label: 'PE(TTM)', operator: 'lt', value: null, value2: null })
}

function removeFilter(index: number) {
  dynamicFilters.value.splice(index, 1)
}

function resetFilters() {
  filterForm.keyword = ''
  filterForm.groupNames = []
  filterForm.industries = []
  filterForm.conceptNames = []
  filterForm.markets = []
  filterForm.securityTypes = []
  filterForm.excludeSt = true
  dynamicFilters.value = []
}

function matchCondition(
  item: Api.Stock.WatchlistItem,
  f: { field: string; operator: string; value: number | null; value2: number | null }
) {
  const raw = (item as unknown as Record<string, number | null>)[f.field]
  if (raw === null || raw === undefined) return false
  switch (f.operator) {
    case 'gt':
      return f.value !== null && raw > f.value
    case 'gte':
      return f.value !== null && raw >= f.value
    case 'lt':
      return f.value !== null && raw < f.value
    case 'lte':
      return f.value !== null && raw <= f.value
    case 'eq':
      return f.value !== null && raw === f.value
    case 'between':
      return f.value !== null && f.value2 !== null && raw >= f.value && raw <= f.value2
    default:
      return true
  }
}

const filteredList = computed(() => {
  const kw = filterForm.keyword.trim().toLowerCase()
  return list.value.filter(item => {
    if (filterForm.excludeSt && item.isSt) return false
    if (
      filterForm.groupNames.length > 0 &&
      !filterForm.groupNames.includes(item.groupName || $t('page.tool.watchlist.defaultGroup'))
    )
      return false
    if (filterForm.securityTypes.length > 0 && !filterForm.securityTypes.includes(item.type)) return false
    if (filterForm.markets.length > 0 && !filterForm.markets.includes(item.market)) return false
    if (filterForm.industries.length > 0 && !filterForm.industries.includes(item.industry)) return false
    if (filterForm.conceptNames.length > 0) {
      const concepts = item.concepts || []
      if (!concepts.some(c => filterForm.conceptNames.includes(c))) return false
    }
    if (kw) {
      const hit = item.code.toLowerCase().includes(kw) || (item.name || '').toLowerCase().includes(kw)
      if (!hit) return false
    }
    for (const f of dynamicFilters.value) {
      if (f.value === null && f.value2 === null) continue
      if (!matchCondition(item, f)) return false
    }
    return true
  })
})

const pagedList = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return filteredList.value.slice(start, start + pageSize.value)
})

watch(
  [() => filterForm, dynamicFilters],
  () => {
    currentPage.value = 1
  },
  { deep: true }
)

function goToDetail(code: string) {
  router.push({ name: 'tool_stockdetail', query: { code } })
}

function rowKey(row: Api.Stock.WatchlistItem) {
  return row.id
}

const columns = [
  {
    title: $t('page.tool.watchlist.code'),
    key: 'code',
    width: 80,
    fixed: 'left' as const,
    render: (row: Api.Stock.WatchlistItem) =>
      h(
        'a',
        {
          class: 'text-blue-500 cursor-pointer hover:underline',
          onClick: () => goToDetail(row.code)
        },
        displayCode(row.code)
      )
  },
  { title: $t('page.tool.watchlist.name'), key: 'name', width: 100, fixed: 'left' as const },
  {
    title: $t('page.tool.watchlist.group'),
    key: 'groupName',
    width: 100,
    render: (row: Api.Stock.WatchlistItem) =>
      h(
        NTag,
        { size: 'small', bordered: false },
        { default: () => row.groupName || $t('page.tool.watchlist.defaultGroup') }
      )
  },
  {
    title: $t('page.tool.watchlist.type'),
    key: 'type',
    width: 60,
    render: (row: Api.Stock.WatchlistItem) => {
      if (row.type === 2)
        return h(
          NTag,
          { size: 'small', type: 'info', bordered: false },
          { default: () => $t('page.tool.watchlist.typeIndex') }
        )
      if (row.type === 5)
        return h(
          NTag,
          { size: 'small', type: 'warning', bordered: false },
          { default: () => $t('page.tool.watchlist.typeEtf') }
        )
      return $t('page.tool.watchlist.typeStock')
    }
  },
  {
    title: $t('page.tool.watchlist.price'),
    key: 'price',
    width: 80,
    render: (row: Api.Stock.WatchlistItem) => row.price?.toFixed(2) || '-'
  },
  {
    title: $t('page.tool.watchlist.changePct'),
    key: 'changePct',
    width: 80,
    render: (row: Api.Stock.WatchlistItem) => {
      if (row.changePct === null) return '-'
      const color = row.changePct >= 0 ? 'text-red-500' : 'text-green-500'
      return h('span', { class: color }, `${row.changePct >= 0 ? '+' : ''}${row.changePct.toFixed(2)}%`)
    }
  },
  {
    title: $t('page.tool.watchlist.peTtm'),
    key: 'peTtm',
    width: 80,
    render: (row: Api.Stock.WatchlistItem) => row.peTtm?.toFixed(2) || '-'
  },
  {
    title: $t('page.tool.watchlist.pb'),
    key: 'pb',
    width: 60,
    render: (row: Api.Stock.WatchlistItem) => row.pb?.toFixed(2) || '-'
  },
  {
    title: $t('page.tool.watchlist.roe'),
    key: 'roe',
    width: 70,
    render: (row: Api.Stock.WatchlistItem) => row.roe?.toFixed(2) || '-'
  },
  {
    title: $t('page.tool.watchlist.turnoverRate'),
    key: 'turnoverRate',
    width: 80,
    render: (row: Api.Stock.WatchlistItem) => row.turnoverRate?.toFixed(2) || '-'
  },
  {
    title: $t('page.tool.watchlist.marketCap'),
    key: 'marketCap',
    width: 90,
    render: (row: Api.Stock.WatchlistItem) => row.marketCap?.toFixed(2) || '-'
  },
  {
    title: $t('page.tool.watchlist.revenueYoy'),
    key: 'revenueYoy',
    width: 90,
    render: (row: Api.Stock.WatchlistItem) => row.revenueYoy?.toFixed(2) || '-'
  },
  { title: $t('page.tool.watchlist.industry'), key: 'industry', width: 80 },
  {
    title: $t('common.action'),
    key: 'actions',
    width: 100,
    fixed: 'right' as const,
    render: (row: Api.Stock.WatchlistItem) =>
      h('div', { class: 'flex gap-2' }, [
        h(
          NButton,
          { size: 'tiny', text: true, onClick: () => goToDetail(row.code) },
          { default: () => $t('page.tool.watchlist.detail') }
        ),
        canEdit.value
          ? h(
              NPopconfirm,
              { onPositiveClick: () => handleDelete(row.id) },
              {
                trigger: () =>
                  h(NButton, { size: 'tiny', text: true, type: 'error' }, { default: () => $t('common.delete') }),
                default: () => $t('page.tool.watchlist.deleteConfirm')
              }
            )
          : null
      ])
  }
]

async function loadWatchlist() {
  loading.value = true
  try {
    const { data } = await getWatchlist()
    if (data) {
      list.value = data
    }
  } catch (e: any) {
    message.error(e.message || $t('page.tool.watchlist.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function handleDelete(id: number) {
  try {
    await deleteWatchlist(id)
    message.success($t('page.tool.watchlist.deleted'))
    await loadWatchlist()
    // 客户端分页: 删除后若当前页超出最大页, 回退避免空页
    const maxPage = Math.max(1, Math.ceil(filteredList.value.length / pageSize.value))
    if (currentPage.value > maxPage) {
      currentPage.value = maxPage
    }
  } catch (e: any) {
    message.error(e.message || $t('page.tool.watchlist.deleteFailed'))
  }
}

onMounted(async () => {
  try {
    const [industriesRes, conceptsRes] = await Promise.all([stockIndustries(), stockConcepts()])
    if (industriesRes.data) {
      industryOptions.value = industriesRes.data
    }
    if (conceptsRes.data) {
      conceptOptions.value = conceptsRes.data.map((c: { name: string; code: string }) => ({
        label: c.name,
        value: c.name
      }))
    }
  } catch {
    // ignore
  }
  loadWatchlist()
})
</script>

<template>
  <div class="flex h-full">
    <!-- 左侧筛选面板 -->
    <div class="w-72 border-r border-gray-200 p-4 overflow-y-auto flex-shrink-0">
      <h3 class="text-lg font-bold mb-3">{{ $t('page.tool.watchlist.filters') }}</h3>

      <div class="space-y-3">
        <NInput
          v-model:value="filterForm.keyword"
          :placeholder="$t('page.tool.watchlist.keywordPlaceholder')"
          clearable
        />

        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">{{ $t('page.tool.watchlist.group') }}</label>
          <NSelect
            v-model:value="filterForm.groupNames"
            :options="groupOptions.map(g => ({ label: g, value: g }))"
            multiple
            :placeholder="$t('page.tool.watchlist.groupPlaceholder')"
            clearable
          />
        </div>

        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">{{ $t('page.tool.watchlist.industry') }}</label>
          <NSelect
            v-model:value="filterForm.industries"
            :options="industryOptions.map(i => ({ label: i, value: i }))"
            multiple
            :placeholder="$t('page.tool.watchlist.industryPlaceholder')"
            clearable
          />
        </div>

        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">{{ $t('page.tool.watchlist.concept') }}</label>
          <NSelect
            v-model:value="filterForm.conceptNames"
            :options="conceptOptions"
            multiple
            :placeholder="$t('page.tool.watchlist.conceptPlaceholder')"
            clearable
            filterable
          />
        </div>

        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">
            {{ $t('page.tool.watchlist.securityType') }}
          </label>
          <NSelect
            v-model:value="filterForm.securityTypes"
            :options="securityTypeOptions"
            multiple
            :placeholder="$t('page.tool.watchlist.securityTypePlaceholder')"
            clearable
          />
        </div>

        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">{{ $t('page.tool.watchlist.market') }}</label>
          <NSelect
            v-model:value="filterForm.markets"
            :options="marketOptions"
            multiple
            :placeholder="$t('page.tool.watchlist.marketPlaceholder')"
            clearable
          />
        </div>

        <div class="flex items-center">
          <NCheckbox v-model:checked="filterForm.excludeSt">{{ $t('page.tool.watchlist.excludeSt') }}</NCheckbox>
        </div>
      </div>

      <!-- 动态指标筛选 -->
      <div class="mt-4">
        <div class="flex items-center justify-between mb-2">
          <h3 class="text-sm font-bold">{{ $t('page.tool.watchlist.metricFilters') }}</h3>
          <NButton size="small" @click="addFilter">{{ $t('page.tool.watchlist.add') }}</NButton>
        </div>
        <div class="space-y-2">
          <div v-for="(filter, index) in dynamicFilters" :key="index" class="flex items-center gap-1">
            <NSelect
              v-model:value="filter.field"
              :options="filterTemplates.map(t => ({ label: t.label, value: t.field }))"
              size="small"
              class="w-24"
              @update:value="
                (val: string) => {
                  filter.label = filterTemplates.find(t => t.field === val)?.label || val
                }
              "
            />
            <NSelect v-model:value="filter.operator" :options="operatorOptions" size="small" class="w-16" />
            <NInputNumber
              v-model:value="filter.value"
              size="small"
              class="w-20"
              :placeholder="$t('page.tool.watchlist.valuePlaceholder')"
            />
            <NInputNumber
              v-if="filter.operator === 'between'"
              v-model:value="filter.value2"
              size="small"
              class="w-20"
              :placeholder="$t('page.tool.watchlist.toPlaceholder')"
            />
            <NButton size="small" text @click="removeFilter(index)">
              <template #icon><span class="i-material-icons-close text-red-500" /></template>
            </NButton>
          </div>
        </div>
      </div>

      <div class="mt-4 space-y-2">
        <NButton block :loading="loading" @click="loadWatchlist">{{ $t('page.tool.watchlist.refresh') }}</NButton>
        <NButton block @click="resetFilters">{{ $t('page.tool.watchlist.reset') }}</NButton>
      </div>
    </div>

    <!-- 右侧列表区域 -->
    <div class="flex-1 flex flex-col overflow-hidden">
      <div class="p-3 border-b border-gray-200 flex items-center justify-between">
        <span class="text-sm text-gray-600">
          {{ $t('page.tool.watchlist.totalPrefix') }}
          <strong>{{ filteredList.length }}</strong>
          {{ $t('page.tool.watchlist.totalSuffix') }}
        </span>
      </div>

      <div class="flex-1 overflow-auto">
        <NDataTable
          :columns="columns"
          :data="pagedList"
          :loading="loading"
          :row-key="rowKey"
          :scroll-x="1200"
          size="small"
          striped
        >
          <template #empty>
            <NEmpty :description="$t('page.tool.watchlist.empty')" />
          </template>
        </NDataTable>
      </div>

      <div class="p-3 border-t border-gray-200 flex items-center justify-end">
        <NPagination
          v-model:page="currentPage"
          v-model:page-size="pageSize"
          :item-count="filteredList.length"
          :page-sizes="[20, 50, 100]"
          show-size-picker
        />
      </div>
    </div>
  </div>
</template>