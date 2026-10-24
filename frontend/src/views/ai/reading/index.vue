<script setup lang="ts">
import { h, onMounted, ref } from "vue";
import { NButton, NPopconfirm, NTag, useMessage } from "naive-ui";
import type { DataTableColumns } from "naive-ui";
import { useRouterPush } from "@/hooks/common/router";
import {
  fetchImportReading,
  fetchGetReadingDocs,
  fetchDeleteReadingDoc,
} from "@/service/api";
import { useAppStore } from "@/store/modules/app";

const appStore = useAppStore();
const message = useMessage();
const { routerPushByKey } = useRouterPush();

const loading = ref(false);
const docs = ref<any[]>([]);
const keyword = ref("");

// ===================== 常用网站收藏 (rss 为实测可用的 feed 地址) =====================
interface FavoriteSite {
  name: string;
  url: string;
  note: string;
  rss?: string;
  group: "分级新闻" | "真实语料";
}

const favoriteSites: FavoriteSite[] = [
  { name: "News in Levels", url: "https://www.newsinlevels.com", note: "同一新闻 3 级难度 + 音频", rss: "https://www.newsinlevels.com/feed/", group: "分级新闻" },
  { name: "VOA Learning English", url: "https://learningenglish.voanews.com", note: "词汇受控简明新闻, RSS 按栏目", rss: "https://learningenglish.voanews.com/rssfeeds", group: "分级新闻" },
  { name: "Breaking News English", url: "https://www.breakingnewsenglish.com", note: "同一新闻 7 个难度 (有反爬, URL 导入待实测)", group: "分级新闻" },
  { name: "ESL News Stories", url: "https://www.eslnewsstories.com", note: "A1~B2 分级短文", group: "分级新闻" },
  { name: "British Council LearnEnglish", url: "https://learnenglish.britishcouncil.org", note: "B1~B2, RSS 已退役", group: "分级新闻" },
  { name: "The Guardian", url: "https://www.theguardian.com", note: "免费综合大报, 版块加 /rss", rss: "https://www.theguardian.com/uk/rss", group: "真实语料" },
  { name: "Aeon", url: "https://aeon.co", note: "思想类长文", rss: "https://aeon.co/feed.rss", group: "真实语料" },
  { name: "Hacker News", url: "https://news.ycombinator.com", note: "科技资讯", rss: "https://hnrss.org/frontpage", group: "真实语料" },
  { name: "Ars Technica", url: "https://arstechnica.com", note: "科技深度报道", rss: "https://feeds.arstechnica.com/arstechnica/index", group: "真实语料" },
  { name: "Project Gutenberg", url: "https://www.gutenberg.org", note: "公版书, 建议整章文本导入", group: "真实语料" },
];

const siteGroups = ["分级新闻", "真实语料"] as const;
const sitesOf = (group: string) => favoriteSites.filter((s) => s.group === group);

const copyRss = async (url: string) => {
  try {
    await navigator.clipboard.writeText(url);
    message.success("已复制 RSS 地址");
  } catch {
    message.error("复制失败");
  }
};

// 导入
const showImportModal = ref(false);
const importTitle = ref("");
const importType = ref<"url" | "text">("url");
const importUrl = ref("");
const importText = ref("");
const importing = ref(false);

const statusTag = (status: string) => {
  switch (status) {
    case "annotated":
      return { type: "success" as const, label: "已标注" };
    case "annotating":
      return { type: "info" as const, label: "标注中" };
    case "failed":
      return { type: "error" as const, label: "标注失败" };
    default:
      return { type: "default" as const, label: "待标注" };
  }
};

const columns = ref<DataTableColumns<any>>([
  {
    title: "标题",
    key: "title",
    minWidth: 200,
    render(row) {
      return h(
        "a",
        {
          class: "text-primary cursor-pointer font-bold",
          onClick: () => routerPushByKey("ai_reading-detail", { params: { id: String(row.id) } }),
        },
        row.title
      );
    },
  },
  {
    title: "来源",
    key: "sourceType",
    width: 90,
    render: (row) => (row.sourceType === "url" ? "网页" : "文本"),
  },
  { title: "词数", key: "wordCount", width: 90 },
  {
    title: "生词率",
    key: "unknownRate",
    width: 110,
    render(row) {
      if (row.status !== "annotated") return "—";
      const pct = Math.round(row.unknownRate * 1000) / 10;
      const type = pct < 5 ? "success" : pct < 10 ? "warning" : "error";
      return h(NTag, { size: "small", type, bordered: false }, { default: () => `${pct}%` });
    },
  },
  {
    title: "状态",
    key: "status",
    width: 100,
    render(row) {
      const s = statusTag(row.status);
      return h(NTag, { size: "small", type: s.type, bordered: false }, { default: () => s.label });
    },
  },
  {
    title: "操作",
    key: "actions",
    width: 150,
    render(row) {
      return h("div", { class: "flex gap-1" }, [
        h(
          NButton,
          { size: "small", quaternary: true, type: "primary", onClick: () => routerPushByKey("ai_reading-detail", { params: { id: String(row.id) } }) },
          { default: () => "阅读" }
        ),
        h(
          NPopconfirm,
          {
            // eslint-disable-next-line @typescript-eslint/no-use-before-define
            onPositiveClick: () => handleDelete(row.id)
          },
          {
            trigger: () => h(NButton, { size: "small", quaternary: true, type: "error" }, { default: () => "删除" }),
            default: () => "确认删除该文档?",
          }
        ),
      ]);
    },
  },
]);

const loadData = async () => {
  loading.value = true;
  const { data: resp, error } = await fetchGetReadingDocs({ keyword: keyword.value.trim() || undefined });
  loading.value = false;
  if (error) {
    message.error("获取文档列表失败");
    return;
  }
  docs.value = resp || [];
};

const handleImport = async () => {
  if (importType.value === "url" && !importUrl.value.trim()) {
    message.warning("请输入 URL");
    return;
  }
  if (importType.value === "text" && !importText.value.trim()) {
    message.warning("请输入正文");
    return;
  }
  importing.value = true;
  const { data: resp, error } = await fetchImportReading({
    title: importTitle.value,
    sourceType: importType.value,
    sourceUrl: importType.value === "url" ? importUrl.value : undefined,
    content: importType.value === "text" ? importText.value : undefined,
  });
  importing.value = false;
  if (error || !resp) {
    message.error("导入失败");
    return;
  }
  message.success("导入成功, 进入详情页可一键标注生词");
  showImportModal.value = false;
  importTitle.value = "";
  importUrl.value = "";
  importText.value = "";
  loadData();
};

const handleDelete = async (id: number) => {
  const { error } = await fetchDeleteReadingDoc(id);
  if (error) {
    message.error("删除失败");
    return;
  }
  loadData();
};

onMounted(() => {
  loadData();
});
</script>

<template>
  <div class="h-full flex-col flex gap-4" :class="appStore.isMobile ? 'p-2' : 'p-4'">
    <NCard :bordered="false" :shadow="appStore.isMobile ? false : 'sm'" class="flex-1">
      <template #header>
        <div class="flex items-center justify-between w-full">
          <span class="font-bold text-18px">分级阅读</span>
          <div class="flex gap-2">
            <NPopover trigger="click" placement="bottom-end" :width="460">
              <template #trigger>
                <NButton>常用网站</NButton>
              </template>
              <div class="max-h-[60vh] overflow-auto flex flex-col gap-3">
                <div v-for="group in siteGroups" :key="group">
                  <div class="text-xs font-bold text-gray-400 mb-1">{{ group }}</div>
                  <div
                    v-for="site in sitesOf(group)"
                    :key="site.name"
                    class="flex items-start justify-between gap-2 py-1.5 border-b border-gray-100 dark:border-gray-800 last:border-b-0"
                  >
                    <div>
                      <a :href="site.url" target="_blank" rel="noopener noreferrer" class="text-primary font-bold text-sm">
                        {{ site.name }}
                      </a>
                      <div class="text-xs text-gray-400">{{ site.note }}</div>
                    </div>
                    <NTag
                      v-if="site.rss"
                      size="tiny"
                      type="success"
                      :bordered="false"
                      class="cursor-pointer shrink-0 mt-0.5"
                      title="点击复制 RSS 地址"
                      @click="copyRss(site.rss)"
                    >
                      RSS
                    </NTag>
                    <NTag v-else size="tiny" :bordered="false" class="shrink-0 mt-0.5">无 RSS</NTag>
                  </div>
                </div>
                <div class="text-xs text-gray-300">点名称打开网站; 带 RSS 标记的可点击复制 feed 地址。</div>
              </div>
            </NPopover>
            <NButton type="primary" @click="showImportModal = true">导入文章</NButton>
          </div>
        </div>
      </template>
      <template #header-extra>
        <span class="text-xs text-gray-400">生词率 &lt;5% 适合泛读, 5%~10% 适合精读; AI 写课文在课程包页</span>
      </template>

      <div class="flex gap-2 mb-4" :class="appStore.isMobile ? '' : 'w-96'">
        <NInput
          v-model:value="keyword"
          placeholder="搜索标题或正文关键词"
          clearable
          @keyup.enter="loadData"
          @clear="loadData"
        />
        <NButton type="primary" @click="loadData">
          <template #icon>
            <IconMdiMagnify class="text-icon" />
          </template>
        </NButton>
      </div>

      <NDataTable :columns="columns" :data="docs" :loading="loading" :row-key="(row: any) => row.id" />
    </NCard>

    <!-- 导入 -->
    <NModal v-model:show="showImportModal" preset="card" title="导入阅读材料" :style="{ width: appStore.isMobile ? '92%' : '560px' }">
      <div class="flex flex-col gap-4 py-2">
        <NTabs v-model:value="importType" type="segment">
          <NTab name="url">网页 URL</NTab>
          <NTab name="text">粘贴文本</NTab>
        </NTabs>
        <NInput v-model:value="importTitle" placeholder="标题 (可选)" />
        <NInput v-if="importType === 'url'" v-model:value="importUrl" placeholder="https://example.com/article" />
        <NInput
          v-else
          v-model:value="importText"
          type="textarea"
          placeholder="粘贴英文文章, 空行分段"
          :autosize="{ minRows: 8 }"
        />
        <NButton type="primary" block :loading="importing" @click="handleImport">导入</NButton>
      </div>
    </NModal>
  </div>
</template>

<style scoped></style>
