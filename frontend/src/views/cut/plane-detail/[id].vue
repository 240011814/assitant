<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { NButton, NResult, NSpin, useMessage } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { $t } from '@/locales';
import { fetchCutRecord } from '@/service/api';

const message = useMessage();
const route = useRoute();
const router = useRouter();

const loading = ref(false);
const parseError = ref(false);

// 内容全部在按 id 拉取记录后填充
const summaryData = ref<Api.Cut.PlaneSummary | null>(null);
const newBoards = ref<Api.Cut.Item[]>([]);
const items = ref<Api.Cut.Item[]>([]);
const materials = ref<Api.Cut.Item[]>([]);
const results = ref<Api.Cut.BinResult[]>([]);
const unplaced = ref<Api.Cut.UnplacedItem[]>([]);
const strategy = ref('Guillotine');

onMounted(async () => {
  const id = String(route.params.id ?? '');
  if (!id) {
    parseError.value = true;
    return;
  }
  loading.value = true;
  try {
    const { data, error } = await fetchCutRecord(id);
    if (error || !data?.request || !data.response) {
      parseError.value = true;
      return;
    }
    const req = JSON.parse(data.request) as Api.Cut.BinRequest & {
      rowItems?: Api.Cut.Item[];
    };
    // 兼容新旧记录: 旧记录响应为数组, 新记录响应为 { results, unplaced, summary }
    const raw = JSON.parse(data.response) as Api.Cut.BinResult[] | Api.Cut.PlaneCutResponse;
    newBoards.value = req.newMaterials?.length
      ? req.newMaterials
      : [{ label: '', width: req.width, height: req.height }];
    items.value = req.rowItems ?? [];
    materials.value = req.materials ?? [];
    strategy.value = req.strategy || 'Guillotine';
    results.value = Array.isArray(raw) ? raw : raw.results;
    unplaced.value = Array.isArray(raw) ? [] : (raw.unplaced ?? []);
    summaryData.value = Array.isArray(raw) ? null : (raw.summary ?? null);
  } catch {
    parseError.value = true;
    message.error($t('page.cut.recordLoadFailed'));
  } finally {
    loading.value = false;
  }
});

function goBack() {
  router.back();
}

const group = ref(false);
const strategyOptions = [
  { label: $t('page.cut.strategyGuillotine'), value: 'Guillotine' },
  { label: $t('page.cut.strategyMaxRects'), value: 'MaxRects' },
  // 精确模式记录回显 (sidecar 超时/失败时后端直接报错, 结果仍标为 Precise 保存)
  { label: $t('page.cut.strategyPrecise'), value: 'Precise' }
];

// 未排入件表格 (旧记录响应为数组时无此数据)
const unplacedTotal = computed(() => unplaced.value.reduce((sum, item) => sum + item.quantity, 0));
const unplacedColumns: DataTableColumns<Api.Cut.UnplacedItem> = [
  { title: $t('page.cut.label'), key: 'label' },
  { title: $t('page.cut.sizeCm'), key: 'size', render: row => `${fmtNum(row.width)} × ${fmtNum(row.height)}` },
  { title: $t('page.cut.quantity'), key: 'quantity', width: 80 },
  {
    title: $t('page.cut.unplacedReason'),
    key: 'reason',
    render: row => (row.reason === 'oversized' ? $t('page.cut.reasonOversized') : $t('page.cut.reasonExhausted'))
  }
];

function fmtNum(n: number): string {
  return Number.isInteger(n) ? String(n) : n.toFixed(1);
}

// item 表格 (零件: 材料类型取 spec, 空=通用)
const itemColumns = [
  { title: $t('page.cut.materialType'), key: 'spec', render: (row: Api.Cut.Item) => row.spec?.trim() || $t('page.cut.general') },
  { title: $t('page.cut.label'), key: 'label' },
  { title: $t('page.cut.widthCm'), key: 'width' },
  { title: $t('page.cut.heightCm'), key: 'height' },
  { title: $t('page.cut.quantity'), key: 'quantity' }
];

// material 表格 (旧料: label 即材料类型)
const materialColumns = [
  { title: $t('page.cut.materialType'), key: 'label' },
  { title: $t('page.cut.widthCm'), key: 'width' },
  { title: $t('page.cut.heightCm'), key: 'height' },
  { title: $t('page.cut.quantity'), key: 'quantity' }
];

// 按材料类型分组统计表 (取保存响应里的 summary.byMaterialType, 旧记录无此字段则不展示)
const typeSummaryColumns: DataTableColumns<Api.Cut.PlaneMaterialTypeSummary> = [
  {
    title: $t('page.cut.materialType'),
    key: 'materialType',
    render: row => row.materialType?.trim() || $t('page.cut.newBoard')
  },
  { title: $t('page.cut.summaryBinCount'), key: 'count' },
  { title: $t('page.cut.summaryTotalAreaCm2'), key: 'totalArea', render: row => fmtNum(row.totalArea) },
  { title: $t('page.cut.summaryUsedAreaCm2'), key: 'usedArea', render: row => fmtNum(row.usedArea) },
  { title: $t('page.cut.summaryTypeUtilization'), key: 'utilization', render: row => `${row.utilization}%` }
];
</script>

<template>
  <NResult
    v-if="parseError"
    status="error"
    :title="$t('page.cut.loadFailedTitle')"
    :description="$t('page.cut.loadFailedDesc')"
    class="mt-16"
  >
    <template #footer>
      <NButton type="primary" @click="goBack">{{ $t('page.cut.goBack') }}</NButton>
    </template>
  </NResult>

  <NSpin v-else :show="loading">
    <div class="p-4">
    <NCard :title="$t('page.cut.vizTitle')" size="large" class="mb-4">
      <!-- 切割项目列表 -->

      <h3 class="mb-2 text-lg font-semibold">{{ $t('page.cut.cutItems') }}</h3>
      <NDataTable :columns="itemColumns" :data="items" />

      <!-- 剩余材料列表 -->

      <h3 class="mb-2 text-lg font-semibold">{{ $t('page.cut.remainingMaterial') }}</h3>
      <NDataTable :columns="materialColumns" :data="materials" />

      <h3 class="mt-6">{{ $t('page.cut.paramsConfig') }}</h3>
      <div class="mb-4 flex flex-wrap items-start gap-6">
        <div class="flex items-start gap-2">
          <span class="w-24 pt-1">{{ $t('page.cut.newMaterialSpec') }}</span>
          <div class="flex flex-col gap-2">
            <div v-for="(row, idx) in newBoards" :key="idx" class="flex items-center gap-2">
              <span class="w-32 text-gray-600">{{ row.label?.trim() || $t('page.cut.newBoard') }}</span>
              <NInputNumber :value="row.width" :placeholder="$t('page.cut.widthCm')" disabled class="w-40" />
              <NInputNumber :value="row.height" :placeholder="$t('page.cut.heightCm')" disabled class="w-40" />
            </div>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">{{ $t('page.cut.strategy') }}</span>
          <NSelect v-model:value="strategy" :options="strategyOptions" disabled />
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">{{ $t('page.cut.aggregateDisplay') }}</span>
          <NSwitch v-model:value="group" class="w-40" />
        </div>
      </div>

      <!-- 操作按钮 -->
      <div class="mt-4 flex gap-2">
        <PlanePrinter :results="results" :materials="materials" :group-data="group"></PlanePrinter>
      </div>
    </NCard>

    <!-- 未排入件警示 (保存时的 unplaced 数据, 旧记录无) -->
    <NAlert
      v-if="unplaced.length > 0"
      type="error"
      :closable="false"
      class="mb-4"
      :title="$t('page.cut.unplacedCountTitle', { count: unplacedTotal })"
    >
      <NDataTable size="small" :columns="unplacedColumns" :data="unplaced" />
    </NAlert>

    <PlaneStats :results="results"></PlaneStats>

    <!-- 按材料类型分组统计 (旧记录无此字段则不展示) -->
    <NCard v-if="summaryData?.byMaterialType?.length" size="large" class="mb-4">
      <template #header>{{ $t('page.cut.summaryByTypeTitle') }}</template>
      <NDataTable size="small" :columns="typeSummaryColumns" :data="summaryData.byMaterialType" :bordered="false" />
    </NCard>

    <PlaneCanvas :results="results" :group-data="group" :materials="materials"></PlaneCanvas>
    </div>
  </NSpin>
</template>

<style scoped></style>
