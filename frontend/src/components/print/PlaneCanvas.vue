<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue';
import { NCard } from 'naive-ui';
import { $t } from '@/locales';

const props = defineProps<{
  results: Api.Cut.BinResult[];
  materials: Api.Cut.Item[];
  groupData: boolean;
}>();

// canvas 引用
const canvases = ref<HTMLCanvasElement[]>([]);
const canvasImages = ref<string[]>([]);

function isRemainderMaterial(bin: Api.Cut.BinResult) {
  if (!bin.materialType) return false;
  return props.materials.some(m => bin.materialType?.startsWith(m.label));
}

// 给切割方案生成唯一 key（忽略 label）
function getBinKey(bin: Api.Cut.BinResult) {
  const piecesKey = bin.pieces
    .map(p => `${p.x},${p.y},${p.w},${p.h}`) // ⚠️ 不要 label
    .sort()
    .join('|');
  return `${bin.materialType}-${bin.materialWidth}x${bin.materialHeight}-${piecesKey}`;
}

// 聚合结果
const groupedResults = computed(() => {
  if (!props.groupData) {
    return props.results.map((bin: Api.Cut.BinResult) => ({ bin, count: 1, labels: bin.pieces.map(p => p.label) }));
  }
  const groups = new Map<string, { bin: Api.Cut.BinResult; count: number; labels: string[] }>();
  props.results.forEach((bin: Api.Cut.BinResult) => {
    const key = getBinKey(bin);
    if (!groups.has(key)) {
      groups.set(key, { bin, count: 1, labels: bin.pieces.map(p => p.label) });
    } else {
      const g = groups.get(key)!;
      g.count += 1;
      g.labels.push(...bin.pieces.map(p => p.label));
    }
  });
  return Array.from(groups.values());
});

// 绘制逻辑
function drawAllBins() {
  if (!groupedResults.value.length) return;

  canvasImages.value = [];
  canvases.value = canvases.value.slice(0, groupedResults.value.length);
  const maxMaterialWidth = Math.max(...groupedResults.value.map(g => g.bin.materialWidth));
  const maxMaterialHeight = Math.max(...groupedResults.value.map(g => g.bin.materialHeight));
  const maxCanvasSize = 400;
  const scale = Math.min(maxCanvasSize / maxMaterialWidth, maxCanvasSize / maxMaterialHeight);

  canvases.value.forEach((canvas: HTMLCanvasElement | null, index: number) => {
    const group = groupedResults.value[index];
    const bin = group.bin;
    if (!canvas) return;

    const ctx = canvas.getContext('2d')!;
    const widthPx = bin.materialWidth * scale;
    const heightPx = bin.materialHeight * scale;

    canvas.width = widthPx;
    canvas.height = heightPx;

    const isRemainder = isRemainderMaterial(bin);

    // 背景
    ctx.fillStyle = isRemainder ? '#e8f5e8' : '#e3f2fd';
    ctx.fillRect(0, 0, widthPx, heightPx);

    // 边框
    ctx.strokeStyle = '#000';
    ctx.lineWidth = 2;
    ctx.strokeRect(0, 0, widthPx, heightPx);

    // 网格线 (10cm)
    ctx.strokeStyle = '#bbb';
    ctx.lineWidth = 1;
    for (let x = 0; x <= bin.materialWidth; x += 10) {
      const px = x * scale;
      ctx.beginPath();
      ctx.moveTo(px, 0);
      ctx.lineTo(px, heightPx);
      ctx.stroke();
    }
    for (let y = 0; y <= bin.materialHeight; y += 10) {
      const py = y * scale;
      ctx.beginPath();
      ctx.moveTo(0, py);
      ctx.lineTo(widthPx, py);
      ctx.stroke();
    }

    // 绘制 piece (确定性颜色: 同一零件每次绘制颜色一致, 不随重绘变化)
    bin.pieces.forEach((piece: Api.Cut.Piece) => {
      let hash = 0;
      for (let i = 0; i < piece.label.length; i += 1) {
        hash = (hash * 31 + piece.label.charCodeAt(i)) % 360;
      }
      const color = `hsl(${hash}, 70%, 80%)`;

      const x = piece.x * scale;
      const y = piece.y * scale;
      const w = piece.w * scale;
      const h = piece.h * scale;

      ctx.fillStyle = color;
      ctx.fillRect(x, y, w, h);

      ctx.strokeStyle = '#000';
      ctx.lineWidth = 1;
      ctx.strokeRect(x, y, w, h);

      // 标签
      ctx.fillStyle = 'rgba(f,f,f,1)';
      const labelWidth = Math.min(w - 4, 120);
      ctx.fillRect(x + 2, y + 2, labelWidth, 36);

      ctx.fillStyle = 'white';
      ctx.font = '12px Arial';
      ctx.fillText(piece.label, x + 6, y + 16);

      const sizeText = `${piece.w.toFixed(1)}×${piece.h.toFixed(1)}cm`;
      ctx.font = '11px Arial';
      ctx.fillText(sizeText, x + 6, y + 30);
    });

    canvasImages.value[index] = canvas.toDataURL('image/png');
  });
}

// 监听变化 (挂载后画一次 + 数据变化再画; 旧实现 immediate + onMounted 双路径会重复绘制两遍)
watch(
  () => [props.results, props.groupData],
  async () => {
    await nextTick();
    drawAllBins();
  },
  { deep: true }
);

onMounted(async () => {
  await nextTick();
  drawAllBins();
});
</script>

<template>
  <NCard v-if="groupedResults.length">
    <h3 class="mb-4 text-xl font-semibold">
      {{
        $t('page.cut.planeResultTitle', {
          materials: props.results.length,
          pieces: props.results.reduce((s, b) => s + b.pieces.length, 0)
        })
      }}
    </h3>

    <div class="grid grid-cols-1 gap-6 lg:grid-cols-3 sm:grid-cols-2">
      <div
        v-for="(group, index) in groupedResults"
        :key="index"
        class="bin-card overflow-hidden border rounded-lg"
        :class="{
          'border-green-400 bg-green-50': isRemainderMaterial(group.bin),
          'border-blue-400 bg-blue-50': !isRemainderMaterial(group.bin)
        }"
      >
        <div class="bg-gray-100 p-3">
          <h3 class="text-gray-800 font-semibold">
            {{ group.bin.materialType || $t('page.cut.material') }}
          </h3>
          <p class="text-sm text-gray-600">
            {{
              $t('page.cut.binStats', {
                width: group.bin.materialWidth.toFixed(1),
                height: group.bin.materialHeight.toFixed(1),
                utilization: group.bin.utilization.toFixed(1),
                count: group.count
              })
            }}
            <span v-if="isRemainderMaterial(group.bin)" class="text-green-600">{{ $t('page.cut.remainingMaterial') }}</span>
          </p>
        </div>
        <div class="aspect-square bg-white">
          <canvas :ref="el => (canvases[index] = el as HTMLCanvasElement)" class="h-full w-full"></canvas>
        </div>
      </div>
    </div>
  </NCard>
</template>
