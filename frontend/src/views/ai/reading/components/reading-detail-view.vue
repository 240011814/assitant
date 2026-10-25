<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useMessage } from "naive-ui";
import { fetchGetReadingDoc, fetchAnnotateReadingDoc } from "@/service/api";
import type { DifficultWord, ReadingDoc } from "@/service/api";
import { useRouterPush } from "@/hooks/common/router";
import ReaderView from "@/components/custom/reader-view.vue";

/**
 * ReadingDetailView 分级阅读详情 (ReaderView 之上叠加文档业务):
 * 加载文档 + AI 标注 + 轮询 + 生词高亮 + 生词侧栏。
 * 两个使用方: 路由页 reading-detail/[id].vue (mode=page, 带返回按钮)
 * 与阅读列表页弹窗 (mode=modal, 带关闭按钮, emit close)。
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
}>();

const message = useMessage();
const { routerPushByKey } = useRouterPush();

const doc = ref<(ReadingDoc & { content: string }) | null>(null);
const difficultWords = ref<DifficultWord[]>([]);
const loading = ref(false);
// 标注是异步任务 (Cloudflare 100s 源站响应上限), annotating 态由文档状态驱动
const annotating = computed(() => doc.value?.status === "annotating");

const readerRef = ref<InstanceType<typeof ReaderView> | null>(null);

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

const loadDoc = async () => {
  loading.value = true;
  const { data: resp, error } = await fetchGetReadingDoc(props.docId);
  loading.value = false;
  if (error || !resp) {
    message.error("获取文档失败");
    return;
  }
  doc.value = resp.doc;
  difficultWords.value = resp.difficultWords || [];
  // 页面打开时若标注仍在后台进行, 继续轮询
  if (resp.doc.status === "annotating") {
    // eslint-disable-next-line @typescript-eslint/no-use-before-define
    startAnnotatePolling();
  }
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
      message.info("标注仍在后台进行, 稍后刷新页面查看结果");
      return;
    }
    const { data: resp, error } = await fetchGetReadingDoc(props.docId);
    if (error || !resp || resp.doc.status === "annotating") return;
    stopAnnotatePolling();
    doc.value = resp.doc;
    difficultWords.value = resp.difficultWords || [];
    if (resp.doc.status === "annotated") {
      message.success("标注完成");
    } else if (resp.doc.status === "failed") {
      message.error("标注失败, 可点击重试");
    }
  }, 3000);
};

const handleAnnotate = async () => {
  const { data: resp, error } = await fetchAnnotateReadingDoc(props.docId);
  if (error || !resp) {
    message.error("启动标注失败");
    return;
  }
  if (doc.value) {
    doc.value.status = resp.status;
  }
  message.info("已开始标注, 完成后自动刷新");
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
        title="关闭"
        aria-label="关闭"
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
        title="返回列表"
        aria-label="返回列表"
        @click="routerPushByKey('ai_reading')"
      >
        <template #icon>
          <SvgIcon icon="mdi:arrow-left" />
        </template>
      </NButton>
      <span class="font-bold truncate min-w-0">{{ doc?.title || '...' }}</span>
    </template>
    <template #toolbar-actions>
      <NButton
        size="small"
        type="primary"
        class="ml-1"
        :loading="annotating"
        :disabled="annotating"
        @click="handleAnnotate"
      >
        {{ annotating ? `标注中 ${doc?.annotateProgress || 0}%` : (doc?.status === 'annotated' ? '重新标注生词' : doc?.status === 'failed' ? '重试标注' : 'AI 标注生词') }}
      </NButton>
    </template>
    <template #meta>
      <span v-if="doc?.createdAt">{{ new Date(doc.createdAt).toLocaleString() }}</span>
      <NTag v-if="doc" size="tiny" :bordered="false">{{ doc.wordCount }} 词</NTag>
      <NTag v-if="doc && doc.status === 'annotated'" size="tiny" type="warning" :bordered="false">
        生词率 {{ Math.round(doc.unknownRate * 1000) / 10 }}%
      </NTag>
      <NTag v-if="doc" size="tiny" :type="statusTag(doc.status).type" :bordered="false">
        {{ statusTag(doc.status).label }}
      </NTag>
      <span class="hidden md:inline text-gray-300 dark:text-gray-600">
        点击任意单词即可查词; 高亮为 AI 标注的生词 (灰色为已在生词本)
      </span>
    </template>

    <p v-for="(tokens, pi) in tokenizedParagraphs" :key="pi">
      <template v-for="(tk, ti) in tokens" :key="ti">
        <a
          v-if="tk.isDifficult"
          class="cursor-pointer rounded px-0.5 font-bold"
          :class="tk.known ? 'bg-gray-200 dark:bg-gray-700 text-gray-500' : 'bg-warning/20 text-warning hover:bg-warning/40'"
        >
          {{ tk.text }}
        </a>
        <span v-else-if="tk.word" class="cursor-pointer hover:text-primary">
          {{ tk.text }}
        </span>
        <span v-else>{{ tk.text }}</span>
      </template>
    </p>
    <NEmpty v-if="paragraphs.length === 0 && !loading" description="文档为空" />

    <template #aside>
      <!-- 生词侧栏 (桌面右侧 / 移动端正文下方) -->
      <aside
        v-if="difficultWords.length > 0"
        class="w-full md:w-64 md:shrink-0 md:sticky md:top-4 md:self-start max-h-[40vh] md:max-h-[calc(100vh-220px)] overflow-auto"
      >
        <div class="font-bold mb-2 text-15px text-gray-900 dark:text-gray-50">标注生词 ({{ difficultWords.length }})</div>
        <div class="flex flex-col gap-2">
          <div
            v-for="w in difficultWords"
            :key="w.word"
            class="rounded-lg border border-gray-200 dark:border-gray-700 p-2 text-sm"
          >
            <div class="flex items-center justify-between">
              <a class="cursor-pointer font-bold text-primary" @click="readerRef?.lookupWord(w.word)">{{ w.word }}</a>
              <NTag size="tiny" :type="w.known ? 'default' : 'warning'" :bordered="false">
                {{ w.known ? '已收藏' : '生词' }}
              </NTag>
            </div>
            <div class="text-xs text-gray-500 mt-1">{{ w.meaning }}</div>
          </div>
        </div>
      </aside>
    </template>
  </ReaderView>
</template>
