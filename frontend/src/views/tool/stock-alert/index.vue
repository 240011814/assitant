<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from 'vue'
import { NButton, NPopconfirm, NSwitch, NTag, useMessage } from 'naive-ui'
import type { DataTableColumns, FormInst, FormRules } from 'naive-ui'
import { createStockAlert, deleteStockAlert, getStockAlerts, updateStockAlert } from '@/service/api'
import { $t } from '@/locales'

defineOptions({ name: 'ToolStockAlert' })

const message = useMessage()

const loading = ref(false)
const list = ref<Api.Stock.StockAlert[]>([])

const ruleTypeOptions = computed(() => [
  { label: $t('page.tool.stockAlert.ruleType.price_above'), value: 'price_above' },
  { label: $t('page.tool.stockAlert.ruleType.price_below'), value: 'price_below' },
  { label: $t('page.tool.stockAlert.ruleType.change_pct_above'), value: 'change_pct_above' },
  { label: $t('page.tool.stockAlert.ruleType.change_pct_below'), value: 'change_pct_below' }
])

function ruleTypeText(type: Api.Stock.StockAlertRuleType) {
  return $t(`page.tool.stockAlert.ruleType.${type}`)
}

function formatTime(t: string | null) {
  if (!t) return '-'
  const d = new Date(t)
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return '-'
  return d.toLocaleString('zh-CN', { hour12: false })
}

const columns = computed<DataTableColumns<Api.Stock.StockAlert>>(() => [
  {
    title: $t('page.tool.stockAlert.code'),
    key: 'code',
    width: 110,
    render: row => row.code
  },
  { title: $t('page.tool.stockAlert.name'), key: 'name', width: 100, render: row => row.name || '-' },
  {
    title: $t('page.tool.stockAlert.ruleTypeLabel'),
    key: 'rule_type',
    width: 150,
    render: row => h(NTag, { size: 'small', bordered: false }, { default: () => ruleTypeText(row.rule_type) })
  },
  {
    title: $t('page.tool.stockAlert.threshold'),
    key: 'threshold',
    width: 100,
    render: row => (row.rule_type.startsWith('price_') ? row.threshold.toFixed(2) : `${row.threshold}%`)
  },
  {
    title: $t('page.tool.stockAlert.enabled'),
    key: 'enabled',
    width: 90,
    render: row =>
      h(NSwitch, {
        size: 'small',
        value: row.enabled,
        loading: togglingId.value === row.id,
        'onUpdate:value': (value: boolean) => handleToggleEnabled(row, value)
      })
  },
  {
    title: $t('page.tool.stockAlert.lastTriggeredAt'),
    key: 'last_triggered_at',
    width: 170,
    render: row => {
      if (!row.last_triggered_at) return '-'
      return h('span', null, [
        formatTime(row.last_triggered_at),
        row.last_triggered_value !== null
          ? h('span', { class: 'ml-1 text-gray-400' }, `(${row.last_triggered_value.toFixed(2)})`)
          : null
      ])
    }
  },
  {
    title: $t('page.tool.stockAlert.actions'),
    key: 'actions',
    width: 80,
    render: row =>
      h(
        NPopconfirm,
        { onPositiveClick: () => handleDelete(row) },
        {
          trigger: () =>
            h(NButton, { size: 'small', type: 'error', quaternary: true }, { default: () => $t('common.delete') }),
          default: () => $t('common.confirmDelete')
        }
      )
  }
])

const togglingId = ref<number | null>(null)

async function loadAlerts() {
  loading.value = true
  const { data, error } = await getStockAlerts()
  loading.value = false
  if (error) {
    message.error(error.message || $t('page.tool.stockAlert.loadFailed'))
    return
  }
  list.value = data || []
}

async function handleToggleEnabled(row: Api.Stock.StockAlert, value: boolean) {
  togglingId.value = row.id
  const { error } = await updateStockAlert(row.id, { enabled: value })
  togglingId.value = null
  if (error) {
    message.error(error.message || $t('page.tool.stockAlert.updateFailed'))
    return
  }
  row.enabled = value
}

async function handleDelete(row: Api.Stock.StockAlert) {
  const { error } = await deleteStockAlert(row.id)
  if (error) {
    message.error(error.message || $t('page.tool.stockAlert.deleteFailed'))
    return
  }
  message.success($t('common.deleteSuccess'))
  loadAlerts()
}

// 新建弹窗
const showAddDialog = ref(false)
const submitting = ref(false)
const formRef = ref<FormInst | null>(null)

const addForm = reactive({
  code: '',
  name: '',
  rule_type: 'price_above' as Api.Stock.StockAlertRuleType,
  threshold: null as number | null
})

const rules: FormRules = {
  code: [
    {
      validator: (_rule, value: string) => /^(sh|sz|bj)\.\d{6}$/.test(value),
      message: $t('page.tool.stockAlert.codeInvalid'),
      trigger: 'blur'
    }
  ],
  threshold: [
    {
      validator: (_rule, value: number | null) => value !== null && !Number.isNaN(value),
      message: $t('page.tool.stockAlert.thresholdRequired'),
      trigger: 'blur'
    }
  ]
}

function openAddDialog() {
  addForm.code = ''
  addForm.name = ''
  addForm.rule_type = 'price_above'
  addForm.threshold = null
  showAddDialog.value = true
}

const thresholdHint = computed(() =>
  addForm.rule_type.startsWith('price_')
    ? $t('page.tool.stockAlert.priceHint')
    : $t('page.tool.stockAlert.pctHint')
)

async function handleSubmit() {
  await formRef.value?.validate()
  submitting.value = true
  const { data, error } = await createStockAlert({
    code: addForm.code,
    name: addForm.name || undefined,
    rule_type: addForm.rule_type,
    threshold: addForm.threshold as number
  })
  submitting.value = false
  if (error) {
    message.error(error.message || $t('page.tool.stockAlert.createFailed'))
    return
  }
  message.success($t('page.tool.stockAlert.createSuccess'))
  showAddDialog.value = false
  if (data) {
    list.value = [data, ...list.value]
  } else {
    loadAlerts()
  }
}

onMounted(() => {
  loadAlerts()
})
</script>

<template>
  <div class="h-full flex-col flex gap-4 p-4">
    <NCard :bordered="false" :shadow="'sm'" class="flex-1">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-bold text-16px">{{ $t('page.tool.stockAlert.title') }}</span>
          <div class="flex gap-2">
            <ButtonIcon icon="mdi:refresh" :tooltip-content="$t('common.refresh')" @click="loadAlerts" />
            <NButton type="primary" size="small" @click="openAddDialog">
              <template #icon>
                <IconMdiPlus class="text-icon" />
              </template>
              {{ $t('page.tool.stockAlert.addRule') }}
            </NButton>
          </div>
        </div>
      </template>

      <NEmpty
        v-if="!loading && list.length === 0"
        class="py-16"
        :description="$t('page.tool.stockAlert.emptyTip')"
      >
        <template #extra>
          <NButton type="primary" @click="openAddDialog">{{ $t('page.tool.stockAlert.addFirstRule') }}</NButton>
        </template>
      </NEmpty>

      <NDataTable
        v-else
        :columns="columns"
        :data="list"
        :loading="loading"
        :row-key="row => row.id"
        size="small"
        striped
      />
    </NCard>

    <NModal
      v-model:show="showAddDialog"
      preset="card"
      :title="$t('page.tool.stockAlert.addRule')"
      class="w-460px"
    >
      <NForm ref="formRef" :model="addForm" :rules="rules" label-placement="left" :label-width="90">
        <NFormItem :label="$t('page.tool.stockAlert.code')" path="code">
          <NInput
            v-model:value="addForm.code"
            :placeholder="$t('page.tool.stockAlert.codePlaceholder')"
            clearable
            :input-props="{ autocomplete: 'off' }"
          />
        </NFormItem>
        <NFormItem :label="$t('page.tool.stockAlert.name')" path="name">
          <NInput v-model:value="addForm.name" :placeholder="$t('page.tool.stockAlert.namePlaceholder')" clearable />
        </NFormItem>
        <NFormItem :label="$t('page.tool.stockAlert.ruleTypeLabel')" path="rule_type">
          <NSelect v-model:value="addForm.rule_type" :options="ruleTypeOptions" />
        </NFormItem>
        <NFormItem :label="$t('page.tool.stockAlert.threshold')" path="threshold">
          <div class="w-full flex items-center gap-2">
            <NInputNumber
              v-model:value="addForm.threshold"
              class="flex-1"
              :placeholder="thresholdHint"
              :precision="2"
            />
            <span class="w-24 text-xs text-gray-400">{{ thresholdHint }}</span>
          </div>
        </NFormItem>
      </NForm>
      <template #footer>
        <div class="flex justify-end gap-2">
          <NButton @click="showAddDialog = false">{{ $t('common.cancel') }}</NButton>
          <NButton type="primary" :loading="submitting" @click="handleSubmit">{{ $t('common.confirm') }}</NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>
