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
import { $t } from "@/locales";

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
    message.error($t("page.tool.stockDetail.codeRequired"));
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
      message.error($t("page.tool.stockDetail.loadFailed"));
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
    message.error(e.message || $t("page.tool.stockDetail.financeLoadFailed"));
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
    message.error(e.message || $t("page.tool.stockDetail.klineLoadFailed"));
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
        message.error($t("page.tool.stockDetail.syncFailedWithReason", { error: data.lastError }));
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
    message.warning(error.message || $t("page.tool.stockDetail.syncStartFailed"));
    waitForSyncDone(() => loadDetail());
    return;
  }
  message.loading($t("page.tool.stockDetail.syncInProgress"));
  waitForSyncDone(() => {
    message.success($t("page.tool.stockDetail.syncDone"));
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
    { key: "code", label: $t("page.tool.stockDetail.infoCode"), value: d.code, tip: "" },
    { key: "name", label: $t("page.tool.stockDetail.infoName"), value: d.name, tip: "" },
    { key: "market", label: $t("page.tool.stockDetail.infoMarket"), value: d.market, tip: "" },
    { key: "industry", label: $t("page.tool.stockDetail.infoIndustry"), value: d.industry || "-", tip: "" },
    { key: "price", label: $t("page.tool.stockDetail.infoPrice"), value: d.price?.toFixed(2) || "-", tip: "" },
    {
      key: "changePct",
      label: $t("page.tool.stockDetail.infoChangePct"),
      value:
        d.changePct !== null
          ? `${d.changePct >= 0 ? "+" : ""}${d.changePct.toFixed(2)}%`
          : "-",
      tip: $t("page.tool.stockDetail.tipChangePct"),
    },
    {
      key: "turnoverRate",
      label: $t("page.tool.stockDetail.infoTurnover"),
      value: d.turnoverRate?.toFixed(2) ? `${d.turnoverRate.toFixed(2)}%` : "-",
      tip: $t("page.tool.stockDetail.tipTurnover"),
    },
    {
      key: "amount",
      label: $t("page.tool.stockDetail.infoAmount"),
      value: d.amount ? $t("page.tool.stockDetail.yiValue", { value: (d.amount / 10000).toFixed(2) }) : "-",
      tip: "",
    },
    {
      key: "marketCap",
      label: $t("page.tool.stockDetail.infoMarketCap"),
      value: d.marketCap ? $t("page.tool.stockDetail.yiValue", { value: d.marketCap.toFixed(2) }) : "-",
      tip: $t("page.tool.stockDetail.tipMarketCap"),
    },
    {
      key: "floatMarketCap",
      label: $t("page.tool.stockDetail.infoFloatMcap"),
      value: d.floatMarketCap ? $t("page.tool.stockDetail.yiValue", { value: d.floatMarketCap.toFixed(2) }) : "-",
      tip: "",
    },
  ];
  if (d.type === 2 || d.type === 5) {
    return all.filter((i) => !["industry", "turnoverRate", "marketCap", "floatMarketCap"].includes(i.key));
  }
  return all;
});

const financeItems = computed(() => {
  if (!detail.value) return [];
  const d = detail.value;
  return [
    {
      label: $t("page.tool.stockDetail.peTtm"),
      value: d.peTtm?.toFixed(2) || "-",
      tip: $t("page.tool.stockDetail.tipPeTtm"),
    },
    {
      label: $t("page.tool.stockDetail.pb"),
      value: d.pb?.toFixed(2) || "-",
      tip: $t("page.tool.stockDetail.tipPb"),
    },
    {
      label: $t("page.tool.stockDetail.fiRoe"),
      value: d.roe?.toFixed(2) ? `${d.roe.toFixed(2)}%` : "-",
      tip: $t("page.tool.stockDetail.tipRoe"),
    },
    {
      label: $t("page.tool.stockDetail.fiRevenueGrowth"),
      value: d.revenueYoy?.toFixed(2) ? `${d.revenueYoy.toFixed(2)}%` : "-",
      tip: $t("page.tool.stockDetail.tipRevenueGrowth"),
    },
    {
      label: $t("page.tool.stockDetail.fiNetProfitGrowth"),
      value: d.netProfitYoy?.toFixed(2) ? `${d.netProfitYoy.toFixed(2)}%` : "-",
      tip: $t("page.tool.stockDetail.tipNetProfitGrowth"),
    },
    {
      label: $t("page.tool.stockDetail.fiGrossMargin"),
      value: d.grossMargin?.toFixed(2) ? `${d.grossMargin.toFixed(2)}%` : "-",
      tip: $t("page.tool.stockDetail.tipGrossMargin"),
    },
    {
      label: $t("page.tool.stockDetail.fiNetMargin"),
      value: d.netMargin?.toFixed(2) ? `${d.netMargin.toFixed(2)}%` : "-",
      tip: $t("page.tool.stockDetail.tipNetMargin"),
    },
    {
      label: $t("page.tool.stockDetail.fiDebtRatio"),
      value: d.debtRatio?.toFixed(2) ? `${d.debtRatio.toFixed(2)}%` : "-",
      tip: $t("page.tool.stockDetail.tipDebtRatio"),
    },
    {
      label: $t("page.tool.stockDetail.currentRatio"),
      value: d.currentRatio?.toFixed(2) || "-",
      tip: $t("page.tool.stockDetail.tipCurrentRatio"),
    },
    {
      label: $t("page.tool.stockDetail.quickRatio"),
      value: d.quickRatio?.toFixed(2) || "-",
      tip: $t("page.tool.stockDetail.tipQuickRatio"),
    },
    {
      label: $t("page.tool.stockDetail.cashRatio"),
      value: d.cashRatio?.toFixed(2) || "-",
      tip: $t("page.tool.stockDetail.tipCashRatio"),
    },
    {
      label: $t("page.tool.stockDetail.fiNrTurnover"),
      value: d.nrTurnRatio?.toFixed(2) || "-",
      tip: $t("page.tool.stockDetail.tipNrTurnover"),
    },
    {
      label: $t("page.tool.stockDetail.fiInvTurnover"),
      value: d.invTurnRatio?.toFixed(2) || "-",
      tip: $t("page.tool.stockDetail.tipInvTurnover"),
    },
    {
      label: $t("page.tool.stockDetail.fiEquityYoy"),
      value:
        d.yoyEquity !== null
          ? `${d.yoyEquity >= 0 ? "+" : ""}${d.yoyEquity.toFixed(2)}%`
          : "-",
      tip: $t("page.tool.stockDetail.tipEquityYoy"),
    },
    {
      label: $t("page.tool.stockDetail.fiAssetYoy"),
      value:
        d.yoyAsset !== null
          ? `${d.yoyAsset >= 0 ? "+" : ""}${d.yoyAsset.toFixed(2)}%`
          : "-",
      tip: $t("page.tool.stockDetail.tipAssetYoy"),
    },
    {
      label: $t("page.tool.stockDetail.cfoToOr"),
      value: d.cfoToOr?.toFixed(2) || "-",
      tip: $t("page.tool.stockDetail.tipCfoToOr"),
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
    ok: { label: $t("page.tool.stockDetail.synced"), type: "success" },
    failed: { label: $t("page.tool.stockDetail.syncFailed"), type: "error" },
    pending: { label: $t("page.tool.stockDetail.notSynced"), type: "default" },
  };
  return meta[s] || meta.pending;
});

const financeStatusMeta = computed(() => {
  const s = syncState.value?.financeStatus || "pending";
  const meta: Record<
    string,
    { label: string; type: "success" | "error" | "default" | "info" }
  > = {
    ok: { label: $t("page.tool.stockDetail.synced"), type: "success" },
    failed: { label: $t("page.tool.stockDetail.syncFailed"), type: "error" },
    pending: { label: $t("page.tool.stockDetail.notSynced"), type: "default" },
    skipped: { label: $t("page.tool.stockDetail.noFinanceData"), type: "info" },
  };
  return meta[s] || meta.pending;
});

const syncItems = computed(() => {
  const st = syncState.value;
  const items = [
    { label: $t("page.tool.stockDetail.dailyKTo"), value: fmtSyncDate(st?.klineDailyTo) },
    { label: $t("page.tool.stockDetail.weeklyKTo"), value: fmtSyncDate(st?.klineWeeklyTo) },
    { label: $t("page.tool.stockDetail.monthlyKTo"), value: fmtSyncDate(st?.klineMonthlyTo) },
  ];
  if (!isIndex.value) {
    items.push({ label: $t("page.tool.stockDetail.hourlyKTo"), value: fmtSyncDateTime(st?.klineHourlyTo) });
  }
  items.push({ label: $t("page.tool.stockDetail.klineSyncedAt"), value: fmtSyncDateTime(st?.klineSyncedAt) });
  if (!noFinance.value) {
    items.push(
      { label: $t("page.tool.stockDetail.financeTo"), value: fmtSyncDate(st?.financeTo) },
      { label: $t("page.tool.stockDetail.financeSyncedAt"), value: fmtSyncDateTime(st?.financeSyncedAt) }
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
        message.info($t("page.tool.stockDetail.syncRunningDetected"));
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
        {{ $t("page.tool.stockDetail.backToScreen") }}
      </NButton>
      <div class="flex gap-2">
        <NButton
          type="primary"
          :loading="syncLoading || syncRunning"
          :disabled="syncRunning"
          @click="handleSync"
        >
          <template #icon><span class="i-mdi:refresh" /></template>
          {{ syncRunning ? $t("page.tool.stockDetail.syncing") : $t("page.tool.stockDetail.syncLatest") }}
        </NButton>
        <NButton v-if="!watchlistCodes.has(code)" type="primary" @click="handleAddWatchlist">{{ $t("page.tool.stockDetail.addWatchlist") }}</NButton>
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
          <NTabPane name="local" :tab="$t('page.tool.stockDetail.localData')">
            <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
              <!-- 基本信息 -->
              <div class="p-4 bg-white rounded-lg shadow">
                <h2 class="text-lg font-bold mb-3">{{ $t("page.tool.stockDetail.basicInfo") }}</h2>
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
                <h2 class="text-lg font-bold mb-3">{{ $t("page.tool.stockDetail.financeIndicators") }}</h2>
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
                <h2 class="text-lg font-bold mb-3">{{ $t("page.tool.stockDetail.syncStatusTitle") }}</h2>
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
                    <span class="text-gray-500">{{ $t("page.tool.stockDetail.klineStatusLabel") }}</span>
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
                    <span class="text-gray-500">{{ $t("page.tool.stockDetail.financeStatusLabel") }}</span>
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
              <h2 class="text-lg font-bold mb-3">{{ $t("page.tool.stockDetail.concepts") }}</h2>
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

            <!-- 历史财务数据 (指数/ETF无财务, 不显示; v-show 保持图表 DOM 稳定) -->
            <div
              v-show="!noFinance && financeHistory.length > 0"
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
          </NTabPane>

          <!-- 实时行情Tab -->
          <NTabPane name="realtime" :tab="$t('page.tool.stockDetail.realtime')">
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
