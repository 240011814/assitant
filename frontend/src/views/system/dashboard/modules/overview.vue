<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useEcharts } from "@/hooks/common/echarts";
import { fetchAdminDashboardStats } from "@/service/api";
import type { AdminDashboardStats } from "@/service/api";

defineOptions({ name: "DashboardOverviewPanel" });

const loading = ref(false);
const stats = ref<AdminDashboardStats | null>(null);

const { domRef, updateOptions } = useEcharts(() => ({
  tooltip: {
    trigger: "axis",
  },
  grid: {
    left: "3%",
    right: "4%",
    bottom: "3%",
    top: "15%",
  },
  xAxis: {
    type: "category",
    boundaryGap: false,
    data: [] as string[],
  },
  yAxis: {
    type: "value",
    minInterval: 1,
  },
  series: [
    {
      color: "#8e9dff",
      name: "操作次数",
      type: "line",
      smooth: true,
      areaStyle: {
        color: {
          type: "linear",
          x: 0,
          y: 0,
          x2: 0,
          y2: 1,
          colorStops: [
            { offset: 0.25, color: "#8e9dff" },
            { offset: 1, color: "#fff" },
          ],
        },
      },
      emphasis: { focus: "series" },
      data: [] as number[],
    },
  ],
}));

const statCards = computed(() => {
  const s = stats.value;
  return [
    { label: "用户总数", value: s?.total_users ?? "-", extra: s ? `近7天新增 ${s.new_users_7d}` : "" },
    { label: "成功登录 (24h)", value: s?.recent_logins_24h ?? "-", extra: "含账密与 2FA 登录" },
    { label: "今日消息", value: s?.messages_today ?? "-", extra: s ? `累计会话 ${s.total_conversations}` : "" },
    { label: "启用任务", value: s?.enabled_jobs ?? "-", extra: s ? `24h 失败执行 ${s.failed_runs_24h}` : "" },
    { label: "今日操作", value: s?.operations_today ?? "-", extra: s ? `失败 ${s.failed_ops_today}` : "" },
  ];
});

async function loadData() {
  loading.value = true;
  try {
    const { data, error } = await fetchAdminDashboardStats();
    if (!error && data) {
      stats.value = data;
      updateOptions((opts) => {
        opts.xAxis.data = data.ops_trend.map((item) => item.date.slice(5));
        opts.series[0].data = data.ops_trend.map((item) => item.count);
        return opts;
      });
    }
  } finally {
    loading.value = false;
  }
}

onMounted(loadData);
</script>

<template>
  <div>
    <NSpin :show="loading">
      <NGrid :x-gap="16" :y-gap="16" cols="2 s:3 m:5" responsive="screen">
        <NGi v-for="card in statCards" :key="card.label">
          <NCard size="small" :bordered="false" shadow="sm">
            <div class="text-sm text-gray-400">{{ card.label }}</div>
            <div class="mt-2 text-2xl font-semibold">{{ card.value }}</div>
            <div class="mt-1 text-xs text-gray-400">{{ card.extra }}</div>
          </NCard>
        </NGi>
      </NGrid>

      <NCard :bordered="false" shadow="sm" title="近 7 天操作趋势" class="mt-4">
        <div ref="domRef" class="h-320px" />
      </NCard>
    </NSpin>
  </div>
</template>
