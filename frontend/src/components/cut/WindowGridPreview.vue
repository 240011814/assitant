<script setup lang="ts">
import { computed, ref } from 'vue';
import { $t } from '@/locales';
import { CELL_FIXED, CELL_SASH, CELL_SPAN, type ProductItem } from './window-template';

/**
 * 窗户分格 SVG 预览: 按真实比例绘制外框/中梃/分格, 格内标注类型与净尺寸。
 * 延伸格 (CELL_SPAN) 与左侧格合并绘制为一个整面板 (无竖梃)。
 * 编辑模式 (readonly=false) 下: 单击面板切换 固定/开启;
 * 右键面板弹出操作菜单 (设为固定/开启、与右侧合并拼通亮、拆分连通格)。
 * showDims=true 时绘制工程式尺寸标注 (每段净宽/净高 + 总宽总高)。
 */
const props = withDefaults(
  defineProps<{
    item: Pick<ProductItem, 'width' | 'height' | 'frameWidth' | 'grid'>;
    /** 只读 (列表缩略图), 不可点击 */
    readonly?: boolean;
    /** 预览区最大高度 px */
    maxHeight?: number;
    /** 绘制尺寸标注 */
    showDims?: boolean;
  }>(),
  { readonly: false, maxHeight: 380, showDims: false }
);

const emit = defineEmits<{
  (e: 'toggleCell', row: number, col: number): void;
  (e: 'splitCell', row: number, col: number): void;
  (e: 'mergeCell', row: number, col: number, dir: 'left' | 'right'): void;
}>();

const rootEl = ref<HTMLDivElement | null>(null);

const FRAME_COLOR = '#94a3b8';
const FIXED_FILL = '#dbeafe';
const SASH_FILL = '#fde68a';
const DIM_COLOR = '#64748b';
const EXT_COLOR = '#cbd5e1';
const DIM_TEXT = '#475569';
/** 尺寸标注区宽度 (viewBox 单位) 与画布外边距 */
const DIM = 16;
const PAD = 2;

const w = computed(() => Math.max(props.item.width, 1));
const h = computed(() => Math.max(props.item.height, 1));
const c = computed(() => props.item.frameWidth);

const viewBox = computed(() => `${-DIM} ${-DIM} ${w.value + DIM + PAD} ${h.value + DIM + PAD}`);

function fmtLen(n: number): string {
  return Number.isInteger(n) ? String(n) : String(Math.round(n * 10) / 10);
}

interface PaneBox {
  x: number;
  y: number;
  w: number;
  h: number;
  row: number;
  /** 起始列 */
  col: number;
  /** 结束列 (延伸合并时 > col) */
  endCol: number;
  cell: number;
}

/** 面板盒: 延伸格并入左侧面板 */
const cellBoxes = computed<PaneBox[]>(() => {
  const { grid } = props.item;
  const frame = c.value;
  const boxes: PaneBox[] = [];
  let y = frame;
  grid.rows.forEach((rowH, r) => {
    let x = frame;
    let prev: PaneBox | null = null;
    grid.cols.forEach((colW, ci) => {
      const cell = grid.cells[r]?.[ci] ?? CELL_FIXED;
      if (cell === CELL_SPAN && prev && prev.row === r && prev.endCol === ci - 1) {
        // 并入左侧面板 (推拉窗无竖梃位可吸收)
        prev.w += (grid.sliding ? 0 : frame) + colW;
        prev.endCol = ci;
      } else {
        prev = { x, y, w: colW, h: rowH, row: r, col: ci, endCol: ci, cell };
        boxes.push(prev);
      }
      x += colW + (grid.sliding ? 0 : frame);
    });
    y += rowH + frame;
  });
  return boxes;
});

/** 竖向中梃分段 (被延伸格跨过的行断开, 段含段内横向中梃); 推拉窗无 */
const vMullionSegs = computed<Array<{ x: number; y: number; h: number }>>(() => {
  const { grid } = props.item;
  if (grid.sliding) return [];
  const frame = c.value;
  const segs: Array<{ x: number; y: number; h: number }> = [];
  let x = frame;
  grid.cols.slice(0, -1).forEach((colW, j) => {
    x += colW;
    let segTop = frame;
    let segH = 0;
    grid.rows.forEach((rowH, r) => {
      if (grid.cells[r]?.[j + 1] === CELL_SPAN) {
        if (segH > 0) segs.push({ x, y: segTop, h: segH });
        segH = 0;
        segTop += rowH + frame;
      } else {
        segH += rowH + frame;
      }
    });
    // 末尾多累计了一根横向中梃, 去掉
    if (segH > 0) segs.push({ x, y: segTop, h: segH - frame });
    x += frame;
  });
  return segs;
});

/** 横向中梃 y 坐标 (通宽) */
const hMullions = computed<number[]>(() => {
  const { grid } = props.item;
  const frame = c.value;
  const ys: number[] = [];
  let y = frame;
  grid.rows.slice(0, -1).forEach(rowH => {
    y += rowH;
    ys.push(y);
    y += frame;
  });
  return ys;
});

/** 开启扇内框示意 (内缩量) */
function sashInset(box: PaneBox): number {
  return Math.min(c.value * 0.5, Math.min(box.w, box.h) * 0.3);
}

function fontSize(box: PaneBox): number {
  return Math.min(box.w, box.h) / 5;
}

function onCellClick(box: PaneBox) {
  if (!props.readonly) emit('toggleCell', box.row, box.col);
}

// ===== 右键操作菜单 =====

const menu = ref<{ x: number; y: number; box: PaneBox } | null>(null);

const canMergeLeft = computed(() => {
  const m = menu.value;
  return !!m && m.box.col > 0 && m.box.endCol === m.box.col;
});
const canMergeRight = computed(() => {
  const m = menu.value;
  return !!m && m.box.endCol === m.box.col && m.box.col + 1 < props.item.grid.cols.length;
});
const canSplit = computed(() => !!menu.value && menu.value.box.endCol > menu.value.box.col);

function onPaneContextmenu(e: MouseEvent, box: PaneBox) {
  if (props.readonly) return;
  e.preventDefault();
  const rect = rootEl.value?.getBoundingClientRect();
  if (!rect) return;
  // 菜单宽度约 176px, 贴边时向内收
  const x = Math.min(e.clientX - rect.left, Math.max(rect.width - 185, 0));
  const y = Math.min(e.clientY - rect.top, Math.max(rect.height - 160, 0));
  menu.value = { x, y, box };
}

function closeMenu() {
  menu.value = null;
}

function menuToggle() {
  if (!menu.value) return;
  emit('toggleCell', menu.value.box.row, menu.value.box.col);
  closeMenu();
}

function menuMerge(dir: 'left' | 'right') {
  if (!menu.value) return;
  if ((dir === 'left' && !canMergeLeft.value) || (dir === 'right' && !canMergeRight.value)) return;
  emit('mergeCell', menu.value.box.row, menu.value.box.col, dir);
  closeMenu();
}

function menuSplit() {
  if (!canSplit.value || !menu.value) return;
  emit('splitCell', menu.value.box.row, menu.value.box.col);
  closeMenu();
}

// ===== 尺寸标注 =====

interface DimSeg {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
  label: string;
  vertical?: boolean;
}
interface Line {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
}

/** 列/行分段边界 (含首尾), 供尺寸界线引出 */
const colBounds = computed<number[]>(() => {
  const { grid } = props.item;
  const frame = c.value;
  const xs: number[] = [frame];
  let x = frame;
  grid.cols.forEach((colW, ci) => {
    x += colW;
    xs.push(x);
    if (ci < grid.cols.length - 1 && !grid.sliding) {
      x += frame;
      xs.push(x);
    }
  });
  return xs;
});

const rowBounds = computed<number[]>(() => {
  const { grid } = props.item;
  const frame = c.value;
  const ys: number[] = [frame];
  let y = frame;
  grid.rows.forEach((rowH, r) => {
    y += rowH;
    ys.push(y);
    if (r < grid.rows.length - 1) {
      y += frame;
      ys.push(y);
    }
  });
  return ys;
});

/** 尺寸线: 列净宽/行净高 (内侧一层) + 总宽/总高 (外侧一层) */
const dimSegs = computed<DimSeg[]>(() => {
  if (!props.showDims) return [];
  const { grid } = props.item;
  const frame = c.value;
  const segs: DimSeg[] = [];
  let x = frame;
  grid.cols.forEach(colW => {
    segs.push({ x1: x, y1: -5.5, x2: x + colW, y2: -5.5, label: fmtLen(colW) });
    x += colW + (grid.sliding ? 0 : frame);
  });
  let y = frame;
  grid.rows.forEach(rowH => {
    segs.push({ x1: -5.5, y1: y, x2: -5.5, y2: y + rowH, label: fmtLen(rowH), vertical: true });
    y += rowH + frame;
  });
  // 总宽/总高
  segs.push({ x1: 0, y1: -12, x2: w.value, y2: -12, label: fmtLen(w.value) });
  segs.push({ x1: -12, y1: 0, x2: -12, y2: h.value, label: fmtLen(h.value), vertical: true });
  return segs;
});

/** 尺寸界线 (从窗框边引出到尺寸线) */
const extLines = computed<Line[]>(() => {
  if (!props.showDims) return [];
  const lines: Line[] = [];
  colBounds.value.forEach(x => lines.push({ x1: x, y1: -0.4, x2: x, y2: -12.8 }));
  rowBounds.value.forEach(y => lines.push({ x1: -0.4, y1: y, x2: -12.8, y2: y }));
  // 总宽/总高两端的界线落在窗框外角
  lines.push({ x1: 0, y1: -0.4, x2: 0, y2: -12.8 });
  lines.push({ x1: w.value, y1: -0.4, x2: w.value, y2: -12.8 });
  lines.push({ x1: -0.4, y1: 0, x2: -12.8, y2: 0 });
  lines.push({ x1: -0.4, y1: h.value, x2: -12.8, y2: h.value });
  return lines;
});

/** 尺寸线两端 45° 斜切符号 */
const dimTicks = computed<Line[]>(() => {
  if (!props.showDims) return [];
  const t = 0.7;
  const lines: Line[] = [];
  dimSegs.value.forEach(d => {
    lines.push({ x1: d.x1 - t, y1: d.y1 + t, x2: d.x1 + t, y2: d.y1 - t });
    lines.push({ x1: d.x2 - t, y1: d.y2 + t, x2: d.x2 + t, y2: d.y2 - t });
  });
  return lines;
});
</script>

<template>
  <div ref="rootEl" class="relative flex justify-center" :style="{ maxHeight: `${maxHeight}px` }">
    <svg
      :viewBox="viewBox"
      preserveAspectRatio="xMidYMid meet"
      class="block h-auto max-h-full w-auto max-w-full"
    >
      <!-- 尺寸标注 (最底层) -->
      <g v-if="showDims">
        <line
          v-for="(l, i) in extLines"
          :key="`ext${i}`"
          :x1="l.x1"
          :y1="l.y1"
          :x2="l.x2"
          :y2="l.y2"
          :stroke="EXT_COLOR"
          :stroke-width="0.3"
        />
        <line
          v-for="(d, i) in dimSegs"
          :key="`dim${i}`"
          :x1="d.x1"
          :y1="d.y1"
          :x2="d.x2"
          :y2="d.y2"
          :stroke="DIM_COLOR"
          :stroke-width="0.35"
        />
        <line
          v-for="(l, i) in dimTicks"
          :key="`tick${i}`"
          :x1="l.x1"
          :y1="l.y1"
          :x2="l.x2"
          :y2="l.y2"
          :stroke="DIM_COLOR"
          :stroke-width="0.5"
        />
        <text
          v-for="(d, i) in dimSegs"
          :key="`dimt${i}`"
          :x="d.vertical ? d.x1 - 1 : (d.x1 + d.x2) / 2"
          :y="d.vertical ? (d.y1 + d.y2) / 2 : d.y1 - 0.9"
          :transform="d.vertical ? `rotate(-90 ${d.x1 - 1} ${(d.y1 + d.y2) / 2})` : undefined"
          text-anchor="middle"
          dominant-baseline="middle"
          :fill="DIM_TEXT"
          :font-size="5"
          pointer-events="none"
        >
          {{ d.label }}
        </text>
      </g>

      <g v-for="box in cellBoxes" :key="`${box.row}-${box.col}`">
        <rect
          :x="box.x"
          :y="box.y"
          :width="box.w"
          :height="box.h"
          :fill="box.cell === CELL_SASH ? SASH_FILL : FIXED_FILL"
          :class="readonly ? '' : 'cursor-pointer'"
          @click="onCellClick(box)"
          @contextmenu="onPaneContextmenu($event, box)"
        />
        <!-- 开启扇内框示意 -->
        <rect
          v-if="box.cell === CELL_SASH"
          :x="box.x + sashInset(box)"
          :y="box.y + sashInset(box)"
          :width="box.w - sashInset(box) * 2"
          :height="box.h - sashInset(box) * 2"
          fill="none"
          stroke="#64748b"
          :stroke-width="Math.max(c / 5, 0.4)"
          pointer-events="none"
        />
        <!-- 类型 + 净尺寸标注 (空间足够时显示) -->
        <text
          v-if="fontSize(box) >= 3.2"
          :x="box.x + box.w / 2"
          :y="box.y + box.h / 2"
          text-anchor="middle"
          dominant-baseline="middle"
          fill="#334155"
          :font-size="fontSize(box)"
          pointer-events="none"
        >
          <tspan :x="box.x + box.w / 2" dy="-0.3em">{{ box.cell === CELL_SASH ? '开启' : '固定' }}</tspan>
          <tspan :x="box.x + box.w / 2" dy="1.3em">{{ box.w }}×{{ box.h }}</tspan>
        </text>
      </g>

      <!-- 中梃 -->
      <rect
        v-for="(seg, i) in vMullionSegs"
        :key="`v${i}`"
        :x="seg.x"
        :y="seg.y"
        :width="c"
        :height="seg.h"
        :fill="FRAME_COLOR"
        pointer-events="none"
      />
      <rect
        v-for="(y, i) in hMullions"
        :key="`hz${i}`"
        :x="c"
        :y="y"
        :width="w - 2 * c"
        :height="c"
        :fill="FRAME_COLOR"
        pointer-events="none"
      />

      <!-- 外框 (描边居中在边缘内) -->
      <rect
        :x="c / 2"
        :y="c / 2"
        :width="w - c"
        :height="h - c"
        fill="none"
        :stroke="FRAME_COLOR"
        :stroke-width="c"
        pointer-events="none"
      />
    </svg>

    <!-- 右键操作菜单 -->
    <template v-if="menu">
      <div class="fixed inset-0 z-40" @click="closeMenu" @contextmenu.prevent="closeMenu" />
      <div
        class="absolute z-50 w-44 border border-gray-200 rounded-md bg-white py-1 shadow-lg"
        :style="{ left: `${menu.x}px`, top: `${menu.y}px` }"
      >
        <button
          class="block w-full px-3 py-1.5 text-left text-sm hover:bg-gray-100"
          @click="menuToggle"
        >
          {{ menu.box.cell === CELL_SASH ? $t('page.cut.pdMenuFixed') : $t('page.cut.pdMenuSash') }}
        </button>
        <button
          class="block w-full px-3 py-1.5 text-left text-sm hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-40"
          :disabled="!canMergeLeft"
          @click="menuMerge('left')"
        >
          {{ $t('page.cut.pdMenuMergeLeft') }}
        </button>
        <button
          class="block w-full px-3 py-1.5 text-left text-sm hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-40"
          :disabled="!canMergeRight"
          @click="menuMerge('right')"
        >
          {{ $t('page.cut.pdMenuMerge') }}
        </button>
        <button
          class="block w-full px-3 py-1.5 text-left text-sm hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-40"
          :disabled="!canSplit"
          @click="menuSplit"
        >
          {{ $t('page.cut.pdMenuSplit') }}
        </button>
      </div>
    </template>
  </div>
</template>
