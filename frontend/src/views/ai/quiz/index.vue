<script setup lang="ts">
import { computed, onActivated, onMounted, onUnmounted, ref } from "vue";
import { useMessage } from "naive-ui";
import {
  fetchCreateQuizSession,
  fetchSubmitQuizAnswer,
  fetchSentencePractice,
  fetchGetVocabularyList,
  fetchGetActiveQuizSession,
} from "@/service/api";
import type { QuizQuestion, QuizSession, QuizType } from "@/service/api";
import { speak } from "@/utils/tts";
import { useAppStore } from "@/store/modules/app";

const appStore = useAppStore();
const message = useMessage();

// ===================== 配置 =====================
const typeOptions: Array<{ value: QuizType; label: string; icon: string }> = [
  { value: "word2def", label: "看词选义", icon: "mdi:book-open-variant" },
  { value: "def2word", label: "看义选词", icon: "mdi:book-alphabet" },
  { value: "cloze", label: "例句挖空", icon: "mdi:text-box-edit-outline" },
  { value: "confusing", label: "易混词辨析", icon: "mdi:shuffle-variant" },
  { value: "context", label: "原文语境", icon: "mdi:file-find-outline" },
  { value: "spell", label: "听音拼写", icon: "mdi:headphones" },
  { value: "translate", label: "中译英", icon: "mdi:translate" },
];
const sourceOptions = [
  { value: "all", label: "全部生词" },
  { value: "due", label: "今日到期" },
  { value: "errorbook", label: "错题重练" },
];

const quizSize = ref(10);
const quizTypes = ref<QuizType[]>(["word2def", "def2word", "cloze", "confusing", "context", "spell", "translate"]);
const quizSource = ref<"all" | "due" | "errorbook">("all");
const creating = ref(false);

const toggleType = (t: string) => {
  const i = quizTypes.value.indexOf(t as QuizType);
  if (i >= 0) {
    if (quizTypes.value.length > 1) quizTypes.value.splice(i, 1);
  } else {
    quizTypes.value.push(t as QuizType);
  }
};

// ===================== 会话运行 =====================
const session = ref<QuizSession | null>(null);
const questions = ref<QuizQuestion[]>([]);
const qIndex = ref(0);
const showSummary = ref(false);
const selectedOption = ref("");
const typedAnswer = ref("");
const submitting = ref(false);
const lastResult = ref<{ correct: boolean; answer: string; recorded: boolean; decayed?: boolean; explanation?: string } | null>(null);
const questionShownAt = ref(0);

const optionLetters = ["A", "B", "C", "D"];

const currentQuestion = computed(() => questions.value[qIndex.value] || null);
const isChoice = computed(() =>
  currentQuestion.value ? ["word2def", "def2word", "cloze", "confusing", "context"].includes(currentQuestion.value.type) : false
);
const isInput = computed(() =>
  currentQuestion.value ? ["spell", "translate"].includes(currentQuestion.value.type) : false
);
// 听例句拼写: prompt 是挖空句 (非退化提示语时展示题面)
const isSentenceSpell = computed(
  () => currentQuestion.value?.type === "spell" && currentQuestion.value.prompt !== "听音拼写单词"
);
const progressPercent = computed(() =>
  session.value ? Math.round((session.value.answered / session.value.total) * 100) : 0
);

// 选项卡片态: 选中 / 判分后正确项 / 判分后错选
const optionClass = (opt: string) => {
  if (!lastResult.value) {
    return selectedOption.value === opt
      ? "border-primary bg-primary/5 shadow-sm"
      : "border-gray-200 dark:border-gray-700 hover:border-primary/60 hover:bg-gray-50 dark:hover:bg-gray-800";
  }
  if (opt === lastResult.value.answer) return "border-success bg-success/10";
  if (opt === selectedOption.value) return "border-error bg-error/10";
  return "border-gray-200 dark:border-gray-700 opacity-60";
};

// 总结: 按题型统计正确率 + 错题回顾 (判分结果随题下发)
const typeStats = computed(() => {
  const map = new Map<string, { total: number; correct: number }>();
  for (const q of questions.value) {
    if (!q.done) continue;
    const s = map.get(q.type) || { total: 0, correct: 0 };
    s.total += 1;
    if (q.correct) s.correct += 1;
    map.set(q.type, s);
  }
  return [...map.entries()].map(([type, s]) => ({
    type,
    label: typeOptions.find((t) => t.value === type)?.label || type,
    ...s
  }));
});
const wrongQuestions = computed(() => questions.value.filter((q) => q.done && !q.correct));

// ===================== 未完成会话恢复 (48h 内) =====================
const resumable = ref<{ session: QuizSession; questions: QuizQuestion[] } | null>(null);

const checkResumable = async () => {
  if (session.value) return;
  const { data: resp } = await fetchGetActiveQuizSession();
  if (resp && resp.session && resp.session.answered < resp.session.total) {
    resumable.value = resp;
  }
};

const dismissResumable = () => {
  resumable.value = null;
};

const onQuestionShown = () => {
  questionShownAt.value = Date.now();
  selectedOption.value = "";
  typedAnswer.value = "";
  lastResult.value = null;
  // 听音拼写: 自动朗读 (有例句读例句, 否则读单词), 不展示答案文本
  if (currentQuestion.value?.type === "spell" && currentQuestion.value.hint) {
    speak(currentQuestion.value.hint, { lang: "en-US", rate: 0.9 }).catch(() => {});
  }
};

const resumeSession = () => {
  if (!resumable.value) return;
  session.value = resumable.value.session;
  questions.value = resumable.value.questions;
  const firstUndone = questions.value.findIndex((q) => !q.done);
  qIndex.value = firstUndone >= 0 ? firstUndone : 0;
  showSummary.value = false;
  resumable.value = null;
  onQuestionShown();
};

const startQuiz = async () => {
  if (quizTypes.value.length === 0) {
    message.warning("请至少选择一种题型");
    return;
  }
  creating.value = true;
  const { data: resp, error } = await fetchCreateQuizSession({
    size: quizSize.value,
    types: quizTypes.value,
    source: quizSource.value,
  });
  creating.value = false;
  if (error || !resp) {
    message.error("创建练习失败");
    return;
  }
  if (resp.questions.length < quizSize.value) {
    message.info(`词汇数据所限, 本轮 ${resp.questions.length} 题`);
  }
  session.value = resp.session;
  questions.value = resp.questions;
  qIndex.value = 0;
  showSummary.value = false;
  lastResult.value = null;
  selectedOption.value = "";
  typedAnswer.value = "";
  resumable.value = null;
  onQuestionShown();
};

const restart = () => {
  session.value = null;
  questions.value = [];
  qIndex.value = 0;
  showSummary.value = false;
};

// 错题重练: 范围切到错题本再开一轮
const retryWrong = () => {
  restart();
  quizSource.value = "errorbook";
  quizTypes.value = ["word2def", "def2word", "cloze", "spell", "context"];
  startQuiz();
};

const submitAnswer = async () => {
  const q = currentQuestion.value;
  if (!q || !session.value || submitting.value || lastResult.value) return;
  const answer = isChoice.value ? selectedOption.value : typedAnswer.value;
  if (!answer.trim()) {
    message.warning("请先作答");
    return;
  }
  submitting.value = true;
  const durationMs = Date.now() - questionShownAt.value;
  const { data: resp, error } = await fetchSubmitQuizAnswer(session.value.id, {
    questionId: q.id,
    answer,
    durationMs,
  });
  submitting.value = false;
  if (error || !resp) {
    message.error("判分失败");
    return;
  }
  lastResult.value = resp.result;
  session.value = resp.session;
  questions.value = resp.questions;
  if (!resp.result.correct) {
    speak(resp.result.answer, { lang: "en-US", rate: 0.9 }).catch(() => {});
  }
};

const nextQuestion = () => {
  if (qIndex.value + 1 >= questions.value.length) {
    showSummary.value = true;
    return;
  }
  qIndex.value += 1;
  onQuestionShown();
};

// ===================== 键盘流: 1-4/a-d 选选项, Enter 提交/下一题 =====================
const handleKeydown = (e: KeyboardEvent) => {
  if (!currentQuestion.value || showSummary.value) return;
  const target = e.target as HTMLElement | null;
  if (target && ["INPUT", "TEXTAREA"].includes(target.tagName)) return;
  if (isChoice.value && !lastResult.value) {
    let idx = -1;
    if (/^[1-4]$/.test(e.key)) idx = Number(e.key) - 1;
    else if (/^[a-dA-D]$/.test(e.key)) idx = "abcd".indexOf(e.key.toLowerCase());
    if (idx >= 0) {
      const opt = currentQuestion.value.options?.[idx];
      if (opt) {
        selectedOption.value = opt;
        e.preventDefault();
      }
      return;
    }
  }
  if (e.key === "Enter") {
    e.preventDefault();
    if (lastResult.value) {
      nextQuestion();
    } else if (isChoice.value && selectedOption.value) {
      submitAnswer();
    }
  }
};

// ===================== 造句批改 =====================
const spWordId = ref<number | null>(null);
const spSentence = ref("");
const spSubmitting = ref(false);
const spResult = ref<{ score: number; correct: boolean; grammar: string[]; improved: string; comment: string; word: string } | null>(null);
const vocabOptions = ref<{ label: string; value: number }[]>([]);
const vocabSearching = ref(false);
const showSentencePractice = ref(false);

// 远程搜索目标词 (大词库不再截断前 200)
const searchVocabOptions = async (keyword: string) => {
  vocabSearching.value = true;
  const { data: resp } = await fetchGetVocabularyList({ keyword: keyword.trim() || undefined, isMastered: false });
  vocabSearching.value = false;
  vocabOptions.value = (resp || []).slice(0, 50).map((v) => ({
    label: `${v.word} (${v.definition?.slice(0, 30) || "无释义"})`,
    value: v.id,
  }));
};

const handleSentencePractice = async () => {
  if (!spWordId.value || !spSentence.value.trim()) {
    message.warning("请选择目标词并输入句子");
    return;
  }
  spSubmitting.value = true;
  spResult.value = null;
  const { data: resp, error } = await fetchSentencePractice({ wordId: spWordId.value, sentence: spSentence.value.trim() });
  spSubmitting.value = false;
  if (error || !resp) {
    message.error("批改失败");
    return;
  }
  spResult.value = resp;
};

onMounted(() => {
  searchVocabOptions("");
  checkResumable();
  window.addEventListener("keydown", handleKeydown);
});

onUnmounted(() => {
  window.removeEventListener("keydown", handleKeydown);
});

onActivated(() => {
  checkResumable();
});
</script>

<template>
  <div class="h-full flex-col flex gap-4 overflow-auto" :class="appStore.isMobile ? 'p-2' : 'p-4'">
    <!-- 未完成会话恢复横幅 -->
    <NAlert v-if="resumable && !session" type="info" :bordered="false">
      <div class="flex items-center justify-between gap-4 w-full flex-wrap">
        <span>有一轮练习进行中 ({{ resumable.session.answered }}/{{ resumable.session.total }} 题), 要继续吗?</span>
        <span class="flex gap-2 shrink-0">
          <NButton size="small" type="primary" @click="resumeSession">继续练习</NButton>
          <NButton size="small" quaternary @click="dismissResumable">忽略</NButton>
        </span>
      </div>
    </NAlert>

    <!-- 配置 -->
    <template v-if="!session">
      <NCard :bordered="false" :shadow="appStore.isMobile ? false : 'sm'" class="flex-1">
        <div class="flex flex-col items-center gap-8 py-4 md:py-8 max-w-3xl mx-auto w-full">
          <div class="text-center">
            <div class="text-2xl font-bold">智能练习</div>
            <div class="text-sm text-gray-400 mt-1">答题计入 FSRS 复习调度 · 答错进错题本 · 答对自动消错</div>
          </div>

          <!-- 题型: 可点选标签块 -->
          <div class="w-full">
            <div class="text-sm font-bold mb-3 text-center">选择题型</div>
            <div class="flex flex-wrap justify-center gap-2.5">
              <button
                v-for="opt in typeOptions"
                :key="opt.value"
                type="button"
                class="quiz-type-chip"
                :class="quizTypes.includes(opt.value) ? 'quiz-type-chip-on' : ''"
                @click="toggleType(opt.value)"
              >
                <SvgIcon :icon="opt.icon" class="text-16px" />
                <span>{{ opt.label }}</span>
              </button>
            </div>
          </div>

          <!-- 参数 -->
          <div class="flex flex-wrap items-end justify-center gap-x-10 gap-y-4">
            <div class="flex flex-col items-center gap-1.5">
              <span class="text-xs text-gray-400">题量</span>
              <NInputNumber v-model:value="quizSize" :min="3" :max="50" size="small" style="width: 110px" />
            </div>
            <div class="flex flex-col items-center gap-1.5">
              <span class="text-xs text-gray-400">出题范围</span>
              <NSelect v-model:value="quizSource" :options="sourceOptions" size="small" style="width: 140px" />
            </div>
          </div>

          <NButton type="primary" size="large" round :loading="creating" class="w-56" @click="startQuiz">
            <template #icon>
              <SvgIcon icon="mdi:play-circle-outline" class="text-20px" />
            </template>
            开始练习
          </NButton>

          <div class="text-xs text-gray-300 dark:text-gray-600">
            <span class="hidden md:inline">键盘 1-4 / A-D 选择选项, Enter 提交、下一题 · </span>答案由服务端判分
          </div>
        </div>
      </NCard>

      <!-- 造句批改 (默认折叠的次要入口) -->
      <NCard :bordered="false" :shadow="false" size="small">
        <div class="flex items-center justify-between cursor-pointer select-none" @click="showSentencePractice = !showSentencePractice">
          <div class="flex items-center gap-2 text-sm font-bold">
            <SvgIcon :icon="showSentencePractice ? 'mdi:chevron-down' : 'mdi:chevron-right'" class="text-18px text-gray-400" />
            造句批改 (AI)
          </div>
          <span class="text-xs text-gray-400">用生词造句, AI 打分与批改</span>
        </div>
        <div v-if="showSentencePractice" class="flex flex-col gap-4 pt-4">
          <div class="flex flex-wrap gap-3 md:gap-4 items-center">
            <NSelect
              v-model:value="spWordId"
              :options="vocabOptions"
              :loading="vocabSearching"
              filterable
              remote
              clearable
              placeholder="搜索目标词"
              class="w-full md:w-300px"
              @search="searchVocabOptions"
            />
            <NInput
              v-model:value="spSentence"
              placeholder="用这个词造一个英文句子"
              class="flex-1 min-w-0 md:min-w-240px"
              @keyup.enter="handleSentencePractice"
            />
            <NButton type="primary" :loading="spSubmitting" @click="handleSentencePractice">批改</NButton>
          </div>
          <NAlert v-if="spResult" :type="spResult.correct ? 'success' : 'warning'" :bordered="false">
            <div class="font-bold mb-1">{{ spResult.word }} — 得分 {{ spResult.score }}/100</div>
            <ul v-if="spResult.grammar?.length" class="list-disc pl-5 text-sm">
              <li v-for="(g, i) in spResult.grammar" :key="i">{{ g }}</li>
            </ul>
            <div class="text-sm mt-1">改进: {{ spResult.improved }}</div>
            <div class="text-xs mt-1 text-gray-500">{{ spResult.comment }}</div>
          </NAlert>
        </div>
      </NCard>
    </template>

    <!-- 答题 -->
    <NCard v-else-if="currentQuestion && !showSummary" :bordered="false" :shadow="appStore.isMobile ? false : 'sm'" class="flex-1">
      <template #header>
        <div class="flex items-center gap-2 md:gap-4 min-w-0">
          <span class="shrink-0 text-sm md:text-base">第 {{ qIndex + 1 }} / {{ questions.length }} 题</span>
          <NProgress type="line" :percentage="progressPercent" class="w-70px md:w-160px" :show-indicator="false" />
          <NTag size="small" :bordered="false" class="shrink-0">
            {{ typeOptions.find((t) => t.value === currentQuestion.type)?.label }}
          </NTag>
        </div>
      </template>
      <template #header-extra>
        <NButton quaternary size="small" @click="restart">退出</NButton>
      </template>

      <div class="flex flex-col gap-6 py-2 md:py-4 max-w-2xl mx-auto w-full">
        <!-- 题面 -->
        <div class="min-h-100px flex flex-col items-center justify-center text-center gap-2 px-2">
          <template v-if="currentQuestion.type === 'spell'">
            <div class="text-sm text-gray-400">
              {{ isSentenceSpell ? "听例句, 拼写出句中挖空的单词" : "听音, 拼写出你听到的单词" }}
            </div>
            <div v-if="isSentenceSpell" class="text-xl md:text-2xl font-bold leading-relaxed">{{ currentQuestion.prompt }}</div>
            <NButton
              circle
              type="primary"
              size="large"
              class="!w-64px !h-64px mt-2"
              @click="currentQuestion.hint && speak(currentQuestion.hint, { lang: 'en-US', rate: 0.9 })"
            >
              <template #icon>
                <SvgIcon icon="mdi:volume-high" class="text-32px" />
              </template>
            </NButton>
          </template>
          <template v-else>
            <div class="text-xl md:text-2xl font-bold leading-relaxed">{{ currentQuestion.prompt }}</div>
            <div v-if="currentQuestion.hint && currentQuestion.type !== 'translate'" class="text-sm text-gray-400">
              {{ currentQuestion.hint }}
            </div>
          </template>
        </div>

        <!-- 选项: 字母徽标卡片行 -->
        <div v-if="isChoice" class="flex flex-col gap-2.5">
          <button
            v-for="(opt, i) in currentQuestion.options"
            :key="i"
            type="button"
            class="quiz-option"
            :class="optionClass(opt)"
            :disabled="!!lastResult"
            @click="!lastResult && (selectedOption = opt)"
          >
            <span class="quiz-option-letter" :class="lastResult && opt === lastResult.answer ? 'bg-success text-white' : selectedOption === opt ? 'bg-primary text-white' : ''">
              {{ optionLetters[i] }}
            </span>
            <span class="flex-1 text-left leading-relaxed">{{ opt }}</span>
            <SvgIcon
              v-if="lastResult && opt === lastResult.answer"
              icon="mdi:check-circle"
              class="text-20px text-success shrink-0"
            />
            <SvgIcon
              v-else-if="lastResult && opt === selectedOption"
              icon="mdi:close-circle"
              class="text-20px text-error shrink-0"
            />
          </button>
        </div>
        <!-- 输入 -->
        <div v-else-if="isInput">
          <NInput
            v-model:value="typedAnswer"
            size="large"
            :placeholder="currentQuestion.type === 'spell' ? '输入单词' : '输入英文句子'"
            :disabled="!!lastResult"
            @keyup.enter="lastResult ? nextQuestion() : submitAnswer()"
          />
        </div>

        <!-- 结果反馈 + 讲解 -->
        <NAlert v-if="lastResult" :type="lastResult.correct ? 'success' : 'error'" :bordered="false">
          <div class="flex items-center gap-2 font-bold">
            <SvgIcon :icon="lastResult.correct ? 'mdi:check-circle' : 'mdi:close-circle'" class="text-20px" />
            {{ lastResult.correct ? "回答正确" : `回答错误, 正确答案: ${lastResult.answer}` }}
          </div>
          <div v-if="lastResult.recorded" class="text-xs mt-1.5">已自动加入错题本</div>
          <div v-if="lastResult.decayed" class="text-xs mt-1.5 text-success">已消掉一条对应错题 ✓</div>
          <div v-if="lastResult.explanation" class="mt-2 text-sm whitespace-pre-line border-t border-gray-200 dark:border-gray-700 pt-2">
            {{ lastResult.explanation }}
          </div>
        </NAlert>

        <div class="flex justify-center pt-1">
          <NButton v-if="!lastResult" type="primary" size="large" round class="w-48" :loading="submitting" @click="submitAnswer">
            提交答案
          </NButton>
          <NButton v-else type="primary" size="large" round class="w-48" @click="nextQuestion">
            {{ qIndex + 1 >= questions.length ? "查看总结" : "下一题" }}
          </NButton>
        </div>
      </div>
    </NCard>

    <!-- 完成 -->
    <NCard v-else :bordered="false" :shadow="appStore.isMobile ? false : 'sm'" class="flex-1">
      <div class="flex flex-col items-center gap-7 py-6 md:py-10 max-w-xl mx-auto w-full">
        <div class="text-center">
          <div class="text-lg font-bold">练习完成</div>
          <div class="text-5xl font-bold mt-3" :class="(session?.correct || 0) / Math.max(session?.total || 1, 1) >= 0.8 ? 'text-success' : 'text-primary'">
            {{ session && session.total > 0 ? Math.round((session.correct / session.total) * 100) : 0 }}%
          </div>
          <div class="text-xs text-gray-400 mt-1">
            共 {{ session?.total || 0 }} 题 · 对 {{ session?.correct || 0 }} · 错 {{ (session?.total || 0) - (session?.correct || 0) }}
          </div>
        </div>

        <!-- 各题型正确率条 -->
        <div v-if="typeStats.length > 0" class="w-full flex flex-col gap-3">
          <div v-for="ts in typeStats" :key="ts.type" class="flex items-center gap-3">
            <span class="text-xs text-gray-500 w-90px shrink-0 text-right">{{ ts.label }}</span>
            <NProgress
              type="line"
              :percentage="ts.total > 0 ? Math.round((ts.correct / ts.total) * 100) : 0"
              :status="ts.correct === ts.total ? 'success' : ts.correct === 0 ? 'error' : 'warning'"
              class="flex-1"
            />
            <span class="text-xs text-gray-400 w-40px shrink-0">{{ ts.correct }}/{{ ts.total }}</span>
          </div>
        </div>

        <!-- 错题回顾 -->
        <div v-if="wrongQuestions.length > 0" class="w-full">
          <div class="text-sm font-bold mb-2">错题回顾 ({{ wrongQuestions.length }})</div>
          <div class="flex flex-col gap-1.5">
            <div
              v-for="q in wrongQuestions"
              :key="q.id"
              class="flex items-center justify-between gap-3 rounded border border-gray-100 dark:border-gray-800 px-3 py-1.5 text-sm"
            >
              <span class="truncate">
                <NTag size="tiny" :bordered="false" class="mr-1">{{ typeOptions.find((t) => t.value === q.type)?.label }}</NTag>
                <span class="font-medium">{{ q.word || q.prompt?.slice(0, 30) }}</span>
              </span>
              <span class="text-xs text-gray-400 truncate shrink-0 max-w-[45%]" :title="q.prompt">{{ q.prompt?.slice(0, 24) }}</span>
            </div>
          </div>
        </div>

        <div class="flex gap-4">
          <NButton type="primary" round size="large" @click="restart">再来一轮</NButton>
          <NButton v-if="wrongQuestions.length > 0" round size="large" @click="retryWrong">错题重练</NButton>
        </div>
      </div>
    </NCard>
  </div>
</template>

<style scoped>
/* 题型选择块: 图标+文字的胶囊, 选中主题色 */
.quiz-type-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 10px 16px;
  border-radius: 999px;
  border: 1.5px solid rgba(128, 128, 128, 0.25);
  font-size: 14px;
  cursor: pointer;
  transition: all 0.15s;
  color: inherit;
  background: transparent;
}
.quiz-type-chip:hover {
  border-color: var(--primary-color);
}
.quiz-type-chip-on {
  border-color: var(--primary-color);
  background: rgba(var(--primary-color-rgb, 100 108 255), 0.08);
  color: var(--primary-color);
  font-weight: 600;
}

/* 选项卡片行: 字母徽标 + 文本 + 判分标记 */
.quiz-option {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  border-radius: 10px;
  border: 1.5px solid;
  cursor: pointer;
  transition: all 0.15s;
  color: inherit;
  background: transparent;
  font-size: 15px;
  text-align: left;
  width: 100%;
}
.quiz-option:disabled {
  cursor: default;
}
.quiz-option-letter {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border-radius: 50%;
  border: 1.5px solid rgba(128, 128, 128, 0.35);
  font-size: 13px;
  font-weight: 600;
  flex-shrink: 0;
}
</style>
