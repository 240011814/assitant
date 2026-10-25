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
const refreshingId = ref<number | null>(null);

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
    message.error("删除失败");
    return;
  }
  reloadAll();
};

const handleFetchOne = async (source: RSSSource) => {
  refreshingId.value = source.id;
  const { data: resp, error } = await fetchRSSFetchSource(source.id);
  refreshingId.value = null;
  if (error) {
    message.error(`「${source.title}」抓取失败`);
    return;
  }
  message.success(`「${source.title}」新增 ${resp?.inserted ?? 0} 篇`);
  loadSources();
  loadItems();
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
const importing = ref(false);

const openItem = async (item: RSSItem) => {
  viewItem.value = item;
  viewContent.value = "";
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
    message.error("抓取正文失败, 可打开原文链接查看");
    return;
  }
  viewContent.value = resp.content;
  viewItem.value = resp;
};

const viewerParagraphs = computed(() =>
  viewContent.value
    .split(/\n\s*\n/)
    .map((p) => p.trim())
    .filter(Boolean)
);

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
          class="group flex items-center justify-between rounded px-2 py-1.5 cursor-pointer"
          :class="filterSourceId === src.id ? 'bg-primary/10' : 'hover:bg-gray-100 dark:hover:bg-gray-800'"
          @click="selectSource(src.id)"
        >
          <span class="text-sm truncate flex-1" :title="src.title">{{ src.title }}</span>
          <!-- 原生 span 拦截冒泡: 操作按钮不触发行的切换 (SvgIcon 组件可能不透传事件修饰符) -->
          <span class="flex items-center gap-1 shrink-0" @click.stop>
            <NTag v-if="(src.unread || 0) > 0" size="tiny" type="warning" :bordered="false">{{ src.unread }}</NTag>
            <SvgIcon
              icon="mdi:refresh"
              class="text-14px cursor-pointer text-gray-300 group-hover:text-primary"
              :class="{ 'animate-spin text-primary': refreshingId === src.id }"
              @click="handleFetchOne(src)"
            />
            <NPopconfirm @positive-click="handleDeleteSource(src.id)">
              <template #trigger>
                <SvgIcon
                  icon="mdi:delete-outline"
                  class="text-14px cursor-pointer text-gray-300 group-hover:text-error"
                />
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

    <!-- 阅读弹窗 -->
    <NModal v-model:show="showViewer" preset="card" :title="viewItem?.title" :style="{ width: appStore.isMobile ? '96%' : '760px' }">
      <template #header-extra>
        <div class="flex gap-2">
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
            type="primary"
            quaternary
            @click="routerPushByKey('ai_reading-detail', { params: { id: String(viewItem.importedDocId) } })"
          >
            去标注
          </NButton>
        </div>
      </template>
      <NSpin :show="viewLoading">
        <div class="text-xs text-gray-400 mb-3">
          {{ viewItem ? sourceTitle(viewItem.sourceId) : '' }}
          <template v-if="viewItem?.publishedAt"> · {{ new Date(viewItem.publishedAt).toLocaleString() }}</template>
        </div>
        <div class="flex flex-col gap-3 leading-8 text-16px max-h-[65vh] overflow-auto">
          <p v-for="(p, i) in viewerParagraphs" :key="i" class="text-justify">{{ p }}</p>
          <NEmpty v-if="!viewLoading && viewerParagraphs.length === 0" description="未抓到正文, 可打开原文链接查看" />
        </div>
      </NSpin>
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
</style>
