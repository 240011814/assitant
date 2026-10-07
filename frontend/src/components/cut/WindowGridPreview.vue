<script setup lang="ts">
import { computed } from 'vue';
import { CELL_FIXED, CELL_SASH, type ProductItem } from './window-template';

/**
 * 窗户分格 SVG 预览: 按真实比例绘制外框/中梃/分格, 格内标注类型与净尺寸。
 * 编辑模式 (readonly=false) 下点击格子切换 固定格/开启扇。
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

const emit = defineEmits<{ (e: 'toggleCell', row: number, col: number): void }>();

const FRAME_COLOR = '#94a3b8';
const FIXED_FILL = '#dbeafe';
const SASH_FILL = '#fde68a';

const w = computed(() => Math.max(props.item.width, 1));
const h = computed(() => Math.max(props.item.height, 1));
const c = computed(() => props.item.frameWidth);

interface CellBox {
  x: number;
  y: number;
  w: number;
  h: number;
  row: number;
  col: number;
  cell: number;
}

const cellBoxes = computed<CellBox[]>(() => {
  const { grid } = props.item;
  const frame = c.value;
  const boxes: CellBox[] = [];
  let y = frame;
  grid.rows.forEach((rowH, r) => {
    let x = frame;
    grid.cols.forEach((colW, ci) => {
      boxes.push({
        x,
        y,
        w: colW,
        h: rowH,
        row: r,
        col: ci,
        cell: grid.cells[r]?.[ci] ?? CELL_FIXED
      });
      x += colW + (grid.sliding ? 0 : frame);
    });
    y += rowH + frame;
  });
  return boxes;
});

/** 竖向中梃 x 坐标 (推拉窗无) */
const vMullions = computed<number[]>(() => {
  const { grid } = props.item;
  if (grid.sliding) return [];
  const frame = c.value;
  const xs: number[] = [];
  let x = frame;
  grid.cols.slice(0, -1).forEach(colW => {
    x += colW;
    xs.push(x);
    x += frame;
  });
  return xs;
});

/** 横向中梃 y 坐标 */
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
function sashInset(box: CellBox): number {
  return Math.min(c.value * 0.5, Math.min(box.w, box.h) * 0.3);
}

function fontSize(box: CellBox): number {
  return Math.min(box.w, box.h) / 5;
}

function onCellClick(box: CellBox) {
  if (!props.readonly) emit('toggleCell', box.row, box.col);
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
        v-for="(x, i) in vMullions"
        :key="`v${i}`"
        :x="x"
        :y="c"
        :width="c"
        :height="h - 2 * c"
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
