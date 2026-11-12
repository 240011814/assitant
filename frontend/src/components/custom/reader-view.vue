<script setup lang="ts">
import { computed, nextTick, onActivated, onBeforeUnmount, onDeactivated, ref, watch } from "vue";
import { useMessage } from "naive-ui";
import { fetchLookupWord, fetchAddVocabulary, fetchTranslateText, fetchReadingHeartbeat } from "@/service/api";
import type { DictionaryEntry } from "@/service/api";
import { speak, stopSpeak, detectChunkLang } from "@/utils/tts";
import { useReaderPreferences } from "@/hooks/common/reader-preferences";
import { $t } from "@/locales";

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
  /** 开始全文朗读 (外层用于暂停其他音源, 如播客收听条) */
  speakStart: [];
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

// 阅读偏好: 字号/版宽/行距/衬线/语速/对照模式 (与另一个使用方共享 localStorage)
const {
  fontSize: readerFontSize,
  width: readerWidth,
  maxWidth: readerMaxWidth,
  lineHeight: readerLineHeight,
  lineHeightValue: readerLineHeightValue,
  serif: readerSerif,
  speechRate: readerSpeechRate,
  setFontSize: setReaderFontSize,
  toggleWidth: toggleReaderWidth,
  setLineHeight: setReaderLineHeight,
  setSerif: setReaderSerif,
  setSpeechRate: setReaderSpeechRate
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
// 查词缓存 (LRU): 本次会话内重复点同一个词直接复用结果, 不再请求——
// 后端在查词命中生词本时顺带计一次"阅读相遇", 缓存命中不上报, 避免重复点词虚增 encounters
const lookupCache = new Map<string, DictionaryEntry>();
const LOOKUP_CACHE_MAX = 200;

async function lookupWord(word: string) {
  const w = word.trim();
  if (!w) return;
  activeWord.value = w;
  const cached = lookupCache.get(w);
  if (cached) {
    // 刷新到队尾 (LRU)
    lookupCache.delete(w);
    lookupCache.set(w, cached);
    wordEntry.value = cached;
    wordLoading.value = false;
    showWordModal.value = true;
    return;
  }
  const seq = ++lookupSeq;
  wordEntry.value = null;
  showWordModal.value = true;
  wordLoading.value = true;
  const { data: resp, error } = await fetchLookupWord(w);
  if (seq !== lookupSeq) return; // 已被更新的点击取代
  wordLoading.value = false;
  if (error || !resp) {
    message.error($t("page.ai.reader.lookupFailed"));
    showWordModal.value = false;
    return;
  }
  wordEntry.value = resp;
  lookupCache.set(w, resp);
  if (lookupCache.size > LOOKUP_CACHE_MAX) {
    const oldest = lookupCache.keys().next().value;
    if (oldest) lookupCache.delete(oldest);
  }
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
    message.error($t("page.ai.reader.translateFailed"));
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
    message.error((error as any)?.message || $t("page.ai.reader.addFailed"));
    return;
  }
  message.success($t("page.ai.reader.addedToVocabulary"));
  if (wordEntry.value) {
    wordEntry.value.inVocabulary = true;
    // 同步缓存 (下次点同一个词展示的也是已收藏态)
    lookupCache.set(wordEntry.value.word, { ...wordEntry.value });
  }
  emit("wordAdded", wordEntry.value.word);
}

defineExpose({
  lookupWord,
  scrollToTop,
  // 切视图 (对照/译文/改写) 重建 DOM 前由外层调用: 旧朗读队列节点已脱离文档, 继续读会错位
  stopSpeaking,
  // 查词/翻译弹窗是否开着 (外层 j/k 快捷键守卫用, 避免弹窗底下悄悄切文章)
  isModalOpen: () => showWordModal.value || showTranslateModal.value,
  // 小窗模式: 最小化前读、恢复后写, 续读位置不丢
  getScrollTop: () => scrollRef.value?.scrollTop ?? 0,
  // 有 progress 读数时按比例恢复 (字号/设备变化导致总高不同也能落回相对位置), 否则用绝对值。
  // 恢复瞬间图片 (BBC 图床等) 往往还没加载完、总高与快照时不同, 图片就绪后再重定位一次;
  // 等待期间用户手动滚动 (滚轮/触摸) 则视为接管, 不再重定位
  setScrollTop: (top: number, progress = 0) => {
    nextTick(() => {
      const el = scrollRef.value;
      if (!el) return;
      const apply = () => {
        const max = el.scrollHeight - el.clientHeight;
        el.scrollTop = progress > 0 && max > 10 ? Math.round((max * Math.min(100, progress)) / 100) : top;
      };
      apply();
      const imgs = Array.from(el.querySelectorAll("img")).filter((im) => !im.complete);
      if (imgs.length === 0) return;
      let touched = false;
      const markTouched = () => {
        touched = true;
      };
      el.addEventListener("wheel", markTouched, { once: true, capture: true });
      el.addEventListener("touchmove", markTouched, { once: true, capture: true });
      let settled = false;
      const resettle = () => {
        if (settled || touched) {
          el.removeEventListener("wheel", markTouched, { capture: true });
          el.removeEventListener("touchmove", markTouched, { capture: true });
          return;
        }
        settled = true;
        el.removeEventListener("wheel", markTouched, { capture: true });
        el.removeEventListener("touchmove", markTouched, { capture: true });
        apply();
      };
      const timer = window.setTimeout(resettle, 4000);
      let pending = imgs.length;
      const one = () => {
        pending -= 1;
        if (pending <= 0) {
          window.clearTimeout(timer);
          resettle();
        }
      };
      imgs.forEach((im) => {
        im.addEventListener("load", one, { once: true });
        im.addEventListener("error", one, { once: true });
      });
    });
  },
  getReadProgress: () => readProgress.value
});

// ===================== 全文朗读 (逐块高亮跟随, 统一走 tts.ts) =====================
const speaking = ref(false);
let speakQueue: HTMLElement[] = [];
let speakIndex = 0;

const contentLang = () => detectChunkLang((scrollRef.value?.textContent || "").slice(0, 2000), "en-US");

const clearSpeakingHighlight = () => {
  scrollRef.value?.querySelectorAll(".reader-speaking").forEach((el) => el.classList.remove("reader-speaking"));
};

const speakNext = async () => {
  if (!speaking.value || speakIndex >= speakQueue.length) {
    stopSpeaking();
    return;
  }
  const el = speakQueue[speakIndex];
  clearSpeakingHighlight();
  el.classList.add("reader-speaking");
  el.scrollIntoView({ behavior: "smooth", block: "center" });
  try {
    await speak(el.textContent || "", { lang: contentLang(), rate: readerSpeechRate.value });
  } catch (err: any) {
    console.error("朗读失败:", err);
    // 中断原因必须可见: 音频加载失败 (如生产 CSP 拦截) 时静默停止会被当成"读一句就停"
    message.error($t("page.ai.reader.speakInterrupted", { msg: err?.message || $t("page.ai.reader.unknownError") }));
    stopSpeaking();
    return;
  }
  // 播放结束/被取消后若已停止朗读则不再继续
  if (!speaking.value) return;
  speakIndex += 1;
  speakNext();
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
    message.info($t("page.ai.reader.speakEmpty"));
    return;
  }
  speakQueue = blocks;
  speakIndex = 0;
  speaking.value = true;
  emit("speakStart");
  speakNext();
};

function stopSpeaking() {
  speaking.value = false;
  stopSpeak();
  clearSpeakingHighlight();
}

// 标题变化 (切文章) 时停止朗读
watch(
  () => props.title,
  () => {
    stopSpeaking();
  }
);

// ===================== 阅读时长心跳 (30s 一次; 页面可见且组件活跃时才计) =====================
let heartbeatTimer: number | null = null;

const startHeartbeat = () => {
  if (heartbeatTimer !== null) return;
  heartbeatTimer = window.setInterval(() => {
    // 标题非空视为正在阅读 (弹窗关闭时 RSS 场景组件会被 v-if 销毁, 不用额外判断);
    // 标签页切到后台不计入, 避免挂机虚增时长
    if (props.title && !document.hidden) {
      fetchReadingHeartbeat(30);
    }
  }, 30_000);
};

const stopHeartbeat = () => {
  if (heartbeatTimer !== null) {
    window.clearInterval(heartbeatTimer);
    heartbeatTimer = null;
  }
};

startHeartbeat();
// KeepAlive 失活 (切 tab/路由) 时组件仍存活但不可见, 心跳要停——
// 否则与另一个存活中的阅读器 (如 RSS 弹窗 + 分级阅读页) 双计时长
onActivated(startHeartbeat);
onDeactivated(stopHeartbeat);

onBeforeUnmount(() => {
  stopSpeaking();
  stopHeartbeat();
});

const contentStyle = computed(() => ({
  fontSize: `${readerFontSize.value}px`,
  lineHeight: String(readerLineHeightValue.value),
  fontFamily: readerSerif.value ? 'Georgia, "Times New Roman", "Songti SC", serif' : undefined
}));
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
          :title="speaking ? $t('page.ai.reader.stopSpeakTitle') : $t('page.ai.reader.speakTitle')"
          :aria-label="speaking ? $t('page.ai.reader.stopSpeakTitle') : $t('page.ai.reader.speakTitle')"
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
          :title="$t('page.ai.reader.fontSizeDown')"
          :aria-label="$t('page.ai.reader.fontSizeDown')"
          @click="setReaderFontSize(readerFontSize - 1)"
        >
          A-
        </NButton>
        <span class="text-11px text-gray-400 w-6 text-center tabular-nums" aria-live="polite">{{ readerFontSize }}</span>
        <NButton
          quaternary
          size="tiny"
          class="!px-1.5"
          :title="$t('page.ai.reader.fontSizeUp')"
          :aria-label="$t('page.ai.reader.fontSizeUp')"
          @click="setReaderFontSize(readerFontSize + 1)"
        >
          A+
        </NButton>
        <!-- 排版设置: 行距 / 衬线 / 朗读语速 (偏好持久化, 双页共享) -->
        <NPopover trigger="click" :show-arrow="false">
          <template #trigger>
            <NButton
              quaternary
              circle
              size="small"
              :title="$t('page.ai.reader.typography')"
              :aria-label="$t('page.ai.reader.typography')"
            >
              <template #icon>
                <SvgIcon icon="mdi:format-line-spacing" />
              </template>
            </NButton>
          </template>
          <div class="flex flex-col gap-4 w-56 p-1">
            <div>
              <div class="text-xs text-gray-400 mb-1.5">{{ $t('page.ai.reader.lineHeight') }}</div>
              <NButtonGroup size="tiny" class="w-full">
                <NButton
                  size="tiny"
                  class="flex-1"
                  :type="readerLineHeight === 'compact' ? 'primary' : 'default'"
                  @click="setReaderLineHeight('compact')"
                >
                  {{ $t('page.ai.reader.lineHeightCompact') }}
                </NButton>
                <NButton
                  size="tiny"
                  class="flex-1"
                  :type="readerLineHeight === 'normal' ? 'primary' : 'default'"
                  @click="setReaderLineHeight('normal')"
                >
                  {{ $t('page.ai.reader.lineHeightNormal') }}
                </NButton>
                <NButton
                  size="tiny"
                  class="flex-1"
                  :type="readerLineHeight === 'loose' ? 'primary' : 'default'"
                  @click="setReaderLineHeight('loose')"
                >
                  {{ $t('page.ai.reader.lineHeightLoose') }}
                </NButton>
              </NButtonGroup>
            </div>
            <div class="flex items-center justify-between">
              <span class="text-xs text-gray-400">{{ $t('page.ai.reader.serifFont') }}</span>
              <NSwitch :value="readerSerif" size="small" @update:value="(v: boolean) => setReaderSerif(v)" />
            </div>
            <div>
              <div class="text-xs text-gray-400 mb-1.5 flex items-center justify-between">
                <span>{{ $t('page.ai.reader.speechRate') }}</span>
                <span class="text-primary tabular-nums">{{ readerSpeechRate.toFixed(2) }}x</span>
              </div>
              <NSlider
                :value="readerSpeechRate"
                :min="0.6"
                :max="1.5"
                :step="0.05"
                :format-tooltip="(v: number) => `${v.toFixed(2)}x`"
                @update:value="(v: number) => setReaderSpeechRate(v)"
              />
            </div>
          </div>
        </NPopover>
        <NButton
          quaternary
          circle
          size="small"
          class="hidden sm:inline-flex"
          :title="readerWidth === 'cozy' ? $t('page.ai.reader.toggleWideTitle') : $t('page.ai.reader.toggleCozyTitle')"
          :aria-label="readerWidth === 'cozy' ? $t('page.ai.reader.toggleWideTitle') : $t('page.ai.reader.toggleCozyTitle')"
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
            {{ $t('page.ai.reader.inVocabulary') }}<span v-if="(wordEntry.encounters || 0) > 1" class="ml-1 text-xs opacity-70">· {{ $t('page.ai.reader.encounters', { count: wordEntry.encounters }) }}</span>
          </NButton>
          <NButton v-else type="primary" block :loading="addingWord" @click="handleAddToVocabulary">{{ $t('page.ai.reader.addToVocabulary') }}</NButton>
        </div>
      </NSpin>
    </NModal>

    <!-- 划词翻译弹窗 -->
    <NModal v-model:show="showTranslateModal" preset="card" :title="$t('page.ai.reader.translate')" :style="{ width: 'min(92vw, 480px)' }">
      <div class="flex flex-col gap-3 py-2">
        <div class="text-sm text-gray-600 dark:text-gray-300 bg-gray-50 dark:bg-gray-800 rounded-lg p-3 max-h-40 overflow-auto">
          {{ translateSource }}
        </div>
        <NSpin :show="translating">
          <div v-if="translateResult" class="text-15px leading-relaxed">{{ translateResult }}</div>
          <div v-else class="text-xs text-gray-400 py-2">{{ $t('page.ai.reader.translating') }}</div>
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
