<script setup lang="ts">
import { computed, h, onMounted, ref } from "vue";
import { NTag } from "naive-ui";
import type { DataTableColumns } from "naive-ui";
import { fetchGetLearningStats, fetchGetReviewForecast } from "@/service/api";
import { useAppStore } from "@/store/modules/app";

const appStore = useAppStore();
const loading = ref(false);
const stats = ref<Api.Vocabulary.LearningStats | null>(null);
const forecast = ref<Api.Vocabulary.Forecast | null>(null);

// 近 90 天热力图: 补齐无记录的日期
interface HeatCell {
  date: string;
  count: number;
}

const heatmapCells = computed<HeatCell[]>(() => {
  const map = new Map<string, number>();
  (stats.value?.heatmap || []).forEach((p) => map.set(p.date, p.count));
  const cells: HeatCell[] = [];
  const now = new Date();
  for (let i = 89; i >= 0; i -= 1) {
    const d = new Date(now);
    d.setDate(now.getDate() - i);
    const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
    cells.push({ date: key, count: map.get(key) || 0 });
  }
  return cells;
});

const heatLevel = (count: number) => {
  if (count === 0) return 0;
  if (count <= 5) return 1;
  if (count <= 15) return 2;
  if (count <= 30) return 3;
  return 4;
};

const heatColors = [
  "bg-gray-100 dark:bg-gray-800",
  "bg-success/25",
  "bg-success/50",
  "bg-success/75",
  "bg-success",
];

// 到期预测柱状 (取前 14 天)
const forecastBars = computed(() => (forecast.value?.forecast || []).slice(0, 14));
const forecastMax = computed(() => Math.max(1, ...forecastBars.value.map((p) => p.count)));

const troubleColumns = ref<DataTableColumns<Api.Vocabulary.Item>>([
  {
    title: "单词",
    key: "word",
    width: 140,
    render(row) {
      return h("span", { class: "font-bold text-primary" }, row.word);
    },
  },
  { title: "释义", key: "definition", minWidth: 200, ellipsis: { tooltip: true } },
  {
    title: "遗忘次数",
    key: "fsrsLapses",
    width: 100,
    render(row) {
      return h(NTag, { size: "small", type: "error", bordered: false }, { default: () => `×${row.fsrsLapses}` });
    },
  },
  {
    title: "下次复习",
    key: "nextReviewAt",
    width: 120,
    render(row) {
      return row.nextReviewAt ? new Date(row.nextReviewAt).toLocaleDateString() : "—";
    },
  },
]);

const statCards = computed(() => {
  if (!stats.value) return [];
  const s = stats.value;
  return [
    { label: "总词数", value: s.total, color: "text-primary" },
    { label: "已掌握", value: s.mastered, color: "text-success" },
    { label: "待学习新词", value: s.newWords, color: "text-warning" },
    { label: "连续复习天数", value: `${s.streak} 天`, color: "text-info" },
    { label: "30 天保持率", value: `${Math.round(s.retention30 * 100)}%`, color: "text-success" },
    { label: "30 天复习量", value: s.reviewCount30, color: "text-primary" },
    { label: "总复习次数", value: s.totalReviews, color: "text-info" },
    { label: "每掌握一词复习", value: Math.round(s.avgReviewsPerMastered * 10) / 10, color: "text-warning" },
  ];
});

const loadData = async () => {
  loading.value = true;
  const [statsRes, forecastRes] = await Promise.all([fetchGetLearningStats(), fetchGetReviewForecast({ days: 30 })]);
  loading.value = false;
  if (!statsRes.error && statsRes.data) {
    stats.value = statsRes.data;
  }
  if (!forecastRes.error && forecastRes.data) {
    forecast.value = forecastRes.data;
  }
};

onMounted(() => {
  loadData();
});

defineExpose({ loadData });
</script>

<template>
  <div class="flex flex-col gap-4">
    <NSpin :show="loading">
      <!-- 统计卡片 -->
      <div class="grid gap-3 mb-4" :class="appStore.isMobile ? 'grid-cols-2' : 'grid-cols-4'">
        <NCard v-for="card in statCards" :key="card.label" size="small" :bordered="false">
          <div class="flex flex-col gap-1">
            <span class="text-xs text-gray-400">{{ card.label }}</span>
            <span class="text-2xl font-bold" :class="card.color">{{ card.value }}</span>
          </div>
        </NCard>
      </div>

      <div class="flex flex-col gap-4">
        <!-- 复习热力图 -->
        <NCard title="复习热力图 (近 90 天)" size="small" :bordered="false">
          <div class="flex gap-[3px] overflow-x-auto py-1">
            <div class="grid grid-rows-7 gap-[3px]" style="grid-auto-flow: column; grid-auto-columns: 12px">
              <div
                v-for="cell in heatmapCells"
                :key="cell.date"
                class="h-12px w-12px rounded-sm"
                :class="heatColors[heatLevel(cell.count)]"
                :title="`${cell.date}: ${cell.count} 次`"
              />
            </div>
          </div>
          <div class="mt-2 flex items-center gap-2 text-xs text-gray-400">
            少
            <span v-for="c in heatColors" :key="c" class="h-10px w-10px rounded-sm inline-block" :class="c" />
            多
          </div>
        </NCard>

        <div class="grid gap-4" :class="appStore.isMobile ? 'grid-cols-1' : 'grid-cols-2'">
          <!-- 到期预测 -->
          <NCard title="到期预测 (未来 14 天)" size="small" :bordered="false">
            <template #header-extra>
              <span class="text-xs text-gray-400">未排期新词: {{ forecast?.newWords ?? '—' }}</span>
            </template>
            <div v-if="forecastBars.length === 0" class="text-sm text-gray-400 py-4 text-center">
              暂无排期数据, 开始复习后自动生成
            </div>
            <div v-else class="flex flex-col gap-1.5">
              <div v-for="p in forecastBars" :key="p.date" class="flex items-center gap-2 text-xs">
                <span class="w-20 shrink-0 text-gray-400">{{ p.date.slice(5) }}</span>
                <div class="flex-1 h-4 rounded bg-gray-100 dark:bg-gray-800 overflow-hidden">
                  <div
                    class="h-full rounded bg-primary/70"
                    :style="{ width: `${Math.round((p.count / forecastMax) * 100)}%` }"
                  />
                </div>
                <span class="w-8 text-right font-bold">{{ p.count }}</span>
              </div>
            </div>
          </NCard>

          <!-- 易错词 -->
          <NCard title="易错词 Top 10" size="small" :bordered="false">
            <NDataTable
              :columns="troubleColumns"
              :data="stats?.troubleWords || []"
              :row-key="(row: any) => row.id"
              size="small"
              :pagination="false"
            />
          </NCard>

          <!-- 说明 -->
          <NCard title="如何提高保持率" size="small" :bordered="false" class="col-span-full">
            <ul class="list-disc pl-5 flex flex-col gap-2 text-sm text-gray-600 dark:text-gray-300">
              <li>保持率低于 85%: 说明复习间隔偏长, 可在复习时对模糊的词诚实点「困难」。</li>
              <li>「每掌握一词复习」次数高: 优先用「易混词辨析」和「造句批改」加深加工深度。</li>
              <li>易错词先做「AI 助记」(生词本页词行上的灯泡按钮), 换例句再复习效果更好。</li>
              <li>每天固定新词限额 (默认 20), 避免到期堆积; 用「碎片时间模式」做小批量复习。</li>
            </ul>
          </NCard>
        </div>
      </div>
    </NSpin>
  </div>
</template>

<style scoped></style>
