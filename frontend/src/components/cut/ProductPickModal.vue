<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { NButton, NDataTable, NModal, useMessage } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { $t } from '@/locales';
import { fetchCutProducts } from '@/service/api';
import { isGlassItem, parseProductSpec } from './window-template';

/** 产品单选择弹窗: 列出已保存的产品单, 勾选后导入切割清单。
 * mode 过滤: bar=只列含窗类(一维)产品, plane=只列含玻璃(平面)产品 (一维/平面只能有一种) */
const show = defineModel<boolean>('show', { default: false });

const props = defineProps<{
  mode?: 'bar' | 'plane';
}>();

const emit = defineEmits<{ (e: 'confirm', records: Api.Cut.CutProduct[]): void }>();

const message = useMessage();

const loading = ref(false);
const records = ref<Api.Cut.CutProduct[]>([]);
const checkedKeys = ref<number[]>([]);

const columns = computed<DataTableColumns<Api.Cut.CutProduct>>(() => [
  { type: 'selection' },
  { title: $t('page.cut.pdName'), key: 'name', minWidth: 140, render: row => row.name || '-' },
  {
    title: $t('page.cut.pdProductCount'),
    key: 'productCount',
    width: 90,
    align: 'center',
    render: row => parseProductSpec(row).items.length
  },
  {
    title: $t('page.cut.pdTotalCount'),
    key: 'totalCount',
    width: 90,
    align: 'center',
    render: row => parseProductSpec(row).items.reduce((s, it) => s + (it.count || 0), 0)
  },
  {
    title: $t('page.cut.pdCreatedAt'),
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
    const { data, error } = await fetchCutProducts();
    if (!error && data) {
      // 按切割方式过滤: 一维吃窗类件, 平面吃玻璃件; 混合单两边都会列出, 导入时各取所需
      records.value = data.filter(row => {
        const items = parseProductSpec(row).items;
        return props.mode === 'plane' ? items.some(isGlassItem) : items.some(item => !isGlassItem(item));
      });
    }
  } finally {
    loading.value = false;
  }
});

function handleConfirm() {
  if (checkedKeys.value.length === 0) {
    message.warning($t('page.cut.pdPickNone'));
    return;
  }
  const selected = records.value.filter(r => checkedKeys.value.includes(r.id));
  emit('confirm', selected);
  show.value = false;
}
</script>

<template>
  <NModal v-model:show="show" preset="card" :title="$t('page.cut.pdPickTitle')" class="w-640px max-w-[96vw]">
    <NDataTable
      v-model:checked-row-keys="checkedKeys"
      :columns="columns"
      :data="records"
      :loading="loading"
      size="small"
      max-height="420"
      :row-key="(row: Api.Cut.CutProduct) => row.id"
    />
    <template #footer>
      <div class="flex justify-end gap-2">
        <NButton @click="show = false">{{ $t('common.cancel') }}</NButton>
        <NButton type="primary" :disabled="checkedKeys.length === 0" @click="handleConfirm">
          {{ $t('page.cut.pdPickConfirm') }}{{ checkedKeys.length > 0 ? `(${checkedKeys.length})` : '' }}
        </NButton>
      </div>
    </template>
  </NModal>
</template>
