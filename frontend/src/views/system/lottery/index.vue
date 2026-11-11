<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from 'vue'
import {
  NButton,
  NCard,
  NDataTable,
  NDatePicker,
  NEmpty,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NModal,
  NPopconfirm,
  NSelect,
  NSpace,
  NSpin,
  NSwitch,
  NTag,
  useMessage
} from 'naive-ui'
import type { DataTableColumns, FormInst, FormRules } from 'naive-ui'
import {
  fetchCreateLotteryActivity,
  fetchCreateLotteryPrize,
  fetchDeleteLotteryActivity,
  fetchDeleteLotteryPrize,
  fetchDeleteLotteryRecord,
  fetchDeleteLotteryRecordsByActivity,
  fetchExportLotteryWinners,
  fetchGetLotteryActivities,
  fetchGetLotteryPrizes,
  fetchGetLotteryRecords,
  fetchGetLotteryWinners,
  fetchSetRecordRedeemed,
  fetchUpdateLotteryActivity,
  fetchUpdateLotteryPrize
} from '@/service/api'
import { useAuth } from '@/hooks/business/auth'
import { useAppStore } from '@/store/modules/app'

const message = useMessage()
const { hasAuth } = useAuth()
const appStore = useAppStore()

// ==================== 状态管理 ====================
const loading = ref(false)
const activities = ref<Api.Lottery.Activity[]>([])
const prizes = ref<Api.Lottery.Prize[]>([])
const records = ref<Api.Lottery.Record[]>([])
const winners = ref<Api.Lottery.Record[]>([])
const recordsTotal = ref(0)
const winnersTotal = ref(0)

const keyword = ref('')
const statusFilter = ref<number | null>(null)

// 弹窗状态
const showActivityModal = ref(false)
const showPrizeModal = ref(false)
const showRecordModal = ref(false)
const showWinnerModal = ref(false)

const editingActivityId = ref<number | null>(null)
const editingPrizeId = ref<number | null>(null)
const currentActivityId = ref<number | null>(null)

// 表单引用
const activityFormRef = ref<FormInst | null>(null)
const prizeFormRef = ref<FormInst | null>(null)

// 分页
const recordPage = ref(1)
const recordPageSize = ref(10)
const winnerPage = ref(1)
const winnerPageSize = ref(10)

// ==================== 表单数据 ====================
const activityForm = reactive({
  name: '',
  description: '',
  startTime: null as number | null,
  endTime: null as number | null,
  drawMode: 0,
  allowAnonymous: true,
  maxParticipants: 0,
  dailyLimit: 0,
  totalLimit: 0
})

const prizeForm = reactive({
  name: '',
  description: '',
  imageUrl: '',
  prizeType: 0,
  prizeLevel: 0,
  prizeValue: 0,
  totalCount: 1,
  probability: 0,
  displayProbability: 0,
  sortOrder: 0
})

// ==================== 选项配置 ====================
const statusOptions = [
  { label: '未开始', value: 0 },
  { label: '进行中', value: 1 },
  { label: '已结束', value: 2 }
]

const drawModeOptions = [
  { label: '转盘模式', value: 0 },
  { label: '原神抽卡模式', value: 1 }
]

const prizeTypeOptions = [
  { label: '实物', value: 0 },
  { label: '虚拟', value: 1 }
]

const prizeLevelOptions = [
  { label: '未设置', value: 0 },
  { label: '特等奖', value: 1 },
  { label: '一等奖', value: 2 },
  { label: '二等奖', value: 3 },
  { label: '三等奖', value: 4 }
]

// ==================== 表单验证规则 ====================
const activityRules: FormRules = {
  name: [{ required: true, message: '请输入活动名称', trigger: ['blur', 'input'] }],
  startTime: [{ type: 'number', required: true, message: '请选择开始时间', trigger: ['change'] }],
  endTime: [{ type: 'number', required: true, message: '请选择结束时间', trigger: ['change'] }]
}

const prizeRules: FormRules = {
  name: [{ required: true, message: '请输入奖品名称', trigger: ['blur'] }],
  totalCount: [{ required: true, type: 'number', min: 1, message: '奖品数量至少为1', trigger: ['blur', 'change'] }],
  probability: [{ required: true, type: 'number', min: 0, max: 1, message: '概率范围 0-1', trigger: ['blur', 'change'] }]
}

// ==================== 状态标签 ====================
const statusTagType = (status: number) => {
  const map: Record<number, 'error' | 'warning' | 'info' | 'success'> = {
    0: 'info',
    1: 'success',
    2: 'warning'
  }
  return map[status] || 'info'
}

const statusLabel = (status: number) => {
  const map: Record<number, string> = {
    0: '未开始',
    1: '进行中',
    2: '已结束'
  }
  return map[status] || '未知'
}

const drawModeLabel = (mode: number) => {
  const map: Record<number, string> = {
    0: '转盘',
    1: '抽卡'
  }
  return map[mode] || '转盘'
}

const prizeLevelLabel = (level: number) => {
  const map: Record<number, string> = {
    0: '未设置',
    1: '特等奖',
    2: '一等奖',
    3: '二等奖',
    4: '三等奖'
  }
  return map[level] || '未设置'
}

const prizeLevelType = (level: number) => {
  const map: Record<number, 'error' | 'warning' | 'info' | 'success' | 'default'> = {
    0: 'default',
    1: 'error',
    2: 'warning',
    3: 'info',
    4: 'success'
  }
  return map[level] || 'default'
}

// ==================== 活动表格列 ====================
const activityColumns = computed<DataTableColumns<Api.Lottery.Activity>>(() => [
  { title: 'ID', key: 'id', width: 80 },
  { title: '活动名称', key: 'name', minWidth: 150 },
  {
    title: '状态',
    key: 'status',
    width: 100,
    render(row) {
      return h(
        NTag,
        { type: statusTagType(row.status), bordered: false, size: 'small' },
        { default: () => statusLabel(row.status) }
      )
    }
  },
  {
    title: '抽奖模式',
    key: 'drawMode',
    width: 100,
    render(row) {
      return h(
        NTag,
        { type: row.drawMode === 1 ? 'warning' : 'info', bordered: false, size: 'small' },
        { default: () => drawModeLabel(row.drawMode) }
      )
    }
  },
  {
    title: '匿名抽奖',
    key: 'allowAnonymous',
    width: 90,
    render(row) {
      return h(
        NTag,
        { type: row.allowAnonymous === false ? 'error' : 'success', bordered: false, size: 'small' },
        { default: () => (row.allowAnonymous === false ? '需登录' : '允许') }
      )
    }
  },
  {
    title: '时间',
    key: 'time',
    minWidth: 200,
    render(row) {
      return h('div', { class: 'text-xs' }, [
        h('div', null, `开始: ${new Date(row.startTime).toLocaleString()}`),
        h('div', null, `结束: ${new Date(row.endTime).toLocaleString()}`)
      ])
    }
  },
  {
    title: '参与人数',
    key: 'participants',
    width: 120,
    render(row) {
      const max = row.maxParticipants || '∞'
      return `${row.currentParticipants}/${max}`
    }
  },
  {
    title: '操作',
    key: 'actions',
    width: 350,
    render(row) {
      return h(NSpace, { size: 'small' }, {
        default: () => [
          h(NButton, { size: 'small', type: 'primary', ghost: true, onClick: () => copyShareLink(row.id) }, { default: () => '分享' }),
          hasAuth('lottery:prize:view')
            ? h(NButton, { size: 'small', type: 'info', ghost: true, onClick: () => openPrizeManager(row) }, { default: () => '奖品' })
            : null,
          hasAuth('lottery:record:view')
            ? h(NButton, { size: 'small', type: 'success', ghost: true, onClick: () => openRecordModal(row) }, { default: () => '记录' })
            : null,
          hasAuth('lottery:winner:view')
            ? h(NButton, { size: 'small', type: 'warning', ghost: true, onClick: () => openWinnerModal(row) }, { default: () => '中奖' })
            : null,
          hasAuth('lottery:activity:update')
            ? h(NButton, { size: 'small', onClick: () => openEditActivity(row) }, { default: () => '编辑' })
            : null,
          hasAuth('lottery:activity:delete')
            ? h(NPopconfirm, { onPositiveClick: () => handleDeleteActivity(row.id) }, {
                default: () => '确定删除此活动？',
                trigger: () => h(NButton, { size: 'small', type: 'error', ghost: true }, { default: () => '删除' })
              })
            : null
        ].filter(Boolean)
      })
    }
  }
])

// ==================== 奖品表格列 ====================
const prizeColumns = computed<DataTableColumns<Api.Lottery.Prize>>(() => [
  { title: '名称', key: 'name', width: 120, ellipsis: { tooltip: true }, fixed: 'left' },
  {
    title: '等级',
    key: 'prizeLevel',
    width: 80,
    render(row) {
      if (row.prizeLevel === 0) return '-'
      return h(
        NTag,
        { type: prizeLevelType(row.prizeLevel), bordered: false, size: 'small' },
        { default: () => prizeLevelLabel(row.prizeLevel) }
      )
    }
  },
  { title: '价值', key: 'prizeValue', width: 70 },
  {
    title: '数量',
    key: 'count',
    width: 70,
    render(row) {
      return h('span', { class: row.remainingCount === 0 ? 'text-red-500' : '' }, `${row.remainingCount}/${row.totalCount}`)
    }
  },
  {
    title: '实际概率',
    key: 'probability',
    width: 80,
    render(row) {
      return `${(row.probability * 100).toFixed(1)}%`
    }
  },
  {
    title: '展示概率',
    key: 'displayProbability',
    width: 80,
    render(row) {
      return `${(row.displayProbability * 100).toFixed(1)}%`
    }
  },
  {
    title: '操作',
    key: 'actions',
    width: 120,
    fixed: 'right',
    render(row) {
      return h(NSpace, { size: 'small' }, {
        default: () => [
          hasAuth('lottery:prize:update')
            ? h(NButton, { size: 'small', type: 'primary', ghost: true, onClick: () => openEditPrize(row) }, { default: () => '编辑' })
            : null,
          hasAuth('lottery:prize:delete')
            ? h(NPopconfirm, { onPositiveClick: () => handleDeletePrize(row.id) }, {
                default: () => '确定删除此奖品？',
                trigger: () => h(NButton, { size: 'small', type: 'error', ghost: true }, { default: () => '删除' })
              })
            : null
        ].filter(Boolean)
      })
    }
  }
])

// ==================== 记录表格列 ====================
const recordColumns = computed<DataTableColumns<Api.Lottery.Record>>(() => [
  { title: 'ID', key: 'id', width: 80 },
  { title: '用户ID', key: 'userId', width: 100 },
  { title: '姓名', key: 'userName', width: 100 },
  { title: '奖品', key: 'prizeName', minWidth: 120 },
  {
    title: '是否中奖',
    key: 'isWinner',
    width: 100,
    render(row) {
      return h(
        NTag,
        { type: row.isWinner ? 'success' : 'default', bordered: false, size: 'small' },
        { default: () => (row.isWinner ? '中奖' : '未中奖') }
      )
    }
  },
  {
    title: '核销',
    key: 'redeemed',
    width: 120,
    render(row) {
      if (!row.isWinner) return '-'
      return h(NSpace, { size: 4, wrap: false, align: 'center' }, {
        default: () => [
          h(
            NTag,
            { type: row.redeemed ? 'success' : 'warning', bordered: false, size: 'small' },
            { default: () => (row.redeemed ? '已核销' : '未核销') }
          ),
          hasAuth('lottery:record:delete')
            ? h(
                NButton,
                { size: 'tiny', text: true, type: 'primary', onClick: () => handleRedeemToggle(row) },
                { default: () => (row.redeemed ? '撤销' : '核销') }
              )
            : null
        ].filter(Boolean)
      })
    }
  },
  {
    title: '时间',
    key: 'createdAt',
    width: 180,
    render(row) {
      return new Date(row.createdAt).toLocaleString()
    }
  },
  {
    title: '操作',
    key: 'actions',
    width: 100,
    render(row) {
      return h(NSpace, { size: 'small' }, {
        default: () => [
          hasAuth('lottery:record:delete')
            ? h(NPopconfirm, { onPositiveClick: () => handleDeleteRecord(row.id) }, {
                default: () => '确定删除此记录？',
                trigger: () => h(NButton, { size: 'small', type: 'error', ghost: true }, { default: () => '删除' })
              })
            : null
        ].filter(Boolean)
      })
    }
  }
])

// ==================== 数据加载 ====================
// 奖品实际概率总和 (编辑中的奖品按表单值实时替换): 后端创建/更新奖品时校验总和 ≤ 1
const prizeProbSum = computed(() => {
  const saved = prizes.value.reduce((sum, p) => sum + (p.id === editingPrizeId.value ? 0 : p.probability), 0)
  return saved + (prizeForm.probability || 0)
})

async function loadActivities() {
  loading.value = true
  const { data, error } = await fetchGetLotteryActivities({
    keyword: keyword.value,
    status: statusFilter.value ?? undefined
  })
  if (!error) {
    activities.value = data
  }
  loading.value = false
}

async function loadPrizes(activityId: number) {
  const { data, error } = await fetchGetLotteryPrizes(activityId)
  if (!error) {
    prizes.value = data
  }
}

async function loadRecords() {
  if (!currentActivityId.value) return
  const { data, error } = await fetchGetLotteryRecords({
    activityId: currentActivityId.value,
    page: recordPage.value,
    pageSize: recordPageSize.value
  })
  if (!error) {
    records.value = data.list
    recordsTotal.value = data.total
  }
}

async function loadWinners() {
  if (!currentActivityId.value) return
  const { data, error } = await fetchGetLotteryWinners({
    activityId: currentActivityId.value,
    page: winnerPage.value,
    pageSize: winnerPageSize.value
  })
  if (!error) {
    winners.value = data.list
    winnersTotal.value = data.total
  }
}

// ==================== 活动操作 ====================
function resetActivityForm() {
  editingActivityId.value = null
  Object.assign(activityForm, {
    name: '',
    description: '',
    startTime: null,
    endTime: null,
    drawMode: 0,
    allowAnonymous: true,
    maxParticipants: 0,
    dailyLimit: 0,
    totalLimit: 0
  })
}

function openCreateActivity() {
  resetActivityForm()
  showActivityModal.value = true
}

function openEditActivity(row: Api.Lottery.Activity) {
  editingActivityId.value = row.id
  Object.assign(activityForm, {
    name: row.name,
    description: row.description,
    startTime: new Date(row.startTime).getTime(),
    endTime: new Date(row.endTime).getTime(),
    drawMode: row.drawMode || 0,
    // 存量数据缺省视为允许 (迁移默认 1)
    allowAnonymous: row.allowAnonymous !== false,
    maxParticipants: row.maxParticipants,
    dailyLimit: row.dailyLimit,
    totalLimit: row.totalLimit
  })
  showActivityModal.value = true
}

async function handleActivitySubmit() {
  try {
    await activityFormRef.value?.validate()
  } catch {
    return
  }
  const payload = {
    name: activityForm.name,
    description: activityForm.description,
    startTime: new Date(activityForm.startTime!).toISOString(),
    endTime: new Date(activityForm.endTime!).toISOString(),
    drawMode: activityForm.drawMode,
    allowAnonymous: activityForm.allowAnonymous,
    maxParticipants: activityForm.maxParticipants,
    dailyLimit: activityForm.dailyLimit,
    totalLimit: activityForm.totalLimit
  }

  const result = editingActivityId.value === null
    ? await fetchCreateLotteryActivity(payload)
    : await fetchUpdateLotteryActivity(editingActivityId.value, payload)

  if (!result.error) {
    message.success(editingActivityId.value === null ? '创建成功' : '更新成功')
    showActivityModal.value = false
    loadActivities()
  }
}

async function handleDeleteActivity(id: number) {
  const { error } = await fetchDeleteLotteryActivity(id)
  if (!error) {
    message.success('删除成功')
    loadActivities()
  }
}

// ==================== 奖品操作 ====================
function resetPrizeForm() {
  editingPrizeId.value = null
  Object.assign(prizeForm, {
    name: '',
    description: '',
    imageUrl: '',
    prizeType: 0,
    prizeLevel: 0,
    prizeValue: 0,
    totalCount: 1,
    probability: 0,
    displayProbability: 0,
    sortOrder: 0
  })
}

function openPrizeManager(row: Api.Lottery.Activity) {
  currentActivityId.value = row.id
  resetPrizeForm()
  loadPrizes(row.id)
  showPrizeModal.value = true
}

function openEditPrize(row: Api.Lottery.Prize) {
  editingPrizeId.value = row.id
  Object.assign(prizeForm, {
    name: row.name,
    description: row.description,
    imageUrl: row.imageUrl,
    prizeType: row.prizeType,
    prizeLevel: row.prizeLevel || 0,
    prizeValue: row.prizeValue,
    totalCount: row.totalCount,
    probability: row.probability,
    displayProbability: row.displayProbability,
    sortOrder: row.sortOrder
  })
}

async function handlePrizeSubmit() {
  await prizeFormRef.value?.validate()
  if (!currentActivityId.value) return

  const payload = {
    name: prizeForm.name,
    description: prizeForm.description,
    imageUrl: prizeForm.imageUrl,
    prizeType: prizeForm.prizeType,
    prizeLevel: prizeForm.prizeLevel,
    prizeValue: prizeForm.prizeValue,
    totalCount: prizeForm.totalCount,
    probability: prizeForm.probability,
    displayProbability: prizeForm.displayProbability,
    sortOrder: prizeForm.sortOrder
  }

  const result = editingPrizeId.value === null
    ? await fetchCreateLotteryPrize(currentActivityId.value, payload)
    : await fetchUpdateLotteryPrize(editingPrizeId.value, payload)

  if (!result.error) {
    message.success(editingPrizeId.value === null ? '创建成功' : '更新成功')
    resetPrizeForm()
    loadPrizes(currentActivityId.value)
  }
}

async function handleDeletePrize(id: number) {
  const { error } = await fetchDeleteLotteryPrize(id)
  if (!error) {
    message.success('删除成功')
    if (currentActivityId.value) {
      loadPrizes(currentActivityId.value)
    }
  }
}

// ==================== 记录操作 ====================
async function handleDeleteRecord(id: number) {
  const { error } = await fetchDeleteLotteryRecord(id)
  if (!error) {
    message.success('删除成功')
    loadRecords()
  }
}

// 核销/撤销核销 (取反); 中奖名单与记录列表共用, 两个列表都要刷新
async function handleRedeemToggle(row: Api.Lottery.Record) {
  const { data, error } = await fetchSetRecordRedeemed(row.id)
  if (!error) {
    message.success(data.redeemed ? '已标记核销' : '已撤销核销')
    row.redeemed = data.redeemed
    loadRecords()
    if (showWinnerModal.value) loadWinners()
  }
}

// 导出中奖名单 CSV
async function handleExportWinners() {
  if (!currentActivityId.value) return
  try {
    await fetchExportLotteryWinners(currentActivityId.value)
  } catch {
    message.error('导出失败')
  }
}

async function handleDeleteAllRecords() {
  if (!currentActivityId.value) return
  const { error } = await fetchDeleteLotteryRecordsByActivity(currentActivityId.value)
  if (!error) {
    message.success('清空成功')
    loadRecords()
  }
}

// ==================== 分享链接 ====================
function copyShareLink(activityId: number) {
  const isHashMode = import.meta.env.VITE_ROUTER_HISTORY_MODE === 'hash'
  const basePath = isHashMode ? '/#/' : '/'
  const url = `${window.location.origin}${basePath}lottery/${activityId}`
  navigator.clipboard.writeText(url).then(() => {
    message.success('抽奖链接已复制')
  }).catch(() => {
    message.error('复制失败')
  })
}

// ==================== 记录和中奖 ====================
function openRecordModal(row: Api.Lottery.Activity) {
  currentActivityId.value = row.id
  recordPage.value = 1
  loadRecords()
  showRecordModal.value = true
}

function openWinnerModal(row: Api.Lottery.Activity) {
  currentActivityId.value = row.id
  winnerPage.value = 1
  loadWinners()
  showWinnerModal.value = true
}

// ==================== 初始化 ====================
onMounted(() => {
  loadActivities()
})
</script>

<template>
  <div class="h-full flex-col flex gap-4" :class="appStore.isMobile ? 'p-2' : 'p-4'">
    <!-- 主内容 -->
    <NCard :bordered="false" :shadow="appStore.isMobile ? false : 'sm'" class="flex-1">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-bold" :class="appStore.isMobile ? 'text-16px' : 'text-18px'">
            抽奖管理
          </span>
          <NButton v-if="hasAuth('lottery:activity:create')" type="primary" @click="openCreateActivity">
            <template #icon>
              <SvgIcon icon="mdi:plus" />
            </template>
            <span v-if="!appStore.isMobile">创建活动</span>
          </NButton>
        </div>
      </template>

      <div class="flex flex-col h-full gap-4">
        <!-- 搜索栏 -->
        <template v-if="!appStore.isMobile">
          <div class="flex justify-between items-center">
            <div class="flex gap-3 items-center">
              <NInput
                v-model:value="keyword"
                clearable
                placeholder="搜索活动名称"
                style="width: 260px"
                @keyup.enter="loadActivities"
              />
              <NSelect
                v-model:value="statusFilter"
                clearable
                :options="statusOptions"
                placeholder="状态筛选"
                style="width: 150px"
              />
              <NButton type="primary" @click="loadActivities">
                <template #icon>
                  <SvgIcon icon="mdi:magnify" />
                </template>
                搜索
              </NButton>
            </div>
            <NButton quaternary size="small" @click="loadActivities">
              <template #icon>
                <SvgIcon icon="mdi:refresh" />
              </template>
            </NButton>
          </div>
        </template>
        <template v-else>
          <div class="flex flex-col gap-2">
            <div class="flex gap-2">
              <NInput
                v-model:value="keyword"
                clearable
                class="flex-1"
                placeholder="搜索活动名称"
                @keyup.enter="loadActivities"
              />
              <NButton type="primary" @click="loadActivities">
                <template #icon>
                  <SvgIcon icon="mdi:magnify" />
                </template>
              </NButton>
            </div>
            <NSelect
              v-model:value="statusFilter"
              clearable
              :options="statusOptions"
              placeholder="状态筛选"
              @update:value="loadActivities"
            />
          </div>
        </template>

        <!-- 活动列表 -->
        <template v-if="!appStore.isMobile">
          <NDataTable
            :columns="activityColumns"
            :data="activities"
            :loading="loading"
            :pagination="{ pageSize: 10, showSizePicker: true, pageSizes: [10, 20, 50] }"
            :row-key="row => row.id"
            flex-height
            class="flex-1"
          />
        </template>

        <!-- 移动端卡片列表 -->
        <template v-else>
          <NSpin v-if="loading" class="flex justify-center py-8" />
          <div v-else-if="activities.length" class="flex flex-col gap-3">
            <NCard v-for="row in activities" :key="row.id" size="small" :bordered="true">
              <div class="flex items-start justify-between">
                <div class="flex flex-col">
                  <span class="font-medium text-gray-800 dark:text-gray-200">{{ row.name }}</span>
                  <span class="text-xs text-gray-400">
                    {{ new Date(row.startTime).toLocaleDateString() }} - {{ new Date(row.endTime).toLocaleDateString() }}
                  </span>
                </div>
                <NTag :type="statusTagType(row.status)" bordered round size="small">
                  {{ statusLabel(row.status) }}
                </NTag>
              </div>
              <div class="flex items-center justify-between mt-3 pt-3 border-t border-gray-100 dark:border-gray-700">
                <span class="text-xs text-gray-400">
                  参与: {{ row.currentParticipants }}/{{ row.maxParticipants || '∞' }}
                </span>
                <div class="flex gap-1">
                  <NButton
                    v-if="hasAuth('lottery:prize:view')"
                    size="tiny"
                    type="info"
                    quaternary
                    @click="openPrizeManager(row)"
                  >
                    奖品
                  </NButton>
                  <NButton
                    v-if="hasAuth('lottery:activity:update')"
                    size="tiny"
                    type="primary"
                    quaternary
                    @click="openEditActivity(row)"
                  >
                    编辑
                  </NButton>
                  <NPopconfirm v-if="hasAuth('lottery:activity:delete')" @positive-click="handleDeleteActivity(row.id)">
                    <template #trigger>
                      <NButton size="tiny" type="error" quaternary>
                        删除
                      </NButton>
                    </template>
                    确定删除此活动？
                  </NPopconfirm>
                </div>
              </div>
            </NCard>
          </div>
          <NEmpty v-else class="py-8" />
        </template>
      </div>
    </NCard>

    <!-- 创建/编辑活动弹窗 -->
    <NModal
      v-model:show="showActivityModal"
      preset="card"
      :title="editingActivityId === null ? '创建抽奖活动' : '编辑抽奖活动'"
      :style="{ width: appStore.isMobile ? '95vw' : '600px' }"
      :segmented="{ content: true, footer: true }"
    >
      <NForm
        ref="activityFormRef"
        :model="activityForm"
        :rules="activityRules"
        label-placement="left"
        :label-width="appStore.isMobile ? '80' : '100'"
      >
        <NFormItem label="活动名称" path="name">
          <NInput v-model:value="activityForm.name" placeholder="请输入活动名称" />
        </NFormItem>
        <NFormItem label="活动描述" path="description">
          <NInput v-model:value="activityForm.description" type="textarea" placeholder="请输入活动描述" />
        </NFormItem>
        <NFormItem label="开始时间" path="startTime">
          <NDatePicker
            v-model:value="activityForm.startTime"
            type="datetime"
            placeholder="请选择开始时间"
            style="width: 100%"
            @update:value="activityFormRef?.validate()"
          />
        </NFormItem>
        <NFormItem label="结束时间" path="endTime">
          <NDatePicker
            v-model:value="activityForm.endTime"
            type="datetime"
            placeholder="请选择结束时间"
            style="width: 100%"
            @update:value="activityFormRef?.validate()"
          />
        </NFormItem>
        <NFormItem label="抽奖模式" path="drawMode">
          <NSelect v-model:value="activityForm.drawMode" :options="drawModeOptions" />
        </NFormItem>
        <NFormItem label="允许匿名抽奖" path="allowAnonymous">
          <div class="flex items-center gap-2">
            <NSwitch v-model:value="activityForm.allowAnonymous" />
            <span class="text-xs text-gray-400">
              {{ activityForm.allowAnonymous ? '游客可输入姓名直接参与' : '需登录后才能参与, 记录强制使用账号用户名' }}
            </span>
          </div>
        </NFormItem>
        <NFormItem label="最大参与人数" path="maxParticipants">
          <NInputNumber v-model:value="activityForm.maxParticipants" :min="0" placeholder="0表示不限制" style="width: 100%" />
        </NFormItem>
        <NFormItem label="每日限制" path="dailyLimit">
          <NInputNumber v-model:value="activityForm.dailyLimit" :min="0" placeholder="0表示不限制" style="width: 100%" />
        </NFormItem>
        <NFormItem label="总限制" path="totalLimit">
          <NInputNumber v-model:value="activityForm.totalLimit" :min="0" placeholder="0表示不限制" style="width: 100%" />
        </NFormItem>
      </NForm>
      <template #footer>
        <div class="flex justify-end gap-3">
          <NButton @click="showActivityModal = false">取消</NButton>
          <NButton type="primary" @click="handleActivitySubmit">确定</NButton>
        </div>
      </template>
    </NModal>

    <!-- 奖品管理弹窗 -->
    <NModal
      v-model:show="showPrizeModal"
      preset="card"
      title="奖品管理"
      :style="{ width: appStore.isMobile ? '95vw' : '900px' }"
      :segmented="{ content: true, footer: true }"
    >
      <div class="prize-manager">
        <!-- 奖品表单 - 折叠面板 -->
        <NCard size="small" :bordered="true" class="prize-form-card">
          <template #header>
            <div class="flex items-center justify-between">
              <span class="font-bold">{{ editingPrizeId === null ? '添加奖品' : '编辑奖品' }}</span>
              <NButton v-if="editingPrizeId !== null" text type="primary" size="small" @click="resetPrizeForm">
                取消编辑
              </NButton>
            </div>
          </template>
          <NForm
            ref="prizeFormRef"
            :model="prizeForm"
            :rules="prizeRules"
            label-placement="left"
            :label-width="80"
          >
            <div class="prize-form-grid">
              <NFormItem label="名称" path="name" class="form-item-full">
                <NInput v-model:value="prizeForm.name" placeholder="请输入奖品名称" />
              </NFormItem>
              <NFormItem label="类型" path="prizeType">
                <NSelect v-model:value="prizeForm.prizeType" :options="prizeTypeOptions" />
              </NFormItem>
              <NFormItem label="等级" path="prizeLevel">
                <NSelect v-model:value="prizeForm.prizeLevel" :options="prizeLevelOptions" />
              </NFormItem>
              <NFormItem label="价值" path="prizeValue">
                <NInputNumber v-model:value="prizeForm.prizeValue" :min="0" style="width: 100%" />
              </NFormItem>
              <NFormItem label="数量" path="totalCount">
                <NInputNumber v-model:value="prizeForm.totalCount" :min="1" style="width: 100%" />
              </NFormItem>
              <NFormItem label="实际概率" path="probability">
                <NInputNumber v-model:value="prizeForm.probability" :min="0" :max="1" :step="0.01" style="width: 100%" />
              </NFormItem>
              <NFormItem label="展示概率" path="displayProbability">
                <NInputNumber v-model:value="prizeForm.displayProbability" :min="0" :max="1" :step="0.01" style="width: 100%" />
              </NFormItem>
              <NFormItem label="图片URL" path="imageUrl" class="form-item-full">
                <NInput v-model:value="prizeForm.imageUrl" placeholder="请输入奖品图片URL（可选）" />
              </NFormItem>
              <NFormItem label="描述" path="description" class="form-item-full">
                <NInput v-model:value="prizeForm.description" type="textarea" placeholder="请输入奖品描述（可选）" :rows="2" />
              </NFormItem>
            </div>
            <div class="flex justify-end mt-2">
              <NButton type="primary" @click="handlePrizeSubmit">
                {{ editingPrizeId === null ? '添加奖品' : '保存修改' }}
              </NButton>
            </div>
          </NForm>
        </NCard>

        <!-- 奖品列表 -->
        <div class="text-xs" :class="prizeProbSum > 1 ? 'text-red-500 font-bold' : 'text-gray-400'">
          实际概率总和: {{ (prizeProbSum * 100).toFixed(1) }}% / 100%
          <span v-if="prizeProbSum > 1">— 已超上限, 保存奖品会被后端拒绝</span>
          <span v-else-if="prizeProbSum < 1">— 未分配的概率将落入"谢谢惠顾"</span>
        </div>
        <NDataTable
          :columns="prizeColumns"
          :data="prizes"
          :pagination="false"
          :row-key="row => row.id"
          size="small"
          :scroll-x="700"
        />
      </div>
    </NModal>

    <!-- 抽奖记录弹窗 -->
    <NModal
      v-model:show="showRecordModal"
      preset="card"
      title="抽奖记录"
      :style="{ width: appStore.isMobile ? '95vw' : '800px' }"
      :segmented="{ content: true, footer: true }"
    >
      <template #header-extra>
        <NPopconfirm v-if="hasAuth('lottery:record:delete')" @positive-click="handleDeleteAllRecords">
          <template #trigger>
            <NButton size="small" type="error" ghost>清空记录</NButton>
          </template>
          确定清空该活动的所有抽奖记录？
        </NPopconfirm>
      </template>
      <NDataTable
        :columns="recordColumns"
        :data="records"
        :pagination="{
          page: recordPage,
          pageSize: recordPageSize,
          itemCount: recordsTotal,
          showSizePicker: true,
          pageSizes: [10, 20, 50],
          onUpdatePage: (page) => { recordPage = page; loadRecords() },
          onUpdatePageSize: (pageSize) => { recordPageSize = pageSize; recordPage = 1; loadRecords() }
        }"
        :row-key="row => row.id"
      />
    </NModal>

    <!-- 中奖名单弹窗 -->
    <NModal
      v-model:show="showWinnerModal"
      preset="card"
      title="中奖名单"
      :style="{ width: appStore.isMobile ? '95vw' : '800px' }"
      :segmented="{ content: true, footer: true }"
    >
      <template #header-extra>
        <NButton size="small" type="primary" ghost @click="handleExportWinners">导出 CSV</NButton>
      </template>
      <NDataTable
        :columns="recordColumns"
        :data="winners"
        :pagination="{
          page: winnerPage,
          pageSize: winnerPageSize,
          itemCount: winnersTotal,
          showSizePicker: true,
          pageSizes: [10, 20, 50],
          onUpdatePage: (page) => { winnerPage = page; loadWinners() },
          onUpdatePageSize: (pageSize) => { winnerPageSize = pageSize; winnerPage = 1; loadWinners() }
        }"
        :row-key="row => row.id"
      />
    </NModal>
  </div>
</template>

<style scoped lang="scss">
.prize-manager {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.prize-form-card {
  background: #fafbfc;
}

.prize-form-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 0 16px;

  .form-item-full {
    grid-column: 1 / -1;
  }
}

@media (max-width: 640px) {
  .prize-form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
