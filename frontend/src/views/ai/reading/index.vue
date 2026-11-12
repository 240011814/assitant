<script setup lang="ts">
import { h, onDeactivated, onMounted, ref } from "vue";
import { NButton, NEmpty, NPopconfirm, NSpin, NTag, useMessage } from "naive-ui";
import type { DataTableColumns } from "naive-ui";
import {
  fetchImportReading,
  fetchGetReadingDocs,
  fetchDeleteReadingDoc,
} from "@/service/api";
import type { ReadingDoc } from "@/service/api";
import { useAppStore } from "@/store/modules/app";
import { useReaderSessionStore } from "@/store/modules/reader-session";
import type { ReaderSessionSnapshot } from "@/store/modules/reader-session";
import { useReaderMiniLeaveSave } from "@/hooks/common/reader-mini-save";
import { $t } from "@/locales";
import ReadingDetailView from "./components/reading-detail-view.vue";
import { difficultyTag, readingStatusTag } from "./shared";

const appStore = useAppStore();
const message = useMessage();

const loading = ref(false);
const docs = ref<ReadingDoc[]>([]);
const keyword = ref("");

// 导入
const showImportModal = ref(false);
const importTitle = ref("");
const importType = ref<"url" | "text">("url");
const importUrl = ref("");
const importText = ref("");
const importing = ref(false);

// ===================== 阅读弹窗 (与 RSS 阅读弹窗同款容器) =====================
const activeDocId = ref<number | null>(null);
const sessionStore = useReaderSessionStore();
const detailRef = ref<InstanceType<typeof ReadingDetailView> | null>(null);

const openDoc = (id: number) => {
  activeDocId.value = id;
};

// 弹窗打开中的阅读快照: 供小窗按钮与离开自动收窗使用 (标题从列表行取)
const captureDocSession = (): ReaderSessionSnapshot | null => {
  if (activeDocId.value === null) return null;
  const row = docs.value.find((d) => d.id === activeDocId.value);
  return {
    kind: "reading",
    itemId: activeDocId.value,
    title: row?.title || `文档 #${activeDocId.value}`,
    subtitle: "分级阅读",
    scrollTop: detailRef.value?.getScrollTop() ?? 0,
    progress: detailRef.value?.getReadProgress() ?? 0
  };
};

// 切 tab / 路由跳转 / 关浏览器标签时弹窗仍开着 → 自动收入全局小窗
useReaderMiniLeaveSave(captureDocSession);

// 失活时关闭阅读弹窗: NModal teleport 到 body 不随 KeepAlive 失活隐藏 (视觉残留),
// 弹窗内 ReaderView 收不到 onDeactivated 心跳不会停; 注册在 leave-save 之后, 先存快照再关弹窗
onDeactivated(() => {
  activeDocId.value = null;
});

// 小窗按钮: 存全局会话并收起弹窗 (不刷新列表, 与 X 关闭区分)
const minimizeDoc = () => {
  const snapshot = captureDocSession();
  if (snapshot) sessionStore.minimize(snapshot);
  activeDocId.value = null;
};

const columns = ref<DataTableColumns<ReadingDoc>>([
  {
    title: $t("page.ai.reading.title"),
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
    title: $t("page.ai.reading.sourceCol"),
    key: "sourceType",
    width: 90,
    render: (row) => (row.sourceType === "url" ? $t("page.ai.reading.web") : $t("page.ai.reading.text")),
  },
  { title: $t("page.ai.reading.wordCountCol"), key: "wordCount", width: 90 },
  {
    title: $t("page.ai.reading.estDifficulty"),
    key: "estUnknownRate",
    width: 90,
    render(row) {
      const tag = difficultyTag(row.estUnknownRate);
      if (!tag) return "—";
      const pct = Math.round(row.estUnknownRate * 1000) / 10;
      return h(NTag, { size: "small", type: tag.type, bordered: false }, { default: () => `${tag.label} ${pct}%` });
    },
  },
  {
    title: $t("page.ai.reading.unknownRateCol"),
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
    title: $t("page.ai.reading.statusCol"),
    key: "status",
    width: 100,
    render(row) {
      const s = readingStatusTag(row.status);
      return h(NTag, { size: "small", type: s.type, bordered: false }, { default: () => s.label });
    },
  },
  {
    title: $t("page.ai.reading.actionsCol"),
    key: "actions",
    width: 150,
    render(row) {
      return h("div", { class: "flex gap-1" }, [
        h(
          NButton,
          { size: "small", quaternary: true, type: "primary", onClick: () => openDoc(row.id) },
          { default: () => $t("page.ai.reading.read") }
        ),
        h(
          NPopconfirm,
          {
            // eslint-disable-next-line @typescript-eslint/no-use-before-define
            onPositiveClick: () => handleDelete(row.id)
          },
          {
            trigger: () => h(NButton, { size: "small", quaternary: true, type: "error" }, { default: () => $t("page.ai.reading.deleteBtn") }),
            default: () => $t("page.ai.reading.deleteConfirm"),
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
    message.error($t("page.ai.reading.fetchListFailed"));
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
    message.warning($t("page.ai.reading.urlRequired"));
    return;
  }
  if (importType.value === "text" && !importText.value.trim()) {
    message.warning($t("page.ai.reading.textRequired"));
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
    message.error($t("page.ai.reading.importFailed"));
    return;
  }
  message.success($t("page.ai.reading.importSuccess"));
  showImportModal.value = false;
  importTitle.value = "";
  importUrl.value = "";
  importText.value = "";
  loadData();
};

const handleDelete = async (id: number) => {
  const { error } = await fetchDeleteReadingDoc(id);
  if (error) {
    message.error($t("page.ai.reading.deleteFailed"));
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
          <span class="font-bold text-18px">{{ $t('page.ai.reading.title') }}</span>
          <div class="flex gap-2">
            <NButton type="primary" @click="showImportModal = true">{{ $t('page.ai.reading.importArticle') }}</NButton>
          </div>
        </div>
      </template>

      <div class="flex gap-2 mb-4" :class="appStore.isMobile ? '' : 'w-96'">
        <NInput
          v-model:value="keyword"
          :placeholder="$t('page.ai.reading.searchPlaceholder')"
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

      <!-- PC: 表格 -->
      <NDataTable
        v-if="!appStore.isMobile"
        :columns="columns"
        :data="docs"
        :loading="loading"
        :row-key="(row: any) => row.id"
      />

      <!-- 移动端: 卡片列表 -->
      <div v-else class="flex flex-col gap-3">
        <NSpin :show="loading">
          <div
            v-for="row in docs"
            :key="row.id"
            class="rounded-lg border border-gray-100 dark:border-gray-800 p-3 flex flex-col gap-2 cursor-pointer active:bg-gray-50 dark:active:bg-gray-800"
            @click="openDoc(row.id)"
          >
            <div class="flex items-center justify-between gap-2">
              <span class="font-bold text-15px truncate">{{ row.title }}</span>
              <NTag size="tiny" :type="readingStatusTag(row.status).type" :bordered="false" class="shrink-0">
                {{ readingStatusTag(row.status).label }}
              </NTag>
            </div>
            <div class="flex items-center gap-2 text-xs text-gray-400 flex-wrap">
              <span>{{ row.sourceType === "url" ? $t('page.ai.reading.web') : $t('page.ai.reading.text') }}</span>
              <span>· {{ row.wordCount }} {{ $t('page.ai.reading.wordUnit') }}</span>
              <NTag
                v-if="difficultyTag(row.estUnknownRate)"
                size="tiny"
                :bordered="false"
                :type="difficultyTag(row.estUnknownRate)!.type"
                class="shrink-0"
              >
                {{ difficultyTag(row.estUnknownRate)!.label }}
                {{ Math.round(row.estUnknownRate * 1000) / 10 }}%
              </NTag>
              <NTag v-if="row.status === 'annotated'" size="tiny" type="warning" :bordered="false" class="shrink-0">
                {{ $t('page.ai.reading.unknownRateText', { rate: `${Math.round(row.unknownRate * 1000) / 10}%` }) }}
              </NTag>
            </div>
            <div class="flex items-center justify-between">
              <span class="text-xs text-gray-400">{{ row.createdAt ? new Date(row.createdAt).toLocaleDateString() : "" }}</span>
              <span class="flex gap-1" @click.stop>
                <NButton size="tiny" quaternary type="primary" @click="openDoc(row.id)">{{ $t('page.ai.reading.read') }}</NButton>
                <NPopconfirm @positive-click="handleDelete(row.id)">
                  <template #trigger>
                    <NButton size="tiny" quaternary type="error">{{ $t('page.ai.reading.deleteBtn') }}</NButton>
                  </template>
                  {{ $t('page.ai.reading.deleteConfirm') }}
                </NPopconfirm>
              </span>
            </div>
          </div>
        </NSpin>
        <NEmpty v-if="docs.length === 0 && !loading" class="py-10" :description="$t('page.ai.reading.noDocs')" />
      </div>
    </NCard>

    <!-- 导入 -->
    <NModal v-model:show="showImportModal" preset="card" :title="$t('page.ai.reading.importTitle')" :style="{ width: appStore.isMobile ? '92%' : '560px' }">
      <div class="flex flex-col gap-4 py-2">
        <NTabs v-model:value="importType" type="segment">
          <NTab name="url">{{ $t('page.ai.reading.tabUrl') }}</NTab>
          <NTab name="text">{{ $t('page.ai.reading.tabText') }}</NTab>
        </NTabs>
        <NInput v-model:value="importTitle" :placeholder="$t('page.ai.reading.titlePlaceholder')" />
        <NInput v-if="importType === 'url'" v-model:value="importUrl" :placeholder="$t('page.ai.reading.urlPlaceholder')" />
        <NInput
          v-else
          v-model:value="importText"
          type="textarea"
          :placeholder="$t('page.ai.reading.textPlaceholder')"
          :autosize="{ minRows: 8 }"
        />
        <NButton type="primary" block :loading="importing" @click="handleImport">{{ $t('page.ai.reading.importBtn') }}</NButton>
      </div>
    </NModal>
    <!-- 阅读弹窗 (与 RSS 阅读弹窗同款: 卡片容器 + ReaderView 排版) -->
    <NModal :show="activeDocId !== null" :auto-focus="false" @update:show="(v: boolean) => { if (!v) closeDoc(); }">
      <div class="w-full h-full md:w-[min(94vw,960px)] md:h-[88vh] flex flex-col rounded-none md:rounded-xl bg-white dark:bg-[#18181c] shadow-2xl overflow-hidden">
        <ReadingDetailView
          v-if="activeDocId !== null"
          ref="detailRef"
          :doc-id="activeDocId"
          mode="modal"
          @close="closeDoc"
          @minimize="minimizeDoc"
        />
      </div>
    </NModal>
  </div>
</template>

<style scoped></style>
