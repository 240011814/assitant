<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { NDataTable, NSpin, NTabPane, NTabs, useMessage } from "naive-ui";
import { stockFinanceHistory } from "@/service/api";
import { useEcharts } from "@/hooks/common/echarts";
import type { ECOption } from "@/hooks/common/echarts";
import { $t } from "@/locales";

/**
 * 历史财务数据面板: 报告期区间(近16期/全部) + 图表(按指标分组)/表格两种视图。
 * 数据加载与渲染自持, 父组件经 reload() 在同步完成后触发刷新。
 */
const props = defineProps<{
  code: string;
}>();

const message = useMessage();

const financeHistory = ref<Api.Stock.FinanceHistory[]>([]);
const financeViewMode = ref("chart");
const financeRange = ref<"recent" | "all">("recent");
const financeLoading = ref(false);
const financeChartGroup = ref<"profit" | "growth" | "operation" | "solvency">("profit");

// 图表由 useEcharts hook 托管(domRef + updateOptions, 自带 resize 监听/主题跟随/销毁)
const { domRef: chartRef, updateOptions: updateFinanceChart } = useEcharts<ECOption>(() => ({
  tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
  legend: { data: [], top: 0, textStyle: { fontSize: 12 } },
  grid: { left: 50, right: 20, top: 40, bottom: 30 },
  xAxis: { type: 'category', data: [], axisLabel: { fontSize: 11 } },
  yAxis: [{ type: 'value', name: '', axisLabel: { fontSize: 11 } }],
  series: []
}));

const financeColumns = [
  {
    title: $t("page.tool.stockDetail.reportDate"),
    key: "reportDate",
    width: 100,
    fixed: "left" as const,
    render: (row: Api.Stock.FinanceHistory) => row.reportDate?.slice(0, 10) || "-",
  },
  {
    title: $t("page.tool.stockDetail.roePct"),
    key: "roe",
    width: 70,
    render: (row: Api.Stock.FinanceHistory) => row.roe?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.grossMarginPct"),
    key: "grossMargin",
    width: 70,
    render: (row: Api.Stock.FinanceHistory) => row.grossMargin?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.netMarginPct"),
    key: "netMargin",
    width: 70,
    render: (row: Api.Stock.FinanceHistory) => row.netMargin?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.revenueWan"),
    key: "revenue",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) => row.revenue?.toFixed(0) || "-",
  },
  {
    title: $t("page.tool.stockDetail.revenueYoyPct"),
    key: "revenueYoy",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) =>
      row.revenueYoy !== null
        ? `${row.revenueYoy >= 0 ? "+" : ""}${row.revenueYoy.toFixed(2)}%`
        : "-",
  },
  {
    title: $t("page.tool.stockDetail.netProfitWan"),
    key: "netProfit",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) => row.netProfit?.toFixed(0) || "-",
  },
  {
    title: $t("page.tool.stockDetail.netProfitYoyPct"),
    key: "netProfitYoy",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) =>
      row.netProfitYoy !== null
        ? `${row.netProfitYoy >= 0 ? "+" : ""}${row.netProfitYoy.toFixed(2)}%`
        : "-",
  },
  {
    title: $t("page.tool.stockDetail.eps"),
    key: "eps",
    width: 60,
    render: (row: Api.Stock.FinanceHistory) => row.eps?.toFixed(3) || "-",
  },
  {
    title: $t("page.tool.stockDetail.equityYoyPct"),
    key: "yoyEquity",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) =>
      row.yoyEquity !== null
        ? `${row.yoyEquity >= 0 ? "+" : ""}${row.yoyEquity.toFixed(2)}%`
        : "-",
  },
  {
    title: $t("page.tool.stockDetail.assetYoyPct"),
    key: "yoyAsset",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) =>
      row.yoyAsset !== null
        ? `${row.yoyAsset >= 0 ? "+" : ""}${row.yoyAsset.toFixed(2)}%`
        : "-",
  },
  {
    title: $t("page.tool.stockDetail.epsYoyPct"),
    key: "yoyEps",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) =>
      row.yoyEps !== null
        ? `${row.yoyEps >= 0 ? "+" : ""}${row.yoyEps.toFixed(2)}%`
        : "-",
  },
  {
    title: $t("page.tool.stockDetail.debtRatioPct"),
    key: "debtRatio",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) => row.debtRatio?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.currentRatio"),
    key: "currentRatio",
    width: 70,
    render: (row: Api.Stock.FinanceHistory) => row.currentRatio?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.quickRatio"),
    key: "quickRatio",
    width: 70,
    render: (row: Api.Stock.FinanceHistory) => row.quickRatio?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.cashRatio"),
    key: "cashRatio",
    width: 70,
    render: (row: Api.Stock.FinanceHistory) => row.cashRatio?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.nrTurn"),
    key: "nrTurnRatio",
    width: 70,
    render: (row: Api.Stock.FinanceHistory) => row.nrTurnRatio?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.invTurn"),
    key: "invTurnRatio",
    width: 70,
    render: (row: Api.Stock.FinanceHistory) => row.invTurnRatio?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.caTurn"),
    key: "caTurnRatio",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) => row.caTurnRatio?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.assetTurn"),
    key: "assetTurnRatio",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) => row.assetTurnRatio?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.cfoToOr"),
    key: "cfoToOr",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) => row.cfoToOr?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.cfoToNp"),
    key: "cfoToNp",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) => row.cfoToNp?.toFixed(2) || "-",
  },
];

let financeReqSeq = 0;
async function loadFinance() {
  const seq = ++financeReqSeq;
  financeLoading.value = true;
  try {
    const res = await stockFinanceHistory(props.code, {
      limit: financeRange.value === "all" ? 0 : 16,
    });
    if (seq !== financeReqSeq) return;
    financeHistory.value = res.data || [];
  } catch (e: any) {
    if (seq !== financeReqSeq) return;
    message.error(e.message || $t("page.tool.stockDetail.financeLoadFailed"));
  } finally {
    if (seq === financeReqSeq) {
      financeLoading.value = false;
    }
  }
}

// 财务图表指标分组
const financeChartGroups: Record<
  string,
  {
    name: string;
    unit: string;
    series: { key: keyof Api.Stock.FinanceHistory; name: string; color: string }[];
  }
> = {
  profit: {
    name: $t("page.tool.stockDetail.groupProfit"),
    unit: "%",
    series: [
      { key: "roe", name: $t("page.tool.stockDetail.roePct"), color: "#1890ff" },
      { key: "grossMargin", name: $t("page.tool.stockDetail.grossMarginPct"), color: "#52c41a" },
      { key: "netMargin", name: $t("page.tool.stockDetail.netMarginPct"), color: "#faad14" },
      { key: "debtRatio", name: $t("page.tool.stockDetail.debtRatioPct"), color: "#f5222d" },
    ],
  },
  growth: {
    name: $t("page.tool.stockDetail.groupGrowth"),
    unit: "%",
    series: [
      { key: "revenueYoy", name: $t("page.tool.stockDetail.revenueYoyPct"), color: "#1890ff" },
      { key: "netProfitYoy", name: $t("page.tool.stockDetail.netProfitYoyPct"), color: "#52c41a" },
      { key: "yoyEquity", name: $t("page.tool.stockDetail.equityYoyPct"), color: "#faad14" },
      { key: "yoyAsset", name: $t("page.tool.stockDetail.assetYoyPct"), color: "#f5222d" },
      { key: "yoyEps", name: $t("page.tool.stockDetail.epsYoyPct"), color: "#722ed1" },
    ],
  },
  operation: {
    name: $t("page.tool.stockDetail.groupOperation"),
    unit: $t("page.tool.stockDetail.unitTimes"),
    series: [
      { key: "nrTurnRatio", name: $t("page.tool.stockDetail.nrTurn"), color: "#1890ff" },
      { key: "invTurnRatio", name: $t("page.tool.stockDetail.invTurn"), color: "#52c41a" },
      { key: "caTurnRatio", name: $t("page.tool.stockDetail.caTurn"), color: "#faad14" },
      { key: "assetTurnRatio", name: $t("page.tool.stockDetail.assetTurn"), color: "#f5222d" },
    ],
  },
  solvency: {
    name: $t("page.tool.stockDetail.groupSolvency"),
    unit: $t("page.tool.stockDetail.unitPctPerX"),
    series: [
      { key: "debtRatio", name: $t("page.tool.stockDetail.debtRatioPct"), color: "#f5222d" },
      { key: "currentRatio", name: $t("page.tool.stockDetail.currentRatio"), color: "#1890ff" },
      { key: "quickRatio", name: $t("page.tool.stockDetail.quickRatio"), color: "#52c41a" },
      { key: "cashRatio", name: $t("page.tool.stockDetail.cashRatio"), color: "#faad14" },
      { key: "cfoToOr", name: $t("page.tool.stockDetail.cfoToOr"), color: "#722ed1" },
      { key: "cfoToNp", name: $t("page.tool.stockDetail.cfoToNp"), color: "#13c2c2" },
    ],
  },
};

function getChartOption(): ECOption {
  const data = [...financeHistory.value].reverse();
  const dates = data.map((d) => d.reportDate?.slice(0, 10) || "");
  const group = financeChartGroups[financeChartGroup.value] || financeChartGroups.profit;
  return {
    tooltip: {
      trigger: "axis",
      axisPointer: { type: "shadow" },
    },
    legend: {
      data: group.series.map((s) => s.name),
      top: 0,
      textStyle: { fontSize: 12 },
    },
    grid: { left: 50, right: 20, top: 40, bottom: 30 },
    xAxis: {
      type: "category",
      data: dates,
      axisLabel: { fontSize: 11 },
    },
    yAxis: [
      {
        type: "value",
        name: group.unit,
        axisLabel: { fontSize: 11 },
      },
    ],
    series: group.series.map((s) => ({
      name: s.name,
      type: "line",
      data: data.map((d) => d[s.key] as number | null),
      smooth: true,
      itemStyle: { color: s.color },
    })),
  };
}

// updateOptions 内部先 clear 再 setOption; 图表未渲染时 option 会缓存, 由 hook 在尺寸变化时渲染
function renderChart() {
  if (financeHistory.value.length === 0) return;
  updateFinanceChart(() => getChartOption());
}

watch(financeRange, () => {
  loadFinance();
});

// 路由复用换股时 (父组件 code 变化) 重新加载
watch(() => props.code, () => {
  loadFinance();
});

watch(financeViewMode, (val) => {
  if (val === "chart") {
    renderChart();
  }
});

watch(financeHistory, () => {
  if (financeViewMode.value === "chart") {
    renderChart();
  }
});

watch(financeChartGroup, () => {
  if (financeViewMode.value === "chart") {
    renderChart();
  }
});

onMounted(() => {
  loadFinance();
});

defineExpose({
  /** 同步完成后由父组件触发重新加载 */
  reload: loadFinance,
});
</script>

<template>
  <!-- v-show 保持图表 DOM 稳定 -->
  <div
    v-show="financeHistory.length > 0"
    class="mt-4 p-4 bg-white rounded-lg shadow"
  >
    <div class="flex items-center justify-between mb-3 flex-wrap gap-2">
      <h2 class="text-lg font-bold">
        {{ $t("page.tool.stockDetail.financeHistoryTitle", { count: financeHistory.length }) }}
      </h2>
      <div class="flex items-center gap-2">
        <NTabs
          v-model:value="financeRange"
          type="segment"
          size="small"
          style="width: 200px"
        >
          <NTabPane name="recent" :tab="$t('page.tool.stockDetail.recent16')" />
          <NTabPane name="all" :tab="$t('page.tool.stockDetail.all')" />
        </NTabs>
        <NTabs
          v-model:value="financeViewMode"
          type="segment"
          size="small"
          style="width: 160px"
        >
          <NTabPane name="chart" :tab="$t('page.tool.stockDetail.chartView')" />
          <NTabPane name="table" :tab="$t('page.tool.stockDetail.tableView')" />
        </NTabs>
      </div>
    </div>

    <NSpin :show="financeLoading">
      <!-- 指标分组切换(图表模式) -->
      <div v-if="financeViewMode === 'chart'" class="mb-3">
        <NTabs v-model:value="financeChartGroup" type="segment" size="small">
          <NTabPane name="profit" :tab="$t('page.tool.stockDetail.groupProfit')" />
          <NTabPane name="growth" :tab="$t('page.tool.stockDetail.groupGrowth')" />
          <NTabPane name="operation" :tab="$t('page.tool.stockDetail.groupOperation')" />
          <NTabPane name="solvency" :tab="$t('page.tool.stockDetail.groupSolvency')" />
        </NTabs>
      </div>
      <!-- 图表模式 -->
      <div
        v-show="financeViewMode === 'chart'"
        ref="chartRef"
        style="width: 100%; height: 380px"
      />

      <!-- 表格模式 -->
      <NDataTable
        v-show="financeViewMode === 'table'"
        :columns="financeColumns"
        :data="financeHistory"
        :bordered="false"
        size="small"
        striped
        :scroll-x="2100"
      />
    </NSpin>
  </div>
</template>
