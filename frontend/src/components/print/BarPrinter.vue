<script setup lang="ts">
import { computed, ref } from 'vue';
import { NButton } from 'naive-ui';
import { $t } from '@/locales';

const props = defineProps<{
  data: Api.Cut.BarResult[] | null;
}>();

const printArea = ref<HTMLDivElement | null>(null);

// 颜色池
const randomColors = Array.from({ length: 50 }, (_, i) => `hsl(${(i * 30) % 360}, 70%, 50%)`);

// 统计信息
const summary = computed(() => {
  if (!props.data || props.data.length === 0) return null;

  const totalMaterials = props.data.length;
  let totalLength = 0;
  let totalRemaining = 0;
  let totalUsed = 0;

  props.data.forEach((item: Api.Cut.BarResult) => {
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

// 段宽占比(%), 低于阈值不直标尺寸(过窄会溢出), 靠下方切割顺序行补充
function segPercent(value: number, totalLength: number): number {
  return totalLength > 0 ? (value / totalLength) * 100 : 0;
}

function printResult() {
  if (!printArea.value) return;

  const printContent = printArea.value.innerHTML;
  const printWindow = window.open('', '');
  // 弹窗被浏览器拦截时不直接崩, 提示用户放行
  if (!printWindow) {
    window.$message?.warning($t('page.cut.printBlocked'));
    return;
  }

  printWindow.document.write(`
    <html>
      <head>
        <style>
          @page { size: A4; margin: 15mm; }
          body { font-family: Arial, sans-serif; }
          table { width: 100%; border-collapse: collapse; }
          th, td { border: 1px solid #000; padding: 4px; text-align: center; }
          .bar-container { display: grid; grid-auto-flow: column; width: 100%; height: 24px; }
          .bar-segment {
            border-right: 2px solid #1f2937;
            display: flex; align-items: center; justify-content: center;
            font-size: 11px; font-weight: bold; color: #fff;
            text-shadow: 0 0 2px rgba(0, 0, 0, 0.6);
            overflow: hidden;
            -webkit-print-color-adjust: exact; print-color-adjust: exact;
          }
          .bar-remaining {
            background-color: #d1d5db; color: #111827;
            display: flex; align-items: center; justify-content: center;
            font-size: 11px; overflow: hidden;
            -webkit-print-color-adjust: exact; print-color-adjust: exact;
          }
          .cut-seq { margin-top: 2px; font-size: 10px; color: #374151; text-align: left; }
          .hidden-print { display: block !important; }
        </style>
      </head>
      <body>
        ${printContent}
      </body>
    </html>
  `);

  printWindow!.document.close();
  printWindow!.focus();
  printWindow!.print();
  printWindow!.close();
}
</script>

<template>
  <div>
    <NButton type="primary" :disabled="!data || data.length === 0" @click="printResult">{{ $t('page.cut.printChart') }}</NButton>

    <!-- 隐藏打印区域 -->
    <div ref="printArea" class="hidden-print">
      <h2>{{ $t('page.cut.barPrintTitle') }}</h2>

      <!-- 表格 -->
      <table border="1" cellspacing="0" cellpadding="4" class="bar-table">
        <thead>
          <tr>
            <th>{{ $t('page.cut.colBarIndex') }}</th>
            <th>{{ $t('page.cut.materialType') }}</th>
            <th>{{ $t('page.cut.colTotalLengthCm') }}</th>
            <th>{{ $t('page.cut.colUsedCm') }}</th>
            <th>{{ $t('page.cut.colRemainingCm') }}</th>
            <th>{{ $t('page.cut.colCutDetail') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in data" :key="item.index">
            <td>{{ item.index }}</td>
            <td>{{ item.materialType?.trim() || $t('page.cut.summaryNewMaterial') }}</td>
            <td>{{ item.totalLength }}</td>
            <td>{{ item.used }}</td>
            <td>{{ item.remaining }}</td>
            <td>
              <div
                class="bar-container"
                :style="{
                  gridTemplateColumns: [
                    ...item.cuts.map(c => `${segPercent(Number(c), item.totalLength)}%`),
                    ...(item.remaining > 0 ? [`${segPercent(item.remaining, item.totalLength)}%`] : [])
                  ].join(' ')
                }"
              >
                <div
                  v-for="(cut, idx) in item.cuts"
                  :key="idx"
                  class="bar-segment"
                  :style="{ backgroundColor: randomColors[Number(idx) % randomColors.length] }"
                >
                  <span v-if="segPercent(Number(cut), item.totalLength) >= 8">{{ cut }}</span>
                </div>
                <div v-if="item.remaining > 0" class="bar-remaining">
                  <span v-if="segPercent(item.remaining, item.totalLength) >= 8">
                    {{ $t('page.cut.remainShort', { remaining: item.remaining }) }}
                  </span>
                </div>
              </div>
              <div class="cut-seq">
                {{ $t('page.cut.cutOrder', { cuts: item.cuts.join(' + ') }) }}{{
                  item.remaining > 0 ? $t('page.cut.remainTail', { remaining: item.remaining }) : ''
                }}
              </div>
            </td>
          </tr>
        </tbody>
      </table>

      <!-- 总统计 -->
      <p v-if="summary">
        {{
          $t('page.cut.barSummary', {
            count: summary.totalMaterials,
            total: summary.totalLength,
            used: summary.totalUsed,
            remaining: summary.totalRemaining,
            usage: summary.usagePercent
          })
        }}
      </p>
    </div>
  </div>
</template>

<style>
.hidden-print {
  display: none;
}

.bar-table {
  width: 100%;
  border-collapse: collapse;
}

/* 与 printResult 写入打印窗口的样式保持一致, 页面直接 Ctrl+P 时同样生效 */
.bar-container {
  display: grid;
  grid-auto-flow: column;
  width: 100%;
  height: 24px;
}

.bar-segment {
  border-right: 2px solid #1f2937;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-weight: bold;
  color: #fff;
  text-shadow: 0 0 2px rgba(0, 0, 0, 0.6);
  overflow: hidden;
}

.bar-remaining {
  background-color: #d1d5db;
  color: #111827;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  overflow: hidden;
}

.cut-seq {
  margin-top: 2px;
  font-size: 10px;
  color: #374151;
  text-align: left;
}

@media print {
  .hidden-print {
    display: block !important;
  }
}
</style>
