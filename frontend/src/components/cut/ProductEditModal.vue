<script setup lang="ts">
import { computed, h, ref, watch } from 'vue';
import { NButton, NCheckbox, NInput, NInputNumber, NModal, NRadioButton, NRadioGroup, NSelect, NTag, NTooltip, useMessage } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { $t } from '@/locales';
import { saveCutProduct } from '@/service/api';
import WindowGridPreview from './WindowGridPreview.vue';
import {
  CELL_FIXED,
  CELL_SASH,
  CELL_SPAN,
  DIMENSION_LINEAR,
  DIMENSION_PLANE,
  FIT_DEFAULTS,
  GLASS_LABEL,
  GLASS_THICKNESS_OPTIONS,
  GLASS_TYPE,
  WINDOW_TEMPLATES,
  availableColWidth,
  availableRowHeight,
  buildPieces,
  checkGridGeometry,
  findWindowTemplate,
  glassItemFromDefaults,
  isGlassItem,
  itemDimension,
  itemsDimension,
  parseProductSpec,
  productTypeLabel,
  round2,
  type CutDimension,
  type ProductItem
} from './window-template';

/**
 * 产品单编辑器: 左侧分格预览 (点击格子切换 固定/开启) + 右侧参数, 配好的产品加入清单,
 * 一单可含多种类型的多件产品, 整单保存入库。
 * 新建/编辑产品时先选「切割维度」(一维=窗类 / 平面=玻璃), 把窗户与玻璃彻底分开:
 * 一维走窗型模板 (框料/分格/拼装参数), 平面只填 宽×高×厚度×数量, 走平面切割。
 * 产品级只设材料规格 (窗 = 型材系列 mm; 玻璃 = 玻璃厚度 mm); 外框/中梃/扇等部件由切割件自动生成,
 * 导入切割时按部件族+规格分组。
 */
const show = defineModel<boolean>('show', { default: false });

const props = defineProps<{
  /** 待编辑的产品单 (null = 新建) */
  order?: Api.Cut.CutProduct | null;
}>();

const emit = defineEmits<{ (e: 'saved'): void }>();

const message = useMessage();

/** 窗型模板候选 (仅一维切割); 平面切割不选模板, 直接进入玻璃模式 */
const windowTemplateOptions = WINDOW_TEMPLATES.map(t => ({ label: t.label, value: t.key }));

/** 框料截面宽度候选 (mm, 即型材系列), 支持手动输入其他系列; 选择后自动带入框料宽 */
const SERIES_OPTIONS = ['55', '60', '65', '70', '75', '80'].map(v => ({ label: `${v}mm`, value: v }));
/** 新建产品默认系列 (框料宽 = 系列/10 cm) */
const DEFAULT_SERIES = '55';

// ===== 产品单内容 =====
const name = ref('');
const items = ref<ProductItem[]>([]);
const saving = ref(false);

// ===== 编辑区 (当前正在配置的一件产品) =====
/** 当前编辑产品的切割维度: 一维(窗类) / 平面(玻璃); 决定产品类别与可选模板 */
const dimension = ref<CutDimension>(DIMENSION_LINEAR);
const curType = ref<string>(WINDOW_TEMPLATES[0]!.key);
const width = ref<number | null>(150);
const height = ref<number | null>(200);
const frameWidth = ref<number | null>(5);
const cols = ref<number[]>([140]);
const rows = ref<number[]>([190]);
const cells = ref<number[][]>([[CELL_FIXED]]);
const sliding = ref(false);
const series = ref<string | null>(null);
const gapMm = ref<number | null>(FIT_DEFAULTS.gap);
const reachMm = ref<number | null>(FIT_DEFAULTS.reach);
const overlapMm = ref<number | null>(FIT_DEFAULTS.overlap);
const beadDeductMm = ref<number | null>(FIT_DEFAULTS.beadDeduct);
const bead = ref(false);
const beadSeries = ref('');
const count = ref<number | null>(1);
/** 正在回改清单中的下标, -1 = 新产品 */
const editingIndex = ref(-1);

const modalTitle = computed(() => (props.order ? $t('page.cut.pdEdit') : $t('page.cut.pdAdd')));

/** 玻璃编辑模式: 只填 宽×高×厚度×数量, 隐藏分格/框料/拼装参数 */
const isGlassMode = computed(() => curType.value === GLASS_TYPE);

/** 整单统一维度: 清单内产品维度一致时返回该维度 (空单/混选为 null), 锁定后可加入的产品类型 */
const orderDimension = computed(() => itemsDimension(items.value));

/** 均分并吸收舍入差 (总和精确等于 total) */
function evenSplit(total: number, n: number): number[] {
  const each = round2(total / n);
  const arr = Array.from({ length: n }, () => each);
  if (n > 0) {
    const diff = round2(total - each * n);
    arr[n - 1] = round2(arr[n - 1]! + diff);
  }
  return arr;
}

/** 按比例缩放到新的总和 (末位吸收舍入差, 总和精确) */
function rescale(arr: number[], total: number): number[] {
  const sum = arr.reduce((s, v) => s + v, 0);
  const out = sum > 0 ? arr.map(v => round2((v / sum) * total)) : evenSplit(total, arr.length);
  if (out.length > 0) {
    const diff = round2(total - out.reduce((s, v) => s + v, 0));
    out[out.length - 1] = round2(out[out.length - 1]! + diff);
  }
  return out;
}

function applyTemplate(key: string) {
  const def = findWindowTemplate(key);
  if (!def) return;
  dimension.value = DIMENSION_LINEAR;
  curType.value = key;
  sliding.value = def.sliding;
  width.value = def.defaults.width;
  height.value = def.defaults.height;
  // 框料宽优先取当前系列 (mm/10), 与框料截面宽度保持同步; 模板默认仅作未选系列时兜底
  const mm = parseFloat(series.value ?? '');
  frameWidth.value = Number.isFinite(mm) && mm > 0 ? round2(mm / 10) : def.defaults.frameWidth;
  const c = frameWidth.value;
  cols.value = evenSplit(availableColWidth(width.value, c, def.cols, def.sliding), def.cols);
  rows.value = evenSplit(availableRowHeight(height.value, c, def.rows), def.rows);
  cells.value = def.cells.map(r => [...r]);
  editingIndex.value = -1;
}

/** 切到玻璃模板: 清空规格 (窗系列≠玻璃厚度), 尺寸回玻璃默认 */
function onTemplateChange(key: string) {
  if (key === GLASS_TYPE) {
    const def = glassItemFromDefaults();
    dimension.value = DIMENSION_PLANE;
    curType.value = GLASS_TYPE;
    width.value = def.width;
    height.value = def.height;
    frameWidth.value = 0;
    series.value = null;
    editingIndex.value = -1;
    return;
  }
  applyTemplate(key);
}

/** 切换切割维度: 一维→窗型模板(窗户), 平面→玻璃; 把窗户与玻璃分成两个入口 */
function onDimensionChange(v: CutDimension) {
  if (v === DIMENSION_PLANE) {
    onTemplateChange(GLASS_TYPE);
  } else {
    applyTemplate(curType.value === GLASS_TYPE ? WINDOW_TEMPLATES[0]!.key : curType.value);
  }
}

/** 总尺寸/框料宽变化: 先写入值再等比缩放净宽高, 保持几何自洽 */
function onOuterChange() {
  const w = width.value;
  const h = height.value;
  const c = frameWidth.value;
  if (!w || !h || c === null || c < 0) return;
  cols.value = rescale(cols.value, availableColWidth(w, c, cols.value.length, sliding.value));
  rows.value = rescale(rows.value, availableRowHeight(h, c, rows.value.length));
}

function onWidthChange(v: number | null) {
  width.value = v;
  onOuterChange();
}

function onHeightChange(v: number | null) {
  height.value = v;
  onOuterChange();
}

function onFrameWidthChange(v: number | null) {
  frameWidth.value = v;
  onOuterChange();
}

/** 选择框料截面宽度 (系列): 框料宽自动按 系列/10 cm 带入并重算分格 */
function onSeriesChange(v: string | null) {
  series.value = v;
  const mm = parseFloat(v ?? '');
  if (Number.isFinite(mm) && mm > 0) {
    frameWidth.value = round2(mm / 10);
    onOuterChange();
  }
}

/** 选择框料截面宽度 (系列): 框料宽自动按 系列/10 cm 带入并重算分格; 玻璃模式无此换算 */
function onSpecChange(v: string | null) {
  if (isGlassMode.value) return;
  onSeriesChange(v);
}

function setColCount(n: number) {
  const w = width.value;
  const c = frameWidth.value;
  if (!w || c === null || c < 0 || n < 1 || n > 20) return;
  cols.value = evenSplit(availableColWidth(w, c, n, sliding.value), n);
  cells.value = cells.value.map(rowCells => {
    const next = [...rowCells];
    while (next.length < n) next.push(next[next.length - 1] ?? CELL_FIXED);
    return next.slice(0, n);
  });
}

function setRowCount(n: number) {
  const h = height.value;
  const c = frameWidth.value;
  if (!h || c === null || c < 0 || n < 1 || n > 20) return;
  rows.value = evenSplit(availableRowHeight(h, c, n), n);
  const next = cells.value.map(r => [...r]);
  while (next.length < n) next.push([...(next[next.length - 1] ?? Array.from({ length: cols.value.length }, () => CELL_FIXED))]);
  cells.value = next.slice(0, n);
}

/** 手改某列净宽: 差额按其余列现值比例分摊, 总和保持自洽 */
function setColWidth(index: number, v: number | null) {
  const w = width.value;
  const c = frameWidth.value;
  if (!w || c === null || c < 0 || v === null || v <= 0) return;
  const avail = availableColWidth(w, c, cols.value.length, sliding.value);
  const target = Math.min(v, round2(avail - 0.1 * (cols.value.length - 1)));
  const restSum = round2(avail - target);
  const rest = cols.value.filter((_, i) => i !== index);
  const oldRestSum = rest.reduce((s, cv) => s + cv, 0);
  const newRest = oldRestSum > 0 ? rest.map(cv => round2((cv / oldRestSum) * restSum)) : evenSplit(restSum, rest.length);
  if (newRest.length > 0) {
    const diff = round2(restSum - newRest.reduce((s, v) => s + v, 0));
    newRest[newRest.length - 1] = round2(newRest[newRest.length - 1]! + diff);
  }
  let ri = 0;
  cols.value = cols.value.map((_, i) => (i === index ? round2(target) : newRest[ri++]!));
}

function setRowHeight(index: number, v: number | null) {
  const h = height.value;
  const c = frameWidth.value;
  if (!h || c === null || c < 0 || v === null || v <= 0) return;
  const avail = availableRowHeight(h, c, rows.value.length);
  const target = Math.min(v, round2(avail - 0.1 * (rows.value.length - 1)));
  const restSum = round2(avail - target);
  const rest = rows.value.filter((_, i) => i !== index);
  const oldRestSum = rest.reduce((s, cv) => s + cv, 0);
  const newRest = oldRestSum > 0 ? rest.map(cv => round2((cv / oldRestSum) * restSum)) : evenSplit(restSum, rest.length);
  if (newRest.length > 0) {
    const diff = round2(restSum - newRest.reduce((s, v) => s + v, 0));
    newRest[newRest.length - 1] = round2(newRest[newRest.length - 1]! + diff);
  }
  let ri = 0;
  rows.value = rows.value.map((_, i) => (i === index ? round2(target) : newRest[ri++]!));
}

/** 编辑区当前产品 (尺寸不全时给最小占位, 供校验与预览兜底) */
const currentItem = computed<ProductItem>(() => {
  const w = width.value ?? 0;
  const h = height.value ?? 0;
  if (isGlassMode.value) {
    // 玻璃: 无框料/拼装参数, 分格为 1×1 平凡格 (兼容后端几何校验), series 承载厚度
    return {
      ...glassItemFromDefaults(w, h),
      series: series.value?.trim() ?? '',
      count: Math.max(count.value ?? 1, 1)
    };
  }
  const c = frameWidth.value ?? 0;
  return {
    type: curType.value,
    width: w,
    height: h,
    frameWidth: c,
    series: series.value?.trim() ?? '',
    fit: {
      gap: gapMm.value ?? FIT_DEFAULTS.gap,
      reach: reachMm.value ?? FIT_DEFAULTS.reach,
      overlap: overlapMm.value ?? FIT_DEFAULTS.overlap,
      beadDeduct: beadDeductMm.value ?? FIT_DEFAULTS.beadDeduct
    },
    bead: bead.value,
    beadSeries: beadSeries.value.trim() || undefined,
    count: Math.max(count.value ?? 1, 1),
    grid: {
      cols: cols.value,
      rows: rows.value,
      cells: cells.value,
      sliding: sliding.value
    }
  };
});

const geometryError = computed(() => {
  if (isGlassMode.value) {
    if ((width.value ?? 0) <= 0 || (height.value ?? 0) <= 0) return '玻璃宽高必须大于 0';
    return null;
  }
  return checkGridGeometry(currentItem.value);
});

const previewPieces = computed(() => (isGlassMode.value ? [] : buildPieces(currentItem.value)));
const previewTotal = computed(() =>
  isGlassMode.value ? Math.max(count.value ?? 1, 1) : previewPieces.value.reduce((s, p) => s + p.quantity, 0)
);

function toggleCell(row: number, col: number) {
  const line = cells.value[row];
  if (!line) return;
  line[col] = line[col] === CELL_SASH ? CELL_FIXED : CELL_SASH;
}

/** 双击合并面板: 把最右一列从延伸格拆出为独立固定格 */
function splitCell(row: number, col: number) {
  const line = cells.value[row];
  if (!line) return;
  let end = col;
  while (end + 1 < line.length && line[end + 1] === CELL_SPAN) end++;
  if (end > col) line[end] = CELL_FIXED;
}

/** 双击/右键合并: dir=right 把右侧格并入, dir=left 把本格并入左侧 (类型随左格) */
function mergeCell(row: number, col: number, dir: 'left' | 'right' = 'right') {
  const line = cells.value[row];
  if (!line) return;
  if (dir === 'right' && col + 1 < line.length) line[col + 1] = CELL_SPAN;
  if (dir === 'left' && col > 0) line[col] = CELL_SPAN;
}

/** 系列选择器 (下拉 + 手输) */
function renderSeriesSelect(value: string, onUpdate: (v: string) => void) {
  return h(NSelect, {
    value: value || null,
    options: SERIES_OPTIONS,
    filterable: true,
    tag: true,
    size: 'small',
    placeholder: $t('page.cut.pdSeries'),
    onUpdateValue: (v: string | null) => onUpdate(v ?? '')
  });
}

/** 把清单中第 index 件载回编辑区 */
function editItem(index: number) {
  const item = items.value[index];
  if (!item) return;
  editingIndex.value = index;
  dimension.value = itemDimension(item);
  curType.value = item.type;
  sliding.value = item.grid.sliding;
  width.value = item.width;
  height.value = item.height;
  frameWidth.value = item.frameWidth;
  cols.value = [...item.grid.cols];
  rows.value = [...item.grid.rows];
  cells.value = item.grid.cells.map(r => [...r]);
  series.value = item.series;
  gapMm.value = item.fit?.gap ?? FIT_DEFAULTS.gap;
  reachMm.value = item.fit?.reach ?? FIT_DEFAULTS.reach;
  overlapMm.value = item.fit?.overlap ?? FIT_DEFAULTS.overlap;
  beadDeductMm.value = item.fit?.beadDeduct ?? FIT_DEFAULTS.beadDeduct;
  bead.value = item.bead ?? false;
  beadSeries.value = item.beadSeries ?? '';
  count.value = item.count;
}

function removeItem(index: number) {
  items.value.splice(index, 1);
  if (editingIndex.value === index) editingIndex.value = -1;
  else if (editingIndex.value > index) editingIndex.value -= 1;
}

/** 加入/更新清单 (更新时替换原下标, 然后编辑区重置为新产品) */
function addToList() {
  const err = geometryError.value;
  if (err) {
    message.error(err);
    return;
  }
  if (isGlassMode.value) {
    if (!currentItem.value.series) {
      message.error($t('page.cut.pdNeedThickness'));
      return;
    }
  } else if (!currentItem.value.series) {
    message.error($t('page.cut.pdNeedSeries'));
    return;
  }
  const copy = JSON.parse(JSON.stringify(currentItem.value)) as ProductItem;
  // 单一维度: 窗类走一维、玻璃走平面, 同一产品单不允许混入另一维度
  if (orderDimension.value && orderDimension.value !== itemDimension(copy)) {
    const need = orderDimension.value === DIMENSION_LINEAR ? $t('page.cut.pdDimensionLinear') : $t('page.cut.pdDimensionPlane');
    message.error($t('page.cut.pdDimensionConflict', { type: need }));
    return;
  }
  if (editingIndex.value >= 0) {
    items.value.splice(editingIndex.value, 1, copy);
  } else {
    items.value.push(copy);
  }
  editingIndex.value = -1;
  // 重置编辑区为新产品 (保留规格/拼装参数与数量, 方便连续录入同系列产品)
  if (isGlassMode.value) {
    const def = glassItemFromDefaults();
    width.value = def.width;
    height.value = def.height;
    frameWidth.value = 0;
  } else {
    applyTemplate(curType.value);
  }
}

const listColumns = computed<DataTableColumns<ProductItem>>(() => [
  {
    title: $t('page.cut.pdTemplate'),
    key: 'type',
    width: 150,
    render: row => productTypeLabel(row.type)
  },
  {
    title: $t('page.cut.pdDimension'),
    key: 'dimension',
    width: 110,
    render: row => {
      const plane = itemDimension(row) === DIMENSION_PLANE;
      return h(NTag, { size: 'small', bordered: false, type: plane ? 'info' : 'success' }, {
        default: () => (plane ? $t('page.cut.pdDimensionPlane') : $t('page.cut.pdDimensionLinear'))
      });
    }
  },
  { title: '尺寸(cm)', key: 'size', width: 110, render: row => `${row.width}×${row.height}` },
  {
    title: $t('page.cut.pdSeries'),
    key: 'series',
    width: 130,
    render: row => renderSeriesSelect(row.series, v => (row.series = v))
  },
  {
    title: $t('page.cut.pdCount'),
    key: 'count',
    width: 100,
    render: row =>
      h(NInputNumber, {
        value: row.count,
        min: 1,
        size: 'small',
        showButton: false,
        class: 'w-20',
        onUpdateValue: (v: number | null) => {
          row.count = Math.max(v ?? 1, 1);
        }
      })
  },
  {
    title: $t('page.cut.pdGrid'),
    key: 'grid',
    width: 130,
    render: row =>
      h('span', null, [
        isGlassItem(row)
          ? $t('page.cut.pdGlassPlain')
          : `${row.grid.cols.length}列×${row.grid.rows.length}行`,
        row.grid.sliding ? h(NTag, { size: 'small', bordered: false, type: 'info', class: 'ml-1' }, { default: () => $t('page.cut.pdSliding') }) : null
      ])
  },
  {
    title: '操作',
    key: 'actions',
    width: 110,
    render(row, index) {
      return h('div', { class: 'flex gap-1' }, [
        h(
          NButton,
          { size: 'small', type: 'primary', quaternary: true, onClick: () => editItem(index) },
          { default: () => '编辑' }
        ),
        h(
          NButton,
          { size: 'small', type: 'error', quaternary: true, onClick: () => removeItem(index) },
          { default: () => $t('common.delete') }
        )
      ]);
    }
  }
]);

async function handleSave() {
  if (editingIndex.value >= 0) {
    message.warning($t('page.cut.pdPendingEdit'));
    return;
  }
  if (items.value.length === 0) {
    message.error($t('page.cut.pdEmptyOrder'));
    return;
  }
  for (const item of items.value) {
    if (isGlassItem(item)) {
      if (item.width <= 0 || item.height <= 0) {
        message.error(`${GLASS_LABEL}: 玻璃宽高必须大于 0`);
        return;
      }
      if (!item.series?.trim()) {
        message.error($t('page.cut.pdNeedThickness'));
        return;
      }
    } else {
      const err = checkGridGeometry(item);
      if (err) {
        message.error(`${productTypeLabel(item.type)}: ${err}`);
        return;
      }
      if (!item.series?.trim()) {
        message.error($t('page.cut.pdNeedSeries'));
        return;
      }
    }
    if (item.count < 1) item.count = 1;
  }
  saving.value = true;
  try {
    const { error } = await saveCutProduct({
      id: props.order?.id,
      name: name.value.trim() || undefined,
      spec: { items: items.value }
    });
    if (error) return;
    message.success($t('page.cut.pdSaved'));
    emit('saved');
    show.value = false;
  } finally {
    saving.value = false;
  }
}

// 打开时初始化: 编辑模式载入单据内容, 新建模式清空
watch(show, opened => {
  if (!opened) return;
  editingIndex.value = -1;
  saving.value = false;
  let list: ProductItem[] = [];
  if (props.order) {
    list = parseProductSpec(props.order).items;
  } else {
    name.value = '';
  }
  items.value = list.map(item => JSON.parse(JSON.stringify(item)) as ProductItem);
  series.value = items.value[0]?.series || DEFAULT_SERIES;
  gapMm.value = items.value[0]?.fit?.gap ?? FIT_DEFAULTS.gap;
  reachMm.value = items.value[0]?.fit?.reach ?? FIT_DEFAULTS.reach;
  overlapMm.value = items.value[0]?.fit?.overlap ?? FIT_DEFAULTS.overlap;
  beadDeductMm.value = items.value[0]?.fit?.beadDeduct ?? FIT_DEFAULTS.beadDeduct;
  bead.value = items.value[0]?.bead ?? false;
  beadSeries.value = items.value[0]?.beadSeries ?? '';
  count.value = items.value[0]?.count ?? 1;
  const first = items.value[0];
  if (first && isGlassItem(first)) {
    // 玻璃单: 编辑区直接进玻璃模式 (平面) 并带入首件尺寸
    const def = glassItemFromDefaults(first.width, first.height);
    dimension.value = DIMENSION_PLANE;
    curType.value = GLASS_TYPE;
    width.value = def.width;
    height.value = def.height;
    frameWidth.value = 0;
  } else {
    applyTemplate(WINDOW_TEMPLATES[0]!.key);
  }
});
</script>

<template>
  <NModal v-model:show="show" preset="card" :title="modalTitle" class="w-1200px max-w-[97vw]">
    <div class="flex flex-col gap-3">
      <div class="flex items-center gap-2">
        <span class="w-30 shrink-0 whitespace-nowrap">{{ $t('page.cut.pdName') }}</span>
        <NInput v-model:value="name" :placeholder="$t('page.cut.pdNamePh')" />
      </div>

      <!-- 编辑区: 左预览 + 右参数 -->
      <div class="flex gap-4">
        <div class="w-460px shrink-0">
          <div class="border border-gray-200 rounded-md p-2">
            <WindowGridPreview
              :item="currentItem"
              :max-height="430"
              show-dims
              @toggle-cell="toggleCell"
              @split-cell="splitCell"
              @merge-cell="mergeCell"
            />
          </div>
          <div class="mt-1 text-center text-gray-400 text-xs">
            {{ isGlassMode ? $t('page.cut.pdGlassTip') : $t('page.cut.pdPreviewTip') }}
          </div>
        </div>

        <div class="flex flex-1 flex-col gap-2">
          <div class="flex items-center gap-2">
            <span class="w-30 shrink-0 whitespace-nowrap">{{ $t('page.cut.pdDimension') }}</span>
            <NRadioGroup :value="dimension" size="small" @update:value="(v: CutDimension) => onDimensionChange(v)">
              <NRadioButton value="linear" :disabled="orderDimension !== null && orderDimension !== DIMENSION_LINEAR">
                {{ $t('page.cut.pdDimensionLinear') }}
              </NRadioButton>
              <NRadioButton value="plane" :disabled="orderDimension !== null && orderDimension !== DIMENSION_PLANE">
                {{ $t('page.cut.pdDimensionPlane') }}
              </NRadioButton>
            </NRadioGroup>
          </div>
          <div class="flex items-center gap-2">
            <span class="w-30 shrink-0 whitespace-nowrap">{{ $t('page.cut.pdTemplate') }}</span>
            <NSelect v-if="!isGlassMode" :value="curType" :options="windowTemplateOptions" @update:value="onTemplateChange" />
            <NTag v-else type="info" :bordered="false">{{ GLASS_LABEL }}</NTag>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <span class="w-30 shrink-0 whitespace-nowrap">{{ $t('page.cut.pdWidthLabel') }}</span>
            <NInputNumber :value="width" :min="1" class="w-40 min-w-20" @update:value="onWidthChange" />
            <span class="text-gray-400">×</span>
            <NInputNumber :value="height" :min="1" class="w-40 min-w-20" @update:value="onHeightChange" />
            <template v-if="!isGlassMode">
              <span class="w-24 shrink-0 whitespace-nowrap text-right">{{ $t('page.cut.pdFrameWidth') }}</span>
              <NInputNumber :value="frameWidth" :min="0" class="w-30" @update:value="onFrameWidthChange" />
            </template>
          </div>
          <template v-if="!isGlassMode">
            <div class="flex flex-wrap items-center gap-2">
              <span class="w-30 shrink-0 whitespace-nowrap">{{ $t('page.cut.pdCols') }}</span>
              <NInputNumber
                :value="cols.length"
                :min="1"
                :max="20"
                class="w-24 shrink-0"
                @update:value="(v: number | null) => setColCount(v ?? 1)"
              />
              <span class="w-16 shrink-0 whitespace-nowrap text-right">{{ $t('page.cut.pdRows') }}</span>
              <NInputNumber
                :value="rows.length"
                :min="1"
                :max="20"
                class="w-24 shrink-0"
                @update:value="(v: number | null) => setRowCount(v ?? 1)"
              />
              <span class="w-16 shrink-0 whitespace-nowrap text-right">{{ $t('page.cut.pdCount') }}</span>
              <NInputNumber v-model:value="count" :min="1" class="w-24 shrink-0" />
            </div>
            <div class="flex items-center gap-2">
              <span class="w-30 shrink-0 whitespace-nowrap">{{ $t('page.cut.pdColWidths') }}</span>
              <div class="flex flex-wrap gap-1">
                <NInputNumber
                  v-for="(cw, i) in cols"
                  :key="`c${i}`"
                  :value="cw"
                  size="small"
                  :min="0.1"
                  class="w-24"
                  @update:value="(v: number | null) => setColWidth(i, v)"
                />
              </div>
            </div>
            <div class="flex items-center gap-2">
              <span class="w-30 shrink-0 whitespace-nowrap">{{ $t('page.cut.pdRowHeights') }}</span>
              <div class="flex flex-wrap gap-1">
                <NInputNumber
                  v-for="(rh, i) in rows"
                  :key="`r${i}`"
                  :value="rh"
                  size="small"
                  :min="0.1"
                  class="w-24"
                  @update:value="(v: number | null) => setRowHeight(i, v)"
                />
              </div>
            </div>
          </template>
          <div v-else class="flex flex-wrap items-center gap-2">
            <span class="w-30 shrink-0 whitespace-nowrap">{{ $t('page.cut.pdCount') }}</span>
            <NInputNumber v-model:value="count" :min="1" class="w-24 shrink-0" />
          </div>
          <div class="flex items-center gap-2">
            <NTooltip v-if="!isGlassMode" trigger="hover" placement="top" :style="{ maxWidth: '360px' }">
              <template #trigger>
                <span class="w-30 shrink-0 whitespace-nowrap cursor-help">{{ $t('page.cut.pdSeries') }}</span>
              </template>
              <div class="text-left">
                框料宽(cm) = 系列 ÷ 10, 选择后自动带入并重算分格。
                <br />影响: 外框横梃/竖梃(上下横/边封)、横向中梃、竖向中梃的下料长度与各分格净尺寸。
              </div>
            </NTooltip>
            <span v-else class="w-30 shrink-0 whitespace-nowrap">{{ $t('page.cut.pdGlassThickness') }}</span>
            <NSelect
              v-model:value="series"
              :options="isGlassMode ? GLASS_THICKNESS_OPTIONS : SERIES_OPTIONS"
              class="flex-1"
              filterable
              tag
              clearable
              :placeholder="isGlassMode ? $t('page.cut.pdGlassThicknessPh') : $t('page.cut.pdSeriesPh')"
              @update:value="onSpecChange"
            />
          </div>
          <template v-if="!isGlassMode">
            <div class="flex flex-wrap items-center gap-2">
              <span class="w-30 shrink-0 whitespace-nowrap">{{ $t('page.cut.pdFit') }}</span>
              <NTooltip trigger="hover" placement="top" :style="{ maxWidth: '360px' }">
                <template #trigger>
                  <span class="text-gray-500 text-xs whitespace-nowrap shrink-0 cursor-help">{{ $t('page.cut.pdFitGap') }}</span>
                </template>
                <div class="text-left">
                  平开: 扇横梃、扇竖梃 = 分格净尺寸 − 2×缝隙 (扇比开口每边小缝隙, 保证能开合)。
                  <br />推拉: 扇上下横在边封侧/固定侧各 − 缝隙 (留出滑动间隙)。
                </div>
              </NTooltip>
              <NInputNumber v-model:value="gapMm" :min="0" :max="50" :step="0.5" size="small" class="w-20 shrink-0" show-button />
              <template v-if="sliding">
                <NTooltip trigger="hover" placement="top" :style="{ maxWidth: '360px' }">
                  <template #trigger>
                    <span class="text-gray-500 text-xs whitespace-nowrap shrink-0 cursor-help">{{ $t('page.cut.pdFitReach') }}</span>
                  </template>
                  <div class="text-left">
                    仅推拉窗: 扇竖梃(光企/勾企) = 行净高 + 2×搭入,
                    <br />扇钩伸入上滑/下滑轨道, 防止脱轨; 取值参考型材轨道深度。
                  </div>
                </NTooltip>
                <NInputNumber v-model:value="reachMm" :min="0" :max="50" :step="1" size="small" class="w-20 shrink-0" show-button />
                <NTooltip trigger="hover" placement="top" :style="{ maxWidth: '360px' }">
                  <template #trigger>
                    <span class="text-gray-500 text-xs whitespace-nowrap shrink-0 cursor-help">{{ $t('page.cut.pdFitOverlap') }}</span>
                  </template>
                  <div class="text-left">
                    仅推拉窗: 相邻两扇光企/勾企互搭, 每扇的扇上下横各 + 搭接÷2,
                    <br />关窗时中间重叠密封; 取值参考光企/勾企搭接配合尺寸。
                  </div>
                </NTooltip>
                <NInputNumber v-model:value="overlapMm" :min="0" :max="50" :step="1" size="small" class="w-20 shrink-0" show-button />
              </template>
            </div>
            <div class="flex items-center gap-2">
              <span class="w-30 shrink-0 whitespace-nowrap"></span>
              <NCheckbox v-model:checked="bead" class="shrink-0">{{ $t('page.cut.pdBead') }}</NCheckbox>
            </div>
            <div v-if="bead" class="flex flex-wrap items-center gap-2">
              <span class="w-30 shrink-0 whitespace-nowrap"></span>
              <NTooltip trigger="hover" placement="top" :style="{ maxWidth: '360px' }">
                <template #trigger>
                  <span class="text-gray-500 text-xs whitespace-nowrap shrink-0 cursor-help">{{ $t('page.cut.pdFitBeadDeduct') }}</span>
                </template>
                <div class="text-left">
                  每根压条(横/竖) = 基准长度 − 扣尺; 基准: 固定格取开口净尺寸, 开启扇取扇外框尺寸。
                  <br />45° 拼角取 0, 直拼按压条宽度扣除; 影响全部压条件的下料长度。
                </div>
              </NTooltip>
              <NInputNumber v-model:value="beadDeductMm" :min="0" :max="50" :step="1" size="small" class="w-20 shrink-0" show-button />
              <span class="text-gray-500 text-xs whitespace-nowrap shrink-0">{{ $t('page.cut.pdBeadSeries') }}</span>
              <NInput v-model:value="beadSeries" size="small" class="w-32 shrink-0" clearable />
            </div>
          </template>

          <div v-if="geometryError" class="text-red-500 text-xs">{{ geometryError }}</div>

          <div class="flex items-start gap-2">
            <NButton type="primary" @click="addToList">
              {{ editingIndex >= 0 ? $t('page.cut.pdUpdateToList') : $t('page.cut.pdAddToList') }}
            </NButton>
            <div class="flex-1 rounded-md border border-gray-200 p-2">
              <div class="mb-1 text-gray-500 text-xs">
                {{ $t('page.cut.pdPieces') }} (共 {{ previewTotal }} 件)
              </div>
              <div v-if="isGlassMode" class="flex items-center justify-between text-sm">
                <span>整块玻璃 {{ width }}×{{ height }} cm</span>
                <span class="text-gray-500">× {{ previewTotal }}</span>
              </div>
              <div v-else class="flex max-h-32 flex-col gap-0.5 overflow-y-auto">
                <div v-for="p in previewPieces" :key="`${p.name}|${p.length}`" class="flex items-center justify-between text-sm">
                  <span>{{ p.name }}</span>
                  <span class="text-gray-500">{{ p.length }} cm × {{ p.quantity }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 产品清单 -->
      <div>
        <div class="mb-1 font-bold">{{ $t('page.cut.pdListTitle') }} ({{ items.length }})</div>
        <NDataTable :columns="listColumns" :data="items" size="small" :bordered="false" />
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <NButton @click="show = false">{{ $t('common.cancel') }}</NButton>
        <NButton type="primary" :loading="saving" @click="handleSave">{{ $t('page.cut.pdSave') }}</NButton>
      </div>
    </template>
  </NModal>
</template>
