<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { useRoute } from "vue-router";
import { useMessage } from "naive-ui";
import {
  fetchGetReadingDoc,
  fetchAnnotateReadingDoc,
  fetchLookupWord,
  fetchAddVocabulary,
} from "@/service/api";
import type { DifficultWord, DictionaryEntry, ReadingDoc } from "@/service/api";
import { speak } from "@/utils/tts";
import { useAppStore } from "@/store/modules/app";

const appStore = useAppStore();
const route = useRoute();
const message = useMessage();

const docId = Number(route.params.id);
const doc = ref<(ReadingDoc & { content: string }) | null>(null);
const difficultWords = ref<DifficultWord[]>([]);
const loading = ref(false);
// 标注是异步任务 (Cloudflare 100s 源站响应上限), annotating 态由文档状态驱动
const annotating = computed(() => doc.value?.status === "annotating");

// 点词查词
const showWordModal = ref(false);
const wordLoading = ref(false);
const activeWord = ref("");
const wordEntry = ref<DictionaryEntry | null>(null);
const addingWord = ref(false);

// 生词集合 (小写) 用于高亮
const difficultSet = computed(() => {
  const set = new Map<string, DifficultWord>();
  difficultWords.value.forEach((w) => set.set(w.word.toLowerCase(), w));
  return set;
});

// 段落渲染: 拆成 [文本, token, 文本...] 保留空白
const paragraphs = computed(() => {
  if (!doc.value) return [];
  return doc.value.content.split(/\n\s*\n/).filter((p) => p.trim() !== "");
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

const loadDoc = async () => {
  loading.value = true;
  const { data: resp, error } = await fetchGetReadingDoc(docId);
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
    const { data: resp, error } = await fetchGetReadingDoc(docId);
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
  const { data: resp, error } = await fetchAnnotateReadingDoc(docId);
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

const handleWordClick = async (word: string) => {
  if (!word) return;
  activeWord.value = word;
  wordEntry.value = null;
  showWordModal.value = true;
  wordLoading.value = true;
  const { data: resp, error } = await fetchLookupWord(word);
  wordLoading.value = false;
  if (error || !resp) {
    message.error("查词失败");
    showWordModal.value = false;
    return;
  }
  wordEntry.value = resp;
};

const handleAddToVocabulary = async () => {
  if (!wordEntry.value) return;
  addingWord.value = true;
  const { error } = await fetchAddVocabulary({
    word: wordEntry.value.word,
    phonetic: wordEntry.value.phonetic,
    definition: wordEntry.value.translation || wordEntry.value.definition,
    example: wordEntry.value.example,
    sourceContext: doc.value?.title || "阅读",
  });
  addingWord.value = false;
  if (error) {
    message.error((error as any)?.message || "添加失败 (可能已存在)");
    return;
  }
  message.success("已加入生词本");
  // 更新本地标注状态
  const hit = difficultWords.value.find((w) => w.word.toLowerCase() === wordEntry.value?.word.toLowerCase());
  if (hit) hit.known = true;
};

onMounted(() => {
  loadDoc();
});

onUnmounted(() => {
  stopAnnotatePolling();
});
</script>

<template>
  <div class="h-full flex-col flex gap-4 overflow-auto" :class="appStore.isMobile ? 'p-2' : 'p-4'">
    <NCard :bordered="false" :shadow="appStore.isMobile ? false : 'sm'" class="flex-1">
      <template #header>
        <div class="flex flex-wrap items-center gap-3">
          <span class="font-bold text-18px">{{ doc?.title || '...' }}</span>
          <NTag v-if="doc" size="small" :bordered="false">{{ doc.wordCount }} 词</NTag>
          <NTag v-if="doc && doc.status === 'annotated'" size="small" type="warning" :bordered="false">
            生词率 {{ Math.round(doc.unknownRate * 1000) / 10 }}%
          </NTag>
        </div>
      </template>
      <template #header-extra>
        <div class="flex gap-2">
          <NButton
            type="primary"
            :loading="annotating"
            :disabled="annotating"
            @click="handleAnnotate"
          >
            {{ annotating ? '标注中...' : (doc?.status === 'annotated' ? '重新标注生词' : 'AI 标注生词') }}
          </NButton>
        </div>
      </template>

      <NSpin :show="loading">
        <div class="flex gap-6" :class="appStore.isMobile ? 'flex-col' : ''">
          <!-- 正文 -->
          <div class="flex-1 flex flex-col gap-4 leading-8 text-16px">
            <div
              v-for="(p, pi) in paragraphs"
              :key="pi"
              class="text-justify"
            >
              <template v-for="(tk, ti) in tokenize(p)" :key="ti">
                <a
                  v-if="tk.isDifficult"
                  class="cursor-pointer rounded px-0.5 font-bold"
                  :class="tk.known ? 'bg-gray-200 dark:bg-gray-700 text-gray-500' : 'bg-warning/20 text-warning hover:bg-warning/40'"
                  @click="handleWordClick(tk.word)"
                >
                  {{ tk.text }}
                </a>
                <span
                  v-else-if="tk.word"
                  class="cursor-pointer hover:text-primary"
                  @click="handleWordClick(tk.word)"
                >
                  {{ tk.text }}
                </span>
                <span v-else>{{ tk.text }}</span>
              </template>
            </div>
            <NEmpty v-if="paragraphs.length === 0 && !loading" description="文档为空" />
            <div class="text-xs text-gray-300">点击任意单词即可查词; 高亮为 AI 标注的生词 (灰色为已在生词本)。</div>
          </div>

          <!-- 生词侧栏 -->
          <div v-if="difficultWords.length > 0 && !appStore.isMobile" class="w-64 shrink-0">
            <div class="font-bold mb-2">标注生词 ({{ difficultWords.length }})</div>
            <div class="flex flex-col gap-2 max-h-[60vh] overflow-auto">
              <div
                v-for="w in difficultWords"
                :key="w.word"
                class="rounded-lg border border-gray-200 dark:border-gray-700 p-2 text-sm"
              >
                <div class="flex items-center justify-between">
                  <a class="cursor-pointer font-bold text-primary" @click="handleWordClick(w.word)">{{ w.word }}</a>
                  <NTag size="tiny" :type="w.known ? 'default' : 'warning'" :bordered="false">
                    {{ w.known ? '已收藏' : '生词' }}
                  </NTag>
                </div>
                <div class="text-xs text-gray-500 mt-1">{{ w.meaning }}</div>
              </div>
            </div>
          </div>
        </div>
      </NSpin>
    </NCard>

    <!-- 查词弹窗 -->
    <NModal v-model:show="showWordModal" preset="card" :title="activeWord" :style="{ width: appStore.isMobile ? '92%' : '440px' }">
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
          <NButton type="primary" block :loading="addingWord" @click="handleAddToVocabulary">加入生词本</NButton>
        </div>
      </NSpin>
    </NModal>
  </div>
</template>

<style scoped></style>
