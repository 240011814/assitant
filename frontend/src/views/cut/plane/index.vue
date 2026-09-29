<script setup lang="ts">
import { computed, h, ref } from 'vue';
import { NButton, NGi, NGrid, NStatistic, NTag, useMessage } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { $t } from '@/locales';
import { cutBin } from '@/service/api';
import ScrapLibraryModal from '@/components/cut/ScrapLibraryModal.vue';
import { exportPlaneCutPDF, exportPlaneCutPNG, printPlaneCut } from './cut-export';

const message = useMessage();

// 响应式数据
const label = ref('');
const group = ref(false);
const strategy = ref('Guillotine');
const width = ref<number | null>(null);
const height = ref<number | null>(null);
const quantity = ref(1);
const newMaterialHeight = ref(200);
const newMaterialWidth = ref(200);
const materialName = ref('');
const materialWidth = ref<number | null>(null);
const materialHeight = ref<number | null>(null);
const materialCount = ref(1);
const saveData = ref<Api.Cut.RecordRequest | null>(null);
const items = ref<Api.Cut.Item[]>([]);
const materials = ref<Api.Cut.Item[]>([]);
const results = ref<Api.Cut.BinResult[]>([]);
const unplaced = ref<Api.Cut.UnplacedItem[]>([]);
const summaryData = ref<Api.Cut.PlaneSummary | null>(null);
const strategyOptions = [
  { label: '刀切法', value: 'Guillotine' },
  { label: '最大空闲法', value: 'MaxRects' }
];

// 旧料库 / 导出 / 打印状态
const scrapModalShow = ref(false);
const exporting = ref<'png' | 'pdf' | null>(null);
const printing = ref(false);

const loading = ref(false);

function fmtNum(n: number): string {
  return Number.isInteger(n) ? String(n) : n.toFixed(1);
}

// 未排入总件数
const unplacedTotal = computed(
  () => summaryData.value?.unplacedCount || unplaced.value.reduce((sum, item) => sum + item.quantity, 0)
);

// 未排入件表格
const unplacedColumns = computed<DataTableColumns<Api.Cut.UnplacedItem>>(() => [
  { title: $t('page.cut.unplacedLabel'), key: 'label' },
  {
    title: $t('page.cut.scrapSize'),
    key: 'size',
    render: row => `${fmtNum(row.width)} × ${fmtNum(row.height)} cm`
  },
  { title: $t('page.cut.scrapQuantity'), key: 'quantity', width: 80 },
  {
    title: $t('page.cut.unplacedReason'),
    key: 'reason',
    render: row =>
      h(
        NTag,
        { type: row.reason === 'oversized' ? 'error' : 'warning', size: 'small' },
        {
          default: () => (row.reason === 'oversized' ? $t('page.cut.reasonOversized') : $t('page.cut.reasonExhausted'))
        }
      )
  }
]);

// item 表格
const itemColumns = [
  { title: '标签', key: 'label' },
  { title: '宽(cm)', key: 'width' },
  { title: '高(cm)', key: 'height' },
  { title: '数量', key: 'quantity' },
  {
    title: '操作',
    key: 'actions',
    render(_row: any, index: number) {
      return h(
        NButton,
        {
          size: 'small',
          type: 'error',
          onClick: () => removeItem(index)
        },
        { default: () => $t('common.delete') }
      );
    }
  }
];

// material 表格
const materialColumns = [
  { title: '标签', key: 'label' },
  { title: '宽(cm)', key: 'width' },
  { title: '高(cm)', key: 'height' },
  { title: '数量', key: 'quantity' },
  {
    title: '操作',
    key: 'actions',
    render(_row: any, index: number) {
      return h(
        NButton,
        {
          size: 'small',
          type: 'error',
          onClick: () => removeMaterial(index)
        },
        { default: () => $t('common.delete') }
      );
    }
  }
];

// 添加项目
function addItem() {
  if (
    !label.value ||
    width.value === null ||
    height.value === null ||
    quantity.value < 1 ||
    width.value <= 0 ||
    height.value <= 0
  ) {
    message.error($t('page.cut.inputInvalid'));
    return;
  }

  const existingIndex = items.value.findIndex((item: Api.Cut.Item) => item.label === label.value);
  if (existingIndex !== -1) {
    items.value[existingIndex].quantity = quantity.value;
  } else {
    items.value.push({
      label: label.value,
      width: width.value,
      height: height.value,
      quantity: quantity.value
    });
  }

  clearItemInputs();
}

// 添加材料
function addMaterial() {
  if (
    !materialName.value ||
    materialWidth.value === null ||
    materialHeight.value === null ||
    materialCount.value < 1 ||
    materialWidth.value <= 0 ||
    materialHeight.value <= 0
  ) {
    message.error($t('page.cut.inputMaterialInvalid'));
    return;
  }

  materials.value.push({
    label: materialName.value,
    width: materialWidth.value,
    height: materialHeight.value,
    quantity: materialCount.value
  });

  clearMaterialInputs();
}

// 删除项目
function removeItem(index: number) {
  items.value.splice(index, 1);
}

// 删除材料
function removeMaterial(index: number) {
  materials.value.splice(index, 1);
}

// 清空所有
function clearAll() {
  items.value = [];
  materials.value = [];
  results.value = [];
  unplaced.value = [];
  summaryData.value = null;
}

// 清空输入框
function clearItemInputs() {
  label.value = '';
  width.value = null;
  height.value = null;
  quantity.value = 1;
}

function clearMaterialInputs() {
  materialName.value = '';
  materialWidth.value = null;
  materialHeight.value = null;
  materialCount.value = 1;
}

// 优化主逻辑
async function runOptimization() {
  if (items.value.length === 0) {
    message.error($t('page.cut.inputItemsRequired'));
    return;
  }

  const expandedItems: Api.Cut.Item[] = items.value.flatMap((item: Api.Cut.Item) => {
    const count = item.quantity ?? 0;
    if (count < 1) return [];
    return Array.from({ length: count }, (_, i) => ({
      label: `${item.label}_${i + 1}`,
      width: item.width,
      height: item.height
    }));
  });

  loading.value = true;
  try {
    const request = {
      items: expandedItems,
      materials: materials.value,
      width: newMaterialWidth.value,
      height: newMaterialHeight.value,
      strategy: strategy.value
    };
    const { data, error } = await cutBin(request);
    if (error || !data) return;

    results.value = data.results;
    unplaced.value = data.unplaced ?? [];
    summaryData.value = data.summary;
    if (data.results.length === 0) {
      message.warning($t('page.cut.noResultWarning'));
    }
    saveData.value = {
      type: '2',
      request: JSON.stringify({ rowItems: items.value, ...request }),
      // 序列化完整响应(results + unplaced + summary), 供详情页展示
      response: JSON.stringify(data),
      name: ``
    };
  } finally {
    loading.value = false;
  }
}

// 旧料库带入表单: 库存条目转材料行
function applyScraps(rows: Api.Cut.CutScrap[]) {
  rows.forEach(row => {
    materials.value.push({
      label: $t('page.cut.scrapMaterialLabel'),
      width: row.widthValue,
      height: row.heightValue,
      quantity: row.quantity
    });
  });
  message.success($t('page.cut.scrapApplied', { count: rows.length }));
}

// 导出 PNG (全部板材拼接纵向长图)
async function exportPNG() {
  if (results.value.length === 0) return;
  exporting.value = 'png';
  try {
    await exportPlaneCutPNG(results.value);
  } catch (e) {
    console.error(e);
    message.error($t('page.cut.exportFailed'));
  } finally {
    exporting.value = null;
  }
}

// 导出 PDF (逐板分页)
async function exportPDF() {
  if (results.value.length === 0) return;
  exporting.value = 'pdf';
  try {
    await exportPlaneCutPDF(results.value);
  } catch (e) {
    console.error(e);
    message.error($t('page.cut.exportFailed'));
  } finally {
    exporting.value = null;
  }
}

// 直接打印切割图 (PDF autoPrint)
async function printChart() {
  if (results.value.length === 0) return;
  printing.value = true;
  try {
    const opened = await printPlaneCut(results.value);
    if (!opened) {
      message.warning($t('page.cut.allowPopup'));
    }
  } catch (e) {
    console.error(e);
    message.error($t('page.cut.exportFailed'));
  } finally {
    printing.value = false;
  }
}
</script>

<template>
  <div class="p-4">
    <NCard title="材料裁剪可视化" size="large" class="mb-4">
      <!-- 添加切割项目 -->

      <h3 class="mb-3 text-lg font-semibold">裁剪尺寸</h3>
      <div class="mb-2 flex items-center gap-2">
        <NInput v-model:value="label" class="input-width" type="text" placeholder="标签" />
        <NInputNumber v-model:value="width" type="number" placeholder="宽(cm)" step="0.1" min="0.1" class="w-40" />
        <NInputNumber v-model:value="height" type="number" placeholder="高(cm)" step="0.1" min="0.1" class="w-40" />
        <NInputNumber v-model:value="quantity" type="number" placeholder="数量" class="w-40" min="1" />
        <NButton type="primary" @click="addItem">{{ $t('page.cut.addItem') }}</NButton>
      </div>

      <!-- 切割项目列表 -->

      <h3 class="mb-2 text-lg font-semibold">切割项目</h3>
      <NDataTable :columns="itemColumns" :data="items" />

      <!-- 添加剩余材料 -->

      <h3 class="mb-3 text-lg font-semibold">库存材料</h3>
      <div class="mb-2 flex items-center gap-2">
        <NInput v-model:value="materialName" type="text" placeholder="材料名称" class="input-width" />
        <NInputNumber
          v-model:value="materialWidth"
          type="number"
          placeholder="宽(cm)"
          step="0.1"
          min="0.1"
          class="w-40"
        />
        <NInputNumber
          v-model:value="materialHeight"
          type="number"
          placeholder="高(cm)"
          step="0.1"
          min="0.1"
          class="w-40"
        />
        <NInputNumber v-model:value="materialCount" type="number" placeholder="数量" class="w-40" min="1" />
        <NButton type="primary" @click="addMaterial">{{ $t('page.cut.addMaterial') }}</NButton>
        <NButton type="info" secondary @click="scrapModalShow = true">{{ $t('page.cut.scrapLibrary') }}</NButton>
      </div>

      <!-- 剩余材料列表 -->

      <h3 class="mb-2 text-lg font-semibold">库存材料</h3>
      <NDataTable :columns="materialColumns" :data="materials" />

      <h3 class="mt-6">参数配置</h3>
      <div class="mb-4 flex items-center gap-6">
        <div class="flex items-center gap-2">
          <span class="w-24">方案</span>
          <NSelect v-model:value="strategy" :options="strategyOptions" />
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">新材料长度</span>
          <NInputNumber v-model:value="newMaterialHeight" class="w-40" />
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">新材料高度</span>
          <NInputNumber v-model:value="newMaterialWidth" class="w-40" />
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">聚合显示</span>
          <NSwitch v-model:value="group" class="w-40" />
        </div>
      </div>

      <!-- 操作按钮 -->
      <div class="mt-4 flex gap-2">
        <NButton type="primary" @click="runOptimization">{{ $t('page.cut.startCutting') }}</NButton>
        <PlanePrinter :results="results" :materials="materials"></PlanePrinter>
        <SaveCutRecord :data="saveData" @saved="saveData = null"></SaveCutRecord>
        <NButton type="warning" @click="clearAll">{{ $t('page.cut.clearAll') }}</NButton>
      </div>
    </NCard>

    <!-- 结果统计: 汇总卡片 + 导出/打印 -->
    <NCard v-if="summaryData" size="large" class="mb-4">
      <template #header>
        {{ $t('page.cut.summaryTitle') }}
      </template>
      <template #header-extra>
        <div class="flex items-center gap-2">
          <NButton size="small" secondary type="primary" :loading="exporting === 'png'" @click="exportPNG">
            {{ $t('page.cut.exportPng') }}
          </NButton>
          <NButton size="small" secondary type="primary" :loading="exporting === 'pdf'" @click="exportPDF">
            {{ $t('page.cut.exportPdf') }}
          </NButton>
          <NButton size="small" secondary type="primary" :loading="printing" @click="printChart">
            {{ $t('page.cut.printChart') }}
          </NButton>
        </div>
      </template>
      <NGrid :x-gap="12" :y-gap="12" cols="2 s:3 m:5" responsive="screen">
        <NGi>
          <NStatistic :label="$t('page.cut.summaryBinCount')" :value="summaryData.binCount" />
        </NGi>
        <NGi>
          <NStatistic :label="$t('page.cut.summaryTotalArea')" :value="`${fmtNum(summaryData.totalArea)} cm²`" />
        </NGi>
        <NGi>
          <NStatistic :label="$t('page.cut.summaryUsedArea')" :value="`${fmtNum(summaryData.usedArea)} cm²`" />
        </NGi>
        <NGi>
          <NStatistic :label="$t('page.cut.summaryUtilization')" :value="`${summaryData.utilization}%`" />
        </NGi>
        <NGi>
          <NStatistic
            :label="$t('page.cut.summaryUnplacedCount')"
            :value="summaryData.unplacedCount"
            :value-style="{ color: summaryData.unplacedCount > 0 ? '#d03050' : undefined }"
          />
        </NGi>
      </NGrid>
    </NCard>

    <!-- 未排入件警示 -->
    <NAlert
      v-if="unplaced.length > 0"
      type="error"
      :closable="false"
      class="mb-4"
      :title="$t('page.cut.unplacedAlert', { count: unplacedTotal })"
    >
      <NDataTable size="small" :columns="unplacedColumns" :data="unplaced" />
    </NAlert>

    <PlaneCanvas :results="results" :group-data="group" :materials="materials"></PlaneCanvas>

    <!-- 旧料库弹窗 -->
    <ScrapLibraryModal v-model:show="scrapModalShow" :scrap-type="2" @apply="applyScraps" />

    <NModal v-model:show="loading" preset="dialog" title="计算中...">
      <div class="flex flex-col items-center justify-center p-6">
        <NSpin size="large" />
        <div class="mt-3">{{ $t('page.cut.loading') }}</div>
      </div>
    </NModal>
  </div>
</template>

<style scoped>
.input-width {
  width: 200px;
}
</style>
