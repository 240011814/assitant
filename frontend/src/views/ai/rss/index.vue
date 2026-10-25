<script setup lang="ts">
import { computed, onActivated, onBeforeUnmount, onDeactivated, onMounted, ref } from "vue";
import { NButton, NEmpty, NPopconfirm, NTag, useMessage } from "naive-ui";
import { useRouterPush } from "@/hooks/common/router";
import ReaderView from "@/components/custom/reader-view.vue";
import {
  fetchRSSAddSource,
  fetchRSSListSources,
  fetchRSSDeleteSource,
  fetchRSSFetchSource,
  fetchRSSListItems,
  fetchRSSMarkRead,
  fetchRSSItemContent,
  fetchRSSImportItem,
} from "@/service/api";
import type { RSSItem, RSSSource } from "@/service/api";
import { useAppStore } from "@/store/modules/app";

const appStore = useAppStore();
const message = useMessage();
const { routerPushByKey } = useRouterPush();

const loading = ref(false);
const sources = ref<(RSSSource & { unread?: number })[]>([]);
const items = ref<RSSItem[]>([]);
const filterSourceId = ref<number | null>(null);
const unreadOnly = ref(false);
const timeAsc = ref(false);

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
const sourceTitle = (sourceId: number) => sources.value.find((s) => s.id === sourceId)?.title || `源 #${sourceId}`;

// ===================== Folo 风格辅助: 源头像 / 相对时间 =====================
const avatarColor = (id: number) => `hsl(${(id * 67) % 360}, 62%, 52%)`;
const sourceInitial = (sourceId: number) => {
  const title = sources.value.find((s) => s.id === sourceId)?.title?.trim();
  return (title || "?").charAt(0).toUpperCase();
};

/** 相对时间: 刚刚 / n 分钟前 / n 小时前 / 昨天 / n 天前 / 日期 */
const relTime = (iso?: string | null) => {
  if (!iso) return "";
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return "";
  const diff = Date.now() - t;
  const minute = 60_000;
  const hour = 3_600_000;
  const day = 86_400_000;
  if (diff < minute) return "刚刚";
  if (diff < hour) return `${Math.floor(diff / minute)} 分钟前`;
  if (diff < day) return `${Math.floor(diff / hour)} 小时前`;
  if (diff < 2 * day) return "昨天";
  if (diff < 7 * day) return `${Math.floor(diff / day)} 天前`;
  return new Date(iso).toLocaleDateString();
};

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
    unreadOnly: unreadOnly.value || undefined,
    sort: timeAsc.value ? "time_asc" : "time_desc",
    page: pagination.value.page,
    page_size: pagination.value.pageSize
  });
  loading.value = false;
  if (error) {
    message.error("获取文章列表失败");
    return;
  }
  items.value = resp?.list || [];
  pagination.value.itemCount = resp?.total || 0;
}

async function reloadAll() {
  await Promise.all([loadSources(), loadItems()]);
}

const selectSource = (id: number | null) => {
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
    message.warning("请输入 feed 地址");
    return;
  }
  adding.value = true;
  const { data: resp, error } = await fetchRSSAddSource({ title: addForm.value.title, url: addForm.value.url.trim() });
  adding.value = false;
  if (error || !resp) {
    message.error(`添加订阅失败: ${(error as any)?.message || "请确认地址是可解析的 RSS/Atom feed"}`);
    return;
  }
  message.success(`已添加「${resp.title}」并抓取首批文章`);
  showAddModal.value = false;
  addForm.value = { title: "", url: "" };
  filterSourceId.value = resp.id;
  reloadAll();
};

const handleDeleteSource = async (id: number) => {
  const { error } = await fetchRSSDeleteSource(id);
  if (error) {
    message.error(`删除失败: ${(error as any)?.message || "未知错误"}`);
    return;
  }
  message.success("已删除订阅");
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
  message.success(`刷新完成: 新增 ${inserted} 篇${failed ? `, 失败 ${failed} 个源` : ""}`);
  loadItems();
  loadSources();
};

// ===================== 阅读弹窗 (ReaderView 组件: 居中窄栏 + 可调排版 + 点词查词 + j/k 导航) =====================
const showViewer = ref(false);
const viewLoading = ref(false);
const viewItem = ref<RSSItem | null>(null);
const viewContent = ref("");
const viewHtml = ref("");
const viewError = ref("");
const importing = ref(false);
const readerRef = ref<InstanceType<typeof ReaderView> | null>(null);

// feed 正文常自带与文章标题重复的 H1/H2, 阅读视图下去重 (内容已经服务端净化, 这里只删不加)
const stripDuplicateHeading = (html: string, title: string) => {
  if (!html || !title) return html;
  try {
    const doc = new DOMParser().parseFromString(html, "text/html");
    const first = doc.body.querySelector("h1, h2");
    const norm = (s: string) => s.replace(/\s+/g, "").toLowerCase();
    if (first) {
      const heading = norm(first.textContent || "");
      const artTitle = norm(title);
      if (heading && (artTitle.includes(heading) || heading.includes(artTitle))) {
        first.remove();
        return doc.body.innerHTML;
      }
    }
  } catch {
    // 解析失败按原文渲染
  }
  return html;
};

const openItem = async (item: RSSItem) => {
  viewItem.value = item;
  viewContent.value = "";
  viewHtml.value = "";
  viewError.value = "";
  showViewer.value = true;
  readerRef.value?.scrollToTop();
  if (item.status === "unread") {
    item.status = "read";
    // 同步侧栏未读数
    const src = sources.value.find((s) => s.id === item.sourceId);
    if (src && src.unread) src.unread -= 1;
    fetchRSSMarkRead(item.id);
  }
  viewLoading.value = true;
  const { data: resp, error } = await fetchRSSItemContent(item.id);
  viewLoading.value = false;
  if (error || !resp) {
    // 抓取失败: 展示失败原因 + feed 摘要兜底
    viewError.value = (error as any)?.message || "未知错误";
    viewContent.value = item.summary || "";
    return;
  }
  viewContent.value = resp.content;
  viewHtml.value = stripDuplicateHeading(resp.contentHtml || "", resp.title);
  if (!viewContent.value && !viewHtml.value && item.summary) {
    // 原文页没提取到正文 (纯 JS 渲染等): 用 feed 摘要兜底 (后端已随响应附带净化摘要 HTML)
    viewError.value = "原文页未提取到正文";
    viewContent.value = item.summary;
    viewHtml.value = stripDuplicateHeading(resp.contentHtml || "", resp.title);
  }
  viewItem.value = resp;
};

const viewerParagraphs = computed(() =>
  viewContent.value
    .split(/\n\s*\n/)
    .map((p) => p.trim())
    .filter(Boolean)
);

// 预计阅读时长: 中文按 350 字/分, 英文按 200 词/分 (从纯文本口径统计)
const viewStats = computed(() => {
  const text = viewContent.value;
  const latin = text.match(/[A-Za-z]+(?:'[A-Za-z]+)?/g) || [];
  let cjk = 0;
  for (const r of text) {
    if (r >= "\u4e00" && r <= "\u9fff") cjk += 1;
  }
  const minutes = Math.max(1, Math.round(cjk / 350 + latin.length / 200));
  return { charCount: [...text].length, minutes };
});

// j/k 切换上一篇/下一篇 (与 Folo 一致; 不劫持方向键以免干扰滚动)
const viewIndex = computed(() => items.value.findIndex((i) => i.id === viewItem.value?.id));
const prevItem = computed(() => (viewIndex.value > 0 ? items.value[viewIndex.value - 1] : null));
const nextItem = computed(() =>
  viewIndex.value >= 0 && viewIndex.value < items.value.length - 1 ? items.value[viewIndex.value + 1] : null
);

const navTo = (dir: -1 | 1) => {
  const target = dir === -1 ? prevItem.value : nextItem.value;
  if (target) openItem(target);
};

const handleReaderKey = (e: KeyboardEvent) => {
  if (!showViewer.value || viewLoading.value) return;
  const target = e.target as HTMLElement | null;
  if (target && ["INPUT", "TEXTAREA"].includes(target.tagName)) return;
  if (e.key === "j") navTo(1);
  else if (e.key === "k") navTo(-1);
};

async function handleImport(item: RSSItem) {
  importing.value = true;
  const { data: resp, error } = await fetchRSSImportItem(item.id);
  importing.value = false;
  if (error || !resp) {
    message.error("导入失败");
    return;
  }
  message.success(`已导入「${resp.title}」到分级阅读`);
  item.status = "imported";
  item.importedDocId = resp.id;
  if (viewItem.value?.id === item.id) {
    viewItem.value = { ...viewItem.value, status: "imported", importedDocId: resp.id };
  }
}

onMounted(() => {
  reloadAll();
  window.addEventListener("keydown", handleReaderKey);
});

onBeforeUnmount(() => {
  window.removeEventListener("keydown", handleReaderKey);
});

// KeepAlive 缓存下再次激活时刷新; 失活时摘除 j/k 监听 (路由切走但弹窗未关时按键不再后台拉文章)
let hasActivated = false;
onActivated(() => {
  window.addEventListener("keydown", handleReaderKey);
  if (hasActivated) {
    reloadAll();
  } else {
    hasActivated = true;
  }
});

onDeactivated(() => {
  window.removeEventListener("keydown", handleReaderKey);
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
      :title="`订阅源 (${sources.length})`"
    >
      <template #header-extra>
        <NButton size="tiny" quaternary type="primary" @click="showAddModal = true">添加</NButton>
      </template>
      <div class="flex flex-col gap-0.5">
        <div
          class="flex items-center gap-2 rounded px-2 py-1.5 cursor-pointer"
          :class="filterSourceId === null ? 'bg-primary/10' : 'hover:bg-gray-100 dark:hover:bg-gray-800'"
          @click="selectSource(null)"
        >
          <span class="flex-1 min-w-0 text-sm font-bold truncate">全部</span>
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
            <NPopconfirm @positive-click="handleDeleteSource(src.id)">
              <template #trigger>
                <!-- SvgIcon inheritAttrs=false 会吞掉事件监听, 交互触发必须用原生元素包一层 -->
                <span
                  class="inline-flex cursor-pointer text-gray-300 group-hover:text-error"
                  title="删除订阅"
                >
                  <SvgIcon icon="mdi:delete-outline" class="text-14px" />
                </span>
              </template>
              删除「{{ src.title }}」及其全部文章?
            </NPopconfirm>
          </span>
        </div>
        <NEmpty v-if="sources.length === 0" size="small" description="暂无订阅" class="py-4" />
      </div>
    </NCard>

    <!-- 主区: 工具栏 + Folo 时间线文章列表 -->
    <NCard :bordered="false" :shadow="appStore.isMobile ? false : 'sm'" class="flex-1 min-w-0">
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-2 w-full">
          <span class="font-bold text-18px">{{ filterSourceId ? sourceTitle(filterSourceId) : "全部文章" }}</span>
          <div class="flex flex-wrap gap-2">
            <div class="flex items-center gap-2 mr-2">
              <span class="text-xs text-gray-400">仅未读</span>
              <NSwitch
                v-model:value="unreadOnly"
                size="small"
                @update:value="() => { pagination.page = 1; loadItems(); }"
              />
            </div>
            <NButton
              size="small"
              :title="timeAsc ? '发布时间正序' : '发布时间倒序'"
              @click="() => { timeAsc = !timeAsc; pagination.page = 1; loadItems(); }"
            >
              <template #icon>
                <SvgIcon :icon="timeAsc ? 'mdi:sort-ascending' : 'mdi:sort-descending'" class="text-icon" />
              </template>
              时间{{ timeAsc ? '正序' : '倒序' }}
            </NButton>
            <NButton size="small" :loading="fetchingAll" @click="handleFetchAll">全部刷新</NButton>
            <NButton size="small" @click="showAddModal = true">添加订阅</NButton>
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
          全部
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
                <NTag v-if="row.status === 'imported'" size="tiny" type="success" :bordered="false" class="shrink-0">
                  已导入
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
                去标注
              </NButton>
              <NButton v-else size="tiny" quaternary type="info" @click="handleImport(row)">导入阅读</NButton>
            </div>
          </div>
        </div>
        <NEmpty v-if="items.length === 0 && !loading" class="py-10" description="暂无文章, 从左侧添加订阅开始" />
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
    <NModal v-model:show="showAddModal" preset="card" title="添加订阅" :style="{ width: appStore.isMobile ? '92%' : '460px' }">
      <div class="flex flex-col gap-4 py-2">
        <NInput v-model:value="addForm.url" placeholder="RSS/Atom feed 地址 (如自部署 RSSHub 的路由)" @keyup.enter="handleAdd" />
        <NInput v-model:value="addForm.title" placeholder="自定义名称 (可选, 默认取 feed 标题)" />
        <NButton type="primary" block :loading="adding" @click="handleAdd">添加</NButton>
        <div class="text-xs text-gray-400">
          添加时会先验证 feed 可解析, 成功后自动抓取最近的文章。
          抓正文遇反爬盾时, 可在系统配置里设置 flaresolverr_url 走无头浏览器回退。
        </div>
      </div>
    </NModal>

    <!-- 阅读视图 (ReaderView 组件: 居中窄栏 + 大标题 + 可调排版 + 点词查词 + j/k 导航) -->
    <NModal v-model:show="showViewer" :auto-focus="false">
      <div class="w-full h-full md:w-[min(94vw,960px)] md:h-[88vh] flex flex-col rounded-none md:rounded-xl bg-white dark:bg-[#18181c] shadow-2xl overflow-hidden">
        <ReaderView ref="readerRef" :title="viewItem?.title || ''" :loading="viewLoading">
          <template #toolbar-left>
            <span
              class="w-5 h-5 rounded-full text-white text-11px font-medium flex items-center justify-center shrink-0"
              :style="{ background: avatarColor(viewItem?.sourceId || 0) }"
            >
              {{ sourceInitial(viewItem?.sourceId || 0) }}
            </span>
            <span class="text-13px text-gray-500 dark:text-gray-400 truncate min-w-0">
              {{ viewItem ? sourceTitle(viewItem.sourceId) : '' }}
            </span>
            <template v-if="viewItem?.publishedAt">
              <span class="text-gray-300 dark:text-gray-600">·</span>
              <span class="text-13px text-gray-400 shrink-0">{{ relTime(viewItem.publishedAt) }}</span>
            </template>
            <template v-if="!viewLoading && viewStats.charCount > 0">
              <span class="text-gray-300 dark:text-gray-600">·</span>
              <span class="text-13px text-gray-400 shrink-0">约 {{ viewStats.minutes }} 分钟</span>
            </template>
          </template>
          <template #toolbar-actions>
            <a v-if="viewItem?.link" :href="viewItem.link" target="_blank" rel="noopener noreferrer">
              <NButton quaternary circle size="small" title="打开原文">
                <template #icon>
                  <SvgIcon icon="mdi:open-in-new" />
                </template>
              </NButton>
            </a>
            <NButton
              v-if="viewItem && viewItem.status !== 'imported'"
              quaternary
              circle
              size="small"
              title="导入分级阅读"
              :loading="importing"
              @click="handleImport(viewItem)"
            >
              <template #icon>
                <SvgIcon icon="mdi:import" />
              </template>
            </NButton>
            <NButton
              v-else-if="viewItem?.importedDocId"
              quaternary
              circle
              size="small"
              title="去标注"
              @click="routerPushByKey('ai_reading-detail', { params: { id: String(viewItem.importedDocId) } })"
            >
              <template #icon>
                <SvgIcon icon="mdi:pencil-outline" />
              </template>
            </NButton>
            <NButton quaternary circle size="small" title="关闭 (Esc)" @click="showViewer = false">
              <template #icon>
                <SvgIcon icon="mdi:close" />
              </template>
            </NButton>
          </template>
          <template #meta>
            <span v-if="viewItem?.publishedAt">{{ new Date(viewItem.publishedAt).toLocaleString() }}</span>
            <NTag v-if="viewItem?.status === 'imported'" size="tiny" type="success" :bordered="false">已导入</NTag>
            <span class="hidden md:inline text-gray-300 dark:text-gray-600">
              j / k 键切换上一篇、下一篇; 点击英文单词可查词
            </span>
          </template>

          <NAlert v-if="viewError" type="warning" :bordered="false" class="my-5">
            正文抓取失败: {{ viewError }}<template v-if="viewHtml || viewerParagraphs.length">（以下为 feed 摘要）</template>
          </NAlert>
          <!-- 净化 HTML 渲染 (服务端白名单过滤, 含图片/链接/表格) -->
          <!-- eslint-disable-next-line vue/no-v-html -- 内容为服务端白名单净化后的 HTML -->
          <div v-if="viewHtml" class="article-html" v-html="viewHtml"></div>
          <div v-else class="flex flex-col">
            <p v-for="(p, i) in viewerParagraphs" :key="i">{{ p }}</p>
            <NEmpty
              v-if="!viewLoading && viewerParagraphs.length === 0"
              description="未抓到正文且无摘要, 可打开原文链接查看"
            />
          </div>

          <template #footer>
            <!-- 上一篇 / 下一篇 -->
            <div class="flex items-center justify-between gap-4 mt-14 pt-6 border-t border-gray-100 dark:border-gray-800">
              <NButton
                v-if="prevItem"
                size="small"
                quaternary
                class="max-w-[45%]"
                :title="prevItem.title"
                @click="navTo(-1)"
              >
                ← <span class="truncate ml-1">{{ prevItem.title }}</span>
              </NButton>
              <span v-else class="text-13px text-gray-300 dark:text-gray-600">已是最新一篇</span>
              <NButton
                v-if="nextItem"
                size="small"
                quaternary
                class="max-w-[45%]"
                :title="nextItem.title"
                @click="navTo(1)"
              >
                <span class="truncate mr-1">{{ nextItem.title }}</span> →
              </NButton>
              <span v-else class="text-13px text-gray-300 dark:text-gray-600">已是最后一篇</span>
            </div>
          </template>
        </ReaderView>
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

/* 净化 HTML 正文的富文本元素排版 (v-html 注入的节点须用 :deep);
   基础排版 (字号/行高/颜色) 由 ReaderView 的 .reader-content 提供, 字号以 em 相对单位随 A-/A+ 缩放 */
.article-html > :first-child {
  margin-top: 0;
}
.article-html :deep(p) {
  margin: 0 0 1.5em;
}
.article-html :deep(p:empty) {
  display: none;
}
.article-html :deep(img) {
  display: block;
  max-width: 100%;
  height: auto;
  border-radius: 12px;
  margin: 0.35em auto 1.5em;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.08);
}
.article-html :deep(figcaption) {
  text-align: center;
  font-size: 0.8em;
  line-height: 1.6;
  color: #9ca3af;
  margin: -0.75em 0 1.5em;
}
.article-html :deep(a) {
  color: var(--primary-color);
  text-decoration: underline;
  text-underline-offset: 3px;
}
.article-html :deep(a:hover) {
  opacity: 0.8;
}
.article-html :deep(h1),
.article-html :deep(h2),
.article-html :deep(h3),
.article-html :deep(h4),
.article-html :deep(h5),
.article-html :deep(h6) {
  font-weight: 700;
  line-height: 1.45;
  color: #1f2937;
}
.dark .article-html :deep(h1),
.dark .article-html :deep(h2),
.dark .article-html :deep(h3),
.dark .article-html :deep(h4),
.dark .article-html :deep(h5),
.dark .article-html :deep(h6) {
  color: #f3f4f6;
}
.article-html :deep(h1) {
  font-size: 1.6em;
  margin: 1.8em 0 0.75em;
}
.article-html :deep(h2) {
  font-size: 1.35em;
  margin: 1.7em 0 0.7em;
}
.article-html :deep(h3) {
  font-size: 1.18em;
  margin: 1.5em 0 0.65em;
}
.article-html :deep(h4) {
  font-size: 1.05em;
  margin: 1.4em 0 0.6em;
}
.article-html :deep(h5),
.article-html :deep(h6) {
  font-size: 1em;
  margin: 1.4em 0 0.6em;
}
.article-html :deep(blockquote) {
  border-left: 3px solid var(--primary-color);
  background: rgba(128, 128, 128, 0.08);
  border-radius: 0 8px 8px 0;
  padding: 0.6em 1.2em;
  margin: 0 0 1.5em;
  color: #6b7280;
}
.dark .article-html :deep(blockquote) {
  background: rgba(255, 255, 255, 0.06);
  color: #9ca3af;
}
.article-html :deep(blockquote p:last-child) {
  margin-bottom: 0;
}
.article-html :deep(ul) {
  list-style: disc;
  padding-left: 1.6em;
  margin: 0 0 1.5em;
}
.article-html :deep(ol) {
  list-style: decimal;
  padding-left: 1.6em;
  margin: 0 0 1.5em;
}
.article-html :deep(li) {
  margin-bottom: 0.4em;
}
.article-html :deep(pre) {
  background: rgba(128, 128, 128, 0.12);
  padding: 1.2em;
  border-radius: 10px;
  overflow: auto;
  font-size: 0.85em;
  line-height: 1.7;
  margin: 0 0 1.5em;
  white-space: pre-wrap;
  word-break: break-all;
}
.dark .article-html :deep(pre) {
  background: rgba(255, 255, 255, 0.06);
}
.article-html :deep(code) {
  font-family: Menlo, Consolas, monospace;
  font-size: 0.875em;
  background: rgba(128, 128, 128, 0.12);
  padding: 0.15em 0.45em;
  border-radius: 5px;
}
.dark .article-html :deep(code) {
  background: rgba(255, 255, 255, 0.1);
}
.article-html :deep(pre code) {
  background: transparent;
  padding: 0;
  font-size: inherit;
}
.article-html :deep(kbd) {
  font-family: Menlo, Consolas, monospace;
  font-size: 0.85em;
  border: 1px solid rgba(128, 128, 128, 0.4);
  border-bottom-width: 2px;
  border-radius: 4px;
  padding: 0.1em 0.4em;
  background: rgba(128, 128, 128, 0.08);
}
.article-html :deep(mark) {
  background: #fde68a;
  color: inherit;
  padding: 0 0.15em;
  border-radius: 3px;
}
.dark .article-html :deep(mark) {
  background: rgba(250, 204, 21, 0.28);
}
.article-html :deep(hr) {
  border: none;
  border-top: 1px solid rgba(128, 128, 128, 0.25);
  margin: 2.5em 0;
}
.article-html :deep(table) {
  border-collapse: collapse;
  margin: 0 0 1.5em;
  width: 100%;
  font-size: 0.9em;
  display: block;
  overflow-x: auto;
}
.article-html :deep(th),
.article-html :deep(td) {
  border: 1px solid rgba(128, 128, 128, 0.3);
  padding: 0.5em 0.8em;
  text-align: left;
}
.article-html :deep(th) {
  font-weight: 600;
  background: rgba(128, 128, 128, 0.08);
}
.article-html :deep(tbody tr:nth-child(2n)) {
  background: rgba(128, 128, 128, 0.05);
}
.dark .article-html :deep(tbody tr:nth-child(2n)) {
  background: rgba(255, 255, 255, 0.04);
}
.article-html :deep(del),
.article-html :deep(s) {
  opacity: 0.65;
}
</style>
