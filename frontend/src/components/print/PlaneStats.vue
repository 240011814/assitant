<script setup lang="ts">
import { computed } from 'vue';
import { $t } from '@/locales';

const props = defineProps<{
  results: Api.Cut.BinResult[];
}>();

// 总项目数
const totalItems = computed(() =>
  props.results.reduce((sum: number, bin: Api.Cut.BinResult) => sum + bin.pieces.length, 0)
);

// 项目面积
const totalItemArea = computed(() =>
  props.results.reduce(
    (sum: number, bin: Api.Cut.BinResult) => sum + bin.pieces.reduce((s, p) => s + ((p.w / 100) * p.h) / 100, 0),
    0
  )
);

// 材料面积
const totalMaterialArea = computed(() =>
  props.results.reduce(
    (sum: number, bin: Api.Cut.BinResult) => sum + ((bin.materialWidth / 100) * bin.materialHeight) / 100,
    0
  )
);

// 总体利用率
const overallUtilization = computed(() => {
  if (totalMaterialArea.value === 0) return 0;
  return (totalItemArea.value / totalMaterialArea.value) * 100;
});
</script>

<template>
  <NCard v-if="results.length" :title="$t('page.cut.summaryTitle')" size="large" class="mb-4">
    <p>
      {{
        $t('page.cut.planeStatsLine', {
          items: totalItems,
          itemArea: totalItemArea.toFixed(1),
          materialArea: totalMaterialArea.toFixed(1),
          utilization: overallUtilization.toFixed(1),
          materialCount: results.length
        })
      }}
    </p>
  </NCard>
</template>
