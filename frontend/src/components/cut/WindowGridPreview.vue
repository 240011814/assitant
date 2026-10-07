<script setup lang="ts">
import { computed } from 'vue';
import { CELL_FIXED, CELL_SASH, CELL_SPAN, type ProductItem } from './window-template';

/**
 * 窗户分格 SVG 预览: 按真实比例绘制外框/中梃/分格, 格内标注类型与净尺寸。
 * 延伸格 (CELL_SPAN) 与左侧格合并绘制为一个整面板 (无竖梃)。
 * 编辑模式 (readonly=false) 下: 单击面板切换 固定/开启; 双击合并面板从右侧拆出一列。
 */
const props = withDefaults(
  defineProps<{
    item: Pick<ProductItem, 'width' | 'height' | 'frameWidth' | 'grid'>;
    /** 只读 (列表缩略图), 不可点击 */
    readonly?: boolean;
    /** 预览区最大高度 px */
    maxHeight?: number;
  }>(),
  { readonly: false, maxHeight: 380 }
);

const emit = defineEmits<{ (e: 'toggleCell', row: number, col: number): void; (e: 'splitCell', row: number, col: number): void }>();

const FRAME_COLOR = '#94a3b8';
const FIXED_FILL = '#dbeafe';
const SASH_FILL = '#fde68a';

const w = computed(() => Math.max(props.item.width, 1));
const h = computed(() => Math.max(props.item.height, 1));
const c = computed(() => props.item.frameWidth);

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

function onCellDblclick(box: PaneBox) {
  if (!props.readonly && box.endCol > box.col) emit('splitCell', box.row, box.col);
}
</script>

<template>
  <div class="flex justify-center" :style="{ maxHeight: `${maxHeight}px` }">
    <svg
      :viewBox="`0 0 ${w} ${h}`"
      preserveAspectRatio="xMidYMid meet"
      class="block h-auto max-h-full w-auto max-w-full"
    >
      <g v-for="box in cellBoxes" :key="`${box.row}-${box.col}`">
        <rect
          :x="box.x"
          :y="box.y"
          :width="box.w"
          :height="box.h"
          :fill="box.cell === CELL_SASH ? SASH_FILL : FIXED_FILL"
          :class="readonly ? '' : 'cursor-pointer'"
          @click="onCellClick(box)"
          @dblclick.prevent="onCellDblclick(box)"
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
  </div>
</template>
