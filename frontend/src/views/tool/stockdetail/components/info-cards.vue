<script setup lang="ts">
import { computed } from "vue";
import { NTag } from "naive-ui";
import { $t } from "@/locales";

/**
 * 本地数据 Tab 的信息卡片组: 基本信息 / 财务指标 / 数据同步状态。
 * 纯展示组件, 数据由父组件传入。
 */
const props = defineProps<{
  detail: Api.Stock.ScreenResult;
  syncState: Api.Stock.SyncState | null;
}>();

const isIndex = computed(() => props.detail.type === 2);
const isEtf = computed(() => props.detail.type === 5);
// 指数与 ETF 均无财务数据, 相关区块不显示
const noFinance = computed(() => isIndex.value || isEtf.value);

const infoItems = computed(() => {
  const d = props.detail;
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
  const d = props.detail;
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
  const s = props.syncState?.klineStatus || "pending";
  const meta: Record<string, { label: string; type: "success" | "error" | "default" }> = {
    ok: { label: $t("page.tool.stockDetail.synced"), type: "success" },
    failed: { label: $t("page.tool.stockDetail.syncFailed"), type: "error" },
    pending: { label: $t("page.tool.stockDetail.notSynced"), type: "default" },
  };
  return meta[s] || meta.pending;
});

const financeStatusMeta = computed(() => {
  const s = props.syncState?.financeStatus || "pending";
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
  const st = props.syncState;
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
</script>

<template>
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
</template>
