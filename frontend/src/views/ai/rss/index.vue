<script setup lang="ts">
import { computed, nextTick, onActivated, onDeactivated, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { NButton, NButtonGroup, NEmpty, NPopconfirm, NSpin, NTag, useMessage } from "naive-ui";
import { useRouterPush } from "@/hooks/common/router";
import { useReaderMiniLeaveSave } from "@/hooks/common/reader-mini-save";
import { fetchRSSAddSource, fetchRSSListSources, fetchRSSUpdateSource, fetchRSSDeleteSource, fetchRSSFetchSource, fetchRSSListItems, fetchRSSImportItem } from "@/service/api";
import type { RSSItem, RSSSource } from "@/service/api";
import { useAppStore } from "@/store/modules/app";
import { useReaderSessionStore } from "@/store/modules/reader-session";
import type { ReaderSessionSnapshot } from "@/store/modules/reader-session";
import { $t } from "@/locales";
import { difficultyTag } from "../reading/shared";
import RssViewer from "./components/rss-viewer.vue";
import { avatarColor, relTime, sourceInitial as initialOf, sourceTitle as titleOf } from "./shared";

const appStore = useAppStore();
const message = useMessage();
const { routerPushByKey } = useRouterPush();
const route = useRoute();
const vueRouter = useRouter();

const loading = ref(false);
const sources = ref<(RSSSource & { unread?: number })[]>([]);
const items = ref<RSSItem[]>([]);
const filterSourceId = ref<number | null>(null);
// 状态过滤: 全部 / 已读 (含已导入) / 未读
const statusFilter = ref<"all" | "read" | "unread">("all");
const statusOptions = [
  { label: "全部", value: "all" as const },
  { label: "已读", value: "read" as const },
  { label: "未读", value: "unread" as const }
];
const timeAsc = ref(false);
const easyOnly = ref(false);

const selectStatus = (value: "all" | "read" | "unread") => {
  if (statusFilter.value === value) return;
  statusFilter.value = value;
  // eslint-disable-next-line @typescript-eslint/no-use-before-define -- 既有写法 (分页声明在下方)
  pagination.value.page = 1;
  loadItems();
};

const pagination = ref({
  page: 1,
  pageSize: 20,
  itemCount: 0,
  showSizePicker: true,
  pageSizes: [20, 50, 100],
  onChange: (page: number) => {
    pagination.value.page = page;
    loadItems();
  },
  onUpdatePageSize: (pageSize: number) => {
    pagination.value.pageSize = pageSize;
    pagination.value.page = 1;
    loadItems();
  }
});

const totalUnread = computed(() => sources.value.reduce((sum, s) => sum + (s.unread || 0), 0));
const sourceTitle = (sourceId: number) => titleOf(sources.value, sourceId);
const sourceInitial = (sourceId: number) => initialOf(sources.value, sourceId);

// ===================== 数据加载 =====================
const loadSources = async () => {
  const { data: resp, error } = await fetchRSSListSources();
  if (error) return;
  sources.value = resp || [];
  if (filterSourceId.value && !sources.value.some((s) => s.id === filterSourceId.value)) {
    filterSourceId.value = null;
  }
};

async function loadItems() {
  loading.value = true;
  const { data: resp, error } = await fetchRSSListItems({
    sourceId: filterSourceId.value || undefined,
    status: statusFilter.value === "all" ? undefined : statusFilter.value,
    easyOnly: easyOnly.value || undefined,
    sort: timeAsc.value ? "time_asc" : "time_desc",
    page: pagination.value.page,
    page_size: pagination.value.pageSize
  });
  loading.value = false;
  if (error) {
    message.error($t("page.ai.rss.fetchListFailed"));
    return;
  }
  items.value = resp?.list || [];
  pagination.value.itemCount = resp?.total || 0;
}

async function reloadAll() {
  await Promise.all([loadSources(), loadItems()]);
}

const selectSource = (id: number | null) => {
  if (filterSourceId.value === id) return; // 同值守卫: 重复点击不重复请求
  filterSourceId.value = id;
  pagination.value.page = 1;
  loadItems();
};

// ===================== 订阅管理 =====================
const showAddModal = ref(false);
const addForm = ref({ title: "", url: "" });
const adding = ref(false);
const fetchingAll = ref(false);

const handleAdd = async () => {
  if (!addForm.value.url.trim()) {
    message.warning($t("page.ai.rss.feedInvalid"));
    return;
  }
  adding.value = true;
  const { data: resp, error } = await fetchRSSAddSource({ title: addForm.value.title, url: addForm.value.url.trim() });
  adding.value = false;
  if (error || !resp) {
    message.error($t("page.ai.rss.addFailed", { msg: (error as any)?.message || $t("page.ai.rss.feedInvalid") }));
    return;
  }
  message.success($t("page.ai.rss.addSuccess", { title: resp.title }));
  showAddModal.value = false;
  addForm.value = { title: "", url: "" };
  filterSourceId.value = resp.id;
  reloadAll();
};

const showEditModal = ref(false);
const editForm = ref({ id: 0, title: "", url: "" });
const editing = ref(false);

const openEditSource = (src: RSSSource) => {
  editForm.value = { id: src.id, title: src.title, url: src.url };
  showEditModal.value = true;
};

const handleUpdate = async () => {
  if (!editForm.value.url.trim()) {
    message.warning($t("page.ai.rss.feedInvalid"));
    return;
  }
  editing.value = true;
  const { data: resp, error } = await fetchRSSUpdateSource(editForm.value.id, {
    title: editForm.value.title.trim(),
    url: editForm.value.url.trim()
  });
  editing.value = false;
  if (error || !resp) {
    message.error($t("page.ai.rss.saveFailed", { msg: (error as any)?.message || $t("page.ai.rss.feedInvalid") }));
    return;
  }
  message.success($t("page.ai.rss.saveSuccess"));
  showEditModal.value = false;
  reloadAll();
};

const handleDeleteSource = async (id: number) => {
  const { error } = await fetchRSSDeleteSource(id);
  if (error) {
    message.error($t("page.ai.rss.deleteFailed", { msg: (error as any)?.message || $t("page.ai.reader.unknownError") }));
    return;
  }
  message.success($t("page.ai.rss.deleteSuccess"));
  reloadAll();
};

// 逐源顺序抓取: 每个请求都短, 避免 Cloudflare 100s 上限
const handleFetchAll = async () => {
  const enabled = sources.value.filter((s) => s.enabled);
  if (enabled.length === 0) return;
  fetchingAll.value = true;
  let inserted = 0;
  let failed = 0;
  for (const s of enabled) {
    const { data: resp, error } = await fetchRSSFetchSource(s.id);
    if (error) {
      failed += 1;
    } else {
      inserted += resp?.inserted ?? 0;
    }
  }
  fetchingAll.value = false;
  message.success(
    `${$t("page.ai.rss.fetchDone", { count: inserted })}${failed ? `, ${$t("page.ai.rss.fetchFailedSources", { count: failed })}` : ""}`
  );
  loadItems();
  loadSources();
};

// 列表行"导入阅读": 导入分级阅读 (阅读弹窗内的同款按钮在 RssViewer 内自持)
const rowImporting = ref(false);
const handleImport = async (item: RSSItem) => {
  rowImporting.value = true;
  const { data: resp, error } = await fetchRSSImportItem(item.id);
  rowImporting.value = false;
  if (error || !resp) {
    message.error("导入失败");
    return;
  }
  message.success(`已导入「${resp.title}」到分级阅读`);
  item.status = "imported";
  item.importedDocId = resp.id;
};

// ===================== 阅读弹窗 (RssViewer 组件: 排版/查词/翻译/改写/生词都在组件内, 这里只管开合与全局小窗会话) =====================
const activeItem = ref<RSSItem | null>(null);
// 小窗恢复的续读位置 (随条目一起传给 RssViewer, 正文渲染完成后按比例写回)
const pendingRestore = ref<{ scrollTop: number; progress: number } | null>(null);
// 恢复时强制重建 viewer: 弹窗已开着同一篇时 item 引用不变, 不重建不会触发重新加载与位置恢复
const viewerKey = ref(0);
const viewerRef = ref<InstanceType<typeof RssViewer> | null>(null);
const sessionStore = useReaderSessionStore();

const openItem = (item: RSSItem) => {
  pendingRestore.value = null;
  activeItem.value = item;
};

const closeViewer = () => {
  activeItem.value = null;
};

// 弹窗打开中的阅读快照: 供小窗按钮与离开自动收窗使用 (弹窗关闭时 viewer 已卸载, 返回 null)
const captureViewerSession = (): ReaderSessionSnapshot | null => {
  const snap = viewerRef.value?.getSessionSnapshot();
  return snap ? { kind: "rss", ...snap } : null;
};

// 切 tab / 路由跳转 / 关浏览器标签时弹窗仍开着 → 自动收入全局小窗
useReaderMiniLeaveSave(captureViewerSession);

// 小窗按钮: 存全局会话并收起弹窗 (不刷新列表, 与 X 关闭区分)
function minimizeViewer() {
  const snapshot = captureViewerSession();
  if (snapshot) sessionStore.minimize(snapshot);
  activeItem.value = null;
}

// 全局小窗卡片"继续阅读": 只消费 rss 会话 (分级阅读的留给详情页消费), 找回条目并打开, 位置由 RssViewer 恢复
async function tryConsumeRestore() {
  const snapshot = sessionStore.consumeRestore({ kind: "rss" });
  if (!snapshot) return;
  // 列表尚未加载时先拉一次 (整页刷新后直点小窗卡片的场景)
  if (!items.value.length) {
    await loadItems();
  }
  const target = items.value.find((i) => i.id === snapshot.itemId);
  const item: RSSItem =
    target ||
    ({
      id: snapshot.itemId,
      title: snapshot.title,
      link: "",
      summary: "",
      publishedAt: null,
      status: "read",
      importedDocId: 0,
      createdAt: ""
    } as RSSItem);
  pendingRestore.value = { scrollTop: snapshot.scrollTop, progress: snapshot.progress };
  viewerKey.value += 1;
  activeItem.value = item;
}

// 小窗卡片在任意页面被点击 (restorePending 置位) → 本页消费恢复请求
watch(
  () => sessionStore.restorePending,
  (pending) => {
    if (pending) tryConsumeRestore();
  }
);

// 生词本"回到原文"跳转: /ai/rss?openItem=123 → 打开指定条目 (路由 query, 代替旧 localStorage 方案)
const openFromQuery = (rawId: unknown) => {
  const id = Number(rawId);
  if (!Number.isFinite(id) || id <= 0) return;
  // 用完即清 query: 支持同一篇文章再次"回到原文"也能触发
  vueRouter.replace({ query: { ...route.query, openItem: undefined } });
  nextTick(async () => {
    if (!items.value.length) await loadItems();
    const target = items.value.find((i) => i.id === id);
    if (target) {
      openItem(target);
    } else {
      message.info($t("page.ai.rss.itemNotInList"));
    }
  });
};

onMounted(() => {
  reloadAll();
  if (route.query.openItem) {
    openFromQuery(route.query.openItem);
    return;
  }
  // 全局小窗卡片"继续阅读"跳转过来 (卡片点击先于本页挂载): 消费恢复请求
  if (sessionStore.restorePending) {
    tryConsumeRestore();
  }
});

// KeepAlive 存活期间从生词本再次跳转 (query 变化触发)
watch(
  () => route.query.openItem,
  (v) => {
    if (v) openFromQuery(v);
  }
);

// KeepAlive 缓存下再次激活时刷新 (j/k 监听与阅读弹窗状态在 RssViewer 内自持)
let hasActivated = false;
onActivated(() => {
  if (hasActivated) {
    reloadAll();
  } else {
    hasActivated = true;
  }
});

// 失活时关闭阅读弹窗: NModal teleport 到 body, 不随 KeepAlive 失活隐藏 (视觉残留) 且
// 弹窗内 ReaderView 收不到 onDeactivated, 心跳不会停; 会话已由 leave-save 收入小窗, 恢复走卡片。
// 注册在 useReaderMiniLeaveSave 之后, 先存快照再关弹窗
onDeactivated(() => {
  activeItem.value = null;
});
</script>

<template>
  <div class="h-full flex gap-4" :class="appStore.isMobile ? 'flex-col overflow-auto p-2' : 'p-4'">
    <!-- 左侧: 订阅源列表 (桌面) -->
    <NCard
      v-if="!appStore.isMobile"
      size="small"
      :bordered="false"
      shadow="sm"
      class="w-60 shrink-0 self-start"
      :title="$t('page.ai.rss.sources', { count: sources.length })"
    >
      <div class="flex flex-col gap-0.5">
        <div
          class="flex items-center gap-2 rounded px-2 py-1.5 cursor-pointer"
          :class="filterSourceId === null ? 'bg-primary/10' : 'hover:bg-gray-100 dark:hover:bg-gray-800'"
          @click="selectSource(null)"
        >
          <span class="flex-1 min-w-0 text-sm font-bold truncate">{{ $t('page.ai.rss.all') }}</span>
          <NTag v-if="totalUnread > 0" size="tiny" type="warning" :bordered="false">{{ totalUnread }}</NTag>
        </div>
        <div
          v-for="src in sources"
          :key="src.id"
          class="group flex items-center gap-2 rounded px-2 py-1.5"
          :class="filterSourceId === src.id ? 'bg-primary/10' : 'hover:bg-gray-100 dark:hover:bg-gray-800'"
        >
          <span
            class="w-[18px] h-[18px] rounded-full text-white text-10px font-medium flex items-center justify-center shrink-0"
            :style="{ background: avatarColor(src.id) }"
          >
            {{ sourceInitial(src.id) }}
          </span>
          <!-- 切换只绑在标题上: 行内操作按钮永远不会误触发切换 -->
          <span class="flex-1 min-w-0 text-sm truncate cursor-pointer" :title="src.title" @click="selectSource(src.id)">
            {{ src.title }}
          </span>
          <span class="flex items-center gap-1 shrink-0" @click.stop>
            <NTag v-if="(src.unread || 0) > 0" size="tiny" type="warning" :bordered="false">{{ src.unread }}</NTag>
            <span
              class="inline-flex cursor-pointer text-gray-300 group-hover:text-primary"
              :title="$t('page.ai.rss.editSource')"
              @click="openEditSource(src)"
            >
              <SvgIcon icon="mdi:pencil-outline" class="text-14px" />
            </span>
            <NPopconfirm @positive-click="handleDeleteSource(src.id)">
              <template #trigger>
                <!-- SvgIcon inheritAttrs=false 会吞掉事件监听, 交互触发必须用原生元素包一层 -->
                <span
                  class="inline-flex cursor-pointer text-gray-300 group-hover:text-error"
                  :title="$t('page.ai.rss.deleteSource')"
                >
                  <SvgIcon icon="mdi:delete-outline" class="text-14px" />
                </span>
              </template>
              {{ $t('page.ai.rss.deleteSourceConfirm', { title: src.title }) }}
            </NPopconfirm>
          </span>
        </div>
        <NEmpty v-if="sources.length === 0" size="small" :description="$t('page.ai.rss.noSources')" class="py-4" />
      </div>
    </NCard>

    <!-- 主区: 工具栏 + Folo 时间线文章列表 -->
    <NCard :bordered="false" :shadow="appStore.isMobile ? false : 'sm'" class="flex-1 min-w-0">
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-2 w-full">
          <span class="font-bold text-18px">{{ filterSourceId ? sourceTitle(filterSourceId) : $t('page.ai.rss.allArticles') }}</span>
          <div class="flex flex-wrap gap-2">
            <div class="flex items-center gap-2 mr-2">
              <span class="text-xs text-gray-400">{{ $t('page.ai.rss.status') }}</span>
              <NButtonGroup size="small">
                <NButton
                  v-for="opt in statusOptions"
                  :key="opt.value"
                  size="small"
                  :type="statusFilter === opt.value ? 'primary' : 'default'"
                  @click="selectStatus(opt.value)"
                >
                  {{ $t(`page.ai.rss.${opt.value}`) }}
                </NButton>
              </NButtonGroup>
            </div>
            <div class="flex items-center gap-2 mr-2">
              <span class="text-xs text-gray-400">{{ $t('page.ai.rss.easyOnly') }}</span>
              <NSwitch
                v-model:value="easyOnly"
                size="small"
                @update:value="() => { pagination.page = 1; loadItems(); }"
              />
            </div>
            <NButton
              size="small"
              :title="timeAsc ? $t('page.ai.rss.timeAsc') : $t('page.ai.rss.timeDesc')"
              @click="() => { timeAsc = !timeAsc; pagination.page = 1; loadItems(); }"
            >
              <template #icon>
                <SvgIcon :icon="timeAsc ? 'mdi:sort-ascending' : 'mdi:sort-descending'" class="text-icon" />
              </template>
              {{ timeAsc ? $t('page.ai.rss.timeAsc') : $t('page.ai.rss.timeDesc') }}
            </NButton>
            <NButton size="small" :loading="fetchingAll" @click="handleFetchAll">{{ $t('page.ai.rss.fetchAll') }}</NButton>
            <NButton size="small" @click="showAddModal = true">{{ $t('page.ai.rss.addSource') }}</NButton>
          </div>
        </div>
      </template>

      <!-- Mobile: 源横向标签条 -->
      <div v-if="appStore.isMobile && sources.length" class="flex gap-2 overflow-x-auto mb-3 pb-1">
        <NTag
          checkable
          :checked="filterSourceId === null"
          size="small"
          @update:checked="() => selectSource(null)"
        >
          {{ $t('page.ai.rss.all') }}
        </NTag>
        <NTag
          v-for="src in sources"
          :key="src.id"
          checkable
          :checked="filterSourceId === src.id"
          size="small"
          @update:checked="(checked: boolean) => selectSource(checked ? src.id : null)"
        >
          {{ src.title }}
          <span v-if="(src.unread || 0) > 0" class="ml-1 text-warning">{{ src.unread }}</span>
        </NTag>
      </div>

      <!-- 文章列表 (桌面/移动共用 Folo 时间线风格) -->
      <NSpin :show="loading">
        <div class="flex flex-col">
          <div
            v-for="row in items"
            :key="row.id"
            class="group flex gap-2.5 px-3 py-3 rounded-lg cursor-pointer transition-colors hover:bg-gray-100 dark:hover:bg-gray-800/60"
            @click="openItem(row)"
          >
            <!-- 未读圆点占位: 未读显示主题色圆点, 已读占位保持缩进 -->
            <span v-if="row.status === 'unread'" class="w-1.5 h-1.5 rounded-full bg-primary mt-[9px] shrink-0" />
            <span v-else class="w-1.5 shrink-0" />
            <div class="flex-1 min-w-0">
              <div class="flex items-baseline justify-between gap-3">
                <span
                  class="truncate"
                  :class="
                    row.status === 'unread'
                      ? 'font-semibold text-15px text-gray-900 dark:text-gray-50'
                      : 'text-15px text-gray-500 dark:text-gray-400'
                  "
                  :title="row.title"
                >
                  {{ row.title }}
                </span>
                <span class="text-xs text-gray-400 dark:text-gray-500 shrink-0" :title="row.publishedAt ? new Date(row.publishedAt).toLocaleString() : ''">
                  {{ relTime(row.publishedAt) }}
                </span>
              </div>
              <div v-if="row.summary" class="line-clamp-1 mt-1 text-13px text-gray-400 dark:text-gray-500">
                {{ row.summary }}
              </div>
              <div class="flex items-center gap-1.5 mt-1.5 min-w-0">
                <span
                  class="w-4 h-4 rounded-full text-white text-9px font-medium flex items-center justify-center shrink-0"
                  :style="{ background: avatarColor(row.sourceId) }"
                >
                  {{ sourceInitial(row.sourceId) }}
                </span>
                <span class="text-xs text-gray-400 dark:text-gray-500 truncate">{{ sourceTitle(row.sourceId) }}</span>
                <NTag v-if="difficultyTag(row.estUnknownRate)" size="tiny" :type="difficultyTag(row.estUnknownRate)!.type" :bordered="false" class="shrink-0">
                  {{ difficultyTag(row.estUnknownRate)!.label }}
                </NTag>
                <NTag v-if="row.status === 'imported'" size="tiny" type="success" :bordered="false" class="shrink-0">
                  {{ $t('page.ai.rss.importedTag') }}
                </NTag>
              </div>
            </div>
            <!-- 行内操作: 桌面悬停显现, 移动端常显 -->
            <div
              class="self-center shrink-0 transition-opacity md:opacity-0 md:group-hover:opacity-100"
              @click.stop
            >
              <NButton
                v-if="row.status === 'imported'"
                size="tiny"
                quaternary
                type="primary"
                @click="routerPushByKey('ai_reading-detail', { params: { id: String(row.importedDocId) } })"
              >
                {{ $t('page.ai.rss.goToAnnotate') }}
              </NButton>
              <NButton v-else size="tiny" quaternary type="info" :loading="rowImporting" @click="handleImport(row)">{{ $t('page.ai.rss.importReading') }}</NButton>
            </div>
          </div>
        </div>
        <NEmpty v-if="items.length === 0 && !loading" class="py-10" :description="$t('page.ai.rss.noArticles')" />
      </NSpin>

      <NPagination
        v-if="pagination.itemCount > pagination.pageSize"
        class="mt-3 justify-center"
        :page="pagination.page"
        :item-count="pagination.itemCount"
        :page-size="pagination.pageSize"
        :page-slot="5"
        show-size-picker
        :page-sizes="pagination.pageSizes"
        @update:page="(p: number) => pagination.onChange(p)"
        @update:page-size="(s: number) => pagination.onUpdatePageSize(s)"
      />
    </NCard>

    <!-- 添加订阅 -->
    <NModal v-model:show="showAddModal" preset="card" :title="$t('page.ai.rss.addTitle')" :style="{ width: appStore.isMobile ? '92%' : '460px' }">
      <div class="flex flex-col gap-4 py-2">
        <NInput v-model:value="addForm.url" :placeholder="$t('page.ai.rss.addUrlPlaceholder')" @keyup.enter="handleAdd" />
        <NInput v-model:value="addForm.title" :placeholder="$t('page.ai.rss.addTitlePlaceholder')" />
        <NButton type="primary" block :loading="adding" @click="handleAdd">{{ $t('page.ai.rss.addSubmit') }}</NButton>
        <div class="text-xs text-gray-400">
          {{ $t('page.ai.rss.addHint') }}
        </div>
      </div>
    </NModal>

    <!-- 编辑订阅 (改名 / 换 feed 地址) -->
    <NModal v-model:show="showEditModal" preset="card" :title="$t('page.ai.rss.editTitle')" :style="{ width: appStore.isMobile ? '92%' : '460px' }">
      <div class="flex flex-col gap-4 py-2">
        <NInput v-model:value="editForm.url" :placeholder="$t('page.ai.rss.editUrlPlaceholder')" @keyup.enter="handleUpdate" />
        <NInput v-model:value="editForm.title" :placeholder="$t('page.ai.rss.editTitlePlaceholder')" />
        <NButton type="primary" block :loading="editing" @click="handleUpdate">{{ $t('page.ai.rss.save') }}</NButton>
        <div class="text-xs text-gray-400">
          {{ $t('page.ai.rss.editHint') }}
        </div>
      </div>
    </NModal>

    <!-- 阅读视图 (RssViewer 组件: 居中窄栏 + 大标题 + 可调排版 + 点词查词 + j/k 导航) -->
    <NModal :show="activeItem !== null" :auto-focus="false" @update:show="(v: boolean) => { if (!v) closeViewer(); }">
      <div class="w-full h-full md:w-[min(94vw,960px)] md:h-[88vh] flex flex-col rounded-none md:rounded-xl bg-white dark:bg-[#18181c] shadow-2xl overflow-hidden">
        <RssViewer
          v-if="activeItem !== null"
          :key="viewerKey"
          ref="viewerRef"
          :item="activeItem"
          :items="items"
          :sources="sources"
          :restore="pendingRestore"
          @update:item="(v: RSSItem) => { activeItem = v; pendingRestore = null; }"
          @close="closeViewer"
          @minimize="minimizeViewer"
        />
      </div>
    </NModal>
  </div>
</template>

<style scoped>
.line-clamp-1 {
  display: -webkit-box;
  -webkit-line-clamp: 1;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
