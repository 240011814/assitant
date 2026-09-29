<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  stockDetail,
  stockKline,
  stockFinanceHistory,
  syncSingleStock,
  fetchSyncStatus,
  fetchStockSyncState,
  getWatchlistCodes,
} from "@/service/api";
import WatchlistAddDialog from "@/components/custom/watchlist-add-dialog.vue";
import { useMessage, NButton, NDataTable, NTag, NSpin, NTabs, NTabPane } from "naive-ui";
import { useEcharts } from '@/hooks/common/echarts';
import type { ECOption } from '@/hooks/common/echarts';
import type { BarSeriesOption, LineSeriesOption } from 'echarts/charts';

defineOptions({ name: "ToolStockdetail" });

const route = useRoute();
const router = useRouter();
const message = useMessage();

const code = computed(() => {
  const c = route.query.code;
  if (!c) return "";
  return Array.isArray(c) ? c[0] || "" : c;
});
const detail = ref<Api.Stock.ScreenResult | null>(null);
const klineData = ref<Api.Stock.KlineData[]>([]);
const financeHistory = ref<Api.Stock.FinanceHistory[]>([]);
const loading = ref(false);
const activeTab = ref("local");
const financeViewMode = ref("chart");
const klineViewMode = ref("chart");
const klinePeriod = ref<"60" | "daily" | "weekly" | "monthly">("daily");
const isHourly = computed(() => klinePeriod.value === "60");
const klineRequestCount = computed(() => (isHourly.value ? 240 : 120));
const klineEmpty = ref(false);
const klineLoading = ref(false);
const financeRange = ref<"recent" | "all">("recent");
const financeLoading = ref(false);
const financeChartGroup = ref<"profit" | "growth" | "operation" | "solvency">("profit");
const syncState = ref<Api.Stock.SyncState | null>(null);

// 两个图表均由 useEcharts hook 托管(domRef + updateOptions, 自带 resize 监听/主题跟随/销毁)
const { domRef: chartRef, updateOptions: updateFinanceChart } = useEcharts<ECOption>(() => ({
  tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
  legend: { data: [], top: 0, textStyle: { fontSize: 12 } },
  grid: { left: 50, right: 20, top: 40, bottom: 30 },
  xAxis: { type: 'category', data: [], axisLabel: { fontSize: 11 } },
  yAxis: [{ type: 'value', name: '', axisLabel: { fontSize: 11 } }],
  series: []
}));

const { domRef: klineChartRef, updateOptions: updateKlineChart } = useEcharts<ECOption>(() => ({
  tooltip: { trigger: 'axis', axisPointer: { type: 'cross' } },
  legend: { data: [], top: 0, textStyle: { fontSize: 12 } },
  grid: { left: 50, right: 50, top: 40, bottom: 30 },
  xAxis: { type: 'category', data: [], axisLabel: { fontSize: 11, rotate: 30 } },
  yAxis: [
    { type: 'value', name: '价格', position: 'left', axisLabel: { fontSize: 11 } },
    { type: 'value', name: '涨跌%', position: 'right', show: false, axisLabel: { fontSize: 11 } }
  ],
  series: []
}));

const allKlineColumns = [
  { title: "日期", key: "date", width: 100, fixed: "left" as const },
  {
    title: "状态",
    key: "tradeStatus",
    width: 60,
    render: (row: Api.Stock.KlineData) =>
      row.tradeStatus === null ? "-" : row.tradeStatus === 0 ? "停牌" : "交易",
  },
  {
    title: "开盘",
    key: "open",
    width: 80,
    render: (row: Api.Stock.KlineData) => row.open?.toFixed(2) || "-",
  },
  {
    title: "最高",
    key: "high",
    width: 80,
    render: (row: Api.Stock.KlineData) => row.high?.toFixed(2) || "-",
  },
  {
    title: "最低",
    key: "low",
    width: 80,
    render: (row: Api.Stock.KlineData) => row.low?.toFixed(2) || "-",
  },
  {
    title: "昨收",
    key: "preclose",
    width: 80,
    render: (row: Api.Stock.KlineData) => row.preclose?.toFixed(2) || "-",
  },
  {
    title: "收盘",
    key: "close",
    width: 80,
    render: (row: Api.Stock.KlineData) => row.close?.toFixed(2) || "-",
  },
  {
    title: "涨跌%",
    key: "changePct",
    width: 80,
    render: (row: Api.Stock.KlineData) =>
      row.changePct !== null
        ? `${row.changePct >= 0 ? "+" : ""}${row.changePct.toFixed(2)}%`
        : "-",
  },
  {
    title: "换手%",
    key: "turnoverRate",
    width: 80,
    render: (row: Api.Stock.KlineData) => row.turnoverRate?.toFixed(2) || "-",
  },
  {
    title: "成交量",
    key: "volume",
    width: 100,
    render: (row: Api.Stock.KlineData) =>
      row.volume ? `${(row.volume / 10000).toFixed(2)}万手` : "-",
  },
  {
    title: "PE(TTM)",
    key: "peTtm",
    width: 90,
    render: (row: Api.Stock.KlineData) => row.peTtm?.toFixed(2) || "-",
  },
  {
    title: "PB",
    key: "pbMrq",
    width: 70,
    render: (row: Api.Stock.KlineData) => row.pbMrq?.toFixed(2) || "-",
  },
  {
    title: "PS(TTM)",
    key: "psTtm",
    width: 90,
    render: (row: Api.Stock.KlineData) => row.psTtm?.toFixed(2) || "-",
  },
  {
    title: "市现率",
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
    .map((c) => (c.key === "date" ? { ...c, title: "时间", width: 145 } : c));
});
const klineScrollX = computed(() =>
  klineColumns.value.reduce((sum, c) => sum + c.width, 0)
);

const financeColumns = [
  {
    title: "报告期",
    key: "reportDate",
    width: 100,
    fixed: "left" as const,
    render: (row: Api.Stock.FinanceHistory) => row.reportDate?.slice(0, 10) || "-",
  },
  {
    title: "ROE%",
    key: "roe",
    width: 70,
    render: (row: Api.Stock.FinanceHistory) => row.roe?.toFixed(2) || "-",
  },
  {
    title: "毛利率%",
    key: "grossMargin",
    width: 70,
    render: (row: Api.Stock.FinanceHistory) => row.grossMargin?.toFixed(2) || "-",
  },
  {
    title: "净利率%",
    key: "netMargin",
    width: 70,
    render: (row: Api.Stock.FinanceHistory) => row.netMargin?.toFixed(2) || "-",
  },
  {
    title: "营收(万)",
    key: "revenue",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) => row.revenue?.toFixed(0) || "-",
  },
  {
    title: "营收同比%",
    key: "revenueYoy",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) =>
      row.revenueYoy !== null
        ? `${row.revenueYoy >= 0 ? "+" : ""}${row.revenueYoy.toFixed(2)}%`
        : "-",
  },
  {
    title: "净利润(万)",
    key: "netProfit",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) => row.netProfit?.toFixed(0) || "-",
  },
  {
    title: "净利同比%",
    key: "netProfitYoy",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) =>
      row.netProfitYoy !== null
        ? `${row.netProfitYoy >= 0 ? "+" : ""}${row.netProfitYoy.toFixed(2)}%`
        : "-",
  },
  {
    title: "EPS",
    key: "eps",
    width: 60,
    render: (row: Api.Stock.FinanceHistory) => row.eps?.toFixed(3) || "-",
  },
  {
    title: "净资产同比%",
    key: "yoyEquity",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) =>
      row.yoyEquity !== null
        ? `${row.yoyEquity >= 0 ? "+" : ""}${row.yoyEquity.toFixed(2)}%`
        : "-",
  },
  {
    title: "总资产同比%",
    key: "yoyAsset",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) =>
      row.yoyAsset !== null
        ? `${row.yoyAsset >= 0 ? "+" : ""}${row.yoyAsset.toFixed(2)}%`
        : "-",
  },
  {
    title: "EPS同比%",
    key: "yoyEps",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) =>
      row.yoyEps !== null
        ? `${row.yoyEps >= 0 ? "+" : ""}${row.yoyEps.toFixed(2)}%`
        : "-",
  },
  {
    title: "资产负债率%",
    key: "debtRatio",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) => row.debtRatio?.toFixed(2) || "-",
  },
  {
    title: "流动比率",
    key: "currentRatio",
    width: 70,
    render: (row: Api.Stock.FinanceHistory) => row.currentRatio?.toFixed(2) || "-",
  },
  {
    title: "速动比率",
    key: "quickRatio",
    width: 70,
    render: (row: Api.Stock.FinanceHistory) => row.quickRatio?.toFixed(2) || "-",
  },
  {
    title: "现金比率",
    key: "cashRatio",
    width: 70,
    render: (row: Api.Stock.FinanceHistory) => row.cashRatio?.toFixed(2) || "-",
  },
  {
    title: "应收周转",
    key: "nrTurnRatio",
    width: 70,
    render: (row: Api.Stock.FinanceHistory) => row.nrTurnRatio?.toFixed(2) || "-",
  },
  {
    title: "存货周转",
    key: "invTurnRatio",
    width: 70,
    render: (row: Api.Stock.FinanceHistory) => row.invTurnRatio?.toFixed(2) || "-",
  },
  {
    title: "流动资产周转",
    key: "caTurnRatio",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) => row.caTurnRatio?.toFixed(2) || "-",
  },
  {
    title: "总资产周转",
    key: "assetTurnRatio",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) => row.assetTurnRatio?.toFixed(2) || "-",
  },
  {
    title: "现金流/营收",
    key: "cfoToOr",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) => row.cfoToOr?.toFixed(2) || "-",
  },
  {
    title: "现金流/净利",
    key: "cfoToNp",
    width: 80,
    render: (row: Api.Stock.FinanceHistory) => row.cfoToNp?.toFixed(2) || "-",
  },
];

const displayCode = computed(() =>
  code.value.includes(".") ? code.value.split(".")[1] : code.value
);

const realtimeUrl = computed(() => {
  if (!code.value) return "";
  const digits = displayCode.value;
  const market =
    code.value.startsWith("sh.") || digits.startsWith("6")
      ? "sh"
      : code.value.startsWith("bj.") || digits.startsWith("4") || digits.startsWith("8")
      ? "bj"
      : "sz";
  return `https://quote.eastmoney.com/${market}${digits}.html`;
});

async function loadDetail() {
  if (!code.value) {
    message.error("股票代码不能为空");
    return;
  }
  loading.value = true;
  try {
    const [detailRes, klineRes, , syncStateRes] = await Promise.all([
      stockDetail(code.value),
      stockKline(code.value, {
        period: klinePeriod.value,
        count: klineRequestCount.value,
      }),
      loadFinance(),
      fetchStockSyncState(code.value),
    ]);
    // flat request 永不 reject, 需显式判 error
    if (detailRes.error || klineRes.error || syncStateRes?.error) {
      message.error("加载失败");
      return;
    }
    if (detailRes.data) {
      detail.value = detailRes.data;
    }
    if (klineRes.data) {
      klineData.value = klineRes.data;
    }
    syncState.value = syncStateRes?.data || null;
  } finally {
    loading.value = false;
  }
}

let financeReqSeq = 0;
async function loadFinance() {
  const seq = ++financeReqSeq;
  financeLoading.value = true;
  try {
    const res = await stockFinanceHistory(code.value, {
      limit: financeRange.value === "all" ? 0 : 16,
    });
    if (seq !== financeReqSeq) return;
    financeHistory.value = res.data || [];
  } catch (e: any) {
    if (seq !== financeReqSeq) return;
    message.error(e.message || "财务数据加载失败");
  } finally {
    if (seq === financeReqSeq) {
      financeLoading.value = false;
    }
  }
}

watch(financeRange, () => {
  loadFinance();
});

let klineReqSeq = 0;
async function loadKline() {
  const seq = ++klineReqSeq;
  klineLoading.value = true;
  try {
    const res = await stockKline(code.value, {
      period: klinePeriod.value,
      count: klineRequestCount.value,
    });
    if (seq !== klineReqSeq) return;
    klineData.value = res.data || [];
    klineEmpty.value = !res.data || res.data.length === 0;
  } catch (e: any) {
    if (seq !== klineReqSeq) return;
    message.error(e.message || "K线数据加载失败");
  } finally {
    if (seq === klineReqSeq) {
      klineLoading.value = false;
    }
  }
}

watch(klinePeriod, () => {
  loadKline();
});

// 路由复用换股时刷新详情
watch(code, () => {
  loadDetail();
});

const showAddDialog = ref(false);
const watchlistCodes = ref<Set<string>>(new Set());

async function loadWatchlistCodes() {
  try {
    const { data } = await getWatchlistCodes();
    watchlistCodes.value = new Set(data || []);
  } catch {
    // ignore
  }
}

function handleAddWatchlist() {
  showAddDialog.value = true;
}

function handleWatchlistAdded() {
  watchlistCodes.value = new Set([...watchlistCodes.value, code.value]);
}

const syncLoading = ref(false);
const syncRunning = ref(false);
let syncPollTimer: ReturnType<typeof setInterval> | null = null;

function stopSyncPolling() {
  if (syncPollTimer) {
    clearInterval(syncPollTimer);
    syncPollTimer = null;
  }
  syncRunning.value = false;
}

function waitForSyncDone(onDone: () => void) {
  stopSyncPolling();
  syncRunning.value = true;
  let runningSeen = false;
  let ticks = 0;
  syncPollTimer = setInterval(async () => {
    ticks++;
    if (ticks > 100) {
      stopSyncPolling();
      onDone();
      return;
    }
    try {
      const { data } = await fetchSyncStatus();
      if (!data) return;
      if (data.running) {
        runningSeen = true;
        return;
      }
      if (!runningSeen) {
        runningSeen = true;
        return;
      }
      stopSyncPolling();
      if (data.lastError) {
        message.error(`同步失败: ${data.lastError}`);
      }
      onDone();
    } catch {
      // ignore
    }
  }, 3000);
}

async function handleSync() {
  if (syncLoading.value || syncRunning.value) return;
  syncLoading.value = true;
  const { error } = await syncSingleStock(code.value, detail.value?.market || "");
  syncLoading.value = false;
  if (error) {
    message.warning(error.message || "同步启动失败，可能有其他同步任务在运行");
    waitForSyncDone(() => loadDetail());
    return;
  }
  message.loading("正在同步最新数据，请稍候...");
  waitForSyncDone(() => {
    message.success("同步完成");
    loadDetail();
  });
}

function goBack() {
  router.push({ name: "tool_stockscreen" });
}

const isIndex = computed(() => detail.value?.type === 2);
const isEtf = computed(() => detail.value?.type === 5);
// 指数与 ETF 均无财务数据, 相关区块不显示; 小时K指数不支持(ETF支持)
const noFinance = computed(() => isIndex.value || isEtf.value);

const infoItems = computed(() => {
  if (!detail.value) return [];
  const d = detail.value;
  const all = [
    { label: "代码", value: d.code, tip: "" },
    { label: "名称", value: d.name, tip: "" },
    { label: "市场", value: d.market, tip: "" },
    { label: "行业", value: d.industry || "-", tip: "" },
    { label: "现价", value: d.price?.toFixed(2) || "-", tip: "" },
    {
      label: "涨跌幅",
      value:
        d.changePct !== null
          ? `${d.changePct >= 0 ? "+" : ""}${d.changePct.toFixed(2)}%`
          : "-",
      tip: "公式: (当前价-昨收价)/昨收价×100%",
    },
    {
      label: "换手率",
      value: d.turnoverRate?.toFixed(2) ? `${d.turnoverRate.toFixed(2)}%` : "-",
      tip: "公式: 成交量/流通股本×100%。<3%冷门，3-7%正常，>10%非常活跃",
    },
    {
      label: "成交额",
      value: d.amount ? `${(d.amount / 10000).toFixed(2)}亿` : "-",
      tip: "",
    },
    {
      label: "总市值",
      value: d.marketCap ? `${d.marketCap.toFixed(2)}亿` : "-",
      tip: "公式: 股价×总股本。<50亿小盘，50-200亿中盘，>1000亿超大盘",
    },
    {
      label: "流通市值",
      value: d.floatMarketCap ? `${d.floatMarketCap.toFixed(2)}亿` : "-",
      tip: "",
    },
  ];
  if (d.type === 2 || d.type === 5) {
    return all.filter((i) => !["行业", "换手率", "总市值", "流通市值"].includes(i.label));
  }
  return all;
});

const financeItems = computed(() => {
  if (!detail.value) return [];
  const d = detail.value;
  return [
    {
      label: "PE(TTM)",
      value: d.peTtm?.toFixed(2) || "-",
      tip: "公式: 股价/最近四个季度每股收益之和。<20低估，20-30合理，>30高估",
    },
    {
      label: "PB",
      value: d.pb?.toFixed(2) || "-",
      tip: "公式: 股价/每股净资产。<1破净，1-2低估，>3高估",
    },
    {
      label: "ROE",
      value: d.roe?.toFixed(2) ? `${d.roe.toFixed(2)}%` : "-",
      tip: "公式: 净利润/净资产×100%。>15%优秀，10-15%良好，<10%一般",
    },
    {
      label: "营收增长",
      value: d.revenueYoy?.toFixed(2) ? `${d.revenueYoy.toFixed(2)}%` : "-",
      tip: "公式: (本期营收-去年同期营收)/去年同期营收×100%。>20%高增长",
    },
    {
      label: "净利润增长",
      value: d.netProfitYoy?.toFixed(2) ? `${d.netProfitYoy.toFixed(2)}%` : "-",
      tip: "公式: (本期净利润-去年同期净利润)/去年同期净利润×100%",
    },
    {
      label: "毛利率",
      value: d.grossMargin?.toFixed(2) ? `${d.grossMargin.toFixed(2)}%` : "-",
      tip: "公式: (营业收入-营业成本)/营业收入×100%",
    },
    {
      label: "净利率",
      value: d.netMargin?.toFixed(2) ? `${d.netMargin.toFixed(2)}%` : "-",
      tip: "公式: 净利润/营业收入×100%",
    },
    {
      label: "资产负债率",
      value: d.debtRatio?.toFixed(2) ? `${d.debtRatio.toFixed(2)}%` : "-",
      tip: "公式: 总负债/总资产×100%。<50%低风险，50-70%正常，>70%高风险",
    },
    {
      label: "流动比率",
      value: d.currentRatio?.toFixed(2) || "-",
      tip: "公式: 流动资产/流动负债。>2优秀，1-2正常，<1有风险",
    },
    {
      label: "速动比率",
      value: d.quickRatio?.toFixed(2) || "-",
      tip: "公式: (流动资产-存货)/流动负债。>1优秀，0.5-1正常",
    },
    {
      label: "现金比率",
      value: d.cashRatio?.toFixed(2) || "-",
      tip: "公式: (货币资金+交易性金融资产)/流动负债。>0.5充裕，0.2-0.5正常，<0.2偏紧",
    },
    {
      label: "应收周转率",
      value: d.nrTurnRatio?.toFixed(2) || "-",
      tip: "公式: 营业收入/应收账款。越高回款越快；金融股无此指标",
    },
    {
      label: "存货周转率",
      value: d.invTurnRatio?.toFixed(2) || "-",
      tip: "公式: 营业成本/存货。越高存货消化越快",
    },
    {
      label: "净资产同比",
      value:
        d.yoyEquity !== null
          ? `${d.yoyEquity >= 0 ? "+" : ""}${d.yoyEquity.toFixed(2)}%`
          : "-",
      tip: "净资产较上年同期增速，反映内生积累能力",
    },
    {
      label: "总资产同比",
      value:
        d.yoyAsset !== null
          ? `${d.yoyAsset >= 0 ? "+" : ""}${d.yoyAsset.toFixed(2)}%`
          : "-",
      tip: "总资产较上年同期增速，反映扩张速度",
    },
    {
      label: "现金流/营收",
      value: d.cfoToOr?.toFixed(2) || "-",
      tip: "公式: 经营现金流净额/营业收入。>0.2较好，持续为负需警惕",
    },
  ];
});

function fmtSyncDate(v: string | null | undefined) {
  return v ? v.slice(0, 10) : "-";
}

function fmtSyncDateTime(v: string | null | undefined) {
  return v ? v.slice(0, 19).replace("T", " ") : "-";
}

const klineStatusMeta = computed(() => {
  const s = syncState.value?.klineStatus || "pending";
  const meta: Record<string, { label: string; type: "success" | "error" | "default" }> = {
    ok: { label: "已同步", type: "success" },
    failed: { label: "同步失败", type: "error" },
    pending: { label: "未同步", type: "default" },
  };
  return meta[s] || meta.pending;
});

const financeStatusMeta = computed(() => {
  const s = syncState.value?.financeStatus || "pending";
  const meta: Record<
    string,
    { label: string; type: "success" | "error" | "default" | "info" }
  > = {
    ok: { label: "已同步", type: "success" },
    failed: { label: "同步失败", type: "error" },
    pending: { label: "未同步", type: "default" },
    skipped: { label: "无财务数据", type: "info" },
  };
  return meta[s] || meta.pending;
});

const syncItems = computed(() => {
  const st = syncState.value;
  const items = [
    { label: "日K已到", value: fmtSyncDate(st?.klineDailyTo) },
    { label: "周K已到", value: fmtSyncDate(st?.klineWeeklyTo) },
    { label: "月K已到", value: fmtSyncDate(st?.klineMonthlyTo) },
  ];
  if (!isIndex.value) {
    items.push({ label: "小时K已到", value: fmtSyncDateTime(st?.klineHourlyTo) });
  }
  items.push({ label: "K线同步时间", value: fmtSyncDateTime(st?.klineSyncedAt) });
  if (!noFinance.value) {
    items.push(
      { label: "财务已到", value: fmtSyncDate(st?.financeTo) },
      { label: "财务同步时间", value: fmtSyncDateTime(st?.financeSyncedAt) }
    );
  }
  return items;
});

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
    name: "盈利",
    unit: "%",
    series: [
      { key: "roe", name: "ROE%", color: "#1890ff" },
      { key: "grossMargin", name: "毛利率%", color: "#52c41a" },
      { key: "netMargin", name: "净利率%", color: "#faad14" },
      { key: "debtRatio", name: "资产负债率%", color: "#f5222d" },
    ],
  },
  growth: {
    name: "成长",
    unit: "%",
    series: [
      { key: "revenueYoy", name: "营收同比%", color: "#1890ff" },
      { key: "netProfitYoy", name: "净利润同比%", color: "#52c41a" },
      { key: "yoyEquity", name: "净资产同比%", color: "#faad14" },
      { key: "yoyAsset", name: "总资产同比%", color: "#f5222d" },
      { key: "yoyEps", name: "EPS同比%", color: "#722ed1" },
    ],
  },
  operation: {
    name: "营运",
    unit: "次",
    series: [
      { key: "nrTurnRatio", name: "应收周转", color: "#1890ff" },
      { key: "invTurnRatio", name: "存货周转", color: "#52c41a" },
      { key: "caTurnRatio", name: "流动资产周转", color: "#faad14" },
      { key: "assetTurnRatio", name: "总资产周转", color: "#f5222d" },
    ],
  },
  solvency: {
    name: "偿债与现金流",
    unit: "%/倍",
    series: [
      { key: "debtRatio", name: "资产负债率%", color: "#f5222d" },
      { key: "currentRatio", name: "流动比率", color: "#1890ff" },
      { key: "quickRatio", name: "速动比率", color: "#52c41a" },
      { key: "cashRatio", name: "现金比率", color: "#faad14" },
      { key: "cfoToOr", name: "现金流/营收", color: "#722ed1" },
      { key: "cfoToNp", name: "现金流/净利", color: "#13c2c2" },
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

function getKlineChartOption(): ECOption {
  const data = [...klineData.value];
  const dates = data.map((d) => d.date || "");
  const closeSeries: LineSeriesOption = {
    name: "收盘价",
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
          name: "涨跌幅%",
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
      data: hourly ? ["收盘价", "MA5", "MA10", "MA20"] : ["收盘价", "涨跌幅%"],
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
        name: "价格",
        position: "left",
        axisLabel: { fontSize: 11 },
      },
      {
        type: "value",
        name: "涨跌%",
        position: "right",
        show: !hourly,
        axisLabel: { fontSize: 11 },
      },
    ],
    series,
  };
}

// updateOptions 内部先 clear 再 setOption, 等价于原来的 notMerge 全量替换;
// 若图表尚未渲染(domRef 尺寸为 0), option 会先缓存, 由 hook 在尺寸变化时渲染
function renderChart() {
  if (financeHistory.value.length === 0) return;
  updateFinanceChart(() => getChartOption());
}

function renderKlineChart() {
  if (klineData.value.length === 0) return;
  updateKlineChart(() => getKlineChartOption());
}

watch(financeViewMode, (val) => {
  if (val === "chart") {
    renderChart();
  }
});

watch(klineViewMode, (val) => {
  if (val === "chart") {
    renderKlineChart();
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

// detail 加载完成后整个内容区块才挂载, 若数据先于 detail 到位需要补一次图表渲染
// (updateOptions 内部已等待 nextTick 并在未渲染时缓存 option)
watch(detail, () => {
  if (financeHistory.value.length > 0 && financeViewMode.value === "chart") {
    renderChart();
  }
  if (klineData.value.length > 0 && klineViewMode.value === "chart") {
    renderKlineChart();
  }
});

watch(klineData, () => {
  if (klineViewMode.value === "chart") {
    renderKlineChart();
  }
});

onMounted(() => {
  loadDetail();
  loadWatchlistCodes();

  fetchSyncStatus()
    .then(({ data }) => {
      if (data?.running) {
        message.info("检测到同步任务正在运行，完成后将自动刷新");
        waitForSyncDone(() => loadDetail());
      }
    })
    .catch(() => {
      // ignore
    });
});

onUnmounted(() => {
  stopSyncPolling();
  // echarts 实例由 useEcharts hook 在作用域销毁时自动 dispose, 无需手动释放
});
</script>

<template>
  <div class="p-4">
    <!-- 顶部导航 -->
    <div class="flex items-center justify-between mb-4">
      <NButton @click="goBack">
        <template #icon><span class="i-mdi:arrow-left" /></template>
        返回筛选
      </NButton>
      <div class="flex gap-2">
        <NButton
          type="primary"
          :loading="syncLoading || syncRunning"
          :disabled="syncRunning"
          @click="handleSync"
        >
          <template #icon><span class="i-mdi:refresh" /></template>
          {{ syncRunning ? "同步中..." : "同步最新" }}
        </NButton>
        <NButton v-if="!watchlistCodes.has(code)" type="primary" @click="handleAddWatchlist">加自选</NButton>
      </div>
    </div>

    <NSpin :show="loading">
      <template v-if="detail">
        <!-- 股票头部信息 -->
        <div class="mb-4 p-4 bg-white rounded-lg shadow">
          <div class="flex items-center gap-4">
            <div>
              <h1 class="text-2xl font-bold">{{ detail.name }}</h1>
              <p class="text-gray-500">{{ displayCode }} | {{ detail.market }}</p>
            </div>
            <div v-if="detail.price" class="ml-auto text-right">
              <p
                class="text-3xl font-bold"
                :class="
                  detail.changePct && detail.changePct >= 0
                    ? 'text-red-500'
                    : 'text-green-500'
                "
              >
                {{ detail.price.toFixed(2) }}
              </p>
              <p
                v-if="detail.changePct !== null"
                class="text-lg"
                :class="detail.changePct >= 0 ? 'text-red-500' : 'text-green-500'"
              >
                {{ detail.changePct >= 0 ? "+" : "" }}{{ detail.changePct.toFixed(2) }}%
              </p>
            </div>
          </div>
        </div>

        <NTabs v-model:value="activeTab" type="line" animated>
          <!-- 本地数据Tab -->
          <NTabPane name="local" tab="本地数据">
            <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
              <!-- 基本信息 -->
              <div class="p-4 bg-white rounded-lg shadow">
                <h2 class="text-lg font-bold mb-3">基本信息</h2>
                <div class="grid grid-cols-2 gap-2">
                  <div
                    v-for="item in infoItems"
                    :key="item.label"
                    class="flex justify-between items-center"
                  >
                    <span class="text-gray-500">{{ item.label }}</span>
                    <span class="font-medium flex items-center gap-1">
                      {{ item.value }}
                      <span
                        v-if="item.tip"
                        class="text-gray-400 cursor-help"
                        :title="item.tip"
                      >?</span>
                    </span>
                  </div>
                </div>
              </div>

              <!-- 财务指标 (指数/ETF无财务, 不显示) -->
              <div v-if="!noFinance" class="p-4 bg-white rounded-lg shadow">
                <h2 class="text-lg font-bold mb-3">财务指标</h2>
                <div class="grid grid-cols-2 gap-2">
                  <div
                    v-for="item in financeItems"
                    :key="item.label"
                    class="flex justify-between items-center"
                  >
                    <span class="text-gray-500">{{ item.label }}</span>
                    <span class="font-medium flex items-center gap-1">
                      {{ item.value }}
                      <span
                        v-if="item.tip"
                        class="text-gray-400 cursor-help"
                        :title="item.tip"
                      >?</span>
                    </span>
                  </div>
                </div>
              </div>

              <!-- 数据同步状态 (指数与基本信息同行, 股票独占整行) -->
              <div
                :class="noFinance ? '' : 'lg:col-span-2'"
                class="p-4 bg-white rounded-lg shadow"
              >
                <h2 class="text-lg font-bold mb-3">数据同步状态</h2>
                <div class="grid grid-cols-2 gap-2">
                  <div
                    v-for="item in syncItems"
                    :key="item.label"
                    class="flex justify-between items-center"
                  >
                    <span class="text-gray-500">{{ item.label }}</span>
                    <span class="font-medium">{{ item.value }}</span>
                  </div>
                </div>
                <div class="flex flex-wrap gap-x-8 gap-y-2 mt-3">
                  <div class="flex items-center gap-2">
                    <span class="text-gray-500">K线状态</span>
                    <NTag :type="klineStatusMeta.type" size="small">
                      {{
                        klineStatusMeta.label
                      }}
                    </NTag>
                    <span
                      v-if="syncState?.klineError"
                      class="text-12px text-red-500 cursor-help"
                      :title="syncState.klineError"
                    >?</span>
                  </div>
                  <div v-if="!noFinance" class="flex items-center gap-2">
                    <span class="text-gray-500">财务状态</span>
                    <NTag :type="financeStatusMeta.type" size="small">
                      {{
                        financeStatusMeta.label
                      }}
                    </NTag>
                    <span
                      v-if="syncState?.financeError"
                      class="text-12px text-red-500 cursor-help"
                      :title="syncState.financeError"
                    >?</span>
                  </div>
                </div>
              </div>
            </div>

            <!-- 概念板块 -->
            <div
              v-if="detail.concepts && detail.concepts.length > 0"
              class="mt-4 p-4 bg-white rounded-lg shadow"
            >
              <h2 class="text-lg font-bold mb-3">概念板块</h2>
              <div class="flex flex-wrap gap-2">
                <NTag
                  v-for="concept in detail.concepts"
                  :key="concept"
                  type="info"
                  size="small"
                >
                  {{ concept }}
                </NTag>
              </div>
            </div>

            <!-- K线数据 (v-show 保持图表 DOM 稳定, 由 useEcharts 托管) -->
            <div
              v-show="klineData.length > 0 || klineEmpty"
              class="mt-4 p-4 bg-white rounded-lg shadow"
            >
              <div class="flex items-center justify-between mb-3 flex-wrap gap-2">
                <h2 class="text-lg font-bold">K线走势 (共{{ klineData.length }}条)</h2>
                <div class="flex items-center gap-2">
                  <NTabs
                    v-model:value="klinePeriod"
                    type="segment"
                    size="small"
                    style="width: 260px"
                  >
                    <NTabPane v-if="!isIndex" name="60" tab="小时K" />
                    <NTabPane name="daily" tab="日K" />
                    <NTabPane name="weekly" tab="周K" />
                    <NTabPane name="monthly" tab="月K" />
                  </NTabs>
                  <NTabs
                    v-model:value="klineViewMode"
                    type="segment"
                    size="small"
                    style="width: 160px"
                  >
                    <NTabPane name="chart" tab="图表" />
                    <NTabPane name="table" tab="表格" />
                  </NTabs>
                </div>
              </div>

              <NSpin :show="klineLoading">
                <div
                  v-show="klineData.length === 0"
                  class="py-10 text-center text-gray-400"
                >
                  <div class="text-14px">该周期暂无数据</div>
                  <div class="text-12px mt-1">请先同步该股票的K线数据</div>
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
                    仅显示最近20条, 图表模式可查看全部 {{ klineData.length }} 条
                  </div>
                </div>
              </NSpin>
            </div>

            <!-- 历史财务数据 (指数/ETF无财务, 不显示; v-show 保持图表 DOM 稳定) -->
            <div
              v-show="!noFinance && financeHistory.length > 0"
              class="mt-4 p-4 bg-white rounded-lg shadow"
            >
              <div class="flex items-center justify-between mb-3 flex-wrap gap-2">
                <h2 class="text-lg font-bold">
                  历史财务数据 (共{{ financeHistory.length }}期)
                </h2>
                <div class="flex items-center gap-2">
                  <NTabs
                    v-model:value="financeRange"
                    type="segment"
                    size="small"
                    style="width: 200px"
                  >
                    <NTabPane name="recent" tab="近16期" />
                    <NTabPane name="all" tab="全部" />
                  </NTabs>
                  <NTabs
                    v-model:value="financeViewMode"
                    type="segment"
                    size="small"
                    style="width: 160px"
                  >
                    <NTabPane name="chart" tab="图表" />
                    <NTabPane name="table" tab="表格" />
                  </NTabs>
                </div>
              </div>

              <NSpin :show="financeLoading">
                <!-- 指标分组切换(图表模式) -->
                <div v-if="financeViewMode === 'chart'" class="mb-3">
                  <NTabs v-model:value="financeChartGroup" type="segment" size="small">
                    <NTabPane name="profit" tab="盈利" />
                    <NTabPane name="growth" tab="成长" />
                    <NTabPane name="operation" tab="营运" />
                    <NTabPane name="solvency" tab="偿债与现金流" />
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
          </NTabPane>

          <!-- 实时行情Tab -->
          <NTabPane name="realtime" tab="实时行情">
            <div
              class="bg-white rounded-lg shadow overflow-hidden"
              style="height: calc(100vh - 280px)"
            >
              <iframe
                v-if="realtimeUrl"
                :src="realtimeUrl"
                class="w-full h-full border-0"
                loading="lazy"
              />
            </div>
          </NTabPane>
        </NTabs>
      </template>
    </NSpin>

    <WatchlistAddDialog
      v-model:show="showAddDialog"
      :code="code"
      :name="detail?.name || ''"
      @added="handleWatchlistAdded"
    />
  </div>
</template>
