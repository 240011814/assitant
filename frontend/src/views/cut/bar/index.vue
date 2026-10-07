<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref } from 'vue';
import { NButton, NGi, NGrid, NInput, NInputNumber, NModal, NSelect, NSpin, NStatistic, NTooltip, useMessage } from 'naive-ui';
import { $t } from '@/locales';
import { addCutScraps, cutBar, fetchCutScraps } from '@/service/api';
import ScrapLibraryModal from '@/components/cut/ScrapLibraryModal.vue';
import { exportBarCutPDF, exportBarCutPNG, printBarCut } from './cut-export';

interface NewMaterialRow {
  label: string;
  length: number | null;
}

/** 旧料行: 带入库存余料时保留来源条目 id, 保存记录时按此扣减库存 */
interface MaterialRow {
  label?: string;
  length: number;
  quantity: number;
  invId?: number;
}

const message = useMessage();

const itemsData = ref<Api.Cut.BarItem[]>([]);
const materialsData = ref<MaterialRow[]>([]);

const itemLength = ref<number | null>(null);
const itemQty = ref<number | null>(null);
const itemType = ref<string | null>(null);
const matLabel = ref<string | null>(null);
const matLength = ref<number | null>(null);
const matQty = ref<number | null>(null);
// 材料类型候选: 来自一维余料库存(旧料库)的类型名, 支持手动输入新类型
const materialTypes = ref<string[]>([]);
// 新材料多规格动态行(至少一行)
const newMaterialRows = ref<NewMaterialRow[]>([{ label: '', length: 600 }]);
const loss = ref(0.2);
const utilizationWeight = ref(4);
const group = ref(false);
// 求解模式: fast=内置 DP+贪心; precise=OR-Tools 精确求解 (服务端未配置时自动回退 fast)
const solveMode = ref<'fast' | 'precise'>('fast');
const solveModeOptions = [
  { label: '快速 (DP+贪心)', value: 'fast' },
  { label: '精确 (OR-Tools)', value: 'precise' }
];
const cutResult = ref<Api.Cut.BarResult[] | null>(null);
const summaryData = ref<Api.Cut.BarSummary | null>(null);
const loading = ref(false);
const disabledPrint = ref(true);
const scaleFactor = ref(1);
const saveData = ref<Api.Cut.RecordRequest | null>(null);
const canvasWrapper = ref<HTMLDivElement | null>(null);
const containerWidth = ref(800); // 动态容器宽度

// 余料入库 / 旧料库 / 导出 / 打印状态
const scrapModalShow = ref(false);
const scrapStocking = ref(false);
const scrapStockedIn = ref(false);
const exporting = ref<'png' | 'pdf' | null>(null);
const printing = ref(false);

const canStockIn = computed(
  () => (summaryData.value?.scrapCount ?? 0) > 0 && (cutResult.value ?? []).some(item => item.remaining > 0)
);

function fmtLen(n: number): string {
  return Number.isInteger(n) ? String(n) : n.toFixed(1);
}

// 材料类型下拉候选: 库存类型 + 表单中已出现的类型(含手动输入的新类型)
const materialTypeOptions = computed(() => {
  const set = new Set<string>(materialTypes.value);
  const add = (value?: string | null) => {
    const type = value?.trim();
    if (type) set.add(type);
  };
  itemsData.value.forEach(row => add(row.label));
  materialsData.value.forEach(row => add(row.label));
  newMaterialRows.value.forEach(row => add(row.label));
  add(itemType.value);
  add(matLabel.value);
  return [...set].map(type => ({ label: type, value: type }));
});

// 从一维余料库存(旧料库)拉取已有类型名作为候选 (优先材料类型, 兼容旧数据只填了名称的条目)
async function loadMaterialTypes() {
  const { data, error } = await fetchCutScraps({ scrapType: 1 });
  if (error || !data) return;
  const set = new Set<string>();
  data.forEach(item => {
    const type = item.materialType?.trim() || item.label?.trim();
    if (type) set.add(type);
  });
  materialTypes.value = [...set];
}

// 表格内可编辑的材料类型选择器: 支持下拉选择 + 手动输入新类型
function renderTypeSelect(row: Api.Cut.BarItem, index: number, list: 'items' | 'materials') {
  return h(NSelect, {
    value: row.label ?? null,
    options: materialTypeOptions.value,
    filterable: true,
    tag: true,
    clearable: false,
    size: 'small',
    placeholder: $t('page.cut.materialType'),
    onUpdateValue: (value: string | null) => {
      const target = list === 'items' ? itemsData.value : materialsData.value;
      target[index].label = value ?? undefined;
    }
  });
}

// item 表格
const itemColumns = [
  {
    title: '材料类型',
    key: 'label',
    width: 160,
    render(row: Api.Cut.BarItem, index: number) {
      return renderTypeSelect(row, index, 'items');
    }
  },
  { title: '长度(cm)', key: 'length' },
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
          onClick: () => removeFromList('items', index)
        },
        { default: () => $t('common.delete') }
      );
    }
  }
];

// material 表格(类型名列可直接编辑)
const materialColumns = [
  {
    title: '材料类型',
    key: 'label',
    width: 160,
    render(row: Api.Cut.BarItem, index: number) {
      return renderTypeSelect(row, index, 'materials');
    }
  },
  { title: '长度(cm)', key: 'length' },
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
          onClick: () => removeFromList('materials', index)
        },
        { default: () => $t('common.delete') }
      );
    }
  }
];

function addItem() {
  const type = itemType.value?.trim();
  if (type && itemLength.value && itemQty.value && itemQty.value > 0) {
    itemsData.value.push({ label: type, length: itemLength.value, quantity: itemQty.value });
    itemType.value = null;
    itemLength.value = null;
    itemQty.value = null;
  } else {
    message.error($t('page.cut.inputInvalid'));
  }
}

function addMaterial() {
  const type = matLabel.value?.trim();
  if (type && matLength.value && matQty.value && matQty.value > 0) {
    materialsData.value.push({ label: type, length: matLength.value, quantity: matQty.value });
    matLabel.value = null;
    matLength.value = null;
    matQty.value = null;
  } else {
    message.error($t('page.cut.inputInvalid'));
  }
}

function removeFromList(list: 'items' | 'materials', index: number) {
  if (list === 'items') itemsData.value.splice(index, 1);
  else materialsData.value.splice(index, 1);
}

function clearAll() {
  itemsData.value = [];
  materialsData.value = [];
  cutResult.value = null;
  summaryData.value = null;
  saveData.value = null;
  scrapStockedIn.value = false;
  itemType.value = null;
  matLabel.value = null;
}

// 新材料多规格行操作
function addMaterialRow() {
  newMaterialRows.value.push({ label: '', length: null });
}

function removeMaterialRow(index: number) {
  if (newMaterialRows.value.length > 1) {
    newMaterialRows.value.splice(index, 1);
  }
}

// 裁剪尺寸引用的材料类型若没有同名新材料规格, 自动补一条(使用当前新材料长度)
function ensureSpecsForItemTypes() {
  const defaultLength = newMaterialRows.value.find(row => row.length && row.length > 0)?.length ?? null;
  const existing = new Set(newMaterialRows.value.map(row => row.label.trim()).filter(Boolean));
  const required = new Set(itemsData.value.map(row => row.label?.trim()).filter(Boolean) as string[]);
  required.forEach(type => {
    if (!existing.has(type)) {
      newMaterialRows.value.push({ label: type, length: defaultLength });
      existing.add(type);
    }
  });
}

// 获取数据
async function fetchData() {
  if (itemsData.value.length === 0) {
    message.error($t('page.cut.inputItemsRequired'));
    return;
  }
  // 材料类型必填校验
  if (itemsData.value.some(row => !row.label?.trim())) {
    message.error($t('page.cut.itemMaterialTypeRequired'));
    return;
  }
  // 多规格校验: 每行长度必须 > 0
  const invalidRow = newMaterialRows.value.find(row => !row.length || row.length <= 0);
  if (invalidRow) {
    message.error($t('page.cut.newMaterialInvalid'));
    return;
  }

  // 裁剪尺寸引用的类型若没有同名新材料规格, 自动补一条(使用当前新材料长度)
  ensureSpecsForItemTypes();

  // 零件按材料类型分组, 指定 spec 后只从同名材料规格上切
  const items: Array<number | { length: number; spec?: string }> = itemsData.value.flatMap((i: Api.Cut.BarItem) =>
    Array.from({ length: i.quantity }, () => ({ length: i.length, spec: i.label?.trim() }))
  );
  // 旧料构造为对象数组(带类型名), 空长度行过滤
  const materials: Array<number | { label?: string; length: number }> = materialsData.value.flatMap(row => {
    if (!row.length || row.length <= 0) return [];
    const label = row.label?.trim();
    return Array.from({ length: row.quantity }, () => ({ label: label ? label : undefined, length: row.length }));
  });

  // 构造多规格(不传时后端走 newMaterialLength 单规格, 这里始终传并保留 newMaterialLength 兼容)
  const newMaterials: Api.Cut.NewMaterialSpec[] = newMaterialRows.value.map(row => ({
    label: row.label.trim() ? row.label.trim() : undefined,
    length: row.length as number
  }));
  const request: Api.Cut.BarRequest = {
    items,
    materials,
    newMaterialLength: newMaterials[0]!.length,
    newMaterials,
    loss: loss.value,
    utilizationWeight: utilizationWeight.value
  };
  if (solveMode.value === 'precise') {
    request.mode = 'precise';
  }

  loading.value = true;
  disabledPrint.value = true;
  try {
    const { data, error } = await cutBar(request);
    if (error || !data) return;
    cutResult.value = data.results;
    summaryData.value = data.summary;
    scrapStockedIn.value = false;
    saveData.value = {
      type: '1',
      request: JSON.stringify({ rowItems: itemsData.value, rowMaterials: materialsData.value, ...request }),
      // 序列化完整响应(results + summary), 供详情页展示
      response: JSON.stringify(data),
      name: ``,
      // 从库存带入且被本次切割消耗的旧料, 保存时扣减库存
      deductScraps: collectDeductScraps()
    };
    disabledPrint.value = false;
  } finally {
    loading.value = false;
  }
}

// 从库存带入的旧料行 (invId) 的实际消耗根数 → 保存记录时随请求扣减库存。
// 消耗口径: 结果里每根 (materialType, totalLength) 匹配的旧料即消耗一根;
// 多行导入同一 (类型,长度) 时按行序分摊, 上限为该行数量。
function collectDeductScraps(): Array<{ id: number; count: number }> {
  if (!cutResult.value) return [];
  const consumedPool = new Map<string, number>();
  for (const r of cutResult.value) {
    const key = `${r.materialType ?? ''}|${r.totalLength}`;
    consumedPool.set(key, (consumedPool.get(key) ?? 0) + 1);
  }
  const out = new Map<number, number>();
  for (const row of materialsData.value) {
    if (!row.invId) continue;
    const key = `${row.label?.trim() ?? ''}|${row.length}`;
    const pool = consumedPool.get(key) ?? 0;
    if (pool <= 0) continue;
    const take = Math.min(pool, row.quantity);
    out.set(row.invId, (out.get(row.invId) ?? 0) + take);
    consumedPool.set(key, pool - take);
  }
  return Array.from(out.entries()).map(([id, count]) => ({ id, count }));
}

// 余料一键入库: remaining>0 的每根料登记为一维余料; 材料类型取该根料的来源类型 (新料规格名/旧料类型名)
async function stockInScraps() {
  const scrapRows = (cutResult.value ?? []).filter(item => item.remaining > 0);
  if (scrapRows.length === 0) return;
  scrapStocking.value = true;
  try {
    const { error } = await addCutScraps(
      scrapRows.map(item => ({
        scrapType: 1 as const,
        materialType: item.materialType?.trim() || undefined,
        lengthValue: item.remaining,
        quantity: 1,
        note: $t('page.cut.scrapFromCutting')
      }))
    );
    if (error) return;
    scrapStockedIn.value = true;
    message.success($t('page.cut.scrapStockInSuccess', { count: scrapRows.length }));
  } finally {
    scrapStocking.value = false;
  }
}

// 旧料库带入表单: 库存条目转材料行(保留类型名)
function applyScraps(rows: Api.Cut.CutScrap[]) {
  rows.forEach(row => {
    const type = row.materialType?.trim() || row.label?.trim();
    materialsData.value.push({
      label: type ? type : $t('page.cut.scrapMaterialLabel'),
      length: row.lengthValue,
      quantity: row.quantity,
      invId: row.id
    });
  });
  // 库存可能新增了类型, 刷新下拉候选
  loadMaterialTypes();
  message.success($t('page.cut.scrapApplied', { count: rows.length }));
}

// 裁剪图示排序: 同类型材料相邻展示, 类型内再按根序(聚合模式按剩余长度)排
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

// 导出 PNG / PDF (与页面展示一致: 聚合模式下按聚合行导出, 行标 ×N根)
async function exportPNG() {
  if (!cutResult.value?.length) return;
  exporting.value = 'png';
  try {
    await exportBarCutPNG(processedResult.value, summaryData.value);
  } catch (e) {
    console.error(e);
    message.error($t('page.cut.exportFailed'));
  } finally {
    exporting.value = null;
  }
}

async function exportPDF() {
  if (!cutResult.value?.length) return;
  exporting.value = 'pdf';
  try {
    await exportBarCutPDF(processedResult.value, summaryData.value);
  } catch (e) {
    console.error(e);
    message.error($t('page.cut.exportFailed'));
  } finally {
    exporting.value = null;
  }
}

// 直接打印切割图 (PDF autoPrint, 横向 A4 按页渲染)
async function printChart() {
  if (!cutResult.value?.length) return;
  printing.value = true;
  try {
    const opened = await printBarCut(processedResult.value, summaryData.value);
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
  loadMaterialTypes();
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
  <div class="p-4">
    <!-- 输入区域 -->
    <NCard title="材料裁剪可视化" size="large" class="mb-4">
      <h3>裁剪尺寸</h3>
      <div class="mb-2 flex items-center gap-2">
        <NSelect
          v-model:value="itemType"
          :options="materialTypeOptions"
          filterable
          tag
          :placeholder="$t('page.cut.materialType')"
          class="w-40"
        />
        <NInputNumber v-model:value="itemLength" placeholder="长度" class="w-40" />
        <NInputNumber v-model:value="itemQty" placeholder="数量" class="w-32" />
        <NButton type="primary" @click="addItem">{{ $t('page.cut.addItem') }}</NButton>
      </div>
      <NDataTable :columns="itemColumns" :data="itemsData" />

      <h3 class="mt-6">材料库存</h3>
      <div class="mb-2 flex items-center gap-2">
        <NSelect
          v-model:value="matLabel"
          :options="materialTypeOptions"
          filterable
          tag
          :placeholder="$t('page.cut.materialType')"
          class="w-40"
        />
        <NInputNumber v-model:value="matLength" placeholder="长度" class="w-40" />
        <NInputNumber v-model:value="matQty" placeholder="数量" class="w-32" />
        <NButton type="primary" @click="addMaterial">{{ $t('page.cut.addMaterial') }}</NButton>
        <NButton type="info" secondary @click="scrapModalShow = true">{{ $t('page.cut.scrapLibrary') }}</NButton>
      </div>
      <NDataTable :columns="materialColumns" :data="materialsData" />

      <h3 class="mt-6">参数配置</h3>
      <div class="mb-4 flex flex-wrap items-start gap-6">
        <div class="flex items-start gap-2">
          <span class="w-24 pt-1">新材料规格</span>
          <div class="flex flex-col gap-2">
            <div v-for="(row, idx) in newMaterialRows" :key="idx" class="flex items-center gap-2">
              <NInput v-model:value="row.label" :placeholder="$t('page.cut.newMaterialLabel')" class="w-32" />
              <NInputNumber v-model:value="row.length" :placeholder="$t('page.cut.newMaterialLength')" class="w-40" :min="0" />
              <NButton size="small" type="error" quaternary :disabled="newMaterialRows.length <= 1" @click="removeMaterialRow(idx)">
                {{ $t('common.delete') }}
              </NButton>
            </div>
            <NButton size="small" dashed class="w-32" @click="addMaterialRow">+ {{ $t('common.add') }}</NButton>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">切割损耗</span>
          <NInputNumber v-model:value="loss" class="w-40" />
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">利用率权重</span>
          <NSlider v-model:value="utilizationWeight" :min="1" :max="8" :step="0.1" class="w-40" />
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">聚合显示</span>
          <NSwitch v-model:value="group" class="w-40" />
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">求解模式</span>
          <NTooltip trigger="hover" placement="top-start">
            <template #trigger>
              <NSelect v-model:value="solveMode" class="w-44" :options="solveModeOptions" />
            </template>
            精确模式使用 OR-Tools 列生成求全局更优解, 由服务端 BAOSTOCK_API_URL 指向的求解服务计算; 未配置或求解失败时自动回退快速模式
          </NTooltip>
        </div>
      </div>

      <div class="mt-4 flex gap-2">
        <NButton type="primary" @click="fetchData">{{ $t('page.cut.startCutting') }}</NButton>
        <BarPrinter v-if="!disabledPrint" :data="cutResult" />
        <SaveCutRecord :data="saveData" @saved="saveData = null"></SaveCutRecord>
        <NButton type="warning" @click="clearAll">{{ $t('page.cut.clearAll') }}</NButton>
      </div>
    </NCard>

    <!-- 结果统计: 汇总卡片 + 余料入库 + 导出 -->
    <NCard v-if="summaryData" size="large" class="mb-4">
      <template #header>
        {{ $t('page.cut.summaryTitle') }}
      </template>
      <template #header-extra>
        <div class="flex items-center gap-2">
          <NButton
            size="small"
            type="success"
            :loading="scrapStocking"
            :disabled="!canStockIn || scrapStockedIn"
            @click="stockInScraps"
          >
            {{ scrapStockedIn ? $t('page.cut.scrapStockedIn') : $t('page.cut.scrapStockIn') }}
          </NButton>
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
      <NGrid :x-gap="12" :y-gap="12" cols="2 s:3 m:6" responsive="screen">
        <NGi>
          <NStatistic :label="$t('page.cut.summaryMaterialCount')" :value="summaryData.materialCount" />
        </NGi>
        <NGi>
          <NStatistic :label="$t('page.cut.summaryMaterialLength')" :value="`${fmtLen(summaryData.totalMaterialLength)} cm`" />
        </NGi>
        <NGi>
          <NStatistic :label="$t('page.cut.summaryCutLength')" :value="`${fmtLen(summaryData.totalCutLength)} cm`" />
        </NGi>
        <NGi>
          <NStatistic :label="$t('page.cut.summaryUtilization')" :value="`${summaryData.utilization}%`" />
        </NGi>
        <NGi>
          <NStatistic :label="$t('page.cut.summaryRemaining')" :value="`${fmtLen(summaryData.totalRemaining)} cm`" />
        </NGi>
        <NGi>
          <NStatistic :label="$t('page.cut.summaryScrapCount')" :value="`${summaryData.scrapCount} ${$t('page.cut.unitBar')}`" />
        </NGi>
      </NGrid>
    </NCard>

    <!-- 裁剪图示 -->
    <NCard title="裁剪图示" size="large">
      <div ref="canvasWrapper" class="cursor-grab overflow-x-auto border border-gray-300 rounded-md p-4">
        <div class="origin-top-left" :style="{ transform: `scale(${scaleFactor})` }">
          <div v-for="item in processedResult" :key="item.index" class="mb-6">
            <!-- 标签 -->
            <div class="mb-1 font-bold">
              材料 #{{ item.index }}
              <span class="ml-1 font-normal text-gray-500">[{{ item.materialType || '新材料' }}]</span>
              (总长: {{ item.totalLength }}cm, 已用: {{ item.used }}cm, 剩余:
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

    <!-- 旧料库弹窗 -->
    <ScrapLibraryModal v-model:show="scrapModalShow" :scrap-type="1" @apply="applyScraps" />

    <!-- 加载中弹窗 -->
    <NModal v-model:show="loading" preset="dialog" title="计算中...">
      <div class="flex flex-col items-center justify-center p-6">
        <NSpin size="large" />
        <div class="mt-3">{{ $t('page.cut.loading') }}</div>
      </div>
    </NModal>
  </div>
</template>
