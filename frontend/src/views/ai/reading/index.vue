<script setup lang="ts">
import { h, onMounted, ref } from "vue";
import { NButton, NPopconfirm, NTag, useMessage } from "naive-ui";
import type { DataTableColumns } from "naive-ui";
import {
  fetchImportReading,
  fetchGetReadingDocs,
  fetchDeleteReadingDoc,
} from "@/service/api";
import { useAppStore } from "@/store/modules/app";
import ReadingDetailView from "./components/reading-detail-view.vue";

const appStore = useAppStore();
const message = useMessage();

const loading = ref(false);
const docs = ref<any[]>([]);
const keyword = ref("");

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

// ===================== 阅读弹窗 (与 RSS 阅读弹窗同款容器) =====================
const activeDocId = ref<number | null>(null);

const openDoc = (id: number) => {
  activeDocId.value = id;
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
          onClick: () => openDoc(row.id)
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
          { size: "small", quaternary: true, type: "primary", onClick: () => openDoc(row.id) },
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

// ===================== 数据加载 =====================
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

// 关闭弹窗后刷新列表: 阅读中可能完成标注/加入生词, 状态列需要同步
const closeDoc = () => {
  activeDocId.value = null;
  loadData();
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
    <!-- 阅读弹窗 (与 RSS 阅读弹窗同款: 卡片容器 + ReaderView 排版) -->
    <NModal :show="activeDocId !== null" :auto-focus="false" @update:show="(v: boolean) => { if (!v) closeDoc(); }">
      <div class="w-full h-full md:w-[min(94vw,960px)] md:h-[88vh] flex flex-col rounded-none md:rounded-xl bg-white dark:bg-[#18181c] shadow-2xl overflow-hidden">
        <ReadingDetailView v-if="activeDocId !== null" :doc-id="activeDocId" mode="modal" @close="closeDoc" />
      </div>
    </NModal>
  </div>
</template>

<style scoped></style>
