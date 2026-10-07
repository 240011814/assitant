<script setup lang="tsx">
import { computed, h, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { NButton, NCard, NDataTable, NPopconfirm, useMessage } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { $t } from '@/locales';
import { deleteCutWindow, fetchCutScraps, fetchCutWindows } from '@/service/api';
import WindowEditModal from '@/components/cut/WindowEditModal.vue';
import WindowGridPreview from '@/components/cut/WindowGridPreview.vue';
import { findWindowTemplate, parseWindowOrder } from '@/components/cut/window-template';

/**
 * 窗户管理: 维护待切割窗户单 (一单可含多种类型的多樘窗),
 * 记录上"去裁剪"跳转一维切割页并直接预填所需切割材料。
 */

interface OrderRow extends Api.Cut.CutWindow {
  parsed: Api.Cut.WindowSpec;
}

const message = useMessage();
const router = useRouter();

const loading = ref(false);
const rows = ref<OrderRow[]>([]);
const checkedKeys = ref<number[]>([]);

// 材料类型候选: 来自一维余料库存(旧料库)的类型名
const typeOptions = ref<Array<{ label: string; value: string }>>([]);

async function loadTypeOptions() {
  const { data, error } = await fetchCutScraps({ scrapType: 1 });
  if (error || !data) return;
  const set = new Set<string>();
  data.records.forEach(item => {
    const type = item.materialType?.trim() || item.label?.trim();
    if (type) set.add(type);
  });
  typeOptions.value = [...set].map(type => ({ label: type, value: type }));
}

async function getData() {
  loading.value = true;
  try {
    const { data, error } = await fetchCutWindows();
    if (!error && data) {
      rows.value = data.map(row => ({ ...row, parsed: parseWindowOrder(row) }));
    }
  } finally {
    loading.value = false;
  }
}

/** 去裁剪: 跳一维切割页, 由其消费 query.windows 预填切割材料 */
function goCut(ids: number[]) {
  if (ids.length === 0) return;
  router.push({ path: '/cut/bar', query: { windows: ids.join(',') } });
}

const columns = computed<DataTableColumns<OrderRow>>(() => [
  {
    type: 'expand',
    key: 'expand',
    renderExpand(row) {
      const detailColumns: DataTableColumns<Api.Cut.WindowItem> = [
        {
          title: $t('page.cut.wtTemplate'),
          key: 'type',
          width: 180,
          render: item => findWindowTemplate(item.type)?.label ?? item.type
        },
        { title: '尺寸(cm)', key: 'size', width: 100, align: 'center', render: item => `${item.width}×${item.height}` },
        {
          title: $t('page.cut.wtGrid'),
          key: 'grid',
          width: 110,
          align: 'center',
          render: item => `${item.grid.cols.length}列×${item.grid.rows.length}行`
        },
        { title: $t('page.cut.materialType'), key: 'materialType', width: 130, render: item => item.materialType || '-' },
        { title: $t('page.cut.wtCount'), key: 'count', width: 70, align: 'center' },
        {
          title: $t('page.cut.wtPreview'),
          key: 'preview',
          render: item =>
            h(
              'div',
              { style: 'width: 110px' },
              h(WindowGridPreview, { item, readonly: true, maxHeight: 90 })
            )
        }
      ];
      return h('div', { class: 'p-2' }, [
        h(NDataTable, {
          columns: detailColumns,
          data: row.parsed.windows,
          size: 'small',
          rowKey: (item: Api.Cut.WindowItem) => `${item.type}|${item.width}|${item.height}|${item.materialType}`
        })
      ]);
    }
  },
  { type: 'selection' },
  { title: $t('page.cut.wtName'), key: 'name', minWidth: 160, render: row => row.name || '-' },
  {
    title: $t('page.cut.wtWindowCount'),
    key: 'windowCount',
    width: 90,
    align: 'center',
    render: row => row.parsed.windows.length
  },
  {
    title: $t('page.cut.wtTotalBars'),
    key: 'totalBars',
    width: 90,
    align: 'center',
    render: row => row.parsed.windows.reduce((sum, item) => sum + (item.count || 0), 0)
  },
  {
    title: $t('page.cut.materialType'),
    key: 'materialTypes',
    minWidth: 150,
    render: row => [...new Set(row.parsed.windows.map(item => item.materialType).filter(Boolean))].join(' / ') || '-'
  },
  {
    title: $t('page.cut.wtCreatedAt'),
    key: 'createdAt',
    width: 170,
    align: 'center',
    render: row => (row.createdAt ? new Date(row.createdAt).toLocaleString() : '-')
  },
  {
    title: '操作',
    key: 'operate',
    width: 220,
    align: 'center',
    render(row) {
      return h('div', { class: 'flex justify-center gap-1' }, [
        h(
          NButton,
          { size: 'small', type: 'primary', quaternary: true, onClick: () => goCut([row.id]) },
          { default: () => $t('page.cut.wtGoCut') }
        ),
        h(
          NButton,
          { size: 'small', type: 'info', quaternary: true, onClick: () => openEdit(row) },
          { default: () => $t('common.edit') }
        ),
        h(NPopconfirm, { onPositiveClick: () => removeOrder(row) }, {
          trigger: () => h(NButton, { size: 'small', type: 'error', quaternary: true }, { default: () => $t('common.delete') }),
          default: () => $t('page.cut.wtDeleteConfirm')
        })
      ]);
    }
  }
]);

// ===== 新建 / 编辑 =====
const modalShow = ref(false);
const editing = ref<Api.Cut.CutWindow | null>(null);

function openAdd() {
  editing.value = null;
  modalShow.value = true;
}

function openEdit(row: Api.Cut.CutWindow) {
  editing.value = row;
  modalShow.value = true;
}

async function removeOrder(row: Api.Cut.CutWindow) {
  const { error } = await deleteCutWindow(row.id);
  if (error) return;
  message.success($t('page.cut.wtDeleted'));
  getData();
}

onMounted(() => {
  getData();
  loadTypeOptions();
});
</script>

<template>
  <div class="h-full flex-col flex gap-4 p-4">
    <NCard :bordered="false" shadow="sm" class="flex-1">
      <template #header>
        <div class="flex items-center gap-4">
          <span class="text-18px font-bold">{{ $t('route.cut_window-template') }}</span>
        </div>
      </template>
      <div class="flex flex-col h-full gap-4">
        <div class="flex justify-end gap-2">
          <NButton type="info" ghost :disabled="checkedKeys.length === 0" @click="goCut(checkedKeys)">
            {{ $t('page.cut.wtBatchGoCut') }}{{ checkedKeys.length > 0 ? `(${checkedKeys.length})` : '' }}
          </NButton>
          <NButton type="primary" @click="openAdd">{{ $t('page.cut.wtAdd') }}</NButton>
        </div>

        <NDataTable
          v-model:checked-row-keys="checkedKeys"
          :columns="columns"
          :data="rows"
          :loading="loading"
          :row-key="(row: OrderRow) => row.id"
          flex-height
          class="flex-1"
        />
      </div>
    </NCard>

    <WindowEditModal v-model:show="modalShow" :order="editing" :type-options="typeOptions" @saved="getData" />
  </div>
</template>
