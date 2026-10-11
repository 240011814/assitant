<script setup lang="ts">
import { computed, h, onActivated, onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { NButton, NGi, NGrid, NSelect, NStatistic, NTag, useMessage } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { $t } from '@/locales';
import { cutBin, fetchCutProducts, fetchCutRecord, fetchCutScraps } from '@/service/api';
import ScrapLibraryModal from '@/components/cut/ScrapLibraryModal.vue';
import ProductPickModal from '@/components/cut/ProductPickModal.vue';
import { parseProductSpec, productPlaneRows } from '@/components/cut/window-template';
import { exportPlaneCutPDF, exportPlaneCutPNG, printPlaneCut } from './cut-export';

const message = useMessage();

/** 新板材规格行 (至少一行; label 为材料类型名, 空 = 未命名新板材) */
interface NewBoardRow {
  label: string;
  width: number | null;
  height: number | null;
}

/** 旧料行: 带入库存余料时保留来源条目 id, 保存记录时按此扣减库存 */
interface MaterialRow {
  label: string;
  width: number;
  height: number;
  quantity: number;
  invId?: number;
}

// 响应式数据
const label = ref('');
const group = ref(false);
const strategy = ref('Guillotine');
const width = ref<number | null>(null);
const height = ref<number | null>(null);
const quantity = ref(1);
const itemSpec = ref<string | null>(null);
// 新材料规格行默认板材尺寸 (cm): 手动新增与玻璃导入自动补行均按此预填
const DEFAULT_BOARD_SIZE = 200;
// 新板材多规格动态行 (参照一维新材料规格)
const newMaterialRows = ref<NewBoardRow[]>([{ label: '', width: DEFAULT_BOARD_SIZE, height: DEFAULT_BOARD_SIZE }]);
const materialName = ref<string | null>(null);
const materialWidth = ref<number | null>(null);
const materialHeight = ref<number | null>(null);
const materialCount = ref(1);
// 材料类型候选: 来自二维余料库存(旧料库)的类型名, 支持手动输入新类型
const materialTypes = ref<string[]>([]);
const saveData = ref<Api.Cut.RecordRequest | null>(null);
const items = ref<Api.Cut.Item[]>([]);
const materials = ref<MaterialRow[]>([]);
const results = ref<Api.Cut.BinResult[]>([]);
const unplaced = ref<Api.Cut.UnplacedItem[]>([]);
const summaryData = ref<Api.Cut.PlaneSummary | null>(null);
// 切割损耗 (锯缝 cm): 相邻零件间距, 默认 0 不留缝
const loss = ref<number | null>(0);
const strategyOptions = [
  { label: $t('page.cut.strategyGuillotine'), value: 'Guillotine' },
  { label: $t('page.cut.strategyMaxRects'), value: 'MaxRects' },
  // OR-Tools 精确求解 (限时 2 分钟); 失败/超时直接报错, 不回退启发式 (与一维精确模式口径一致)
  { label: $t('page.cut.strategyPrecise'), value: 'Precise' }
];

// 旧料库 / 产品导入 / 导出 / 打印状态
const scrapModalShow = ref(false);
const productPickShow = ref(false);
const exporting = ref<'png' | 'pdf' | null>(null);
const printing = ref(false);

const loading = ref(false);

const route = useRoute();
const router = useRouter();

/** 把玻璃产品单预填为平面零件行 (替换现有清单, 与一维页导入口径一致); 返回导入件数与自动补的规格行数 */
function fillItemsFromGlassOrders(records: Api.Cut.CutProduct[]): { pieces: number; addedSpecs: number } {
  const rows = records.flatMap(record => productPlaneRows(parseProductSpec(record).items));
  items.value = rows.map(row => ({
    label: row.label,
    width: row.width,
    height: row.height,
    quantity: row.quantity,
    spec: row.spec
  }));
  // 新厚度没有同名新材料规格时自动补行 (默认板材尺寸, 可按实际板材调整)
  let addedSpecs = 0;
  rows.forEach(row => {
    const spec = row.spec?.trim();
    if (spec && !newMaterialRows.value.some(board => board.label.trim() === spec)) {
      newMaterialRows.value.push({ label: spec, width: DEFAULT_BOARD_SIZE, height: DEFAULT_BOARD_SIZE });
      addedSpecs++;
    }
  });
  // 库存可能新增了类型, 刷新下拉候选
  loadMaterialTypes();
  return { pieces: rows.reduce((sum, row) => sum + row.quantity, 0), addedSpecs };
}

function glassImportResult(count: number, result: { pieces: number; addedSpecs: number }) {
  if (result.pieces > 0) {
    message.success($t('page.cut.pdImported', { count, bars: result.pieces }));
    if (result.addedSpecs > 0) {
      message.warning($t('page.cut.planeSpecRowsAdded'));
    }
  } else {
    message.warning($t('page.cut.pdImportEmpty'));
  }
}

// 历史记录"重新编辑"跳入: 按 id 取回完整请求并回填表单 (invId 不回填, 重新保存不会重复扣库存)
async function reuseFromRecord(id: string) {
  const { data, error } = await fetchCutRecord(id);
  if (error || !data?.request) {
    message.error($t('page.cut.reuseLoadFailed'));
    return;
  }
  try {
    const req = JSON.parse(data.request) as Api.Cut.BinRequest & { rowItems?: Api.Cut.Item[] };
    items.value = req.rowItems?.length ? req.rowItems : groupExpandedItems(req.items ?? []);
    materials.value = (req.materials ?? []).map(m => ({
      label: m.label,
      width: m.width,
      height: m.height,
      quantity: m.quantity ?? 1
    }));
    newMaterialRows.value = req.newMaterials?.length
      ? req.newMaterials.map(m => ({ label: m.label ?? '', width: m.width, height: m.height }))
      : [{ label: '', width: req.width, height: req.height }];
    strategy.value = req.strategy || 'Guillotine';
    loss.value = req.loss ?? 0;
    results.value = [];
    unplaced.value = [];
    summaryData.value = null;
    saveData.value = null;
    message.success($t('page.cut.reuseLoaded'));
  } catch {
    message.error($t('page.cut.reuseLoadFailed'));
  }
}

// 旧记录无 rowItems 时, 把展开件归并回零件行 (去掉 _N 后缀, 同尺寸规格合并数量)
function groupExpandedItems(list: Api.Cut.Item[]): Api.Cut.Item[] {
  const map = new Map<string, Api.Cut.Item>();
  const out: Api.Cut.Item[] = [];
  for (const it of list) {
    const base = it.label.replace(/_\d+$/, '');
    const key = `${base}|${it.width}|${it.height}|${it.spec ?? ''}`;
    const existing = map.get(key);
    if (existing) {
      existing.quantity = (existing.quantity ?? 1) + 1;
    } else {
      const row: Api.Cut.Item = { label: base, width: it.width, height: it.height, quantity: 1, spec: it.spec };
      map.set(key, row);
      out.push(row);
    }
  }
  return out;
}

// keepAlive 页面: 每次激活刷新材料类型候选 (库存页可能增删了类型);
// 产品管理页"去裁剪"(玻璃类)跳入时消费 query.products 预填;
// 历史记录"重新编辑"跳入时消费 query.reuse 回填表单
onActivated(async () => {
  loadMaterialTypes();
  const reuse = route.query.reuse;
  if (typeof reuse === 'string' && reuse) {
    router.replace({ query: { ...route.query, reuse: undefined } });
    await reuseFromRecord(reuse);
    return;
  }
  const raw = route.query.products;
  if (typeof raw !== 'string' || !raw) return;
  // 先清掉 query, 避免切回该页时重复导入
  router.replace({ query: { ...route.query, products: undefined } });
  const ids = raw.split(',').map(Number).filter(n => Number.isInteger(n) && n > 0);
  if (ids.length === 0) return;
  const { data, error } = await fetchCutProducts();
  if (error || !data) return;
  const selected = data.records.filter(record => ids.includes(record.id));
  glassImportResult(selected.length, fillItemsFromGlassOrders(selected));
});

// 导入产品弹窗确认: 预填所选玻璃产品单
function handleProductPick(records: Api.Cut.CutProduct[]) {
  glassImportResult(records.length, fillItemsFromGlassOrders(records));
}

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

// 材料类型下拉候选: 库存类型 + 表单中已出现的类型(含手动输入的新类型)
const materialTypeOptions = computed(() => {
  const set = new Set<string>(materialTypes.value);
  const add = (value?: string | null) => {
    const type = value?.trim();
    if (type) set.add(type);
  };
  materials.value.forEach(row => add(row.label));
  newMaterialRows.value.forEach(row => add(row.label));
  add(itemSpec.value);
  add(materialName.value);
  return [...set].map(type => ({ label: type, value: type }));
});

// 从二维余料库存(旧料库)拉取已有类型名作为候选 (优先材料类型, 兼容旧数据只填了名称的条目)
async function loadMaterialTypes() {
  const { data, error } = await fetchCutScraps({ scrapType: 2 });
  if (error || !data) return;
  const set = new Set<string>();
  data.records.forEach(item => {
    const type = item.materialType?.trim() || item.label?.trim();
    if (type) set.add(type);
  });
  materialTypes.value = [...set];
}

// 表格内可编辑的零件材料类型选择器: 支持下拉选择 + 手动输入新类型, 可清空(通用)
function renderItemSpecSelect(row: Api.Cut.Item, index: number) {
  return h(NSelect, {
    value: row.spec ?? null,
    options: materialTypeOptions.value,
    filterable: true,
    tag: true,
    clearable: true,
    size: 'small',
    placeholder: $t('page.cut.materialType'),
    onUpdateValue: (value: string | null) => {
      items.value[index].spec = value?.trim() ? value.trim() : undefined;
    }
  });
}

// item 表格
const itemColumns = [
  {
    title: $t('page.cut.materialType'),
    key: 'spec',
    width: 160,
    render(row: Api.Cut.Item, index: number) {
      return renderItemSpecSelect(row, index);
    }
  },
  { title: $t('page.cut.label'), key: 'label' },
  { title: $t('page.cut.widthCm'), key: 'width' },
  { title: $t('page.cut.heightCm'), key: 'height' },
  { title: $t('page.cut.quantity'), key: 'quantity' },
  {
    title: $t('common.action'),
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
  { title: $t('page.cut.materialType'), key: 'label' },
  { title: $t('page.cut.widthCm'), key: 'width' },
  { title: $t('page.cut.heightCm'), key: 'height' },
  { title: $t('page.cut.quantity'), key: 'quantity' },
  {
    title: $t('common.action'),
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

// 按材料类型分组统计表 (summaryData.byMaterialType, 后端汇总)
const typeSummaryColumns = [
  {
    title: $t('page.cut.materialType'),
    key: 'materialType',
    render: (row: Api.Cut.PlaneMaterialTypeSummary) => row.materialType?.trim() || $t('page.cut.summaryNewMaterial')
  },
  { title: $t('page.cut.summaryBinCount'), key: 'count' },
  {
    title: $t('page.cut.summaryTotalArea'),
    key: 'totalArea',
    render: (row: Api.Cut.PlaneMaterialTypeSummary) => `${fmtNum(row.totalArea)} cm²`
  },
  {
    title: $t('page.cut.summaryUsedArea'),
    key: 'usedArea',
    render: (row: Api.Cut.PlaneMaterialTypeSummary) => `${fmtNum(row.usedArea)} cm²`
  },
  {
    title: $t('page.cut.summaryTypeUtilization'),
    key: 'utilization',
    render: (row: Api.Cut.PlaneMaterialTypeSummary) => `${row.utilization}%`
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
    items.value[existingIndex].spec = itemSpec.value?.trim() ? itemSpec.value.trim() : undefined;
  } else {
    items.value.push({
      label: label.value,
      width: width.value,
      height: height.value,
      quantity: quantity.value,
      spec: itemSpec.value?.trim() ? itemSpec.value.trim() : undefined
    });
  }

  clearItemInputs();
}

// 添加材料
function addMaterial() {
  if (
    !materialName.value?.trim() ||
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
    label: materialName.value.trim(),
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
  saveData.value = null;
  itemSpec.value = null;
  materialName.value = null;
}

// 清空输入框
function clearItemInputs() {
  label.value = '';
  width.value = null;
  height.value = null;
  quantity.value = 1;
  itemSpec.value = null;
}

function clearMaterialInputs() {
  materialName.value = null;
  materialWidth.value = null;
  materialHeight.value = null;
  materialCount.value = 1;
}

// 新板材多规格行操作
function addBoardRow() {
  newMaterialRows.value.push({ label: '', width: DEFAULT_BOARD_SIZE, height: DEFAULT_BOARD_SIZE });
}

function removeBoardRow(index: number) {
  if (newMaterialRows.value.length > 1) {
    newMaterialRows.value.splice(index, 1);
  }
}

// 优化主逻辑
async function runOptimization() {
  if (items.value.length === 0) {
    message.error($t('page.cut.inputItemsRequired'));
    return;
  }
  // 多规格校验: 每行宽高必须 > 0
  const invalidRow = newMaterialRows.value.find(row => !row.width || !row.height || row.width <= 0 || row.height <= 0);
  if (invalidRow) {
    message.error($t('page.cut.planeNewMaterialInvalid'));
    return;
  }
  // 零件引用的材料类型必须已定义为新板材规格 (与一维口径一致)
  for (const row of items.value) {
    const spec = row.spec?.trim();
    if (spec && !newMaterialRows.value.some(board => board.label.trim() === spec)) {
      message.error($t('page.cut.planeSpecUndefined', { label: row.label, spec }));
      return;
    }
  }

  const expandedItems: Api.Cut.Item[] = items.value.flatMap((item: Api.Cut.Item) => {
    const count = item.quantity ?? 0;
    if (count < 1) return [];
    return Array.from({ length: count }, (_, i) => ({
      label: `${item.label}_${i + 1}`,
      width: item.width,
      height: item.height,
      spec: item.spec || undefined
    }));
  });

  // 多规格 (始终传并保留 width/height 兼容后端 binding)
  const newMaterials: Api.Cut.Item[] = newMaterialRows.value.map(row => ({
    label: row.label.trim(),
    width: row.width as number,
    height: row.height as number
  }));
  const lossValue = Math.max(0, loss.value ?? 0);

  loading.value = true;
  try {
    const request = {
      items: expandedItems,
      materials: materials.value.map(row => ({
        label: row.label?.trim() ?? '',
        width: row.width,
        height: row.height,
        quantity: row.quantity
      })),
      width: newMaterials[0]!.width,
      height: newMaterials[0]!.height,
      newMaterials,
      strategy: strategy.value,
      loss: lossValue
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
      name: ``,
      // 从库存带入且被本次排版消耗的旧板材, 保存时扣减库存
      deductScraps: collectDeductScraps()
    };
  } finally {
    loading.value = false;
  }
}

// 从库存带入的旧料行 (invId) 的实际消耗板块数 → 保存记录时随请求扣减库存。
// 消耗口径: 结果里每块 (materialType, materialWidth, materialHeight) 匹配的板即消耗一张;
// 多行导入同一 (类型,尺寸) 时按行序分摊, 上限为该行数量。
function collectDeductScraps(): Array<{ id: number; count: number }> {
  if (!results.value.length) return [];
  const consumedPool = new Map<string, number>();
  for (const r of results.value) {
    const key = `${r.materialType ?? ''}|${r.materialWidth}|${r.materialHeight}`;
    consumedPool.set(key, (consumedPool.get(key) ?? 0) + 1);
  }
  const out = new Map<number, number>();
  for (const row of materials.value) {
    if (!row.invId) continue;
    const key = `${row.label?.trim() ?? ''}|${row.width}|${row.height}`;
    const pool = consumedPool.get(key) ?? 0;
    if (pool <= 0) continue;
    const take = Math.min(pool, row.quantity);
    out.set(row.invId, (out.get(row.invId) ?? 0) + take);
    consumedPool.set(key, pool - take);
  }
  return Array.from(out.entries()).map(([id, count]) => ({ id, count }));
}

// 旧料库带入表单: 库存条目转材料行 (保留类型名与库存 id)
function applyScraps(rows: Api.Cut.CutScrap[]) {
  rows.forEach(row => {
    const type = row.materialType?.trim() || row.label?.trim();
    materials.value.push({
      label: type ? type : $t('page.cut.scrapMaterialLabel'),
      width: row.widthValue,
      height: row.heightValue,
      quantity: row.quantity,
      invId: row.id
    });
  });
  // 库存可能新增了类型, 刷新下拉候选
  loadMaterialTypes();
  message.success($t('page.cut.scrapApplied', { count: rows.length }));
}

onMounted(() => {
  loadMaterialTypes();
});

// 导出 PNG (全部板材拼接纵向长图; 聚合显示开启时按聚合口径导出)
async function exportPNG() {  if (results.value.length === 0) return;
  exporting.value = 'png';
  try {
    await exportPlaneCutPNG(results.value, group.value);
  } catch (e) {
    console.error(e);
    message.error($t('page.cut.exportFailed'));
  } finally {
    exporting.value = null;
  }
}

// 导出 PDF (逐板分页; 聚合口径同上)
async function exportPDF() {
  if (results.value.length === 0) return;
  exporting.value = 'pdf';
  try {
    await exportPlaneCutPDF(results.value, group.value);
  } catch (e) {
    console.error(e);
    message.error($t('page.cut.exportFailed'));
  } finally {
    exporting.value = null;
  }
}

// 直接打印切割图 (PDF autoPrint; 聚合口径同上)
async function printChart() {
  if (results.value.length === 0) return;
  printing.value = true;
  try {
    const opened = await printPlaneCut(results.value, group.value);
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
    <NCard :title="$t('page.cut.vizTitle')" size="large" class="mb-4">
      <!-- 添加切割项目 -->

      <h3 class="mb-3 text-lg font-semibold">{{ $t('page.cut.cutSizes') }}</h3>
      <div class="mb-2 flex items-center gap-2">
        <NSelect
          v-model:value="itemSpec"
          :options="materialTypeOptions"
          filterable
          tag
          clearable
          :placeholder="$t('page.cut.materialType')"
          class="w-40"
        />
        <NInput v-model:value="label" class="input-width" type="text" :placeholder="$t('page.cut.label')" />
        <NInputNumber v-model:value="width" type="number" :placeholder="$t('page.cut.widthCm')" step="0.1" :min="0.1" class="w-40" />
        <NInputNumber v-model:value="height" type="number" :placeholder="$t('page.cut.heightCm')" step="0.1" :min="0.1" class="w-40" />
        <NInputNumber v-model:value="quantity" type="number" :placeholder="$t('page.cut.quantity')" class="w-40" :min="1" />
        <NButton type="primary" @click="addItem">{{ $t('page.cut.addItem') }}</NButton>
        <NButton type="info" secondary @click="productPickShow = true">{{ $t('page.cut.pdImport') }}</NButton>
      </div>

      <!-- 切割项目列表 -->

      <h3 class="mb-2 text-lg font-semibold">{{ $t('page.cut.cutItems') }}</h3>
      <NDataTable :columns="itemColumns" :data="items" />

      <!-- 添加剩余材料 -->

      <h3 class="mb-3 text-lg font-semibold">{{ $t('page.cut.stockMaterial') }}</h3>
      <div class="mb-2 flex items-center gap-2">
        <NSelect
          v-model:value="materialName"
          :options="materialTypeOptions"
          filterable
          tag
          :placeholder="$t('page.cut.materialType')"
          class="input-width"
        />
        <NInputNumber
          v-model:value="materialWidth"
          type="number"
          :placeholder="$t('page.cut.widthCm')"
          step="0.1"
          :min="0.1"
          class="w-40"
        />
        <NInputNumber
          v-model:value="materialHeight"
          type="number"
          :placeholder="$t('page.cut.heightCm')"
          step="0.1"
          :min="0.1"
          class="w-40"
        />
        <NInputNumber v-model:value="materialCount" type="number" :placeholder="$t('page.cut.quantity')" class="w-40" :min="1" />
        <NButton type="primary" @click="addMaterial">{{ $t('page.cut.addMaterial') }}</NButton>
        <NButton type="info" secondary @click="scrapModalShow = true">{{ $t('page.cut.scrapLibrary') }}</NButton>
      </div>

      <!-- 剩余材料列表 -->

      <h3 class="mb-2 text-lg font-semibold">{{ $t('page.cut.stockMaterial') }}</h3>
      <NDataTable :columns="materialColumns" :data="materials" />

      <h3 class="mt-6">{{ $t('page.cut.paramsConfig') }}</h3>
      <div class="mb-4 flex flex-wrap items-start gap-6">
        <div class="flex items-start gap-2">
          <span class="w-24 pt-1">{{ $t('page.cut.newMaterialSpec') }}</span>
          <div class="flex flex-col gap-2">
            <div v-for="(row, idx) in newMaterialRows" :key="idx" class="flex items-center gap-2">
              <NInput v-model:value="row.label" :placeholder="$t('page.cut.newMaterialLabel')" class="w-32" />
              <NInputNumber v-model:value="row.width" :placeholder="$t('page.cut.widthCm')" class="w-40" :min="0.1" />
              <NInputNumber v-model:value="row.height" :placeholder="$t('page.cut.heightCm')" class="w-40" :min="0.1" />
              <NButton size="small" type="error" quaternary :disabled="newMaterialRows.length <= 1" @click="removeBoardRow(idx)">
                {{ $t('common.delete') }}
              </NButton>
            </div>
            <NButton size="small" dashed class="w-32" @click="addBoardRow">+ {{ $t('common.add') }}</NButton>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">{{ $t('page.cut.strategy') }}</span>
          <NSelect v-model:value="strategy" :options="strategyOptions" />
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">{{ $t('page.cut.loss') }}</span>
          <NInputNumber v-model:value="loss" class="w-40" :min="0" :step="0.1">
            <template #suffix>cm</template>
          </NInputNumber>
        </div>
        <div class="flex items-center gap-2">
          <span class="w-24">{{ $t('page.cut.aggregateDisplay') }}</span>
          <NSwitch v-model:value="group" class="w-40" />
        </div>
      </div>

      <!-- 操作按钮 -->
      <div class="mt-4 flex gap-2">
        <NButton type="primary" :loading="loading" @click="runOptimization">{{ $t('page.cut.startCutting') }}</NButton>
        <PlanePrinter :results="results" :materials="materials" :group-data="group"></PlanePrinter>
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

      <template v-if="summaryData.byMaterialType?.length">
        <NDivider title-placement="left" class="!mt-6 !mb-4">
          {{ $t('page.cut.summaryByTypeTitle') }}
        </NDivider>
        <NDataTable size="small" :columns="typeSummaryColumns" :data="summaryData.byMaterialType" :bordered="false" />
      </template>
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

    <!-- 导入产品弹窗: 只列含玻璃产品的单, 预填平面零件 (spec=玻璃厚度) -->
    <ProductPickModal v-model:show="productPickShow" mode="plane" @confirm="handleProductPick" />

    <!-- 加载中弹窗: 不可关闭, 防止求解期间重复提交 -->
    <NModal v-model:show="loading" preset="dialog" :title="$t('page.cut.calculating')" :mask-closable="false" :closable="false">
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
