<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { NButton, NCard, NDataTable, NInputNumber, NResult } from 'naive-ui';
const route = useRoute();
const router = useRouter();

// 解析失败/无参数时不白屏, 显示错误态
const parseError = ref(false);
let parsedRequest: Api.Cut.BarRequest & {
  rowItems: Api.Cut.BarItem[];
  rowMaterials: Api.Cut.BarItem[];
} | null = null;
let parsedResponse: Api.Cut.BarResult[] = [];
try {
  const rawRequest = route.query.request;
  const rawResponse = route.query.response;
  if (typeof rawRequest !== 'string' || !rawRequest || typeof rawResponse !== 'string' || !rawResponse) {
    throw new Error('missing query params');
  }
  parsedRequest = JSON.parse(rawRequest) as Api.Cut.BarRequest & {
    rowItems: Api.Cut.BarItem[];
    rowMaterials: Api.Cut.BarItem[];
  };
  // 兼容新旧记录: 旧记录响应为数组, 新记录响应为 { results, summary }
  const raw = JSON.parse(rawResponse) as Api.Cut.BarResult[] | Api.Cut.BarCutResponse;
  parsedResponse = Array.isArray(raw) ? raw : raw.results;
} catch {
  parseError.value = true;
}
const request = parsedRequest;
const response = parsedResponse;
const itemsData = ref<Api.Cut.BarItem[]>(request?.rowItems || []);
const materialsData = ref<Api.Cut.BarItem[]>(request?.rowMaterials || []);

const newMaterialLength = ref(request?.newMaterialLength || 600);
const loss = ref(request?.loss || 0);
const utilizationWeight = ref(request?.utilizationWeight || 1);
const group = ref(false);
const cutResult = ref<Api.Cut.BarResult[] | null>(response || null);
const scaleFactor = ref(1);
const canvasWrapper = ref<HTMLDivElement | null>(null);
const containerWidth = ref(800); // 动态容器宽度

function goBack() {
  router.back();
}

// item 表格
const itemColumns = [
  { title: '材料类型', key: 'label' },
  { title: '长度(cm)', key: 'length' },
  { title: '数量', key: 'quantity' }
];

// material 表格
const materialColumns = [
  { title: '材料类型', key: 'label' },
  { title: '长度(cm)', key: 'length' },
  { title: '数量', key: 'quantity' }
];

// 统计信息
const result = computed(() => {
  if (!cutResult.value || cutResult.value.length === 0) {
    return {
      totalMaterials: 0,
      totalLength: 0,
      totalUsed: 0,
      totalRemaining: 0,
      usagePercent: '0.00'
    };
  }

  const totalMaterials = cutResult.value.length;
  let totalLength = 0;
  let totalUsed = 0;
  let totalRemaining = 0;

  cutResult.value.forEach((item: Api.Cut.BarResult) => {
    totalLength += item.totalLength;
    totalUsed += item.used;
    totalRemaining += item.remaining;
  });

  return {
    totalMaterials,
    totalLength: totalLength.toFixed(2),
    totalUsed: totalUsed.toFixed(2),
    totalRemaining: totalRemaining.toFixed(2),
    usagePercent: ((totalUsed / totalLength) * 100).toFixed(2)
  };
});

// 裁剪图示排序: 同类型材料相邻展示
function compareByMaterialType(a: Api.Cut.BarResult, b: Api.Cut.BarResult) {
  const ta = a.materialType ?? '';
  const tb = b.materialType ?? '';
  if (ta !== tb) return ta < tb ? -1 : 1;
  return 0;
}

const processedResult = computed(() => {
  if (!cutResult.value) return [];

  if (group.value === false) {
    // 同类型材料排在一起, 类型内保持原根序
    return cutResult.value
      .slice()
      .sort((a, b) => compareByMaterialType(a, b) || a.index - b.index);
  }

  // 按 materialType + cuts + remaining 归一化 key (不同类型不合并)
  const map = new Map<string, any>();
  cutResult.value.forEach((item: Api.Cut.BarResult) => {
    const cutsKey = item.cuts
      .slice()
      .sort((a, b) => a - b)
      .join(',');
    const key = `${item.materialType ?? ''}|${cutsKey}|${item.remaining}`;
    if (!map.has(key)) {
      map.set(key, { ...item, count: 1 });
    } else {
      map.get(key).count = map.get(key).count + 1;
    }
  });

  // 同类型材料排在一起, 类型内按剩余长度从小到大
  return Array.from(map.values()).sort(
    (a, b) => compareByMaterialType(a, b) || a.remaining - b.remaining
  );
});

// 颜色池
const randomColors = Array.from({ length: 50 }, (_, i) => `hsl(${(i * 30) % 360}, 70%, 50%)`);

// 所有材料中的最大总长, 预计算避免模板里对每个 cut 重复展开计算(O(n²))
const maxTotalLength = computed(() => {
  if (!cutResult.value || cutResult.value.length === 0) return 1;
  return Math.max(...cutResult.value.map(d => d.totalLength));
});

// 缩放 + 拖动(具名句柄, 便于卸载时移除)
let isDragging = false;
let startX = 0;
let scrollLeft = 0;

const handleWheel = (e: WheelEvent) => {
  e.preventDefault();
  scaleFactor.value += e.deltaY * -0.001;
  scaleFactor.value = Math.min(Math.max(0.5, scaleFactor.value), 3);
};
const handleMouseDown = (e: MouseEvent) => {
  isDragging = true;
  startX = e.pageX - canvasWrapper.value!.offsetLeft;
  scrollLeft = canvasWrapper.value!.scrollLeft;
};
const handleMouseUp = () => {
  isDragging = false;
};
const handleMouseLeave = () => {
  isDragging = false;
};
const handleMouseMove = (e: MouseEvent) => {
  if (!isDragging) return;
  e.preventDefault();
  const x = e.pageX - canvasWrapper.value!.offsetLeft;
  const walk = (x - startX) * 1.5;
  canvasWrapper.value!.scrollLeft = scrollLeft - walk;
};

onMounted(() => {
  if (canvasWrapper.value) {
    containerWidth.value = canvasWrapper.value.clientWidth;

    canvasWrapper.value.addEventListener('wheel', handleWheel, { passive: false });
    canvasWrapper.value.addEventListener('mousedown', handleMouseDown);
    canvasWrapper.value.addEventListener('mouseup', handleMouseUp);
    canvasWrapper.value.addEventListener('mouseleave', handleMouseLeave);
    canvasWrapper.value.addEventListener('mousemove', handleMouseMove);
  }
});

onUnmounted(() => {
  const el = canvasWrapper.value;
  if (el) {
    el.removeEventListener('wheel', handleWheel);
    el.removeEventListener('mousedown', handleMouseDown);
    el.removeEventListener('mouseup', handleMouseUp);
    el.removeEventListener('mouseleave', handleMouseLeave);
    el.removeEventListener('mousemove', handleMouseMove);
  }
});
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
    <!-- 输入区域 -->
    <NCard title="材料裁剪可视化" size="large" class="mb-4">
      <h3>裁剪尺寸</h3>
      <NDataTable :columns="itemColumns" :data="itemsData" />
      <h3 class="mt-6">材料库存</h3>
      <NDataTable :columns="materialColumns" :data="materialsData" />

      <h3 class="mt-6">参数配置</h3>
      <div class="mb-4 flex items-center gap-6">
        <div class="flex items-center gap-2">
          <span class="w-24">新材料长度</span>
          <NInputNumber v-model:value="newMaterialLength" disabled class="w-40" />
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">切割损耗</span>
          <NInputNumber v-model:value="loss" disabled class="w-40" />
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">利用率权重</span>
          <NSlider v-model:value="utilizationWeight" disabled :min="1" :max="8" :step="0.1" class="w-40" />
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">聚合显示</span>
          <NSwitch v-model:value="group" class="w-40" />
        </div>
      </div>

      <div class="mt-4 flex gap-2">
        <BarPrinter :data="cutResult" />
      </div>
    </NCard>

    <!-- 结果统计 -->
    <NCard title="结果统计" size="large" class="mb-4">
      <p>
        材料总数: {{ result.totalMaterials }} 根 | 总长度: {{ result.totalLength }} cm | 已用长度:
        {{ result.totalUsed }} cm | 剩余长度: {{ result.totalRemaining }} cm | 使用率: {{ result.usagePercent }}%
      </p>
    </NCard>

    <!-- 裁剪图示 -->
    <NCard title="裁剪图示" size="large">
      <div ref="canvasWrapper" class="cursor-grab overflow-x-auto border border-gray-300 rounded-md p-4">
        <div class="origin-top-left" :style="{ transform: `scale(${scaleFactor})` }">
          <div v-for="item in processedResult" :key="item.index" class="mb-6">
            <!-- 标签 -->
            <div class="mb-1 font-bold">
              材料 #{{ item.index }} (总长: {{ item.totalLength }}cm, 已用: {{ item.used }}cm, 剩余:
              {{ item.remaining }}cm) * {{ item.count || 1 }} 根
            </div>

            <!-- 条形图 -->
            <div class="h-10 flex">
              <div
                v-for="(cut, idx) in item.cuts"
                :key="idx"
                class="flex items-center justify-center border border-white text-xs text-white"
                :style="{
                  width: Number(cut) * (containerWidth / maxTotalLength) + 'px',
                  backgroundColor: randomColors[Number(idx) % randomColors.length]
                }"
              >
                {{ cut }}cm
              </div>

              <div
                v-if="item.remaining > 0"
                class="flex items-center justify-center bg-gray-300 text-xs text-black"
                :style="{
                  width: item.remaining * (containerWidth / maxTotalLength) + 'px'
                }"
              >
                剩余{{ item.remaining }}cm
              </div>
            </div>
          </div>
        </div>
      </div>
    </NCard>
  </div>
</template>
