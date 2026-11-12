<script setup lang="ts">
import { computed, nextTick, onActivated, onBeforeUnmount, onDeactivated, onMounted, ref, watch } from "vue";
import { NAlert, NButton, NButtonGroup, NEmpty, NTag, useMessage } from "naive-ui";
import { useRouterPush } from "@/hooks/common/router";
import { useAsyncJobPolling } from "@/hooks/common/async-job-poll";
import { useReaderPreferences } from "@/hooks/common/reader-preferences";
import ReaderView from "@/components/custom/reader-view.vue";
import DifficultWordsAside from "@/components/custom/difficult-words-aside.vue";
import { fetchRSSItemContent, fetchRSSMarkRead, fetchRSSImportItem, fetchRSSAnnotateItem, fetchRSSTranslateItem, fetchRSSRewriteItem } from "@/service/api";
import type { DifficultWord, RSSItem, RSSSource } from "@/service/api";
import { useReaderSessionStore } from "@/store/modules/reader-session";
import { $t } from "@/locales";
import { avatarColor, relTime, sourceInitial as initialOf, sourceTitle as titleOf } from "../shared";

/**
 * RssViewer RSS 阅读弹窗内容 (ReaderView 之上叠加条目业务):
 * 抓正文 + 三态对照翻译 + 按水平改写 + 生词提取高亮 + j/k 上下篇导航。
 * 挂载于列表页 NModal (v-if 随弹窗开合, 关闭即卸载并停掉全部轮询/监听),
 * 开合与全局小窗会话由容器页 (rss/index.vue) 管理, 组件只上报意图。
 */
defineOptions({
  name: "RssViewer"
});

const props = defineProps<{
  /** 当前条目 (列表行对象; j/k 导航通过 update:item 换行) */
  item: RSSItem;
  /** 当前列表 (上一篇/下一篇导航) */
  items: RSSItem[];
  /** 订阅源 (标题/头像) */
  sources: RSSSource[];
  /** 小窗续读位置 (随条目一起设置, 正文渲染完成后消费) */
  restore?: { scrollTop: number; progress: number } | null;
}>();

const emit = defineEmits<{
  "update:item": [item: RSSItem];
  close: [];
  minimize: [];
}>();

const message = useMessage();
const { routerPushByKey } = useRouterPush();
const sessionStore = useReaderSessionStore();
// 对照/译文/原文三态记忆: 打开已有译文的条目时套用上次选择 (watch 在 viewMode 声明后注册)
const { bilingualMode: prefBilingualMode, setBilingualMode } = useReaderPreferences();

const sourceTitle = (sourceId: number) => titleOf(props.sources, sourceId);
const sourceInitial = (sourceId: number) => initialOf(props.sources, sourceId);

// ===================== 正文加载 =====================
const viewLoading = ref(false);
const viewItem = ref<RSSItem | null>(null);
const viewContent = ref("");
const viewHtml = ref("");
const viewError = ref("");
const importing = ref(false);
const readerRef = ref<InstanceType<typeof ReaderView> | null>(null);

// ===================== 生词提取 =====================
const viewDifficultWords = ref<DifficultWord[]>([]);

const parseDifficultWords = (json?: string): DifficultWord[] => {
  if (!json) return [];
  try {
    const arr = JSON.parse(json);
    return Array.isArray(arr) ? arr : [];
  } catch {
    return [];
  }
};

// 正文生词高亮: 遍历 .article-html 文本节点, 命中词包 <mark class="rss-dw">
const applyHighlight = () => {
  nextTick(() => {
    const root = document.querySelector(".article-html");
    if (!root || viewDifficultWords.value.length === 0) return;
    const words = new Set(viewDifficultWords.value.map((w) => w.word.toLowerCase()));
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    const nodes: Text[] = [];
    while (walker.nextNode()) nodes.push(walker.currentNode as Text);
    const re = /[A-Za-z][A-Za-z'-]*/g;
    for (const node of nodes) {
      const text = node.data;
      if (!/[A-Za-z]/.test(text)) continue;
      const frag = document.createDocumentFragment();
      let last = 0;
      let m: RegExpExecArray | null;
      re.lastIndex = 0;
      while ((m = re.exec(text)) !== null) {
        if (words.has(m[0].toLowerCase())) {
          frag.appendChild(document.createTextNode(text.slice(last, m.index)));
          const mark = document.createElement("mark");
          mark.className = "rss-dw";
          mark.textContent = m[0];
          frag.appendChild(mark);
          last = m.index + m[0].length;
        }
      }
      if (last === 0) continue;
      frag.appendChild(document.createTextNode(text.slice(last)));
      node.parentNode?.replaceChild(frag, node);
    }
  });
};

// feed 正文常自带与文章标题重复的 H1/H2, 阅读视图下去重 (内容已经服务端净化, 这里只删不加)。
// 双语 HTML 里被删标题若已生成译文 (.rss-zh 兄弟块), 一并删掉避免残留孤儿译文
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
        const zhSibling = first.nextElementSibling;
        first.remove();
        if (zhSibling?.classList.contains("rss-zh")) zhSibling.remove();
        return doc.body.innerHTML;
      }
    }
  } catch {
    // 解析失败按原文渲染
  }
  return html;
};

/**
 * 净化 HTML 与纯文本双轨择优: 部分原文页 (BBC 弹墙/重 JS 布局) 提取出的净化 HTML
 * 只剩只言片语, 而纯文本版本有完整正文。若 HTML 可见文本明显少于纯文本,
 * 回退纯文本分段渲染 (朗读的段落选择器也依赖 <p> 结构), 避免正文缺失/朗读无段可读。
 */
function pickViewHtml(html: string, text: string): string {
  if (!html) return "";
  const htmlText = html.replace(/<[^>]+>/g, " ").replace(/\s+/g, " ").trim();
  if (!htmlText) return "";
  const textNorm = text.replace(/\s+/g, " ").trim();
  if (textNorm.length > 200 && htmlText.length < textNorm.length * 0.5) return "";
  return html;
}

/**
 * v-html 归一化: 净化 HTML 剥掉 div/span 等结构标签后, 部分正文以裸文本节点存在
 * (段落 <p> 之外), 显示无段落间距且朗读选择器 (.reader-content p 等) 取不到。
 * 把 .article-html 顶层的裸文本连同相邻行内元素包进 <p>, <br> 作为分段点。幂等。
 */
function normalizeArticleHtml() {
  const container = document.querySelector(".article-html");
  if (!container) return;
  // 图片懒加载: 视口外的图不与首屏争带宽 (BBC 图床慢)
  container.querySelectorAll("img").forEach(im => {
    im.loading = "lazy";
    im.decoding = "async";
  });
  const INLINE = new Set([
    "A", "STRONG", "B", "EM", "I", "U", "S", "DEL", "INS", "SMALL", "MARK",
    "SUB", "SUP", "CODE", "KBD", "BR", "IMG"
  ]);
  let run: Node[] = [];
  let hasText = false;
  const wrapRun = () => {
    if (!hasText || run.length === 0) {
      run = [];
      hasText = false;
      return;
    }
    const p = document.createElement("p");
    container.insertBefore(p, run[0]);
    run.forEach(n => p.appendChild(n));
    run = [];
    hasText = false;
  };
  Array.from(container.childNodes).forEach(node => {
    if (node.nodeType === Node.TEXT_NODE) {
      if ((node.textContent || "").trim()) {
        run.push(node);
        hasText = true;
      } else if (run.length) {
        run.push(node);
      }
      return;
    }
    if (node.nodeType === Node.ELEMENT_NODE && INLINE.has((node as Element).tagName)) {
      run.push(node);
      if ((node as Element).tagName === "BR") wrapRun();
      return;
    }
    wrapRun();
  });
  wrapRun();
}

// ===================== 中英对照 (段落级异步翻译, 三态: 对照/仅译文/仅原文) =====================
// 后端把译文 <div class="rss-zh"> 插回原文树、已翻译原文块带 class="rss-src" 标记,
// "仅译文"靠容器 class 隐藏 .rss-src 实现 (图片等未翻译块保持可见)
type BilingualMode = "bi" | "zh" | "en";
const viewMode = ref<BilingualMode>("en");
// 三态切换写入偏好, 下次打开带译文的条目直接套用
watch(viewMode, (v) => {
  setBilingualMode(v);
});
const viewBilingualHtml = ref("");
const hasBilingual = computed(() => !!viewItem.value?.bilingualHtml);
const translationStale = computed(() => !!viewItem.value?.translationStale);

// 展示 HTML: 改写视图 > 对照/仅译文 > 原始净化 HTML (高亮与段落归一化只对原文视图做过)
const viewSource = ref<"original" | "rewrite">("original");
const viewRewriteHtml = ref("");
const hasRewrite = computed(() => !!viewItem.value?.rewriteHtml);
const rewriteStale = computed(() => !!viewItem.value?.rewriteStale);
const viewDisplayHtml = computed(() => {
  if (viewSource.value === "rewrite" && viewRewriteHtml.value) return viewRewriteHtml.value;
  if (hasBilingual.value && viewMode.value !== "en") return viewBilingualHtml.value;
  return viewHtml.value;
});
const articleClass = computed(() => (hasBilingual.value && viewSource.value === "original" ? [`rss-mode-${viewMode.value}`] : []));

const refreshBilingual = async () => {
  const itemId = viewItem.value?.id;
  if (!itemId) return;
  const { data: fresh } = await fetchRSSItemContent(itemId);
  if (!fresh) return;
  viewItem.value = fresh;
  viewBilingualHtml.value = stripDuplicateHeading(fresh.bilingualHtml || "", fresh.title);
  if (fresh.bilingualHtml) viewMode.value = "bi";
};

const { polling: translating, progress: translateProgress, start: startTranslatePoll, stop: stopTranslatePoll } = useAsyncJobPolling({
  failTitle: () => $t("page.ai.rss.translateFailTitle"),
  onDone: async () => {
    await refreshBilingual();
    message.success($t("page.ai.rss.translateDone"));
  }
});

const handleTranslateItem = async () => {
  const itemId = viewItem.value?.id;
  if (!itemId || translating.value) return;
  translating.value = true;
  translateProgress.value = "";
  const { data: job, error } = await fetchRSSTranslateItem(itemId);
  if (viewItem.value?.id !== itemId) return; // 已切到其他条目, 丢弃过期响应
  if (error || !job) {
    translating.value = false;
    message.error($t("page.ai.rss.translateStartFailed", { msg: (error as any)?.message || $t("page.ai.reader.unknownError") }));
    return;
  }
  if (job.status === "done") {
    // 已有未过期译文 (或同一条目任务刚完成): 直接刷新切对照视图
    translating.value = false;
    await refreshBilingual();
    return;
  }
  startTranslatePoll(job.id);
};

// ===================== 分级改写 (按用户档位, 异步任务) =====================
const refreshRewrite = async () => {
  const itemId = viewItem.value?.id;
  if (!itemId) return;
  const { data: fresh } = await fetchRSSItemContent(itemId);
  if (!fresh) return;
  viewItem.value = fresh;
  viewRewriteHtml.value = stripDuplicateHeading(fresh.rewriteHtml || "", fresh.title);
  if (fresh.rewriteHtml) viewSource.value = "rewrite";
};

const { polling: rewriting, progress: rewriteProgress, start: startRewritePoll, stop: stopRewritePoll } = useAsyncJobPolling({
  failTitle: () => $t("page.ai.rss.rewriteFailTitle"),
  onDone: async () => {
    await refreshRewrite();
    message.success($t("page.ai.rss.rewriteDone", { level: viewItem.value?.rewriteLevel || "" }));
  }
});

const handleRewriteItem = async () => {
  const itemId = viewItem.value?.id;
  if (!itemId || rewriting.value) return;
  rewriting.value = true;
  rewriteProgress.value = "";
  const { data: job, error } = await fetchRSSRewriteItem(itemId);
  if (viewItem.value?.id !== itemId) return; // 已切到其他条目, 丢弃过期响应
  if (error || !job) {
    rewriting.value = false;
    message.error($t("page.ai.rss.rewriteStartFailed", { msg: (error as any)?.message || $t("page.ai.reader.unknownError") }));
    return;
  }
  if (job.status === "done") {
    rewriting.value = false;
    await refreshRewrite();
    return;
  }
  startRewritePoll(job.id);
};

// ===================== 生词提取 (异步任务) =====================
const refreshAnnotated = async () => {
  const itemId = viewItem.value?.id;
  if (!itemId) return;
  const { data: fresh } = await fetchRSSItemContent(itemId);
  if (fresh) {
    viewDifficultWords.value = parseDifficultWords(fresh.difficultWords);
    applyHighlight();
  }
  message.success($t("page.ai.rss.extractDone", { count: viewDifficultWords.value.length }));
};

const { polling: annotatingItem, start: startAnnotatePoll, stop: stopAnnotatePoll } = useAsyncJobPolling({
  failTitle: () => $t("page.ai.rss.extractFailTitle"),
  onDone: refreshAnnotated
});

const handleAnnotateItem = async () => {
  const itemId = viewItem.value?.id;
  if (!itemId || annotatingItem.value) return;
  if (viewDifficultWords.value.length > 0) {
    message.info("本文已提取过生词");
    return;
  }
  annotatingItem.value = true;
  const { data: job, error } = await fetchRSSAnnotateItem(itemId);
  if (viewItem.value?.id !== itemId) return; // 已切到其他条目, 丢弃过期响应
  if (error || !job) {
    annotatingItem.value = false;
    message.error($t("page.ai.rss.extractStartFailed", { msg: (error as any)?.message || $t("page.ai.reader.unknownError") }));
    return;
  }
  if (job.status === "done") {
    // 已有缓存: 直接刷新
    const { data: fresh } = await fetchRSSItemContent(itemId);
    if (fresh) viewDifficultWords.value = parseDifficultWords(fresh.difficultWords);
    annotatingItem.value = false;
    applyHighlight();
    return;
  }
  startAnnotatePoll(job.id);
};

// 模式切回"仅原文"时 DOM 重建, 生词高亮与段落归一化需要重做; 切改写视图同样重建 DOM。
// 朗读队列持有的是旧 DOM 节点, 继续读会高亮消失/内容错位 → 切换前先停朗读
watch([viewMode, viewSource], ([, src]) => {
  readerRef.value?.stopSpeaking();
  nextTick(() => {
    if (src === "original" && viewMode.value === "en") normalizeArticleHtml();
    applyHighlight();
  });
});

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

// ===================== j/k 切换上一篇/下一篇 (与 Folo 一致; 不劫持方向键以免干扰滚动) =====================
const viewIndex = computed(() => props.items.findIndex((i) => i.id === viewItem.value?.id));
const prevItem = computed(() => (viewIndex.value > 0 ? props.items[viewIndex.value - 1] : null));
const nextItem = computed(() =>
  viewIndex.value >= 0 && viewIndex.value < props.items.length - 1 ? props.items[viewIndex.value + 1] : null
);

const navTo = (dir: -1 | 1) => {
  const target = dir === -1 ? prevItem.value : nextItem.value;
  if (target) emit("update:item", target);
};

const handleReaderKey = (e: KeyboardEvent) => {
  if (viewLoading.value) return;
  // 带修饰键的组合 (Ctrl+J / Cmd+K 等) 不劫持; 查词/翻译弹窗开着时不切文章 (弹窗底下换文会错位)
  if (e.ctrlKey || e.metaKey || e.altKey) return;
  if (readerRef.value?.isModalOpen()) return;
  const target = e.target as HTMLElement | null;
  if (target && ["INPUT", "TEXTAREA"].includes(target.tagName)) return;
  if (e.key === "j") navTo(1);
  else if (e.key === "k") navTo(-1);
};

onMounted(() => {
  window.addEventListener("keydown", handleReaderKey);
});

onBeforeUnmount(() => {
  window.removeEventListener("keydown", handleReaderKey);
});

// KeepAlive 失活 (路由切走但弹窗未关) 时摘除监听, 按键不再后台拉文章
onActivated(() => {
  window.addEventListener("keydown", handleReaderKey);
});

onDeactivated(() => {
  window.removeEventListener("keydown", handleReaderKey);
});

// ===================== 生词侧栏: 点击查词; 点词弹窗加词后同步侧栏状态 =====================
const lookupDw = (word: string) => {
  readerRef.value?.lookupWord(word);
};

const onWordAdded = (word: string) => {
  const hit = viewDifficultWords.value.find((w) => w.word.toLowerCase() === word.toLowerCase());
  if (hit) hit.known = true;
};

async function handleImport(item: RSSItem) {
  importing.value = true;
  const { data: resp, error } = await fetchRSSImportItem(item.id);
  importing.value = false;
  if (error || !resp) {
    message.error($t("page.ai.rss.importFailed"));
    return;
  }
  message.success($t("page.ai.rss.importSuccess", { title: resp.title }));
  item.status = "imported";
  item.importedDocId = resp.id;
  if (viewItem.value?.id === item.id) {
    viewItem.value = { ...viewItem.value, status: "imported", importedDocId: resp.id };
  }
}

// 移动端下拉选项: 打开原文 / 导入(或去标注) / 最小化
const moreOptions = computed(() => {
  const opts: Array<{ key: string; label: string }> = [];
  if (viewItem.value?.link) opts.push({ key: "open", label: $t("page.ai.rss.openOriginal") });
  if (viewItem.value && viewItem.value.status !== "imported") opts.push({ key: "import", label: $t("page.ai.rss.importToReading") });
  else if (viewItem.value?.importedDocId) opts.push({ key: "annotate", label: $t("page.ai.rss.goToAnnotate") });
  opts.push({ key: "minimize", label: $t("page.ai.reader.minimizeTitle") });
  return opts;
});

const onMoreSelect = (key: string) => {
  if (key === "open" && viewItem.value?.link) {
    window.open(viewItem.value.link, "_blank", "noopener,noreferrer");
  } else if (key === "import" && viewItem.value) {
    handleImport(viewItem.value);
  } else if (key === "annotate" && viewItem.value?.importedDocId) {
    routerPushByKey("ai_reading-detail", { params: { id: String(viewItem.value.importedDocId) } });
  } else if (key === "minimize") {
    emit("minimize");
  }
};

// ===================== 条目加载 (列表打开 / j/k 切换 / 小窗恢复 都走这里) =====================
const loadItem = async (item: RSSItem) => {
  // 切换条目: 停掉上一条目的慢任务轮询, 避免完成后对当前条目误刷新/误提示
  stopAnnotatePoll();
  stopTranslatePoll();
  stopRewritePoll();
  viewItem.value = item;
  viewContent.value = "";
  viewHtml.value = "";
  viewError.value = "";
  viewDifficultWords.value = [];
  viewBilingualHtml.value = "";
  viewMode.value = "en";
  viewSource.value = "original";
  viewRewriteHtml.value = "";
  // 打开新条目即丢弃当前 RSS 小窗会话 (与旧 miniMode 语义一致; 分级阅读的会话不受影响)
  if (sessionStore.session?.kind === "rss") {
    sessionStore.dismiss();
  }
  readerRef.value?.scrollToTop();
  if (item.status === "unread") {
    item.status = "read";
    // 同步侧栏未读数
    const src = props.sources.find((s) => s.id === item.sourceId);
    if (src && src.unread) src.unread -= 1;
    fetchRSSMarkRead(item.id);
  }
  viewLoading.value = true;
  const { data: resp, error } = await fetchRSSItemContent(item.id);
  if (viewItem.value?.id !== item.id) return; // 已切到其他条目, 丢弃过期响应
  viewLoading.value = false;
  if (error || !resp) {
    // 抓取失败: 展示失败原因 + feed 摘要兜底
    viewError.value = (error as any)?.message || "未知错误";
    viewContent.value = item.summary || "";
    return;
  }
  viewContent.value = resp.content;
  viewHtml.value = pickViewHtml(stripDuplicateHeading(resp.contentHtml || "", resp.title), resp.content);
  if (!viewContent.value && !viewHtml.value && item.summary) {
    // 原文页没提取到正文 (纯 JS 渲染等): 用 feed 摘要兜底 (后端已随响应附带净化摘要 HTML)
    viewError.value = "原文页未提取到正文";
    viewContent.value = item.summary;
    viewHtml.value = stripDuplicateHeading(resp.contentHtml || "", resp.title);
  }
  viewItem.value = resp;
  // 之前提取过的生词直接展示并高亮; 已有对照译文/改写则预填 (默认仍显示原文)
  viewDifficultWords.value = parseDifficultWords(resp.difficultWords);
  if (resp.rewriteHtml) {
    viewRewriteHtml.value = stripDuplicateHeading(resp.rewriteHtml, resp.title);
  }
  if (resp.bilingualHtml) {
    viewBilingualHtml.value = stripDuplicateHeading(resp.bilingualHtml, resp.title);
    // 套用记忆的对照/译文/原文偏好
    viewMode.value = prefBilingualMode.value;
  }
  await nextTick();
  // 结构标签剥壳产生的裸文本包进 <p>: 朗读按段落元素取块, 裸文本会被整体跳过
  normalizeArticleHtml();
  applyHighlight();
  // 全局小窗恢复: 内容渲染完成后写回续读位置 (优先按比例, 字号/设备变化不漂移)
  if (props.restore && props.restore.scrollTop > 0) {
    readerRef.value?.setScrollTop(props.restore.scrollTop, props.restore.progress);
  }
};

watch(
  () => props.item,
  (item) => {
    loadItem(item);
  },
  { immediate: true }
);

// 快照当前阅读会话 (位置/进度 + 条目信息), 供容器页组装全局小窗; 弹窗关闭时组件已卸载不会被调用
function getSessionSnapshot(): { itemId: number; title: string; subtitle: string; scrollTop: number; progress: number } | null {
  if (!viewItem.value) return null;
  let scrollTop = 0;
  let progress = 0;
  try {
    scrollTop = readerRef.value?.getScrollTop() || 0;
    progress = readerRef.value?.getReadProgress() || 0;
  } catch {
    // 阅读器实例不可用时按 0 处理
  }
  return {
    itemId: viewItem.value.id,
    title: viewItem.value.title,
    subtitle: sourceTitle(viewItem.value.sourceId),
    scrollTop,
    progress
  };
}

defineExpose({
  getSessionSnapshot
});
</script>

<template>
  <ReaderView
    ref="readerRef"
    :title="viewItem?.title || ''"
    :loading="viewLoading"
    source-kind="rss"
    :source-ref="viewItem?.id || 0"
    @word-added="onWordAdded"
  >
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
        <span class="text-13px text-gray-400 shrink-0">{{ $t('page.ai.rss.minutes', { count: viewStats.minutes }) }}</span>
      </template>
    </template>
    <template #toolbar-actions>
      <NButton
        size="tiny"
        class="mr-1"
        :loading="translating"
        :title="translateProgress || (translationStale ? $t('page.ai.rss.bilingualStaleTitle') : $t('page.ai.rss.bilingualTitle'))"
        @click="handleTranslateItem"
      >
        <template #icon>
          <SvgIcon icon="mdi:translate" />
        </template>
        <span class="hidden sm:inline">{{ translationStale ? $t('page.ai.rss.reBilingual') : $t('page.ai.rss.bilingual') }}</span>
      </NButton>
      <!-- 任务进度可见化: 移动端不靠 hover title 也能看到当前进度 -->
      <span v-if="translating && translateProgress" class="text-11px text-primary mr-1 max-w-30 truncate tabular-nums">
        {{ $t('page.ai.rss.translatingProgress', { progress: translateProgress }) }}
      </span>
      <NButtonGroup v-if="hasBilingual && viewSource === 'original'" size="tiny" class="mr-1">
        <NButton size="tiny" :type="viewMode === 'bi' ? 'primary' : 'default'" @click="viewMode = 'bi'">{{ $t('page.ai.rss.modeBi') }}</NButton>
        <NButton size="tiny" :type="viewMode === 'zh' ? 'primary' : 'default'" @click="viewMode = 'zh'">{{ $t('page.ai.rss.modeZh') }}</NButton>
        <NButton size="tiny" :type="viewMode === 'en' ? 'primary' : 'default'" @click="viewMode = 'en'">{{ $t('page.ai.rss.modeEn') }}</NButton>
      </NButtonGroup>
      <NButton
        size="tiny"
        class="mr-1"
        :loading="rewriting"
        :title="rewriteProgress || (rewriteStale ? $t('page.ai.rss.rewriteStaleTitle') : $t('page.ai.rss.rewriteTitle'))"
        @click="handleRewriteItem"
      >
        <template #icon>
          <SvgIcon icon="mdi:book-open-variant" />
        </template>
        <span class="hidden sm:inline">{{ rewriteStale ? $t('page.ai.rss.reRewrite') : $t('page.ai.rss.rewrite') }}</span>
      </NButton>
      <span v-if="rewriting && rewriteProgress" class="text-11px text-primary mr-1 max-w-30 truncate tabular-nums">
        {{ $t('page.ai.rss.rewritingProgress', { progress: rewriteProgress }) }}
      </span>
      <NButtonGroup v-if="hasRewrite" size="tiny" class="mr-1">
        <NButton size="tiny" :type="viewSource === 'original' ? 'primary' : 'default'" @click="viewSource = 'original'">{{ $t('page.ai.rss.modeEn') }}</NButton>
        <NButton size="tiny" :type="viewSource === 'rewrite' ? 'primary' : 'default'" @click="viewSource = 'rewrite'">
          {{ $t('page.ai.rss.modeRewrite') }}{{ viewItem?.rewriteLevel ? `·${viewItem.rewriteLevel}` : '' }}
        </NButton>
      </NButtonGroup>
      <NButton
        size="tiny"
        class="mr-1"
        :loading="annotatingItem"
        :disabled="viewDifficultWords.length > 0"
        :title="viewDifficultWords.length > 0 ? $t('page.ai.rss.alreadyExtracted') : $t('page.ai.rss.extractTitle')"
        @click="handleAnnotateItem"
      >
        <template #icon>
          <SvgIcon icon="mdi:text-box-search-outline" />
        </template>
        <span v-if="viewDifficultWords.length > 0"> {{ viewDifficultWords.length }}</span>
        <span v-else class="hidden sm:inline">{{ $t('page.ai.rss.extractWords') }}</span>
      </NButton>
      <a v-if="viewItem?.link" :href="viewItem.link" target="_blank" rel="noopener noreferrer" class="hidden sm:inline-flex">
        <NButton quaternary circle size="small" :title="$t('page.ai.rss.openOriginal')">
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
        class="hidden sm:inline-flex"
        :title="$t('page.ai.rss.importToReading')"
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
        class="hidden sm:inline-flex"
        :title="$t('page.ai.rss.goToAnnotate')"
        @click="routerPushByKey('ai_reading-detail', { params: { id: String(viewItem.importedDocId) } })"
      >
        <template #icon>
          <SvgIcon icon="mdi:pencil-outline" />
        </template>
      </NButton>
      <!-- 移动端: 低频操作收进下拉, 缓解工具栏过密 -->
      <NDropdown trigger="click" :options="moreOptions" @select="onMoreSelect">
        <NButton quaternary circle size="small" class="sm:hidden" :title="$t('page.ai.rss.moreActions')" :aria-label="$t('page.ai.rss.moreActions')">
          <template #icon>
            <SvgIcon icon="mdi:dots-vertical" />
          </template>
        </NButton>
      </NDropdown>
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
      <NButton quaternary circle size="small" :title="$t('page.ai.reader.closeTitle')" @click="emit('close')">
        <template #icon>
          <SvgIcon icon="mdi:close" />
        </template>
      </NButton>
    </template>
    <template #meta>
      <span v-if="viewItem?.publishedAt">{{ new Date(viewItem.publishedAt).toLocaleString() }}</span>
      <NTag v-if="viewItem?.status === 'imported'" size="tiny" type="success" :bordered="false">{{ $t('page.ai.rss.importedTag') }}</NTag>
      <span class="hidden md:inline text-gray-300 dark:text-gray-600">
        {{ $t('page.ai.rss.jkHint') }}
      </span>
    </template>

    <NAlert v-if="viewError" type="warning" :bordered="false" class="my-5">
      {{ $t('page.ai.rss.fetchFailed', { msg: viewError }) }}<template v-if="viewHtml || viewerParagraphs.length">{{ $t('page.ai.rss.feedSummaryHint') }}</template>
    </NAlert>
    <!-- 净化 HTML 渲染 (服务端白名单过滤, 含图片/链接/表格); 有对照译文时按三态切换显示 -->
    <!-- eslint-disable-next-line vue/no-v-html -- 内容为服务端白名单净化后的 HTML -->
    <div v-if="viewHtml" class="article-html" :class="articleClass" v-html="viewDisplayHtml"></div>
    <div v-else class="flex flex-col">
      <p v-for="(p, i) in viewerParagraphs" :key="i">{{ p }}</p>
      <NEmpty
        v-if="!viewLoading && viewerParagraphs.length === 0"
        :description="$t('page.ai.rss.noContent')"
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
        <span v-else class="text-13px text-gray-300 dark:text-gray-600">{{ $t('page.ai.rss.noNewer') }}</span>
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
        <span v-else class="text-13px text-gray-300 dark:text-gray-600">{{ $t('page.ai.rss.noLast') }}</span>
      </div>
    </template>

    <!-- 提取出的生词侧栏 (桌面右侧/移动端抽屉, 点击查词, 可一键全部加入生词本) -->
    <template v-if="viewDifficultWords.length > 0" #aside>
      <DifficultWordsAside
        :words="viewDifficultWords"
        :heading="$t('page.ai.reader.thisWords')"
        source-kind="rss"
        :source-ref="viewItem?.id || 0"
        :source-context="viewItem?.title || 'RSS 阅读'"
        @lookup="lookupDw"
      />
    </template>
  </ReaderView>
</template>

<style scoped>
/* 一键提取后的生词高亮 (applyHighlight 注入的 mark 节点) */
.article-html :deep(mark.rss-dw) {
  background: rgba(250, 204, 21, 0.35);
  color: inherit;
  border-radius: 3px;
  padding: 0 2px;
  cursor: pointer;
}
.dark .article-html :deep(mark.rss-dw) {
  background: rgba(250, 204, 21, 0.22);
}

/* 中英对照: 译文块样式 (后端插在原文块后/内的 <div class="rss-zh">) */
.article-html :deep(div.rss-zh) {
  color: #4b5563;
  background: rgba(99, 161, 122, 0.07);
  border-left: 3px solid rgba(99, 161, 122, 0.55);
  border-radius: 0 8px 8px 0;
  padding: 0.4em 1em;
  margin: -0.6em 0 1.5em;
}
.dark .article-html :deep(div.rss-zh) {
  color: #9ca3af;
  background: rgba(99, 161, 122, 0.1);
}
/* pre 块的译文: 与原文一致的等宽换行排版 */
.article-html :deep(div.rss-zh-pre) {
  white-space: pre-wrap;
  font-family: Menlo, Consolas, monospace;
  font-size: 0.85em;
}
/* 三态-仅译文: 隐藏已翻译的原文块 (图片等未翻译块保持可见) */
.article-html.rss-mode-zh :deep(.rss-src) {
  display: none;
}
/* 仅译文模式下 li 内的译文间距修正 (原文隐藏后译文成为 li 唯一内容) */
.article-html.rss-mode-zh :deep(li > div.rss-zh:first-child) {
  margin-top: 0.2em;
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
/* BBC 图床 (ichef.bbci.co.uk) 加载慢 (每张 3-9s), 占位底色避免"空白像挂了" */
.article-html :deep(img) {
  display: block;
  max-width: 100%;
  height: auto;
  margin: 0.75em auto;
  border-radius: 6px;
  background: repeating-linear-gradient(45deg, #f1f1f1, #f1f1f1 12px, #e7e7e7 12px, #e7e7e7 24px);
}
.dark .article-html :deep(img) {
  background: repeating-linear-gradient(45deg, #2a2a2a, #2a2a2a 12px, #333 12px, #333 24px);
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
