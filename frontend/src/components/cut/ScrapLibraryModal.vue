<script setup lang="ts">
import { computed, h, ref, watch } from 'vue';
import dayjs from 'dayjs';
import { NButton, NPopconfirm, useMessage } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { $t } from '@/locales';
import { addCutScraps, deleteCutScrap, fetchCutScraps } from '@/service/api';

const props = defineProps<{
  /** 余料类型: 1=一维余料(长度) 2=二维余料(板材) */
  scrapType: 1 | 2;
}>();

const show = defineModel<boolean>('show', { default: false });

const emit = defineEmits<{
  (e: 'apply', rows: Api.Cut.CutScrap[]): void;
}>();

const message = useMessage();

const list = ref<Api.Cut.CutScrap[]>([]);
const loading = ref(false);
const submitting = ref(false);
const checkedKeys = ref<number[]>([]);
const rowKey = (row: Api.Cut.CutScrap) => row.id;

// 手动添加行
const label = ref('');
const lengthValue = ref<number | null>(null);
const widthValue = ref<number | null>(null);
const heightValue = ref<number | null>(null);
const quantity = ref<number | null>(1);
const note = ref('');

const columns = computed<DataTableColumns<Api.Cut.CutScrap>>(() => [
  { type: 'selection' },
  {
    title: $t('page.cut.scrapLabelName'),
    key: 'label',
    width: 110,
    ellipsis: { tooltip: true },
    render: row => (row.label ? row.label : '-')
  },
  props.scrapType === 1
    ? {
        title: $t('page.cut.scrapSize'),
        key: 'lengthValue',
        render: row => `${row.lengthValue} cm`
      }
    : {
        title: $t('page.cut.scrapSize'),
        key: 'size',
        render: row => `${row.widthValue} × ${row.heightValue} cm`
      },
  { title: $t('page.cut.scrapQuantity'), key: 'quantity', width: 70 },
  { title: $t('page.cut.scrapNote'), key: 'note', ellipsis: { tooltip: true } },
  {
    title: $t('page.cut.scrapCreatedAt'),
    key: 'createdAt',
    width: 150,
    render: row => (row.createdAt ? dayjs(row.createdAt).format('YYYY-MM-DD HH:mm') : '')
  },
  {
    title: $t('common.action'),
    key: 'actions',
    width: 80,
    render: row =>
      h(
        NPopconfirm,
        { onPositiveClick: () => handleDelete(row.id) },
        {
          default: () => $t('page.cut.scrapDeleteConfirm'),
          trigger: () =>
            h(
              NButton,
              { size: 'small', type: 'error', quaternary: true },
              { default: () => $t('common.delete') }
            )
        }
      )
  }
]);

async function loadList() {
  loading.value = true;
  const { data, error } = await fetchCutScraps({ scrapType: props.scrapType });
  loading.value = false;
  if (error || !data) return;
  list.value = data;
  // 默认全选, 方便一键带入
  checkedKeys.value = data.map(item => item.id);
}

watch(show, val => {
  if (val) {
    loadList();
  }
});

async function handleAdd() {
  const qty = quantity.value;
  const valid =
    props.scrapType === 1
      ? Boolean(lengthValue.value && lengthValue.value > 0 && qty && qty > 0)
      : Boolean(widthValue.value && heightValue.value && qty && qty > 0);
  if (!valid) {
    message.error($t('page.cut.scrapInputInvalid'));
    return;
  }

  const payload: Api.Cut.AddCutScrapRequest =
    props.scrapType === 1
      ? { scrapType: 1, lengthValue: lengthValue.value!, quantity: qty! }
      : { scrapType: 2, widthValue: widthValue.value!, heightValue: heightValue.value!, quantity: qty! };
  if (label.value.trim()) {
    payload.label = label.value.trim();
  }
  if (note.value.trim()) {
    payload.note = note.value.trim();
  }

  submitting.value = true;
  const { error } = await addCutScraps([payload]);
  submitting.value = false;
  if (error) return;

  message.success($t('page.cut.scrapAddSuccess'));
  label.value = '';
  lengthValue.value = null;
  widthValue.value = null;
  heightValue.value = null;
  quantity.value = 1;
  note.value = '';
  loadList();
}

async function handleDelete(id: number) {
  const { error } = await deleteCutScrap(id);
  if (error) return;
  message.success($t('page.cut.scrapDeleteSuccess'));
  loadList();
}

function handleApply() {
  const rows = list.value.filter(item => checkedKeys.value.includes(item.id));
  if (rows.length === 0) {
    message.warning($t('page.cut.scrapApplyNone'));
    return;
  }
  emit('apply', rows);
  show.value = false;
}
</script>

<template>
  <NModal v-model:show="show" preset="card" :title="$t('page.cut.scrapLibrary')" class="w-720px">
    <!-- 手动添加 -->
    <div class="mb-3 flex flex-wrap items-center gap-2">
      <NInput v-model:value="label" :placeholder="$t('page.cut.scrapLabelName')" class="w-32" />
      <template v-if="scrapType === 1">
        <NInputNumber v-model:value="lengthValue" :placeholder="$t('page.cut.scrapLength')" class="w-40" :min="0" />
      </template>
      <template v-else>
        <NInputNumber v-model:value="widthValue" :placeholder="$t('page.cut.scrapWidth')" class="w-40" :min="0" />
        <NInputNumber v-model:value="heightValue" :placeholder="$t('page.cut.scrapHeight')" class="w-40" :min="0" />
      </template>
      <NInputNumber v-model:value="quantity" :placeholder="$t('page.cut.scrapQuantity')" class="w-32" :min="1" />
      <NInput v-model:value="note" :placeholder="$t('page.cut.scrapNotePlaceholder')" class="w-48" />
      <NButton type="primary" :loading="submitting" @click="handleAdd">
        {{ $t('page.cut.scrapAdd') }}
      </NButton>
    </div>

    <!-- 库存列表 -->
    <NDataTable
      v-model:checked-row-keys="checkedKeys"
      :columns="columns"
      :data="list"
      :loading="loading"
      :row-key="rowKey"
      :max-height="320"
      size="small"
    />

    <template #footer>
      <div class="flex justify-end gap-2">
        <NButton @click="show = false">{{ $t('common.close') }}</NButton>
        <NButton type="primary" @click="handleApply">{{ $t('page.cut.scrapApply') }}</NButton>
      </div>
    </template>
  </NModal>
</template>
