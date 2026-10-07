<script setup lang="tsx">
import { computed, h, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { NButton, NCard, NDataTable, NPopconfirm, useMessage } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { $t } from '@/locales';
import { deleteCutProduct, fetchCutProducts } from '@/service/api';
import ProductEditModal from '@/components/cut/ProductEditModal.vue';
import WindowGridPreview from '@/components/cut/WindowGridPreview.vue';
import { findWindowTemplate, parseProductSpec } from '@/components/cut/window-template';

/**
 * 产品管理: 维护待切割产品单 (一单可含多种类型的多件产品),
 * 记录上"去裁剪"跳转一维切割页并直接预填所需切割材料。
 */

interface OrderRow extends Api.Cut.CutProduct {
  parsed: Api.Cut.ProductSpec;
}

const message = useMessage();
const router = useRouter();

const loading = ref(false);
const rows = ref<OrderRow[]>([]);
const checkedKeys = ref<number[]>([]);

async function getData() {
  loading.value = true;
  try {
    const { data, error } = await fetchCutProducts();
    if (!error && data) {
      rows.value = data.map(row => ({ ...row, parsed: parseProductSpec(row) }));
    }
  } finally {
    loading.value = false;
  }
}

/** 去裁剪: 跳一维切割页, 由其消费 query.products 预填所需切割材料 */
function goCut(ids: number[]) {
  if (ids.length === 0) return;
  router.push({ path: '/cut/bar', query: { products: ids.join(',') } });
}

const columns = computed<DataTableColumns<OrderRow>>(() => [
  {
    type: 'expand',
    key: 'expand',
    renderExpand(row) {
      const detailColumns: DataTableColumns<Api.Cut.ProductItem> = [
        {
          title: $t('page.cut.pdTemplate'),
          key: 'type',
          width: 180,
          render: item => findWindowTemplate(item.type)?.label ?? item.type
        },
        { title: '尺寸(cm)', key: 'size', width: 100, align: 'center', render: item => `${item.width}×${item.height}` },
        {
          title: $t('page.cut.pdGrid'),
          key: 'grid',
          width: 110,
          align: 'center',
          render: item => `${item.grid.cols.length}列×${item.grid.rows.length}行`
        },
        { title: $t('page.cut.pdThickness'), key: 'thickness', width: 110, align: 'center', render: item => (item.thickness ? `${item.thickness}mm` : '-') },
        { title: $t('page.cut.pdCount'), key: 'count', width: 70, align: 'center' },
        {
          title: $t('page.cut.pdPreview'),
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
          data: row.parsed.items,
          size: 'small',
          rowKey: (item: Api.Cut.ProductItem) => `${item.type}|${item.width}|${item.height}|${item.thickness}`
        })
      ]);
    }
  },
  { type: 'selection' },
  { title: $t('page.cut.pdName'), key: 'name', minWidth: 160, render: row => row.name || '-' },
  {
    title: $t('page.cut.pdProductCount'),
    key: 'productCount',
    width: 90,
    align: 'center',
    render: row => row.parsed.items.length
  },
  {
    title: $t('page.cut.pdTotalCount'),
    key: 'totalCount',
    width: 90,
    align: 'center',
    render: row => row.parsed.items.reduce((sum, item) => sum + (item.count || 0), 0)
  },
  {
    title: $t('page.cut.pdThickness'),
    key: 'thicknesses',
    minWidth: 130,
    render: row => [...new Set(row.parsed.items.map(item => item.thickness).filter(Boolean))].map(v => `${v}mm`).join(' / ') || '-'
  },
  {
    title: $t('page.cut.pdCreatedAt'),
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
          { default: () => $t('page.cut.pdGoCut') }
        ),
        h(
          NButton,
          { size: 'small', type: 'info', quaternary: true, onClick: () => openEdit(row) },
          { default: () => $t('common.edit') }
        ),
        h(NPopconfirm, { onPositiveClick: () => removeOrder(row) }, {
          trigger: () => h(NButton, { size: 'small', type: 'error', quaternary: true }, { default: () => $t('common.delete') }),
          default: () => $t('page.cut.pdDeleteConfirm')
        })
      ]);
    }
  }
]);

// ===== 新建 / 编辑 =====
const modalShow = ref(false);
const editing = ref<Api.Cut.CutProduct | null>(null);

function openAdd() {
  editing.value = null;
  modalShow.value = true;
}

function openEdit(row: Api.Cut.CutProduct) {
  editing.value = row;
  modalShow.value = true;
}

async function removeOrder(row: Api.Cut.CutProduct) {
  const { error } = await deleteCutProduct(row.id);
  if (error) return;
  message.success($t('page.cut.pdDeleted'));
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
          <span class="text-18px font-bold">{{ $t('route.cut_product') }}</span>
        </div>
      </template>
      <div class="flex flex-col h-full gap-4">
        <div class="flex justify-end gap-2">
          <NButton type="info" ghost :disabled="checkedKeys.length === 0" @click="goCut(checkedKeys)">
            {{ $t('page.cut.pdBatchGoCut') }}{{ checkedKeys.length > 0 ? `(${checkedKeys.length})` : '' }}
          </NButton>
          <NButton type="primary" @click="openAdd">{{ $t('page.cut.pdAdd') }}</NButton>
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

    <ProductEditModal v-model:show="modalShow" :order="editing" @saved="getData" />
  </div>
</template>
