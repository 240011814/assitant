<script setup lang="ts">
import { computed, h, onActivated, onMounted, ref } from "vue";
import { NButton, NPopconfirm, NTag, useMessage } from "naive-ui";
import type { DataTableColumns } from "naive-ui";
import { useRouterPush } from "@/hooks/common/router";
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

const statusTag = (status: string) => {
  switch (status) {
    case "unread":
      return { type: "warning" as const, label: "未读" };
    case "imported":
      return { type: "success" as const, label: "已导入" };
    default:
      return { type: "default" as const, label: "已读" };
  }
};

const columns = ref<DataTableColumns<RSSItem>>([
  {
    title: "标题",
    key: "title",
    minWidth: 280,
    render(row) {
      return h(
        "a",
        {
          class: "cursor-pointer font-bold",
          style: row.status === "unread" ? "color: var(--primary-color)" : "",
          // eslint-disable-next-line @typescript-eslint/no-use-before-define
          onClick: () => openItem(row)
        },
        row.title
      );
    }
  },
  {
    title: "源",
    key: "sourceId",
    width: 140,
    ellipsis: { tooltip: true },
    render(row) {
      return h("span", { class: "text-xs text-gray-400" }, sourceTitle(row.sourceId));
    }
  },
  {
    title: "状态",
    key: "status",
    width: 90,
    render(row) {
      const s = statusTag(row.status);
      return h(NTag, { size: "small", type: s.type, bordered: false }, { default: () => s.label });
    }
  },
  {
    title: "发布时间",
    key: "publishedAt",
    width: 160,
    render(row) {
      return row.publishedAt ? new Date(row.publishedAt).toLocaleString() : "-";
    }
  },
  {
    title: "操作",
    key: "actions",
    width: 130,
    render(row) {
      if (row.status === "imported") {
        return h(
          NButton,
          { size: "small", quaternary: true, type: "primary", onClick: () => routerPushByKey("ai_reading-detail", { params: { id: String(row.importedDocId) } }) },
          { default: () => "去标注" }
        );
      }
      return h(
        NButton,
        { size: "small", quaternary: true, type: "info", onClick: () => handleImport(row) },
        { default: () => "导入阅读" }
      );
    }
  }
]);

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

// ===================== 阅读弹窗 =====================
const showViewer = ref(false);
const viewLoading = ref(false);
const viewItem = ref<RSSItem | null>(null);
const viewContent = ref("");
const viewHtml = ref("");
const viewError = ref("");
const importing = ref(false);

const openItem = async (item: RSSItem) => {
  viewItem.value = item;
  viewContent.value = "";
  viewHtml.value = "";
  viewError.value = "";
  showViewer.value = true;
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
  viewHtml.value = resp.contentHtml || "";
  if (!viewContent.value && !viewHtml.value && item.summary) {
    // 原文页没提取到正文 (纯 JS 渲染等): 用 feed 摘要兜底 (后端已随响应附带净化摘要 HTML)
    viewError.value = "原文页未提取到正文";
    viewContent.value = item.summary;
    viewHtml.value = resp.contentHtml || "";
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
});

// KeepAlive 缓存下再次激活时刷新
let hasActivated = false;
onActivated(() => {
  if (hasActivated) {
    reloadAll();
  } else {
    hasActivated = true;
  }
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
      <div class="flex flex-col gap-1">
        <div
          class="flex items-center justify-between rounded px-2 py-1.5 cursor-pointer"
          :class="filterSourceId === null ? 'bg-primary/10' : 'hover:bg-gray-100 dark:hover:bg-gray-800'"
          @click="selectSource(null)"
        >
          <span class="text-sm font-bold">全部</span>
          <NTag v-if="totalUnread > 0" size="tiny" type="warning" :bordered="false">{{ totalUnread }}</NTag>
        </div>
        <div
          v-for="src in sources"
          :key="src.id"
          class="group flex items-center justify-between rounded px-2 py-1.5"
          :class="filterSourceId === src.id ? 'bg-primary/10' : 'hover:bg-gray-100 dark:hover:bg-gray-800'"
        >
          <!-- 切换只绑在标题上: 行内操作按钮永远不会误触发切换 -->
          <span class="text-sm truncate flex-1 cursor-pointer" :title="src.title" @click="selectSource(src.id)">
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

    <!-- 主区: 工具栏 + 文章列表 -->
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

      <!-- PC: DataTable -->
      <NDataTable
        v-if="!appStore.isMobile"
        :columns="columns"
        :data="items"
        :loading="loading"
        remote
        :pagination="pagination"
        :row-key="(row: any) => row.id"
        size="small"
        striped
      />

      <!-- Mobile: 卡片列表 -->
      <NSpin v-if="appStore.isMobile && loading" class="flex justify-center py-8" />
      <div v-else-if="appStore.isMobile" class="flex flex-col gap-3">
        <NCard v-for="row in items" :key="row.id" size="small" :bordered="true">
          <div class="flex flex-col gap-2">
            <div class="flex items-center justify-between gap-2">
              <NTag size="tiny" :type="statusTag(row.status).type" :bordered="false">
                {{ statusTag(row.status).label }}
              </NTag>
              <span class="text-xs text-gray-400 truncate">{{ sourceTitle(row.sourceId) }}</span>
            </div>
            <a class="font-bold cursor-pointer" @click="openItem(row)">{{ row.title }}</a>
            <div v-if="row.summary" class="text-xs text-gray-400 line-clamp-2">{{ row.summary }}</div>
            <div class="flex items-center justify-between">
              <span class="text-xs text-gray-400">{{ row.publishedAt ? new Date(row.publishedAt).toLocaleDateString() : '' }}</span>
              <NButton size="tiny" quaternary type="info" @click="handleImport(row)">
                {{ row.status === 'imported' ? '已导入' : '导入阅读' }}
              </NButton>
            </div>
          </div>
        </NCard>
        <NEmpty v-if="items.length === 0 && !loading" class="py-8" />
        <NPagination
          v-if="pagination.itemCount > pagination.pageSize"
          class="mt-2 justify-center"
          :page="pagination.page"
          :item-count="pagination.itemCount"
          :page-size="pagination.pageSize"
          :page-slot="5"
          @update:page="(p: number) => pagination.onChange(p)"
        />
      </div>

      <NEmpty v-if="!appStore.isMobile && items.length === 0 && !loading" class="py-10" description="暂无文章, 从左侧添加订阅开始" />
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

    <!-- 阅读视图 (借鉴 Miniflux: 无干扰阅读列, 大标题 + 元信息行) -->
    <NModal v-model:show="showViewer" :auto-focus="false">
      <div class="w-[94%] md:w-[780px] max-h-[88vh] flex flex-col rounded-lg bg-white dark:bg-[#18181c] shadow-2xl overflow-hidden">
        <div class="flex items-center justify-between gap-3 px-7 pt-5 shrink-0">
          <div class="flex items-center gap-1.5 text-xs text-gray-400 truncate min-w-0">
            <span class="truncate">{{ viewItem ? sourceTitle(viewItem.sourceId) : '' }}</span>
            <template v-if="viewItem?.publishedAt">
              <span>·</span>
              <span class="shrink-0">{{ new Date(viewItem.publishedAt).toLocaleString() }}</span>
            </template>
            <template v-if="!viewLoading && viewStats.charCount > 0">
              <span>·</span>
              <span class="shrink-0">约 {{ viewStats.minutes }} 分钟读完</span>
            </template>
          </div>
          <NButton quaternary circle size="small" @click="showViewer = false">
            <template #icon>
              <SvgIcon icon="mdi:close" />
            </template>
          </NButton>
        </div>
        <div class="flex-1 overflow-auto px-7 pb-8 pt-4">
          <h1 class="text-22px font-bold leading-snug mb-4">{{ viewItem?.title }}</h1>
          <div class="flex flex-wrap items-center gap-2 mb-5 pb-4 border-b border-gray-100 dark:border-gray-800">
            <a v-if="viewItem?.link" :href="viewItem.link" target="_blank" rel="noopener noreferrer">
              <NButton size="small" quaternary>打开原文</NButton>
            </a>
            <NButton
              v-if="viewItem && viewItem.status !== 'imported'"
              size="small"
              type="primary"
              :loading="importing"
              @click="handleImport(viewItem)"
            >
              导入分级阅读
            </NButton>
            <NButton
              v-else-if="viewItem?.importedDocId"
              size="small"
              quaternary
              type="primary"
              @click="routerPushByKey('ai_reading-detail', { params: { id: String(viewItem.importedDocId) } })"
            >
              去标注
            </NButton>
          </div>
          <NSpin :show="viewLoading">
            <NAlert v-if="viewError" type="warning" :bordered="false" class="mb-5">
              正文抓取失败: {{ viewError }}<template v-if="viewHtml || viewerParagraphs.length">（以下为 feed 摘要）</template>
            </NAlert>
            <!-- 净化 HTML 渲染 (服务端白名单过滤, 含图片/链接/表格) -->
            <!-- eslint-disable-next-line vue/no-v-html -- 内容为服务端白名单净化后的 HTML -->
            <div v-if="viewHtml" class="article-html" v-html="viewHtml"></div>
            <div v-else class="flex flex-col gap-5 text-17px leading-[2] text-gray-700 dark:text-gray-300">
              <p v-for="(p, i) in viewerParagraphs" :key="i">{{ p }}</p>
              <NEmpty
                v-if="!viewLoading && viewerParagraphs.length === 0"
                description="未抓到正文且无摘要, 可打开原文链接查看"
              />
            </div>
          </NSpin>
        </div>
      </div>
    </NModal>
  </div>
</template>

<style scoped>
.line-clamp-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* 净化 HTML 正文的阅读排版 (v-html 注入的节点须用 :deep) */
.article-html {
  font-size: 17px;
  line-height: 2;
  color: #374151;
  word-break: break-word;
}
.dark .article-html {
  color: #d1d5db;
}
.article-html :deep(p) {
  margin: 0 0 1.25em;
}
.article-html :deep(p:empty) {
  display: none;
}
.article-html :deep(img) {
  max-width: 100%;
  height: auto;
  border-radius: 6px;
  margin: 0.5em 0;
}
.article-html :deep(a) {
  color: var(--primary-color);
  text-decoration: underline;
  text-underline-offset: 2px;
}
.article-html :deep(h1),
.article-html :deep(h2),
.article-html :deep(h3),
.article-html :deep(h4) {
  font-size: 20px;
  font-weight: 700;
  line-height: 1.5;
  margin: 1.6em 0 0.8em;
}
.article-html :deep(h5),
.article-html :deep(h6) {
  font-size: 17px;
  font-weight: 700;
  margin: 1.4em 0 0.7em;
}
.article-html :deep(blockquote) {
  border-left: 3px solid rgba(128, 128, 128, 0.35);
  padding-left: 1em;
  margin: 0 0 1.25em;
  color: #6b7280;
}
.dark .article-html :deep(blockquote) {
  color: #9ca3af;
}
.article-html :deep(ul) {
  list-style: disc;
  padding-left: 1.6em;
  margin: 0 0 1.25em;
}
.article-html :deep(ol) {
  list-style: decimal;
  padding-left: 1.6em;
  margin: 0 0 1.25em;
}
.article-html :deep(li) {
  margin-bottom: 0.35em;
}
.article-html :deep(pre) {
  background: rgba(128, 128, 128, 0.12);
  padding: 1em;
  border-radius: 8px;
  overflow: auto;
  font-size: 13px;
  line-height: 1.6;
  margin: 0 0 1.25em;
  white-space: pre-wrap;
  word-break: break-all;
}
.article-html :deep(code) {
  font-family: Menlo, Consolas, monospace;
  font-size: 13px;
  background: rgba(128, 128, 128, 0.12);
  padding: 0.1em 0.35em;
  border-radius: 4px;
}
.article-html :deep(pre code) {
  background: transparent;
  padding: 0;
}
.article-html :deep(hr) {
  border: none;
  border-top: 1px solid rgba(128, 128, 128, 0.25);
  margin: 2em 0;
}
.article-html :deep(table) {
  border-collapse: collapse;
  margin: 0 0 1.25em;
  width: 100%;
  font-size: 15px;
  display: block;
  overflow-x: auto;
}
.article-html :deep(th),
.article-html :deep(td) {
  border: 1px solid rgba(128, 128, 128, 0.3);
  padding: 0.4em 0.6em;
  text-align: left;
}
.article-html :deep(figcaption) {
  font-size: 13px;
  color: #9ca3af;
  margin-top: 0.4em;
}
</style>
