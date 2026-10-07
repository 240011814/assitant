<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { NButton, NDataTable, NModal, useMessage } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { $t } from '@/locales';
import { fetchCutWindows } from '@/service/api';
import { parseWindowOrder } from './window-template';

/** 窗户单选择弹窗: 列出已保存的窗户单, 勾选后导入切割清单 */
const show = defineModel<boolean>('show', { default: false });

const emit = defineEmits<{ (e: 'confirm', records: Api.Cut.CutWindow[]): void }>();

const message = useMessage();

const loading = ref(false);
const records = ref<Api.Cut.CutWindow[]>([]);
const checkedKeys = ref<number[]>([]);

const columns = computed<DataTableColumns<Api.Cut.CutWindow>>(() => [
  { type: 'selection' },
  { title: $t('page.cut.wtName'), key: 'name', minWidth: 140, render: row => row.name || '-' },
  {
    title: $t('page.cut.wtWindowCount'),
    key: 'windowCount',
    width: 90,
    align: 'center',
    render: row => parseWindowOrder(row).windows.length
  },
  {
    title: $t('page.cut.wtTotalBars'),
    key: 'totalBars',
    width: 90,
    align: 'center',
    render: row => parseWindowOrder(row).windows.reduce((s, it) => s + (it.count || 0), 0)
  },
  {
    title: $t('page.cut.wtCreatedAt'),
    key: 'createdAt',
    width: 170,
    align: 'center',
    render: row => (row.createdAt ? new Date(row.createdAt).toLocaleString() : '-')
  }
]);

watch(show, async opened => {
  if (!opened) return;
  checkedKeys.value = [];
  loading.value = true;
  try {
    const { data, error } = await fetchCutWindows();
    if (!error && data) records.value = data;
  } finally {
    loading.value = false;
  }
});

function handleConfirm() {
  if (checkedKeys.value.length === 0) {
    message.warning($t('page.cut.wtPickNone'));
    return;
  }
  const selected = records.value.filter(r => checkedKeys.value.includes(r.id));
  emit('confirm', selected);
  show.value = false;
}
</script>

<template>
  <NModal v-model:show="show" preset="card" :title="$t('page.cut.wtPickTitle')" class="w-640px max-w-[96vw]">
    <NDataTable
      v-model:checked-row-keys="checkedKeys"
      :columns="columns"
      :data="records"
      :loading="loading"
      size="small"
      max-height="420"
      :row-key="(row: Api.Cut.CutWindow) => row.id"
    />
    <template #footer>
      <div class="flex justify-end gap-2">
        <NButton @click="show = false">{{ $t('common.cancel') }}</NButton>
        <NButton type="primary" :disabled="checkedKeys.length === 0" @click="handleConfirm">
          {{ $t('page.cut.wtPickConfirm') }}{{ checkedKeys.length > 0 ? `(${checkedKeys.length})` : '' }}
        </NButton>
      </div>
    </template>
  </NModal>
</template>
