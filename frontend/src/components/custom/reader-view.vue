<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from "vue";
import { useMessage } from "naive-ui";
import { fetchLookupWord, fetchAddVocabulary, fetchTranslateText, fetchReadingHeartbeat } from "@/service/api";
import type { DictionaryEntry } from "@/service/api";
import { speak } from "@/utils/tts";
import { useReaderPreferences } from "@/hooks/common/reader-preferences";

/**
 * ReaderView 共享阅读组件 (Folo 风格): RSS 阅读弹窗与分级阅读详情页共用。
 * - 顶栏: 左侧信息插槽 + A-/A+ 字号 / 舒适宽敞版宽 (localStorage 持久化, 双页共享) + 朗读 + 右侧操作插槽
 * - 正文: 大标题 + 元信息行插槽 + 居中窄栏 + 阅读进度条
 * - 点词查词: 点击正文单词弹查词弹窗 (命中生词本显示状态并计一次阅读相遇); 划选文本翻译
 * - 加入生词本带原文回链 (sourceKind/sourceRef) 与所在原句 (语境复习方向)
 */
defineOptions({
  name: "ReaderView"
});

const props = withDefaults(
  defineProps<{
    /** 文章标题 (同时作为生词本的来源上下文) */
    title: string;
    /** 正文加载中 (NSpin) */
    loading?: boolean;
    /** 是否开启点词查词/划词翻译 (只对英文单词/选中文本生效) */
    lookupEnabled?: boolean;
    /** 生词回链来源: reading=分级阅读 / rss=RSS 条目 */
    sourceKind?: string;
    /** 来源文档/条目 ID */
    sourceRef?: number;
  }>(),
  {
    loading: false,
    lookupEnabled: true,
    sourceKind: "",
    sourceRef: 0
  }
);

const emit = defineEmits<{
  wordAdded: [word: string];
}>();

const slots = defineSlots<{
  "toolbar-left"?: () => any;
  "toolbar-actions"?: () => any;
  meta?: () => any;
  default?: () => any;
  footer?: () => any;
  aside?: () => any;
}>();

const message = useMessage();

// 阅读偏好: 字号 + 版宽 (与另一个使用方共享 localStorage)
const {
  fontSize: readerFontSize,
  width: readerWidth,
  maxWidth: readerMaxWidth,
  setFontSize: setReaderFontSize,
  toggleWidth: toggleReaderWidth
} = useReaderPreferences();

// 阅读进度条
const scrollRef = ref<HTMLElement | null>(null);
const readProgress = ref(0);

const onScroll = () => {
  const el = scrollRef.value;
  if (!el) return;
  const max = el.scrollHeight - el.clientHeight;
  readProgress.value = max > 10 ? Math.min(100, Math.round((el.scrollTop / max) * 100)) : 0;
};

const scrollToTop = () => {
  readProgress.value = 0;
  nextTick(() => {
    if (scrollRef.value) scrollRef.value.scrollTop = 0;
  });
};

// 标题变化即回顶 (RSS 切换文章场景)
watch(
  () => props.title,
  () => {
    scrollToTop();
  }
);

// ===================== 点词查词 =====================
const showWordModal = ref(false);
const wordLoading = ref(false);
const activeWord = ref("");
const wordEntry = ref<DictionaryEntry | null>(null);
const addingWord = ref(false);
// 请求序号: 连续点词时慢的旧请求返回后直接丢弃, 避免覆盖新词的弹窗内容/误关弹窗
let lookupSeq = 0;

async function lookupWord(word: string) {
  const w = word.trim();
  if (!w) return;
  const seq = ++lookupSeq;
  activeWord.value = w;
  wordEntry.value = null;
  showWordModal.value = true;
  wordLoading.value = true;
  const { data: resp, error } = await fetchLookupWord(w);
  if (seq !== lookupSeq) return; // 已被更新的点击取代
  wordLoading.value = false;
  if (error || !resp) {
    message.error("查词失败");
    showWordModal.value = false;
    return;
  }
  wordEntry.value = resp;
}

/** 从点击坐标取所在文本节点里的英文单词与所在句子 (caretRangeFromPoint 双实现) */
const wordAtPoint = (e: MouseEvent): { word: string; sentence: string } => {
  const doc = document as Document & {
    caretRangeFromPoint?: (x: number, y: number) => Range | null;
    caretPositionFromPoint?: (x: number, y: number) => { offsetNode: Node; offset: number } | null;
  };
  let node: Node | null = null;
  let offset = 0;
  if (doc.caretRangeFromPoint) {
    const range = doc.caretRangeFromPoint(e.clientX, e.clientY);
    node = range?.startContainer ?? null;
    offset = range?.startOffset ?? 0;
  } else if (doc.caretPositionFromPoint) {
    const pos = doc.caretPositionFromPoint(e.clientX, e.clientY);
    node = pos?.offsetNode ?? null;
    offset = pos?.offset ?? 0;
  }
  if (!node || node.nodeType !== Node.TEXT_NODE) return { word: "", sentence: "" };
  const text = node.textContent || "";
  const isWordChar = (ch: string) => /[A-Za-z']/.test(ch);
  let start = Math.min(offset, text.length);
  let end = start;
  while (start > 0 && isWordChar(text[start - 1]!)) start -= 1;
  while (end < text.length && isWordChar(text[end]!)) end += 1;
  const word = text
    .slice(start, end)
    .replace(/^'+|'+$/g, "")
    .toLowerCase();
  // 向两侧扩句子边界 (中英文句末标点), 作为加入生词本时的原句
  let sStart = start;
  let sEnd = end;
  const isSentenceEnd = (ch: string) => /[.!?。！？]/.test(ch);
  while (sStart > 0 && !isSentenceEnd(text[sStart - 1]!)) sStart -= 1;
  while (sEnd < text.length && !isSentenceEnd(text[sEnd]!)) sEnd += 1;
  if (sEnd < text.length) sEnd += 1; // 含句末标点
  const sentence = text.slice(sStart, sEnd).trim().slice(0, 300);
  return { word, sentence };
};

// 最近一次点词的所在句子 (加入生词本时随之上报)
let lastSentence = "";

// ===================== 划词翻译 =====================
const showTranslateModal = ref(false);
const translating = ref(false);
const translateSource = ref("");
const translateResult = ref("");

const translateSelection = async (text: string) => {
  translateSource.value = text;
  translateResult.value = "";
  showTranslateModal.value = true;
  translating.value = true;
  const { data: resp, error } = await fetchTranslateText(text);
  translating.value = false;
  if (error || !resp) {
    message.error("翻译失败");
    showTranslateModal.value = false;
    return;
  }
  translateResult.value = resp.translation;
};

const onContentClick = (e: MouseEvent) => {
  if (!props.lookupEnabled) return;
  // 划选了文本: 弹翻译 (不打断 Ctrl+C 复制, 选区保留)
  const selection = window.getSelection();
  const selected = selection ? selection.toString().trim() : "";
  if (selected.length >= 2) {
    translateSelection(selected.slice(0, 500));
    return;
  }
  const target = e.target as HTMLElement | null;
  // 带链接的元素保持原生行为 (打开原文), 其余文本一律支持点词
  if (target?.closest("a[href], button, img, input, textarea, select")) return;
  const { word, sentence } = wordAtPoint(e);
  lastSentence = sentence;
  if (word) {
    lookupWord(word);
  }
};

async function handleAddToVocabulary() {
  if (!wordEntry.value) return;
  addingWord.value = true;
  const { error } = await fetchAddVocabulary({
    word: wordEntry.value.word,
    phonetic: wordEntry.value.phonetic,
    definition: wordEntry.value.translation || wordEntry.value.definition,
    example: wordEntry.value.example,
    sourceContext: props.title || "阅读",
    sourceKind: props.sourceKind || undefined,
    sourceRef: props.sourceRef || undefined,
    contextSentence: lastSentence || undefined
  });
  addingWord.value = false;
  if (error) {
    message.error((error as any)?.message || "添加失败 (可能已存在)");
    return;
  }
  message.success("已加入生词本");
  if (wordEntry.value) wordEntry.value.inVocabulary = true;
  emit("wordAdded", wordEntry.value.word);
}

defineExpose({
  lookupWord,
  scrollToTop
});

// ===================== 全文朗读 (逐块高亮跟随) =====================
const speaking = ref(false);
let speakQueue: HTMLElement[] = [];
let speakIndex = 0;

const contentLang = () => {
  const text = (scrollRef.value?.textContent || "").slice(0, 2000);
  let cjk = 0;
  for (const ch of text) {
    if (ch >= "\u4e00" && ch <= "\u9fff") cjk += 1;
  }
  return cjk * 2 > text.length ? "zh-CN" : "en-US";
};

const clearSpeakingHighlight = () => {
  scrollRef.value?.querySelectorAll(".reader-speaking").forEach((el) => el.classList.remove("reader-speaking"));
};

const speakNext = () => {
  if (!speaking.value || speakIndex >= speakQueue.length) {
    stopSpeaking();
    return;
  }
  const el = speakQueue[speakIndex];
  clearSpeakingHighlight();
  el.classList.add("reader-speaking");
  el.scrollIntoView({ behavior: "smooth", block: "center" });
  const utter = new SpeechSynthesisUtterance(el.textContent || "");
  utter.lang = contentLang();
  utter.rate = 0.95;
  utter.onend = () => {
    speakIndex += 1;
    speakNext();
  };
  utter.onerror = () => stopSpeaking();
  window.speechSynthesis.speak(utter);
};

const toggleSpeaking = () => {
  if (speaking.value) {
    stopSpeaking();
    return;
  }
  const blocks = Array.from(
    scrollRef.value?.querySelectorAll(".reader-content p, .reader-content h1, .reader-content h2, .reader-content h3, .reader-content li, .reader-content blockquote") || []
  ).filter((el) => (el.textContent || "").trim() !== "") as HTMLElement[];
  if (blocks.length === 0) {
    message.info("没有可朗读的内容");
    return;
  }
  speakQueue = blocks;
  speakIndex = 0;
  speaking.value = true;
  speakNext();
};

function stopSpeaking() {
  speaking.value = false;
  window.speechSynthesis.cancel();
  clearSpeakingHighlight();
}

// 标题变化 (切文章) 时停止朗读
watch(
  () => props.title,
  () => {
    stopSpeaking();
  }
);

// ===================== 阅读时长心跳 (30s 一次, 组件挂载期间有效) =====================
let heartbeatTimer: number | null = null;
heartbeatTimer = window.setInterval(() => {
  // 标题非空视为正在阅读 (弹窗关闭时 RSS 场景组件会被 v-if 销毁, 不用额外判断)
  if (props.title) {
    fetchReadingHeartbeat(30);
  }
}, 30_000);

onBeforeUnmount(() => {
  stopSpeaking();
  if (heartbeatTimer !== null) {
    window.clearInterval(heartbeatTimer);
    heartbeatTimer = null;
  }
});

const contentStyle = computed(() => ({ fontSize: `${readerFontSize.value}px` }));
</script>

<template>
  <div class="h-full min-h-0 flex flex-col bg-white dark:bg-[#18181c]">
    <!-- 顶栏: 左侧信息 + 排版设置 + 右侧操作 -->
    <div class="shrink-0 flex items-center gap-2 pl-4 pr-3 h-14 border-b border-gray-100 dark:border-gray-800">
      <slot name="toolbar-left" />
      <div class="flex-1" />
      <div class="flex items-center gap-0.5 shrink-0">
        <NButton
          quaternary
          circle
          size="small"
          :title="speaking ? '停止朗读' : '朗读全文'"
          :aria-label="speaking ? '停止朗读' : '朗读全文'"
          @click="toggleSpeaking"
        >
          <template #icon>
            <SvgIcon :icon="speaking ? 'mdi:stop-circle-outline' : 'mdi:play-circle-outline'" :class="speaking ? 'text-primary' : ''" />
          </template>
        </NButton>
        <NButton
          quaternary
          size="tiny"
          class="!px-1.5"
          title="减小字号"
          aria-label="减小字号"
          @click="setReaderFontSize(readerFontSize - 1)"
        >
          A-
        </NButton>
        <span class="text-11px text-gray-400 w-6 text-center tabular-nums" aria-live="polite">{{ readerFontSize }}</span>
        <NButton
          quaternary
          size="tiny"
          class="!px-1.5"
          title="增大字号"
          aria-label="增大字号"
          @click="setReaderFontSize(readerFontSize + 1)"
        >
          A+
        </NButton>
        <NButton
          quaternary
          circle
          size="small"
          class="hidden sm:inline-flex"
          :title="readerWidth === 'cozy' ? '切换宽敞版式' : '切换舒适版式'"
          :aria-label="readerWidth === 'cozy' ? '切换宽敞版式' : '切换舒适版式'"
          @click="toggleReaderWidth"
        >
          <template #icon>
            <SvgIcon :icon="readerWidth === 'cozy' ? 'mdi:arrow-expand-horizontal' : 'mdi:arrow-collapse-horizontal'" />
          </template>
        </NButton>
        <slot name="toolbar-actions" />
      </div>
    </div>

    <!-- 正文滚动区 -->
    <div class="relative flex-1 min-h-0">
      <!-- 阅读进度条 -->
      <div
        class="absolute top-0 left-0 h-[2px] bg-primary z-10 transition-[width] duration-150"
        :style="{ width: `${readProgress}%` }"
      />
      <div ref="scrollRef" class="h-full overflow-y-auto" @scroll.passive="onScroll">
        <!-- 移动端纵向堆叠 (aside 侧栏落到正文下方), 桌面居中双栏 -->
        <div class="flex flex-col md:flex-row md:justify-center gap-6 md:gap-8 px-4 md:px-6 pt-8 pb-14">
          <!-- 正文 (大标题 + 元信息 + 居中窄栏) -->
          <article class="w-full min-w-0" :style="{ maxWidth: `${readerMaxWidth}px` }">
            <h1 class="text-[26px] md:text-[30px] font-bold leading-[1.35] text-gray-900 dark:text-gray-50 mb-5">
              {{ title }}
            </h1>
            <div
              v-if="slots.meta"
              class="flex flex-wrap items-center gap-x-3 gap-y-1 pb-6 mb-2 border-b border-gray-100 dark:border-gray-800 text-13px text-gray-400"
            >
              <slot name="meta" />
            </div>
            <NSpin :show="loading">
              <div class="reader-content mt-6" :style="contentStyle" @click="onContentClick">
                <slot />
              </div>
            </NSpin>
            <slot name="footer" />
          </article>

          <!-- 侧栏 (如生词列表) -->
          <slot name="aside" />
        </div>
      </div>
    </div>

    <!-- 点词查词弹窗 -->
    <NModal v-model:show="showWordModal" preset="card" :title="activeWord" :style="{ width: 'min(92vw, 440px)' }">
      <NSpin :show="wordLoading">
        <div v-if="wordEntry" class="flex flex-col gap-3 py-2">
          <div class="flex items-center gap-3">
            <span class="text-2xl font-bold text-primary">{{ wordEntry.word }}</span>
            <span class="text-sm text-gray-400">{{ wordEntry.phonetic }}</span>
            <NButton circle size="small" quaternary type="primary" @click="speak(wordEntry.word, { lang: 'en-US', rate: 0.9 })">
              <template #icon>
                <SvgIcon icon="mdi:volume-high" class="text-20px" />
              </template>
            </NButton>
          </div>
          <div class="text-sm text-gray-600 dark:text-gray-300">{{ wordEntry.definition }}</div>
          <div class="text-sm font-bold">{{ wordEntry.translation }}</div>
          <div v-if="wordEntry.example" class="rounded-lg bg-gray-50 dark:bg-gray-800 p-3 text-sm italic">
            {{ wordEntry.example }}
          </div>
          <NButton v-if="wordEntry.inVocabulary" disabled block>
            已在生词本<span v-if="(wordEntry.encounters || 0) > 1" class="ml-1 text-xs opacity-70">· 阅读中已遇 {{ wordEntry.encounters }} 次</span>
          </NButton>
          <NButton v-else type="primary" block :loading="addingWord" @click="handleAddToVocabulary">加入生词本</NButton>
        </div>
      </NSpin>
    </NModal>

    <!-- 划词翻译弹窗 -->
    <NModal v-model:show="showTranslateModal" preset="card" title="翻译" :style="{ width: 'min(92vw, 480px)' }">
      <div class="flex flex-col gap-3 py-2">
        <div class="text-sm text-gray-600 dark:text-gray-300 bg-gray-50 dark:bg-gray-800 rounded-lg p-3 max-h-40 overflow-auto">
          {{ translateSource }}
        </div>
        <NSpin :show="translating">
          <div v-if="translateResult" class="text-15px leading-relaxed">{{ translateResult }}</div>
          <div v-else class="text-xs text-gray-400 py-2">翻译中...</div>
        </NSpin>
      </div>
    </NModal>
  </div>
</template>

<style scoped>
/* 正文基础排版, 字号随顶栏 A-/A+ 缩放; 富文本元素样式由使用方的插槽内容自带 (如 .article-html) */
.reader-content {
  line-height: 1.9;
  color: #374151;
  word-break: break-word;
  letter-spacing: 0.01em;
  cursor: text;
}
.dark .reader-content {
  color: #d1d5db;
}
.reader-content :deep(p) {
  margin: 0 0 1.5em;
}
.reader-content :deep(p:last-child) {
  margin-bottom: 0;
}
/* 朗读跟随高亮 */
.reader-content :deep(.reader-speaking) {
  background: rgba(100, 108, 255, 0.14);
  border-radius: 6px;
  transition: background 0.2s;
}
</style>
