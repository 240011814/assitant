<script setup lang="ts">
import { computed, ref } from 'vue';
import { NButton } from 'naive-ui';

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

function printResult() {
  if (!printArea.value) return;

  const printContent = printArea.value.innerHTML;
  const printWindow = window.open('', '');

  printWindow!.document.write(`
    <html>
      <head>
        <style>
          @page { size: A4; margin: 15mm; }
          body { font-family: Arial, sans-serif; }
          table { width: 100%; border-collapse: collapse; }
          th, td { border: 1px solid #000; padding: 4px; text-align: center; }
          .bar-container { display: grid; grid-auto-flow: column; width: 100%; height: 20px; }
          .bar-segment { border: 1px solid #fff; text-align: center; font-size: 10px; color: white; overflow: hidden; }
          .bar-remaining { background-color: #ccc; color: black; }
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
    <NButton type="primary" :disabled="!data || data.length === 0" @click="printResult">打印</NButton>

    <!-- 隐藏打印区域 -->
    <div ref="printArea" class="hidden-print">
      <h2>材料裁剪结果</h2>

      <!-- 表格 -->
      <table border="1" cellspacing="0" cellpadding="4" class="bar-table">
        <thead>
          <tr>
            <th>材料编号</th>
            <th>材料类型</th>
            <th>总长度(cm)</th>
            <th>已用(cm)</th>
            <th>剩余(cm)</th>
            <th>切割情况</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="item in data" :key="item.index">
            <td>{{ item.index }}</td>
            <td>{{ item.materialType?.trim() || '新材料' }}</td>
            <td>{{ item.totalLength }}</td>
            <td>{{ item.used }}</td>
            <td>{{ item.remaining }}</td>
            <td colspan="4">
              <div
                class="bar-container"
                :style="{
                  gridTemplateColumns: [
                    ...item.cuts.map(c => (c / item.totalLength) * 100 + '%'),
                    item.remaining > 0 ? (item.remaining / item.totalLength) * 100 + '%' : ''
                  ].join(' ')
                }"
              >
                <div
                  v-for="(cut, idx) in item.cuts"
                  :key="idx"
                  class="bar-segment"
                  :style="{ backgroundColor: randomColors[idx % randomColors.length] }"
                >
                  {{ cut }}cm
                </div>
              </div>
            </td>
          </tr>
        </tbody>
      </table>

      <!-- 总统计 -->
      <p v-if="summary">
        材料总数: {{ summary.totalMaterials }} 根 | 总长度: {{ summary.totalLength }} cm | 已用长度:
        {{ summary.totalUsed }} cm | 剩余长度: {{ summary.totalRemaining }} cm | 使用率: {{ summary.usagePercent }}%
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

@media print {
  .hidden-print {
    display: block !important;
  }
}
</style>
