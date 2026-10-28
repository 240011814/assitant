<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { NDataTable, NSpin, NTabPane, NTabs, useMessage } from "naive-ui";
import { stockKline } from "@/service/api";
import { useEcharts } from "@/hooks/common/echarts";
import type { ECOption } from "@/hooks/common/echarts";
import type { BarSeriesOption, LineSeriesOption } from "echarts/charts";
import { $t } from "@/locales";

/**
 * K线趋势面板: 周期切换(60分/日/周/月) + 图表/表格两种视图。
 * 数据加载与渲染自持, 父组件经 reload() 在同步完成后触发刷新。
 */
const props = defineProps<{
  code: string;
  /** 指数不支持小时线 */
  isIndex: boolean;
}>();

const message = useMessage();

const klineData = ref<Api.Stock.KlineData[]>([]);
const klineViewMode = ref("chart");
const klinePeriod = ref<"60" | "daily" | "weekly" | "monthly">("daily");
const isHourly = computed(() => klinePeriod.value === "60");
const klineRequestCount = computed(() => (isHourly.value ? 240 : 120));
const klineEmpty = ref(false);
const klineLoading = ref(false);

// 图表由 useEcharts hook 托管(domRef + updateOptions, 自带 resize 监听/主题跟随/销毁)
const { domRef: klineChartRef, updateOptions: updateKlineChart } = useEcharts<ECOption>(() => ({
  tooltip: { trigger: 'axis', axisPointer: { type: 'cross' } },
  legend: { data: [], top: 0, textStyle: { fontSize: 12 } },
  grid: { left: 50, right: 50, top: 40, bottom: 30 },
  xAxis: { type: 'category', data: [], axisLabel: { fontSize: 11, rotate: 30 } },
  yAxis: [
    { type: 'value', name: $t('page.tool.stockDetail.axisPrice'), position: 'left', axisLabel: { fontSize: 11 } },
    { type: 'value', name: $t('page.tool.stockDetail.changePct'), position: 'right', show: false, axisLabel: { fontSize: 11 } }
  ],
  series: []
}));

const allKlineColumns = [
  { title: $t("page.tool.stockDetail.date"), key: "date", width: 100, fixed: "left" as const },
  {
    title: $t("page.tool.stockDetail.status"),
    key: "tradeStatus",
    width: 60,
    render: (row: Api.Stock.KlineData) =>
      row.tradeStatus === null ? "-" : row.tradeStatus === 0 ? $t("page.tool.stockDetail.suspended") : $t("page.tool.stockDetail.trading"),
  },
  {
    title: $t("page.tool.stockDetail.open"),
    key: "open",
    width: 80,
    render: (row: Api.Stock.KlineData) => row.open?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.high"),
    key: "high",
    width: 80,
    render: (row: Api.Stock.KlineData) => row.high?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.low"),
    key: "low",
    width: 80,
    render: (row: Api.Stock.KlineData) => row.low?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.preclose"),
    key: "preclose",
    width: 80,
    render: (row: Api.Stock.KlineData) => row.preclose?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.close"),
    key: "close",
    width: 80,
    render: (row: Api.Stock.KlineData) => row.close?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.changePct"),
    key: "changePct",
    width: 80,
    render: (row: Api.Stock.KlineData) =>
      row.changePct !== null
        ? `${row.changePct >= 0 ? "+" : ""}${row.changePct.toFixed(2)}%`
        : "-",
  },
  {
    title: $t("page.tool.stockDetail.turnoverPct"),
    key: "turnoverRate",
    width: 80,
    render: (row: Api.Stock.KlineData) => row.turnoverRate?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.volume"),
    key: "volume",
    width: 100,
    render: (row: Api.Stock.KlineData) =>
      row.volume ? $t("page.tool.stockDetail.volumeWan", { value: (row.volume / 10000).toFixed(2) }) : "-",
  },
  {
    title: $t("page.tool.stockDetail.peTtm"),
    key: "peTtm",
    width: 90,
    render: (row: Api.Stock.KlineData) => row.peTtm?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.pb"),
    key: "pbMrq",
    width: 70,
    render: (row: Api.Stock.KlineData) => row.pbMrq?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.psTtm"),
    key: "psTtm",
    width: 90,
    render: (row: Api.Stock.KlineData) => row.psTtm?.toFixed(2) || "-",
  },
  {
    title: $t("page.tool.stockDetail.pcf"),
    key: "pcfNcfTtm",
    width: 80,
    render: (row: Api.Stock.KlineData) => row.pcfNcfTtm?.toFixed(2) || "-",
  },
];

// 小时线无昨收/交易状态/估值/换手, 仅展示 OHLCV 相关列并改为按时间展示
const hourlyHiddenKlineKeys = [
  "tradeStatus",
  "preclose",
  "changePct",
  "turnoverRate",
  "peTtm",
  "pbMrq",
  "psTtm",
  "pcfNcfTtm",
];
const klineColumns = computed(() => {
  if (!isHourly.value) return allKlineColumns;
  return allKlineColumns
    .filter((c) => !hourlyHiddenKlineKeys.includes(c.key))
    .map((c) => (c.key === "date" ? { ...c, title: $t("page.tool.stockDetail.time"), width: 145 } : c));
});
const klineScrollX = computed(() =>
  klineColumns.value.reduce((sum, c) => sum + c.width, 0)
);

let klineReqSeq = 0;
async function loadKline() {
  const seq = ++klineReqSeq;
  klineLoading.value = true;
  try {
    const res = await stockKline(props.code, {
      period: klinePeriod.value,
      count: klineRequestCount.value,
    });
    if (seq !== klineReqSeq) return;
    klineData.value = res.data || [];
    klineEmpty.value = !res.data || res.data.length === 0;
  } catch (e: any) {
    if (seq !== klineReqSeq) return;
    message.error(e.message || $t("page.tool.stockDetail.klineLoadFailed"));
  } finally {
    if (seq === klineReqSeq) {
      klineLoading.value = false;
    }
  }
}

function getKlineChartOption(): ECOption {
  const data = [...klineData.value];
  const dates = data.map((d) => d.date || "");
  const closeSeries: LineSeriesOption = {
    name: $t("page.tool.stockDetail.closePrice"),
    type: "line",
    data: data.map((d) => d.close),
    smooth: true,
    itemStyle: { color: "#1890ff" },
    areaStyle: { color: "rgba(24,144,255,0.1)" },
  };
  // 小时线无涨跌幅, 改为叠加均线
  const hourly = isHourly.value;
  const series: (LineSeriesOption | BarSeriesOption)[] = hourly
    ? [
        closeSeries,
        {
          name: "MA5",
          type: "line",
          data: data.map((d) => d.ma5 ?? null),
          smooth: true,
          showSymbol: false,
          itemStyle: { color: "#faad14" },
        },
        {
          name: "MA10",
          type: "line",
          data: data.map((d) => d.ma10 ?? null),
          smooth: true,
          showSymbol: false,
          itemStyle: { color: "#52c41a" },
        },
        {
          name: "MA20",
          type: "line",
          data: data.map((d) => d.ma20 ?? null),
          smooth: true,
          showSymbol: false,
          itemStyle: { color: "#f5222d" },
        },
      ]
    : [
        closeSeries,
        {
          name: $t("page.tool.stockDetail.changePctFull"),
          type: "bar",
          yAxisIndex: 1,
          data: data.map((d) => d.changePct),
          itemStyle: {
            color: (params: any) => (params.value >= 0 ? "#f5222d" : "#52c41a"),
          },
        },
      ];
  return {
    tooltip: {
      trigger: "axis",
      axisPointer: { type: "cross" },
    },
    legend: {
      data: hourly
        ? [$t("page.tool.stockDetail.closePrice"), "MA5", "MA10", "MA20"]
        : [$t("page.tool.stockDetail.closePrice"), $t("page.tool.stockDetail.changePctFull")],
      top: 0,
      textStyle: { fontSize: 12 },
    },
    grid: { left: 50, right: 50, top: 40, bottom: 30 },
    xAxis: {
      type: "category",
      data: dates,
      axisLabel: { fontSize: 11, rotate: 30 },
    },
    yAxis: [
      {
        type: "value",
        name: $t("page.tool.stockDetail.axisPrice"),
        position: "left",
        axisLabel: { fontSize: 11 },
      },
      {
        type: "value",
        name: $t("page.tool.stockDetail.changePct"),
        position: "right",
        show: !hourly,
        axisLabel: { fontSize: 11 },
      },
    ],
    series,
  };
}

// updateOptions 内部先 clear 再 setOption, 等价于 notMerge 全量替换;
// 若图表尚未渲染(domRef 尺寸为 0), option 会先缓存, 由 hook 在尺寸变化时渲染
function renderKlineChart() {
  if (klineData.value.length === 0) return;
  updateKlineChart(() => getKlineChartOption());
}

watch(klinePeriod, () => {
  loadKline();
});

// 路由复用换股时 (父组件 code 变化) 重新加载
watch(() => props.code, () => {
  loadKline();
});

watch(klineViewMode, (val) => {
  if (val === "chart") {
    renderKlineChart();
  }
});

watch(klineData, () => {
  if (klineViewMode.value === "chart") {
    renderKlineChart();
  }
});

onMounted(() => {
  loadKline();
});

defineExpose({
  /** 同步完成后由父组件触发重新加载 */
  reload: loadKline,
});
</script>

<template>
  <!-- v-show 保持图表 DOM 稳定, 由 useEcharts 托管 -->
  <div
    v-show="klineData.length > 0 || klineEmpty"
    class="mt-4 p-4 bg-white rounded-lg shadow"
  >
    <div class="flex items-center justify-between mb-3 flex-wrap gap-2">
      <h2 class="text-lg font-bold">{{ $t("page.tool.stockDetail.klineTrend", { count: klineData.length }) }}</h2>
      <div class="flex items-center gap-2">
        <NTabs
          v-model:value="klinePeriod"
          type="segment"
          size="small"
          style="width: 260px"
        >
          <NTabPane v-if="!isIndex" name="60" :tab="$t('page.tool.stockDetail.hourlyK')" />
          <NTabPane name="daily" :tab="$t('page.tool.stockDetail.dailyK')" />
          <NTabPane name="weekly" :tab="$t('page.tool.stockDetail.weeklyK')" />
          <NTabPane name="monthly" :tab="$t('page.tool.stockDetail.monthlyK')" />
        </NTabs>
        <NTabs
          v-model:value="klineViewMode"
          type="segment"
          size="small"
          style="width: 160px"
        >
          <NTabPane name="chart" :tab="$t('page.tool.stockDetail.chartView')" />
          <NTabPane name="table" :tab="$t('page.tool.stockDetail.tableView')" />
        </NTabs>
      </div>
    </div>

    <NSpin :show="klineLoading">
      <div
        v-show="klineData.length === 0"
        class="py-10 text-center text-gray-400"
      >
        <div class="text-14px">{{ $t("page.tool.stockDetail.noPeriodData") }}</div>
        <div class="text-12px mt-1">{{ $t("page.tool.stockDetail.syncKlineFirst") }}</div>
      </div>
      <div v-show="klineData.length > 0">
        <!-- 图表模式 -->
        <div
          v-show="klineViewMode === 'chart'"
          ref="klineChartRef"
          style="width: 100%; height: 380px"
        />

        <!-- 表格模式 -->
        <NDataTable
          v-show="klineViewMode === 'table'"
          :columns="klineColumns"
          :data="klineData.slice(-20)"
          :bordered="false"
          size="small"
          striped
          :scroll-x="klineScrollX"
        />
        <div
          v-if="klineData.length > 20 && klineViewMode === 'table'"
          class="mt-2 text-12px text-gray-400 text-center"
        >
          {{ $t("page.tool.stockDetail.tableLimit", { count: klineData.length }) }}
        </div>
      </div>
    </NSpin>
  </div>
</template>
