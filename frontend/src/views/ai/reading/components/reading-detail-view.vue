<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { useMessage } from "naive-ui";
import { fetchGetReadingDoc, fetchAnnotateReadingDoc } from "@/service/api";
import type { DifficultWord, ReadingDoc } from "@/service/api";
import ReaderView from "@/components/custom/reader-view.vue";
import DifficultWordsAside from "@/components/custom/difficult-words-aside.vue";
import { useReaderSessionStore } from "@/store/modules/reader-session";
import { $t } from "@/locales";
import { readingStatusTag } from "../shared";

/**
 * ReadingDetailView 分级阅读详情 (ReaderView 之上叠加文档业务):
 * 加载文档 + AI 标注 + 轮询 + 生词高亮 + 生词侧栏。
 * 两个使用方: 路由页 reading-detail/[id].vue (mode=page, emit back)
 * 与阅读列表页弹窗 (mode=modal, emit minimize/close)。
 * 最小化/返回均交给容器决定 (存全局小窗会话 + 跳转), 组件只上报意图。
 */
defineOptions({
  name: "ReadingDetailView"
});

const props = withDefaults(
  defineProps<{
    docId: number;
    mode?: "page" | "modal";
  }>(),
  { mode: "page" }
);

const emit = defineEmits<{
  close: [];
  minimize: [];
  back: [];
}>();

const message = useMessage();
const sessionStore = useReaderSessionStore();

const doc = ref<(ReadingDoc & { content: string }) | null>(null);
const difficultWords = ref<DifficultWord[]>([]);
const loading = ref(false);
// 标注是异步任务 (Cloudflare 100s 源站响应上限), annotating 态由文档状态驱动
const annotating = computed(() => doc.value?.status === "annotating");

const readerRef = ref<InstanceType<typeof ReaderView> | null>(null);

// 生词集合 (小写) 用于高亮
const difficultSet = computed(() => {
  const set = new Map<string, DifficultWord>();
  difficultWords.value.forEach((w) => set.set(w.word.toLowerCase(), w));
  return set;
});

interface RenderToken {
  text: string;
  word: string;
  isDifficult: boolean;
  known: boolean;
}

const tokenize = (paragraph: string): RenderToken[] => {
  const tokens = paragraph.split(/(\s+)/);
  return tokens.map((t) => {
    if (/^\s+$/.test(t)) {
      return { text: t, word: "", isDifficult: false, known: false };
    }
    // 去掉前后标点取词干
    const bare = t.toLowerCase().replace(/^[^a-zA-Z']+|[^a-zA-Z']+$/g, "");
    const hit = bare ? difficultSet.value.get(bare) : undefined;
    return { text: t, word: bare, isDifficult: !!hit, known: !!hit?.known };
  });
};

const paragraphs = computed(() => {
  if (!doc.value) return [];
  return doc.value.content.split(/\n\s*\n/).filter((p) => p.trim() !== "");
});

// 分词结果缓存: 只在正文或生词集合变化时重算, 标注轮询期间不重复分词
const tokenizedParagraphs = computed(() => paragraphs.value.map(p => tokenize(p)));

// 点词查词在 ReaderView 内置; 加入生词本后同步本地高亮状态
const onWordAdded = (word: string) => {
  const hit = difficultWords.value.find((w) => w.word.toLowerCase() === word.toLowerCase());
  if (hit) hit.known = true;
};

// 全局小窗"继续阅读": 卡片点击 (restorePending) 且指向本文档时, 内容加载完成后写回续读位置。
// 只消费本文档的快照 (kind+itemId 过滤): 卡片指向其他文档/类型时不动, 留给路由切换后的加载流程消费
function tryConsumeScrollRestore() {
  if (!sessionStore.restorePending) return;
  const snapshot = sessionStore.consumeRestore({ kind: "reading", itemId: props.docId });
  if (snapshot) {
    nextTick(() => readerRef.value?.setScrollTop(snapshot.scrollTop, snapshot.progress));
  }
}

// 小窗卡片在任意页面被点击后进入本组件 (含同组件换文档) → 尝试恢复
watch(
  () => sessionStore.restorePending,
  (pending) => {
    if (pending) tryConsumeScrollRestore();
  }
);

const loadDoc = async () => {
  loading.value = true;
  const { data: resp, error } = await fetchGetReadingDoc(props.docId);
  loading.value = false;
  if (error || !resp) {
    message.error($t("page.ai.reading.fetchDocFailed"));
    return;
  }
  doc.value = resp.doc;
  difficultWords.value = resp.difficultWords || [];
  // 页面打开时若标注仍在后台进行, 继续轮询
  if (resp.doc.status === "annotating") {
    // eslint-disable-next-line @typescript-eslint/no-use-before-define
    startAnnotatePolling();
  }
  tryConsumeScrollRestore();
};

// 标注任务轮询: 3s 一次, 状态离开 annotating 即收口
let annotatePollTimer: number | null = null;

const stopAnnotatePolling = () => {
  if (annotatePollTimer !== null) {
    window.clearInterval(annotatePollTimer);
    annotatePollTimer = null;
  }
};

const startAnnotatePolling = () => {
  stopAnnotatePolling();
  let attempts = 0;
  annotatePollTimer = window.setInterval(async () => {
    attempts += 1;
    // 上限 ~8 分钟: 超过则停轮询 (后端 10 分钟后会自动解锁该文档, 可手动重试)
    if (attempts > 160) {
      stopAnnotatePolling();
      message.info($t("page.ai.reading.annotateStillRunning"));
      return;
    }
    const { data: resp, error } = await fetchGetReadingDoc(props.docId);
    if (error || !resp || resp.doc.status === "annotating") return;
    stopAnnotatePolling();
    doc.value = resp.doc;
    difficultWords.value = resp.difficultWords || [];
    if (resp.doc.status === "annotated") {
      message.success($t("page.ai.reading.annotateDone"));
    } else if (resp.doc.status === "failed") {
      message.error($t("page.ai.reading.annotateFailedMsg"));
    }
  }, 3000);
};

const handleAnnotate = async () => {
  const { data: resp, error } = await fetchAnnotateReadingDoc(props.docId);
  if (error || !resp) {
    message.error($t("page.ai.reading.annotateStartFailed"));
    return;
  }
  if (doc.value) {
    doc.value.status = resp.status;
  }
  message.info($t("page.ai.reading.annotateStarted"));
  startAnnotatePolling();
};

// 路由复用同组件切不同文档时重新加载 (弹窗场景由 v-if 重建, 不依赖此 watch)
watch(
  () => props.docId,
  () => {
    stopAnnotatePolling();
    loadDoc();
  }
);

onMounted(() => {
  loadDoc();
});

onUnmounted(() => {
  stopAnnotatePolling();
});

// 暴露阅读位置与文档信息: 容器页 (列表弹窗/详情路由页) 在最小化、切页离开时组装全局小窗快照
defineExpose({
  getScrollTop: () => readerRef.value?.getScrollTop() ?? 0,
  setScrollTop: (top: number, progress = 0) => readerRef.value?.setScrollTop(top, progress),
  getReadProgress: () => readerRef.value?.getReadProgress() ?? 0,
  doc
});
</script>

<template>
  <ReaderView
    ref="readerRef"
    :title="doc?.title || ''"
    :loading="loading"
    source-kind="reading"
    :source-ref="doc?.id || 0"
    @word-added="onWordAdded"
  >
    <template #toolbar-left>
      <NButton
        v-if="mode === 'modal'"
        quaternary
        circle
        size="small"
        :title="$t('page.ai.reading.closeBtn')"
        :aria-label="$t('page.ai.reading.closeBtn')"
        @click="emit('close')"
      >
        <template #icon>
          <SvgIcon icon="mdi:close" />
        </template>
      </NButton>
      <NButton
        v-else
        quaternary
        circle
        size="small"
        :title="$t('page.ai.reader.backToList')"
        :aria-label="$t('page.ai.reader.backToList')"
        @click="emit('back')"
      >
        <template #icon>
          <SvgIcon icon="mdi:arrow-left" />
        </template>
      </NButton>
      <span class="font-bold truncate min-w-0">{{ doc?.title || '...' }}</span>
    </template>
    <template #toolbar-actions>
      <!-- 收入全局小窗 (右下角卡片, 切页不丢, 稍后从卡片恢复续读); 容器决定存会话与跳转 -->
      <NButton
        quaternary
        circle
        size="small"
        :title="$t('page.ai.reader.minimizeTitle')"
        @click="emit('minimize')"
      >
        <template #icon>
          <SvgIcon icon="mdi:window-minimize" />
        </template>
      </NButton>
      <NButton
        size="small"
        type="primary"
        class="ml-1"
        :loading="annotating"
        :disabled="annotating"
        @click="handleAnnotate"
      >
        {{ annotating ? $t('page.ai.reading.annotating', { progress: doc?.annotateProgress || 0 }) : (doc?.status === 'annotated' ? $t('page.ai.reading.reAnnotate') : doc?.status === 'failed' ? $t('page.ai.reading.retryAnnotate') : $t('page.ai.reading.annotate')) }}
      </NButton>
    </template>
    <template #meta>
      <span v-if="doc?.createdAt">{{ new Date(doc.createdAt).toLocaleString() }}</span>
      <NTag v-if="doc" size="tiny" :bordered="false">{{ doc.wordCount }} {{ $t('page.ai.reading.wordUnit') }}</NTag>
      <NTag v-if="doc && doc.status === 'annotated'" size="tiny" type="warning" :bordered="false">
        {{ $t('page.ai.reading.unknownRateText', { rate: `${Math.round(doc.unknownRate * 1000) / 10}%` }) }}
      </NTag>
      <NTag v-if="doc" size="tiny" :type="readingStatusTag(doc.status).type" :bordered="false">
        {{ readingStatusTag(doc.status).label }}
      </NTag>
      <span class="hidden md:inline text-gray-300 dark:text-gray-600">
        {{ $t('page.ai.reading.lookupHint') }}
      </span>
    </template>

    <p v-for="(tokens, pi) in tokenizedParagraphs" :key="pi">
      <template v-for="(tk, ti) in tokens" :key="ti">
        <!-- 生词 token 可键盘访问 (Tab 聚焦 + Enter/Space 查词); 普通词保持纯鼠标 -->
        <a
          v-if="tk.isDifficult"
          role="button"
          tabindex="0"
          class="cursor-pointer rounded px-0.5 font-bold focus:outline-none focus-visible:ring-1 focus-visible:ring-primary"
          :class="tk.known ? 'bg-gray-200 dark:bg-gray-700 text-gray-500' : 'bg-warning/20 text-warning hover:bg-warning/40'"
          @keydown.enter.prevent="readerRef?.lookupWord(tk.word)"
          @keydown.space.prevent="readerRef?.lookupWord(tk.word)"
        >
          {{ tk.text }}
        </a>
        <span v-else-if="tk.word" class="cursor-pointer hover:text-primary">
          {{ tk.text }}
        </span>
        <span v-else>{{ tk.text }}</span>
      </template>
    </p>
    <NEmpty v-if="paragraphs.length === 0 && !loading" :description="$t('page.ai.reading.docEmpty')" />

    <template #aside>
      <!-- 生词侧栏 (桌面右侧 / 移动端抽屉, 与 RSS 阅读弹窗共用组件) -->
      <DifficultWordsAside
        v-if="difficultWords.length > 0"
        :words="difficultWords"
        :heading="$t('page.ai.reader.annotatedWords')"
        source-kind="reading"
        :source-ref="doc?.id || 0"
        :source-context="doc?.title || '阅读'"
        @lookup="(word: string) => readerRef?.lookupWord(word)"
      />
    </template>
  </ReaderView>
</template>
