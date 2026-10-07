<script setup lang="ts">
import { ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { NButton, NResult } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';

const route = useRoute();
const router = useRouter();

// 解析失败/无参数时不白屏, 显示错误态
const parseError = ref(false);
let parsedRequest: Api.Cut.BinRequest & {
  rowItems: Api.Cut.Item[];
} | null = null;
let parsedResponse: Api.Cut.BinResult[] = [];
let parsedSummary: Api.Cut.PlaneSummary | null = null;
try {
  const rawRequest = route.query.request;
  const rawResponse = route.query.response;
  if (typeof rawRequest !== 'string' || !rawRequest || typeof rawResponse !== 'string' || !rawResponse) {
    throw new Error('missing query params');
  }
  parsedRequest = JSON.parse(rawRequest) as Api.Cut.BinRequest & {
    rowItems: Api.Cut.Item[];
  };
  // 兼容新旧记录: 旧记录响应为数组, 新记录响应为 { results, unplaced, summary }
  const raw = JSON.parse(rawResponse) as Api.Cut.BinResult[] | Api.Cut.PlaneCutResponse;
  parsedResponse = Array.isArray(raw) ? raw : raw.results;
  parsedSummary = Array.isArray(raw) ? null : (raw.summary ?? null);
} catch {
  parseError.value = true;
}
const request = parsedRequest;
const response = parsedResponse;
const summaryData = ref<Api.Cut.PlaneSummary | null>(parsedSummary);

function goBack() {
  router.back();
}

const group = ref(false);
const strategy = ref(request?.strategy || 'Guillotine');
// 新板材规格列表: 新记录取 newMaterials, 旧记录回退单一规格 (兼容)
const newBoards = ref<Api.Cut.Item[]>(
  request?.newMaterials?.length
    ? request.newMaterials
    : request
      ? [{ label: '', width: request.width, height: request.height }]
      : []
);
const items = ref<Api.Cut.Item[]>(request?.rowItems || []);
const materials = ref<Api.Cut.Item[]>(request?.materials || []);
const results = ref<Api.Cut.BinResult[]>(response || []);
const strategyOptions = [
  { label: '刀切法', value: 'Guillotine' },
  { label: '最大空闲法', value: 'MaxRects' }
];

function fmtNum(n: number): string {
  return Number.isInteger(n) ? String(n) : n.toFixed(1);
}

// item 表格 (零件: 材料类型取 spec, 空=通用)
const itemColumns = [
  { title: '材料类型', key: 'spec', render: (row: Api.Cut.Item) => row.spec?.trim() || '通用' },
  { title: '标签', key: 'label' },
  { title: '宽(cm)', key: 'width' },
  { title: '高(cm)', key: 'height' },
  { title: '数量', key: 'quantity' }
];

// material 表格 (旧料: label 即材料类型)
const materialColumns = [
  { title: '材料类型', key: 'label' },
  { title: '宽(cm)', key: 'width' },
  { title: '高(cm)', key: 'height' },
  { title: '数量', key: 'quantity' }
];

// 按材料类型分组统计表 (取保存响应里的 summary.byMaterialType, 旧记录无此字段则不展示)
const typeSummaryColumns: DataTableColumns<Api.Cut.PlaneMaterialTypeSummary> = [
  {
    title: '材料类型',
    key: 'materialType',
    render: row => row.materialType?.trim() || '新板材'
  },
  { title: '用料块数', key: 'count' },
  { title: '用料总面积(cm²)', key: 'totalArea', render: row => fmtNum(row.totalArea) },
  { title: '零件总面积(cm²)', key: 'usedArea', render: row => fmtNum(row.usedArea) },
  { title: '利用率', key: 'utilization', render: row => `${row.utilization}%` }
];
</script>

<template>
  <NResult
    v-if="parseError"
    status="error"
    title="数据加载失败"
    description="裁剪记录参数缺失或格式不正确，无法展示详情"
    class="mt-16"
  >
    <template #footer>
      <NButton type="primary" @click="goBack">返回</NButton>
    </template>
  </NResult>

  <div v-else class="p-4">
    <NCard title="材料裁剪可视化" size="large" class="mb-4">
      <!-- 切割项目列表 -->

      <h3 class="mb-2 text-lg font-semibold">切割项目</h3>
      <NDataTable :columns="itemColumns" :data="items" />

      <!-- 剩余材料列表 -->

      <h3 class="mb-2 text-lg font-semibold">剩余材料</h3>
      <NDataTable :columns="materialColumns" :data="materials" />

      <h3 class="mt-6">参数配置</h3>
      <div class="mb-4 flex flex-wrap items-start gap-6">
        <div class="flex items-start gap-2">
          <span class="w-24 pt-1">新材料规格</span>
          <div class="flex flex-col gap-2">
            <div v-for="(row, idx) in newBoards" :key="idx" class="flex items-center gap-2">
              <span class="w-32 text-gray-600">{{ row.label?.trim() || '新板材' }}</span>
              <NInputNumber :value="row.width" placeholder="宽(cm)" disabled class="w-40" />
              <NInputNumber :value="row.height" placeholder="高(cm)" disabled class="w-40" />
            </div>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">方案</span>
          <NSelect v-model:value="strategy" :options="strategyOptions" disabled />
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">聚合显示</span>
          <NSwitch v-model:value="group" class="w-40" />
        </div>
      </div>

      <!-- 操作按钮 -->
      <div class="mt-4 flex gap-2">
        <PlanePrinter :results="results" :materials="materials"></PlanePrinter>
      </div>
    </NCard>

    <PlaneStats :results="results"></PlaneStats>

    <!-- 按材料类型分组统计 (旧记录无此字段则不展示) -->
    <NCard v-if="summaryData?.byMaterialType?.length" size="large" class="mb-4">
      <template #header>按材料类型统计</template>
      <NDataTable size="small" :columns="typeSummaryColumns" :data="summaryData.byMaterialType" :bordered="false" />
    </NCard>

    <PlaneCanvas :results="results" :group-data="group" :materials="materials"></PlaneCanvas>
  </div>
</template>

<style scoped></style>
