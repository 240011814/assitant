<script setup lang="tsx">
import { h, onMounted, ref, computed } from 'vue';
import { NButton, NCard, NDataTable, NForm, NFormItem, NInput, NInputNumber, NModal, NPopconfirm, NSelect, NTag, useMessage } from 'naive-ui';
import type { DataTableColumns, FormInst, FormRules } from 'naive-ui';
import { addCutScraps, deleteCutScrap, fetchCutScraps, updateCutScrap } from '@/service/api';

const message = useMessage();

const loading = ref(false);
const data = ref<Api.Cut.CutScrap[]>([]);

const searchType = ref<0 | 1 | 2>(0);
const searchLabel = ref('');

const typeOptions = [
  { label: '全部', value: 0 },
  { label: '一维余料', value: 1 },
  { label: '二维余料', value: 2 }
];

const filteredData = computed(() =>
  data.value.filter(row => {
    if (searchType.value > 0 && row.scrapType !== searchType.value) return false;
    if (searchLabel.value && !(row.label ?? '').includes(searchLabel.value)) return false;
    return true;
  })
);

const columns = computed<DataTableColumns<Api.Cut.CutScrap>>(() => [
  {
    key: 'scrapType',
    title: '类型',
    align: 'center',
    width: 100,
    render(row) {
      const label = row.scrapType === 1 ? '一维' : '二维';
      return h(NTag, { bordered: false, type: row.scrapType === 1 ? 'info' : 'success' }, { default: () => label });
    }
  },
  {
    key: 'label',
    title: '名称',
    align: 'center',
    minWidth: 120,
    render: row => row.label || '-'
  },
  {
    key: 'lengthValue',
    title: '长度',
    align: 'center',
    width: 110,
    render: row => (row.scrapType === 1 ? row.lengthValue : '-')
  },
  {
    key: 'size',
    title: '宽 × 高',
    align: 'center',
    width: 130,
    render: row => (row.scrapType === 2 ? `${row.widthValue} × ${row.heightValue}` : '-')
  },
  { key: 'quantity', title: '数量', align: 'center', width: 80 },
  {
    key: 'note',
    title: '备注',
    align: 'center',
    minWidth: 120,
    render: row => row.note || '-'
  },
  {
    key: 'createdAt',
    title: '登记时间',
    align: 'center',
    minWidth: 180,
    render: row => (row.createdAt ? new Date(row.createdAt).toLocaleString() : '-')
  },
  {
    key: 'operate',
    title: '操作',
    align: 'center',
    width: 160,
    render(row) {
      return h('div', { class: 'flex gap-2 justify-center' }, [
        h(
          NButton,
          { size: 'small', type: 'primary', quaternary: true, onClick: () => openEdit(row) },
          { default: () => '编辑' }
        ),
        h(
          NPopconfirm,
          { onPositiveClick: () => removeScrap(row) },
          {
            trigger: () => h(NButton, { size: 'small', type: 'error', quaternary: true }, { default: () => '删除' }),
            default: () => '确认删除该条库存?'
          }
        )
      ]);
    }
  }
]);

async function getData() {
  loading.value = true;
  try {
    const params = searchType.value > 0 ? { scrapType: searchType.value as 1 | 2 } : undefined;
    const { data: res, error } = await fetchCutScraps(params);
    if (!error && res) {
      data.value = res;
    }
  } finally {
    loading.value = false;
  }
}

// ===== 新增 / 编辑 =====
const modalShow = ref(false);
const editing = ref<Api.Cut.CutScrap | null>(null);
const formRef = ref<FormInst | null>(null);
const form = ref({
  scrapType: 1 as 1 | 2,
  label: '',
  lengthValue: null as number | null,
  widthValue: null as number | null,
  heightValue: null as number | null,
  quantity: 1,
  note: ''
});

const rules: FormRules = {
  lengthValue: [{ required: true, type: 'number', message: '请输入长度', trigger: ['blur', 'change'] }],
  widthValue: [{ required: true, type: 'number', message: '请输入宽度', trigger: ['blur', 'change'] }],
  heightValue: [{ required: true, type: 'number', message: '请输入高度', trigger: ['blur', 'change'] }],
  quantity: [{ required: true, type: 'number', min: 1, message: '数量至少为 1', trigger: ['blur', 'change'] }]
};

function openAdd() {
  editing.value = null;
  form.value = {
    scrapType: searchType.value === 2 ? 2 : 1,
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
    label: row.label ?? '',
    lengthValue: row.lengthValue,
    widthValue: row.widthValue,
    heightValue: row.heightValue,
    quantity: row.quantity,
    note: row.note ?? ''
  };
  modalShow.value = true;
}

async function submitForm() {
  await formRef.value?.validate();
  if (editing.value) {
    const { error } = await updateCutScrap(editing.value.id, {
      label: form.value.label.trim(),
      quantity: form.value.quantity,
      note: form.value.note
    });
    if (error) return;
    message.success('修改成功');
  } else {
    const payload: Api.Cut.AddCutScrapRequest[] = [
      {
        scrapType: form.value.scrapType,
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
    message.success('登记成功');
  }
  modalShow.value = false;
  getData();
}

async function removeScrap(row: Api.Cut.CutScrap) {
  const { error } = await deleteCutScrap(row.id);
  if (error) return;
  message.success('删除成功');
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
          <span class="text-18px font-bold">库存管理</span>
        </div>
      </template>
      <div class="flex flex-col h-full gap-4">
        <div class="flex justify-between items-center">
          <div class="flex gap-4 items-center">
            <NSelect v-model:value="searchType" :options="typeOptions" style="width: 140px" @update:value="getData" />
            <NInput v-model:value="searchLabel" placeholder="按名称筛选" clearable style="width: 180px" />
          </div>
          <NButton type="primary" @click="openAdd">新增余料</NButton>
        </div>

        <NDataTable
          :columns="columns"
          :data="filteredData"
          :loading="loading"
          :row-key="(row) => row.id"
          flex-height
          class="flex-1"
        />
      </div>
    </NCard>

    <NModal v-model:show="modalShow" preset="card" :title="editing ? '编辑余料' : '新增余料'" class="w-480px">
      <NForm ref="formRef" :model="form" :rules="rules" label-placement="left" label-width="80">
        <NFormItem label="类型" :show-feedback="!editing">
          <NSelect
            v-model:value="form.scrapType"
            :options="[{ label: '一维余料', value: 1 }, { label: '二维余料', value: 2 }]"
            :disabled="!!editing"
          />
        </NFormItem>
        <NFormItem label="名称">
          <NInput v-model:value="form.label" placeholder="选填, 如: 长料余料" />
        </NFormItem>
        <NFormItem v-if="form.scrapType === 1" label="长度" path="lengthValue">
          <NInputNumber v-model:value="form.lengthValue" :min="0" class="w-full" placeholder="mm" />
        </NFormItem>
        <template v-else>
          <NFormItem label="宽" path="widthValue">
            <NInputNumber v-model:value="form.widthValue" :min="0" class="w-full" placeholder="mm" />
          </NFormItem>
          <NFormItem label="高" path="heightValue">
            <NInputNumber v-model:value="form.heightValue" :min="0" class="w-full" placeholder="mm" />
          </NFormItem>
        </template>
        <NFormItem label="数量" path="quantity">
          <NInputNumber v-model:value="form.quantity" :min="1" class="w-full" />
        </NFormItem>
        <NFormItem label="备注">
          <NInput v-model:value="form.note" placeholder="选填" />
        </NFormItem>
      </NForm>
      <template #action>
        <NButton @click="modalShow = false">取消</NButton>
        <NButton type="primary" @click="submitForm">确定</NButton>
      </template>
    </NModal>
  </div>
</template>

<style scoped></style>
