<script setup lang="tsx">
import { h, onMounted, ref, computed } from 'vue';
import { NButton, NCard, NDataTable, NPopconfirm, NTag, NSelect, NDatePicker, useMessage } from 'naive-ui';
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
      // 一维记录且有可入库余料 (remaining > 0) 时提供入库入口
      if (scrapRowsOf(row).length > 0) {
        buttons.push(
          h(
            NPopconfirm,
            { onPositiveClick: () => stockInFromRecord(row) },
            {
              trigger: () =>
                h(
                  NButton,
                  {
                    size: 'small',
                    type: 'warning',
                    quaternary: true,
                    loading: stockingId.value === row.id
                  },
                  { default: () => '余料入库' }
                ),
              default: () => `确认将 ${scrapRowsOf(row).length} 根余料入库?`
            }
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

// 余料名称: 多规格新材料时按 totalLength 匹配规格名 (与计算页 stockInScraps 同规则)
function scrapLabelFor(row: Api.Cut.CutRecord, item: Api.Cut.BarResult): string | undefined {
  try {
    const saved = JSON.parse(row.request || '{}') as { newMaterials?: Api.Cut.NewMaterialSpec[] };
    const specs = saved.newMaterials ?? [];
    if (specs.length < 2) return undefined;
    const spec = specs.find(s => s.length === item.totalLength);
    const name = spec?.label?.trim();
    return name ? `${name}${$t('page.cut.scrapLabelSuffix')}` : undefined;
  } catch {
    return undefined;
  }
}

const stockingId = ref<string | null>(null);

// 历史记录余料入库: 把该记录切割剩余的每根料登记为一维余料库存
async function stockInFromRecord(row: Api.Cut.CutRecord) {
  const scrapRows = scrapRowsOf(row);
  if (scrapRows.length === 0) return;
  stockingId.value = row.id;
  try {
    const { error } = await addCutScraps(
      scrapRows.map(item => ({
        scrapType: 1 as const,
        label: scrapLabelFor(row, item),
        lengthValue: item.remaining,
        quantity: 1,
        note: $t('page.cut.scrapFromCutting')
      }))
    );
    if (error) return;
    message.success($t('page.cut.scrapStockInSuccess', { count: scrapRows.length }));
  } finally {
    stockingId.value = null;
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
  </div>
</template>

<style scoped></style>
