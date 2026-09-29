<script setup lang="ts">
import { ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { NButton, NResult } from 'naive-ui';

const route = useRoute();
const router = useRouter();

// 解析失败/无参数时不白屏, 显示错误态
const parseError = ref(false);
let parsedRequest: Api.Cut.BinRequest & {
  rowItems: Api.Cut.Item[];
} | null = null;
let parsedResponse: Api.Cut.BinResult[] = [];
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
} catch {
  parseError.value = true;
}
const request = parsedRequest;
const response = parsedResponse;

function goBack() {
  router.back();
}

const group = ref(false);
const strategy = ref(request?.strategy || 'Guillotine');
const newMaterialHeight = ref(request?.height || 200);
const newMaterialWidth = ref(request?.width || 200);
const items = ref<Api.Cut.Item[]>(request?.rowItems || []);
const materials = ref<Api.Cut.Item[]>(request?.materials || []);
const results = ref<Api.Cut.BinResult[]>(response || []);
const strategyOptions = [
  { label: '刀切法', value: 'Guillotine' },
  { label: '最大空闲法', value: 'MaxRects' }
];
// item 表格
const itemColumns = [
  { title: '标签', key: 'label' },
  { title: '宽(cm)', key: 'width' },
  { title: '高(cm)', key: 'height' },
  { title: '数量', key: 'quantity' }
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
      <NDataTable :columns="itemColumns" :data="materials" />

      <h3 class="mt-6">参数配置</h3>
      <div class="mb-4 flex items-center gap-6">
        <div class="flex items-center gap-2">
          <span class="w-24">方案</span>
          <NSelect v-model:value="strategy" :options="strategyOptions" />
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">新材料长度</span>
          <NInputNumber v-model:value="newMaterialHeight" disabled class="w-40" />
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">新材料高度</span>
          <NInputNumber v-model:value="newMaterialWidth" disabled class="w-40" />
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
    <PlaneCanvas :results="results" :group-data="group" :materials="materials"></PlaneCanvas>
  </div>
</template>

<style scoped></style>
