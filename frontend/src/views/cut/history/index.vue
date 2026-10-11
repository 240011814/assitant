<script setup lang="tsx">
import { h, onMounted, ref, computed } from 'vue';
import { NButton, NCard, NDataTable, NInputNumber, NModal, NPopconfirm, NSpin, NTag, NSelect, NDatePicker, useMessage } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { cutList, deleteRecod, addCutScraps, fetchCutRecord } from '@/service/api';
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
    width: 320,
    render(row) {
      const buttons = [
        h(
          NButton,
          { size: 'small', type: 'primary', quaternary: true, onClick: () => edit(row) },
          { default: () => '查看' }
        ),
        h(
          NButton,
          { size: 'small', type: 'info', quaternary: true, onClick: () => reuse(row) },
          { default: () => '重新编辑' }
        )
      ];
      // 尚未入库过的记录提供余料入库入口; 是否真有余料在打开弹窗按 id 取全量响应后判断
      if (!row.scrapImported) {
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

// 查看: 详情页按 id 自行取全量数据, query 不再携带整份 request/response (大方案会超 URL 长度)
function edit(row: Api.Cut.CutRecord) {
  if (row.type === '1') {
    routerPushByKey('cut_bar-detail', { params: { id: row.id } });
  } else {
    routerPushByKey('cut_plane-detail', { params: { id: row.id } });
  }
}

// 重新编辑: 跳回切割页, 由页面按 id 取回完整请求回填表单
function reuse(row: Api.Cut.CutRecord) {
  if (row.type === '1') {
    routerPushByKey('cut_bar', { query: { reuse: row.id } });
  } else {
    routerPushByKey('cut_plane', { query: { reuse: row.id } });
  }
}

// ===== 余料入库弹窗: 按 id 取回完整响应后列出可入库余料 =====
// 一维: results 中 remaining > 0 的每根料, 按最小长度过滤 (短料不值得入库)
// 平面: 每块板剩余的最大空矩形 (scrapRect), 按最小宽/高过滤, 同类型同尺寸归并数量
const stockInModal = ref<{ show: boolean; loading: boolean; row: Api.Cut.CutRecord | null }>({
  show: false,
  loading: false,
  row: null
});
const stockInType = ref<'1' | '2'>('1');
const stockInMinLen = ref<number | null>(null);
const stockInMinWidth = ref<number | null>(null);
const stockInMinHeight = ref<number | null>(null);
const stockInConfirmLoading = ref(false);

interface StockInBarRow extends Api.Cut.BarResult {
  kept: boolean;
}

interface StockInPlaneRow {
  materialType: string;
  width: number;
  height: number;
  quantity: number;
  kept: boolean;
}

const barScrapRows = ref<StockInBarRow[]>([]);
const planeScrapRows = ref<StockInPlaneRow[]>([]);

async function openStockIn(row: Api.Cut.CutRecord) {
  stockInType.value = row.type === '1' ? '1' : '2';
  stockInMinLen.value = null;
  stockInMinWidth.value = null;
  stockInMinHeight.value = null;
  barScrapRows.value = [];
  planeScrapRows.value = [];
  stockInModal.value = { show: true, loading: true, row };
  const { data, error } = await fetchCutRecord(row.id);
  stockInModal.value.loading = false;
  if (error || !data?.response) {
    message.error($t('page.cut.recordLoadFailed'));
    stockInModal.value.show = false;
    return;
  }
  try {
    const resp = JSON.parse(data.response) as Api.Cut.BarCutResponse | Api.Cut.BinResult[] | Api.Cut.PlaneCutResponse;
    const results = Array.isArray(resp) ? resp : ((resp as any).results ?? []);
    if (data.type === '1') {
      barScrapRows.value = (results as Api.Cut.BarResult[])
        .filter(item => item.remaining > 0)
        .map(item => ({ ...item, kept: true }));
    } else {
      const map = new Map<string, StockInPlaneRow>();
      (results as Api.Cut.BinResult[]).forEach(r => {
        const rect = r.scrapRect;
        if (!rect || rect.w <= 0 || rect.h <= 0) return;
        const key = `${r.materialType ?? ''}|${rect.w}|${rect.h}`;
        const existing = map.get(key);
        if (existing) {
          existing.quantity += 1;
        } else {
          map.set(key, { materialType: r.materialType ?? '', width: rect.w, height: rect.h, quantity: 1, kept: true });
        }
      });
      planeScrapRows.value = [...map.values()];
    }
    const total = data.type === '1' ? barScrapRows.value.length : planeScrapRows.value.length;
    if (total === 0) {
      message.warning($t('page.cut.noScrapToStockIn'));
    }
  } catch {
    message.error($t('page.cut.recordLoadFailed'));
    stockInModal.value.show = false;
  }
}

// 一维候选: 按最小长度过滤
const stockInBarList = computed<StockInBarRow[]>(() => {
  const min = stockInMinLen.value;
  return barScrapRows.value.map(item => ({ ...item, kept: !(min && min > 0) || item.remaining >= min }));
});

// 平面候选: 按最小宽/高过滤
const stockInPlaneList = computed<StockInPlaneRow[]>(() => {
  const minW = stockInMinWidth.value;
  const minH = stockInMinHeight.value;
  return planeScrapRows.value.map(item => ({
    ...item,
    kept:
      (!(minW && minW > 0) || item.width >= minW) && (!(minH && minH > 0) || item.height >= minH)
  }));
});

const stockInKept = computed(() =>
  stockInType.value === '1'
    ? stockInBarList.value.filter(item => item.kept).length
    : stockInPlaneList.value.filter(item => item.kept).length
);
const stockInTotal = computed(() =>
  stockInType.value === '1' ? stockInBarList.value.length : stockInPlaneList.value.length
);

const stockInBarColumns: DataTableColumns<StockInBarRow> = [
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

const stockInPlaneColumns: DataTableColumns<StockInPlaneRow> = [
  { title: '材料类型', key: 'materialType', render: row => row.materialType?.trim() || '新板材' },
  {
    title: '尺寸(cm)',
    key: 'size',
    align: 'center',
    render: row => `${row.width} × ${row.height}`
  },
  { title: '数量', key: 'quantity', width: 70, align: 'center' },
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

// 确认入库: 只导入未被过滤的余料; 入库成功后端会把记录标记为已入库 (含被过滤的, 不再重复弹窗)
async function confirmStockIn() {
  const row = stockInModal.value.row;
  if (!row || stockInConfirmLoading.value) return;
  const isBar = stockInType.value === '1';
  const rows = isBar ? stockInBarList.value.filter(item => item.kept) : stockInPlaneList.value.filter(item => item.kept);
  if (rows.length === 0) {
    message.warning(isBar ? '没有符合长度条件的余料可入库' : '没有符合尺寸条件的余料可入库');
    return;
  }
  stockInConfirmLoading.value = true;
  try {
    const payload = isBar
      ? (rows as StockInBarRow[]).map(item => ({
          scrapType: 1 as const,
          materialType: item.materialType?.trim() || undefined,
          lengthValue: item.remaining,
          quantity: 1,
          note: $t('page.cut.scrapFromCutting'),
          recordId: row.id
        }))
      : (rows as StockInPlaneRow[]).map(item => ({
          scrapType: 2 as const,
          materialType: item.materialType?.trim() || undefined,
          widthValue: item.width,
          heightValue: item.height,
          quantity: item.quantity,
          note: $t('page.cut.scrapFromCutting'),
          recordId: row.id
        }));
    const { error } = await addCutScraps(payload);
    if (error) return;
    row.scrapImported = true;
    message.success($t('page.cut.scrapStockInSuccess', { count: rows.length }));
    stockInModal.value = { show: false, loading: false, row: null };
  } finally {
    stockInConfirmLoading.value = false;
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

    <!-- 余料入库弹窗: 一维按最小长度过滤 / 平面按最小宽高过滤 -->
    <NModal v-model:show="stockInModal.show" preset="card" title="余料入库" class="w-560px max-w-[96vw]">
      <NSpin :show="stockInModal.loading">
        <div class="flex flex-col gap-3">
          <div v-if="stockInType === '1'" class="flex flex-wrap items-center gap-2">
            <span class="text-sm whitespace-nowrap">只入库长度 ≥</span>
            <NInputNumber v-model:value="stockInMinLen" :min="0" class="w-32" placeholder="不限制" show-button />
            <span class="text-sm">cm</span>
            <span class="text-gray-400 text-xs">小于该尺寸的短料不入库</span>
          </div>
          <div v-else class="flex flex-wrap items-center gap-2">
            <span class="text-sm whitespace-nowrap">只入库宽 ≥</span>
            <NInputNumber v-model:value="stockInMinWidth" :min="0" class="w-28" placeholder="不限制" show-button />
            <span class="text-sm whitespace-nowrap">高 ≥</span>
            <NInputNumber v-model:value="stockInMinHeight" :min="0" class="w-28" placeholder="不限制" show-button />
            <span class="text-sm">cm</span>
            <span class="text-gray-400 text-xs">小于该尺寸的边角料不入库</span>
          </div>
          <NDataTable
            v-if="stockInType === '1'"
            size="small"
            :columns="stockInBarColumns"
            :data="stockInBarList"
            max-height="300"
            :row-key="(row: StockInBarRow) => `${row.materialType ?? ''}|${row.remaining}`"
          />
          <NDataTable
            v-else
            size="small"
            :columns="stockInPlaneColumns"
            :data="stockInPlaneList"
            max-height="300"
            :row-key="(row: StockInPlaneRow) => `${row.materialType}|${row.width}|${row.height}`"
          />
          <div class="text-gray-500 text-xs">
            共 {{ stockInTotal }} {{ stockInType === '1' ? '根' : '种' }}, 将入库
            {{ stockInKept }} {{ stockInType === '1' ? '根' : '种' }}{{
              stockInTotal !== stockInKept ? `, 过滤 ${stockInTotal - stockInKept} ${stockInType === '1' ? '根短料' : '种小料'}` : ''
            }}
          </div>
        </div>
      </NSpin>
      <template #footer>
        <div class="flex justify-end gap-2">
          <NButton @click="stockInModal.show = false">取消</NButton>
          <NButton type="primary" :loading="stockInConfirmLoading" :disabled="stockInModal.loading" @click="confirmStockIn">
            确认入库 ({{ stockInKept }})
          </NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>

<style scoped></style>
