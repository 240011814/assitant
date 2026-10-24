<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  stockDetail,
  syncSingleStock,
  fetchSyncStatus,
  fetchStockSyncState,
  getWatchlistCodes,
} from "@/service/api";
import WatchlistAddDialog from "@/components/custom/watchlist-add-dialog.vue";
import { useMessage, NButton, NSpin, NTag, NTabs, NTabPane } from "naive-ui";
import { $t } from "@/locales";
import InfoCards from "./components/info-cards.vue";
import KlinePanel from "./components/kline-panel.vue";
import FinancePanel from "./components/finance-panel.vue";

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
const loading = ref(false);
const activeTab = ref("local");
const syncState = ref<Api.Stock.SyncState | null>(null);

// K线/财务面板自持数据加载, 同步完成后经 ref 触发刷新
const klinePanelRef = ref<InstanceType<typeof KlinePanel>>();
const financePanelRef = ref<InstanceType<typeof FinancePanel>>();

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

const isIndex = computed(() => detail.value?.type === 2);
const isEtf = computed(() => detail.value?.type === 5);
// 指数与 ETF 均无财务数据, 相关区块不显示
const noFinance = computed(() => isIndex.value || isEtf.value);

async function loadDetail() {
  if (!code.value) {
    message.error($t("page.tool.stockDetail.codeRequired"));
    return;
  }
  loading.value = true;
  try {
    const [detailRes, , syncStateRes] = await Promise.all([
      stockDetail(code.value),
      klinePanelRef.value?.reload(),
      fetchStockSyncState(code.value),
    ]);
    // flat request 永不 reject, 需显式判 error
    if (detailRes.error || syncStateRes.error) {
      message.error($t("page.tool.stockDetail.loadFailed"));
      return;
    }
    if (detailRes.data) {
      detail.value = detailRes.data;
    }
    syncState.value = syncStateRes.data || null;
  } finally {
    loading.value = false;
  }
}

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

/** 同步完成后刷新详情与两个数据面板 */
function refreshAll() {
  loadDetail();
  klinePanelRef.value?.reload();
  financePanelRef.value?.reload();
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
    waitForSyncDone(() => refreshAll());
    return;
  }
  message.loading($t("page.tool.stockDetail.syncInProgress"));
  waitForSyncDone(() => {
    message.success($t("page.tool.stockDetail.syncDone"));
    refreshAll();
  });
}

function goBack() {
  router.push({ name: "tool_stockscreen" });
}

onMounted(() => {
  loadDetail();
  loadWatchlistCodes();

  fetchSyncStatus()
    .then(({ data }) => {
      if (data?.running) {
        message.info($t("page.tool.stockDetail.syncRunningDetected"));
        waitForSyncDone(() => refreshAll());
      }
    })
    .catch(() => {
      // ignore
    });
});

onUnmounted(() => {
  stopSyncPolling();
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
            <!-- 基本信息 / 财务指标 / 同步状态 -->
            <InfoCards :detail="detail" :sync-state="syncState" />

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

            <!-- K线数据 -->
            <KlinePanel ref="klinePanelRef" :code="code" :is-index="isIndex" />

            <!-- 历史财务数据 (指数/ETF无财务, 不显示) -->
            <FinancePanel v-if="!noFinance" ref="financePanelRef" :code="code" />
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
