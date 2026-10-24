<script setup lang="tsx">
import { h, onMounted, ref, computed } from 'vue';
import { NButton, NCard, NDataTable, NForm, NFormItem, NInput, NInputNumber, NModal, NPopconfirm, NSelect, NTag, useMessage } from 'naive-ui';
import type { DataTableColumns, FormInst, FormRules } from 'naive-ui';
import { $t } from '@/locales';
import { addCutScraps, batchDeleteCutScraps, deleteCutScrap, fetchCutScraps, updateCutScrap } from '@/service/api';

const message = useMessage();

const loading = ref(false);
const data = ref<Api.Cut.CutScrap[]>([]);
// 勾选行 (批量操作)
const checkedKeys = ref<number[]>([]);
const batchDeleting = ref(false);

const searchType = ref<0 | 1 | 2>(0);
const searchName = ref('');
// 长度范围筛选 (后端按一维余料 lengthValue 过滤; 二维无长度属性, 不匹配)
const searchLengthMin = ref<number | null>(null);
const searchLengthMax = ref<number | null>(null);

const typeOptions = [
  { label: $t('page.cut.all'), value: 0 },
  { label: $t('page.cut.scrapType1d'), value: 1 },
  { label: $t('page.cut.scrapType2d'), value: 2 }
];

// 服务端分页 (筛选条件变化时回第一页)
const pagination = ref({
  page: 1,
  pageSize: 10,
  itemCount: 0,
  showSizePicker: true,
  pageSizes: [10, 15, 20],
  onChange: (page: number) => {
    pagination.value.page = page;
    getData();
  },
  onUpdatePageSize: (pageSize: number) => {
    pagination.value.pageSize = pageSize;
    pagination.value.page = 1;
    getData();
  }
});

async function getData() {
  loading.value = true;
  try {
    const { data: res, error } = await fetchCutScraps({
      scrapType: searchType.value,
      current: pagination.value.page,
      size: pagination.value.pageSize,
      name: searchName.value.trim() || undefined,
      lengthMin: searchLengthMin.value ?? undefined,
      lengthMax: searchLengthMax.value ?? undefined
    });
    if (!error && res) {
      data.value = res.records;
      pagination.value.itemCount = res.total;
    }
  } finally {
    loading.value = false;
  }
}

// 筛选条件变化: 回到第一页再查
function applyFilter() {
  pagination.value.page = 1;
  getData();
}

const columns = computed<DataTableColumns<Api.Cut.CutScrap>>(() => [
  { type: 'selection' },
  {
    key: 'scrapType',
    title: $t('page.cut.type'),
    align: 'center',
    width: 100,
    render(row) {
      const label = row.scrapType === 1 ? $t('page.cut.dimLinear') : $t('page.cut.dim2d');
      return h(NTag, { bordered: false, type: row.scrapType === 1 ? 'info' : 'success' }, { default: () => label });
    }
  },
  {
    key: 'materialType',
    title: $t('page.cut.materialType'),
    align: 'center',
    minWidth: 120,
    render: row => row.materialType || '-'
  },
  {
    key: 'label',
    title: $t('page.cut.name'),
    align: 'center',
    minWidth: 120,
    render: row => row.label || '-'
  },
  {
    key: 'lengthValue',
    title: $t('page.cut.length'),
    align: 'center',
    width: 110,
    render: row => (row.scrapType === 1 ? row.lengthValue : '-')
  },
  {
    key: 'size',
    title: $t('page.cut.widthXHeight'),
    align: 'center',
    width: 130,
    render: row => (row.scrapType === 2 ? `${row.widthValue} × ${row.heightValue}` : '-')
  },
  { key: 'quantity', title: $t('page.cut.quantity'), align: 'center', width: 80 },
  {
    key: 'note',
    title: $t('page.cut.scrapNote'),
    align: 'center',
    minWidth: 120,
    render: row => row.note || '-'
  },
  {
    key: 'createdAt',
    title: $t('page.cut.pdCreatedAt'),
    align: 'center',
    minWidth: 180,
    render: row => (row.createdAt ? new Date(row.createdAt).toLocaleString() : '-')
  },
  {
    key: 'operate',
    title: $t('common.action'),
    align: 'center',
    width: 160,
    render(row) {
      return h('div', { class: 'flex gap-2 justify-center' }, [
        h(
          NButton,
          { size: 'small', type: 'primary', quaternary: true, onClick: () => openEdit(row) },
          { default: () => $t('common.edit') }
        ),
        h(
          NPopconfirm,
          { onPositiveClick: () => removeScrap(row) },
          {
            trigger: () => h(NButton, { size: 'small', type: 'error', quaternary: true }, { default: () => $t('common.delete') }),
            default: () => $t('page.cut.scrapDeleteConfirm')
          }
        )
      ]);
    }
  }
]);

// 批量删除勾选的库存条目 (跨页勾选保留; 以服务端实际删除数为准)
async function batchRemove() {
  if (checkedKeys.value.length === 0) return;
  batchDeleting.value = true;
  try {
    const { data: res, error } = await batchDeleteCutScraps({ ids: checkedKeys.value });
    if (error) return;
    message.success($t('page.cut.stockDeletedCount', { count: res?.deleted ?? checkedKeys.value.length }));
    checkedKeys.value = [];
    getData();
  } finally {
    batchDeleting.value = false;
  }
}

// ===== 新增 / 编辑 =====
const modalShow = ref(false);
const editing = ref<Api.Cut.CutScrap | null>(null);
const formRef = ref<FormInst | null>(null);
const form = ref({
  scrapType: 1 as 1 | 2,
  materialType: '',
  label: '',
  lengthValue: null as number | null,
  widthValue: null as number | null,
  heightValue: null as number | null,
  quantity: 1,
  note: ''
});

const rules: FormRules = {
  lengthValue: [
    { required: true, type: 'number', message: $t('page.cut.inputLength'), trigger: ['blur', 'change'] },
    { type: 'number', min: 0.1, message: $t('page.cut.lengthPositive'), trigger: ['blur', 'change'] }
  ],
  widthValue: [
    { required: true, type: 'number', message: $t('page.cut.inputWidth'), trigger: ['blur', 'change'] },
    { type: 'number', min: 0.1, message: $t('page.cut.widthPositive'), trigger: ['blur', 'change'] }
  ],
  heightValue: [
    { required: true, type: 'number', message: $t('page.cut.inputHeight'), trigger: ['blur', 'change'] },
    { type: 'number', min: 0.1, message: $t('page.cut.heightPositive'), trigger: ['blur', 'change'] }
  ],
  quantity: [{ required: true, type: 'number', min: 1, message: $t('page.cut.quantityMinOne'), trigger: ['blur', 'change'] }]
};

function openAdd() {
  editing.value = null;
  form.value = {
    scrapType: searchType.value === 2 ? 2 : 1,
    materialType: '',
    label: '',
    lengthValue: null,
    widthValue: null,
    heightValue: null,
    quantity: 1,
    note: ''
  };
  modalShow.value = true;
}

function openEdit(row: Api.Cut.CutScrap) {
  editing.value = row;
  form.value = {
    scrapType: row.scrapType as 1 | 2,
    materialType: row.materialType ?? '',
    label: row.label ?? '',
    lengthValue: row.lengthValue,
    widthValue: row.widthValue,
    heightValue: row.heightValue,
    quantity: row.quantity,
    note: row.note ?? ''
  };
  modalShow.value = true;
}

const saving = ref(false);

async function submitForm() {
  // 校验失败时 validate 会 reject, 捕获避免 unhandled rejection
  try {
    await formRef.value?.validate();
  } catch {
    return;
  }
  if (saving.value) return;
  saving.value = true;
  try {
    if (editing.value) {
      const { error } = await updateCutScrap(editing.value.id, {
        label: form.value.label.trim(),
        materialType: form.value.materialType.trim(),
        quantity: form.value.quantity,
        note: form.value.note
      });
      if (error) return;
      message.success($t('common.modifySuccess'));
    } else {
      const payload: Api.Cut.AddCutScrapRequest[] = [
        {
          scrapType: form.value.scrapType,
          materialType: form.value.materialType.trim() || undefined,
          label: form.value.label.trim() || undefined,
          lengthValue: form.value.scrapType === 1 ? (form.value.lengthValue ?? undefined) : undefined,
          widthValue: form.value.scrapType === 2 ? (form.value.widthValue ?? undefined) : undefined,
          heightValue: form.value.scrapType === 2 ? (form.value.heightValue ?? undefined) : undefined,
          quantity: form.value.quantity,
          note: form.value.note || undefined
        }
      ];
      const { error } = await addCutScraps(payload);
      if (error) return;
      message.success($t('page.cut.registerSuccess'));
    }
    modalShow.value = false;
    getData();
  } finally {
    saving.value = false;
  }
}

async function removeScrap(row: Api.Cut.CutScrap) {
  const { error } = await deleteCutScrap(row.id);
  if (error) return;
  message.success($t('common.deleteSuccess'));
  getData();
}

onMounted(() => {
  getData();
});
</script>

<template>
  <div class="h-full flex-col flex gap-4 p-4">
    <NCard :bordered="false" shadow="sm" class="flex-1">
      <template #header>
        <div class="flex items-center gap-4">
          <span class="text-18px font-bold">{{ $t('page.cut.stockManage') }}</span>
        </div>
      </template>
      <div class="flex flex-col h-full gap-4">
        <div class="flex justify-between items-center">
          <div class="flex gap-4 items-center">
            <NSelect v-model:value="searchType" :options="typeOptions" style="width: 140px" @update:value="applyFilter" />
            <NInput
              v-model:value="searchName"
              :placeholder="$t('page.cut.filterByNamePh')"
              clearable
              style="width: 180px"
              @keyup.enter="applyFilter"
              @clear="applyFilter"
            />
            <div class="flex items-center gap-1">
              <NInputNumber
                v-model:value="searchLengthMin"
                :min="0"
                :placeholder="$t('page.cut.minLengthPh')"
                clearable
                style="width: 130px"
                @update:value="applyFilter"
                @clear="applyFilter"
              />
              <span class="text-gray-400">~</span>
              <NInputNumber
                v-model:value="searchLengthMax"
                :min="0"
                :placeholder="$t('page.cut.maxLengthPh')"
                clearable
                style="width: 130px"
                @update:value="applyFilter"
                @clear="applyFilter"
              />
            </div>
          </div>
          <div class="flex gap-2 items-center">
            <NPopconfirm @positive-click="batchRemove">
              <template #trigger>
                <NButton
                  type="error"
                  ghost
                  :disabled="checkedKeys.length === 0"
                  :loading="batchDeleting"
                >
                  {{ $t('common.batchDelete') }}{{ checkedKeys.length > 0 ? `(${checkedKeys.length})` : '' }}
                </NButton>
              </template>
              {{ $t('page.cut.stockBatchDeleteConfirm', { count: checkedKeys.length }) }}
            </NPopconfirm>
            <NButton type="primary" @click="openAdd">{{ $t('page.cut.scrapAddNew') }}</NButton>
          </div>
        </div>

        <NDataTable
          v-model:checked-row-keys="checkedKeys"
          :columns="columns"
          :data="data"
          :loading="loading"
          :pagination="pagination"
          :scroll-x="640"
          remote
          :row-key="(row) => row.id"
          flex-height
          class="flex-1"
        />
      </div>
    </NCard>

    <NModal v-model:show="modalShow" preset="card" :title="editing ? $t('page.cut.scrapEditTitle') : $t('page.cut.scrapAddNew')" class="w-480px">
      <NForm ref="formRef" :model="form" :rules="rules" label-placement="left" label-width="80">
        <NFormItem :label="$t('page.cut.type')" :show-feedback="!editing">
          <NSelect
            v-model:value="form.scrapType"
            :options="[{ label: $t('page.cut.scrapType1d'), value: 1 }, { label: $t('page.cut.scrapType2d'), value: 2 }]"
            :disabled="!!editing"
          />
        </NFormItem>
        <NFormItem :label="$t('page.cut.materialType')">
          <NInput v-model:value="form.materialType" :placeholder="$t('page.cut.materialTypePh')" />
        </NFormItem>
        <NFormItem :label="$t('page.cut.name')">
          <NInput v-model:value="form.label" :placeholder="$t('page.cut.labelPh')" />
        </NFormItem>
        <NFormItem v-if="form.scrapType === 1" :label="$t('page.cut.length')" path="lengthValue">
          <NInputNumber v-model:value="form.lengthValue" :min="0.1" class="w-full" placeholder="mm" />
        </NFormItem>
        <template v-else>
          <NFormItem :label="$t('page.cut.scrapWidth')" path="widthValue">
            <NInputNumber v-model:value="form.widthValue" :min="0.1" class="w-full" placeholder="mm" />
          </NFormItem>
          <NFormItem :label="$t('page.cut.scrapHeight')" path="heightValue">
            <NInputNumber v-model:value="form.heightValue" :min="0.1" class="w-full" placeholder="mm" />
          </NFormItem>
        </template>
        <NFormItem :label="$t('page.cut.quantity')" path="quantity">
          <NInputNumber v-model:value="form.quantity" :min="1" class="w-full" />
        </NFormItem>
        <NFormItem :label="$t('page.cut.scrapNote')">
          <NInput v-model:value="form.note" :placeholder="$t('page.cut.optional')" />
        </NFormItem>
      </NForm>
      <template #action>
        <NButton @click="modalShow = false">{{ $t('common.cancel') }}</NButton>
        <NButton type="primary" :loading="saving" @click="submitForm">{{ $t('common.confirm') }}</NButton>
      </template>
    </NModal>
  </div>
</template>

<style scoped></style>
