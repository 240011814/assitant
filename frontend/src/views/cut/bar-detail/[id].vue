<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { NButton, NCard, NDataTable, NInputNumber, NResult, NSpin, useMessage } from 'naive-ui';
import { $t } from '@/locales';
import { fetchCutRecord } from '@/service/api';

const message = useMessage();
const route = useRoute();
const router = useRouter();

const loading = ref(false);
const parseError = ref(false);

// 内容全部在按 id 拉取记录后填充
const itemsData = ref<Api.Cut.BarItem[]>([]);
const materialsData = ref<Api.Cut.BarItem[]>([]);
const newMaterials = ref<Array<{ label?: string; length: number }>>([]);
const loss = ref(0);
const solveModeLabel = ref($t('page.cut.solveModeFast'));
const protectEnabled = ref(false);
const protectMin = ref<number | null>(null);
const protectMax = ref<number | null>(null);
const group = ref(false);
const cutResult = ref<Api.Cut.BarResult[] | null>(null);
const summaryData = ref<Api.Cut.BarSummary | null>(null);
const scaleFactor = ref(1);
const canvasWrapper = ref<HTMLDivElement | null>(null);
const containerWidth = ref(800); // 动态容器宽度

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
    const req = JSON.parse(data.request) as Api.Cut.BarRequest & {
      rowItems?: Api.Cut.BarItem[];
      rowMaterials?: Api.Cut.BarItem[];
    };
    // 兼容新旧记录: 旧记录响应为数组, 新记录响应为 { results, summary }
    const raw = JSON.parse(data.response) as Api.Cut.BarResult[] | Api.Cut.BarCutResponse;
    itemsData.value = req.rowItems ?? [];
    materialsData.value = req.rowMaterials ?? [];
    // 多规格回显 (旧记录回退单一规格)
    newMaterials.value = req.newMaterials?.length
      ? req.newMaterials.map(row => ({ label: row.label, length: row.length }))
      : [{ label: '', length: req.newMaterialLength || 600 }];
    loss.value = req.loss ?? 0;
    solveModeLabel.value = req.mode === 'precise' ? $t('page.cut.solveModePreciseCloud') : $t('page.cut.solveModeFastLocal');
    protectEnabled.value = req.protectEnabled ?? false;
    protectMin.value = req.protectMin ?? null;
    protectMax.value = req.protectMax ?? null;
    cutResult.value = Array.isArray(raw) ? raw : raw.results;
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

// item 表格
const itemColumns = [
  { title: $t('page.cut.materialType'), key: 'label' },
  { title: $t('page.cut.lengthCm'), key: 'length' },
  { title: $t('page.cut.quantity'), key: 'quantity' }
];

// material 表格
const materialColumns = [
  { title: $t('page.cut.materialType'), key: 'label' },
  { title: $t('page.cut.lengthCm'), key: 'length' },
  { title: $t('page.cut.quantity'), key: 'quantity' }
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

// 按材料类型分组统计表 (取保存响应里的 summary.byMaterialType, 旧记录无此字段则不展示)
const typeSummaryColumns = [
  {
    title: $t('page.cut.materialType'),
    key: 'materialType',
    render: (row: Api.Cut.BarMaterialTypeSummary) => row.materialType?.trim() || $t('page.cut.summaryNewMaterial')
  },
  { title: $t('page.cut.summaryMaterialCount'), key: 'count' },
  { title: $t('page.cut.summaryMaterialLengthCm'), key: 'totalMaterialLength', render: (row: Api.Cut.BarMaterialTypeSummary) => row.totalMaterialLength.toFixed(2) },
  { title: $t('page.cut.summaryCutLengthCm'), key: 'totalCutLength', render: (row: Api.Cut.BarMaterialTypeSummary) => row.totalCutLength.toFixed(2) },
  { title: $t('page.cut.summaryRemainingCm'), key: 'totalRemaining', render: (row: Api.Cut.BarMaterialTypeSummary) => row.totalRemaining.toFixed(2) },
  { title: $t('page.cut.summaryScrapCount'), key: 'scrapCount' },
  { title: $t('page.cut.utilizationPercent'), key: 'utilization' }
];

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
  // Ctrl+滚轮缩放; 无修饰键时不拦截, 悬停图示区仍可正常滚动页面
  if (!e.ctrlKey) return;
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
    <!-- 输入区域 -->
    <NCard :title="$t('page.cut.vizTitle')" size="large" class="mb-4">
      <h3>{{ $t('page.cut.cutSizes') }}</h3>
      <NDataTable :columns="itemColumns" :data="itemsData" />
      <h3 class="mt-6">{{ $t('page.cut.materialStock') }}</h3>
      <NDataTable :columns="materialColumns" :data="materialsData" />

      <h3 class="mt-6">{{ $t('page.cut.paramsConfig') }}</h3>
      <div class="mb-4 flex flex-wrap items-start gap-6">
        <div class="flex items-start gap-2">
          <span class="w-24 pt-1">{{ $t('page.cut.newMaterialSpec') }}</span>
          <div class="flex flex-col gap-2">
            <div v-for="(row, idx) in newMaterials" :key="idx" class="flex items-center gap-2">
              <span class="w-32 text-gray-600">{{ row.label?.trim() || $t('page.cut.unnamedSpec') }}</span>
              <NInputNumber :value="row.length" disabled class="w-40" />
            </div>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">{{ $t('page.cut.loss') }}</span>
          <NInputNumber v-model:value="loss" disabled class="w-40" />
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">{{ $t('page.cut.solveMode') }}</span>
          <NInput :value="solveModeLabel" disabled class="w-52" />
        </div>
        <div v-if="protectEnabled" class="flex items-center gap-2">
          <span class="w-24">{{ $t('page.cut.materialProtect') }}</span>
          <span class="whitespace-nowrap text-gray-600">
            {{ $t('page.cut.protectRange', { min: protectMin ?? '-', max: protectMax ?? '-' }) }}
          </span>
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">{{ $t('page.cut.aggregateDisplay') }}</span>
          <NSwitch v-model:value="group" class="w-40" />
        </div>
      </div>

      <div class="mt-4 flex gap-2">
        <BarPrinter :data="processedResult" />
      </div>
    </NCard>

    <!-- 结果统计 -->
    <NCard :title="$t('page.cut.summaryTitle')" size="large" class="mb-4">
      <p>
        {{
          $t('page.cut.barSummary', {
            count: result.totalMaterials,
            total: result.totalLength,
            used: result.totalUsed,
            remaining: result.totalRemaining,
            usage: result.usagePercent
          })
        }}
      </p>
      <NDataTable
        v-if="summaryData?.byMaterialType?.length"
        class="mt-4"
        size="small"
        :columns="typeSummaryColumns"
        :data="summaryData.byMaterialType"
        :bordered="false"
      />
    </NCard>

    <!-- 裁剪图示 -->
    <NCard :title="$t('page.cut.cutChart')" size="large">
      <div ref="canvasWrapper" class="cursor-grab overflow-x-auto border border-gray-300 rounded-md p-4">
        <div class="origin-top-left" :style="{ transform: `scale(${scaleFactor})` }">
          <div v-for="item in processedResult" :key="item.index" class="mb-6">
            <!-- 标签 -->
            <div class="mb-1 font-bold">
              {{ $t('page.cut.barLabel', { index: item.index }) }}
              {{
                $t('page.cut.barStats', { total: item.totalLength, used: item.used, remaining: item.remaining, count: item.count || 1 })
              }}
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
                {{ $t('page.cut.tailRemaining', { remaining: item.remaining }) }}
              </div>
            </div>
          </div>
        </div>
      </div>
    </NCard>
    </div>
  </NSpin>
</template>
