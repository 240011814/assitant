<script setup lang="tsx">
import { h, onMounted, ref, computed } from 'vue';
import { NButton, NCard, NDataTable, NInputNumber, NModal, NPopconfirm, NTag, NSelect, NDatePicker, useMessage } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { cutList, deleteRecod, addCutScraps } from '@/service/api';
import { useRouterPush } from '@/hooks/common/router';
import { $t } from '@/locales';

const message = useMessage();
const { routerPushByKey } = useRouterPush();

const loading = ref(false);
const data = ref<any[]>([]);
const total = ref(0);

const searchParams = ref({
  name: null as string | null,
  type: null as string | null,
  startTime: null as number | null,
  endTime: null as number | null
});

const typeOptions = [
  { label: '一维', value: '1' },
  { label: '平面', value: '2' }
];

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

const columns = computed<DataTableColumns<any>>(() => [
  { key: 'code', title: '编号', align: 'center', minWidth: 100 },
  { key: 'name', title: '名称', align: 'center', minWidth: 100 },
  {
    key: 'type',
    title: '类型',
    align: 'center',
    width: 120,
    render(row) {
      const label = row.type === '1' ? '一维' : '平面';
      return h(NTag, { bordered: false }, { default: () => label });
    }
  },
  { key: 'createTime', title: '创建时间', align: 'center', minWidth: 200 },
  {
    key: 'operate',
    title: '操作',
    align: 'center',
    width: 240,
    render(row) {
      const buttons = [
        h(
          NButton,
          { size: 'small', type: 'primary', quaternary: true, onClick: () => edit(row) },
          { default: () => '查看' }
        )
      ];
      // 一维记录且有可入库余料 (remaining > 0) 且尚未入库时提供入库入口; 入库过一次就不再展示, 避免重复入库
      if (row.type === '1' && !row.scrapImported && scrapRowsOf(row).length > 0) {
        buttons.push(
          h(
            NButton,
            {
              size: 'small',
              type: 'warning',
              quaternary: true,
              onClick: () => openStockIn(row)
            },
            { default: () => '余料入库' }
          )
        );
      }
      buttons.push(
        h(
          NPopconfirm,
          { onPositiveClick: () => deleteData(row.id) },
          {
            trigger: () =>
              h(NButton, { size: 'small', type: 'error', quaternary: true }, { default: () => '删除' }),
            default: () => '确认删除?'
          }
        )
      );
      return h('div', { class: 'flex gap-2 justify-center' }, buttons);
    }
  }
]);

async function getData() {
  loading.value = true;
  try {
    const params: Api.Cut.CutRecordSearchParams = {
      current: pagination.value.page,
      size: pagination.value.pageSize,
      name: searchParams.value.name || undefined,
      type: searchParams.value.type || undefined,
      startTime: searchParams.value.startTime || undefined,
      endTime: searchParams.value.endTime || undefined,
    };

    const { data: res, error } = await cutList(params);
    if (!error && res) {
      data.value = res.records || [];
      total.value = res.total || 0;
      pagination.value.itemCount = total.value;
    }
  } catch {
    message.error('加载失败');
  } finally {
    loading.value = false;
  }
}

function resetSearchParams() {
  searchParams.value = { name: null, type: null, startTime: null, endTime: null };
  getData();
}

async function deleteData(id: string) {
  const { error } = await deleteRecod(id);
  if (!error) {
    message.success($t('common.deleteSuccess'));
    getData();
  } else {
    message.error('删除失败');
  }
}

function edit(row: Api.Cut.CutRecord) {
  if (row.type === '1') {
    routerPushByKey('cut_bar-detail', {
      params: { id: row.id },
      query: { request: row.request, response: row.response }
    });
  } else {
    routerPushByKey('cut_plane-detail', {
      params: { id: row.id },
      query: { request: row.request, response: row.response }
    });
  }
}

// 一维记录的可入库余料: response.results 中 remaining > 0 的每根料 (解析失败视为无)
function scrapRowsOf(row: Api.Cut.CutRecord): Api.Cut.BarResult[] {
  if (row.type !== '1' || !row.response) return [];
  try {
    const resp = JSON.parse(row.response) as Api.Cut.BarCutResponse;
    return (resp.results ?? []).filter(item => item.remaining > 0);
  } catch {
    return [];
  }
}

// ===== 余料入库弹窗: 列出该记录的剩余余料, 支持按最小长度过滤 (短料不值得入库) =====
const stockInModal = ref<{ show: boolean; row: Api.Cut.CutRecord | null }>({ show: false, row: null });
const stockInMinLen = ref<number | null>(null);
const stockInLoading = ref(false);

interface StockInRow extends Api.Cut.BarResult {
  kept: boolean;
}

function openStockIn(row: Api.Cut.CutRecord) {
  stockInMinLen.value = null;
  stockInModal.value = { show: true, row };
}

const stockInList = computed<StockInRow[]>(() => {
  const row = stockInModal.value.row;
  if (!row) return [];
  const min = stockInMinLen.value;
  return scrapRowsOf(row).map(item => ({ ...item, kept: !(min && min > 0) || item.remaining >= min }));
});

const stockInKept = computed(() => stockInList.value.filter(item => item.kept));

const stockInColumns: DataTableColumns<StockInRow> = [
  { title: '材料类型', key: 'materialType', render: row => row.materialType?.trim() || '新材料' },
  { title: '长度(cm)', key: 'remaining', width: 100, align: 'center' },
  {
    title: '处理',
    key: 'kept',
    width: 90,
    align: 'center',
    render: row =>
      row.kept
        ? h(NTag, { size: 'small', type: 'success', bordered: false }, { default: () => '入库' })
        : h(NTag, { size: 'small', bordered: false }, { default: () => '已过滤' })
  }
];

// 确认入库: 只导入未被过滤的余料; 入库成功后端会把记录标记为已入库 (含被过滤的短料, 不再重复弹窗)
async function confirmStockIn() {
  const row = stockInModal.value.row;
  if (!row) return;
  const rows = stockInKept.value;
  if (rows.length === 0) {
    message.warning('没有符合长度条件的余料可入库');
    return;
  }
  stockInLoading.value = true;
  try {
    const { error } = await addCutScraps(
      rows.map(item => ({
        scrapType: 1 as const,
        materialType: item.materialType?.trim() || undefined,
        lengthValue: item.remaining,
        quantity: 1,
        note: $t('page.cut.scrapFromCutting'),
        recordId: row.id
      }))
    );
    if (error) return;
    row.scrapImported = true;
    message.success($t('page.cut.scrapStockInSuccess', { count: rows.length }));
    stockInModal.value = { show: false, row: null };
  } finally {
    stockInLoading.value = false;
  }
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
          <span class="text-18px font-bold">历史记录</span>
        </div>
      </template>
      <div class="flex flex-col h-full gap-4">
        <div class="flex justify-between items-center">
          <div class="flex gap-4 items-center">
            <NInput
              v-model:value="searchParams.name"
              placeholder="输入名称搜索"
              clearable
              style="width: 180px"
              @keyup.enter="getData"
            />
            <NSelect
              v-model:value="searchParams.type"
              placeholder="选择类型"
              clearable
              :options="typeOptions"
              style="width: 120px"
              @update:value="getData"
            />
            <NDatePicker
              v-model:value="searchParams.startTime"
              type="datetime"
              clearable
              placeholder="开始时间"
              style="width: 180px"
            />
            <NDatePicker
              v-model:value="searchParams.endTime"
              type="datetime"
              clearable
              placeholder="结束时间"
              style="width: 180px"
            />
            <NButton type="primary" @click="getData">
              <template #icon>
                <IconIcRoundSearch class="text-icon" />
              </template>
              {{ $t('common.search') }}
            </NButton>
          </div>
          <div class="flex gap-2 items-center">
            <NButton quaternary @click="resetSearchParams">
              <template #icon>
                <IconIcRoundRefresh class="text-icon" />
              </template>
            </NButton>
          </div>
        </div>

        <NDataTable
          :columns="columns"
          :data="data"
          :loading="loading"
          :pagination="pagination"
          remote
          :row-key="(row) => row.id"
          flex-height
          class="flex-1"
        />
      </div>
    </NCard>

    <!-- 余料入库弹窗 -->
    <NModal v-model:show="stockInModal.show" preset="card" title="余料入库" class="w-560px max-w-[96vw]">
      <div class="flex flex-col gap-3">
        <div class="flex flex-wrap items-center gap-2">
          <span class="text-sm whitespace-nowrap">只入库长度 ≥</span>
          <NInputNumber v-model:value="stockInMinLen" :min="0" class="w-32" placeholder="不限制" show-button />
          <span class="text-sm">cm</span>
          <span class="text-gray-400 text-xs">小于该尺寸的短料不入库</span>
        </div>
        <NDataTable
          size="small"
          :columns="stockInColumns"
          :data="stockInList"
          max-height="300"
          :row-key="(row: StockInRow) => `${row.materialType ?? ''}|${row.remaining}`"
        />
        <div class="text-gray-500 text-xs">
          共 {{ stockInList.length }} 根, 将入库 {{ stockInKept.length }} 根{{
            stockInList.length !== stockInKept.length ? `, 过滤 ${stockInList.length - stockInKept.length} 根短料` : ''
          }}
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <NButton @click="stockInModal.show = false">取消</NButton>
          <NButton type="primary" :loading="stockInLoading" @click="confirmStockIn">
            确认入库 ({{ stockInKept.length }})
          </NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>

<style scoped></style>
