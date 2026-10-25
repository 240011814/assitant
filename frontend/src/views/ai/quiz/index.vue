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
const typeOptions = [
  { value: "word2def", label: "看词选义" },
  { value: "def2word", label: "看义选词" },
  { value: "cloze", label: "例句挖空" },
  { value: "confusing", label: "易混词辨析" },
  { value: "context", label: "原文语境" },
  { value: "spell", label: "听音拼写" },
  { value: "translate", label: "中译英" },
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
  // 跳到第一道未作答的题
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
  // 答错的听音拼写揭示后朗读正确答案
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

// ===================== 键盘流: 1-4 选选项, Enter 提交/下一题 =====================
const handleKeydown = (e: KeyboardEvent) => {
  if (!currentQuestion.value || showSummary.value) return;
  const target = e.target as HTMLElement | null;
  if (target && ["INPUT", "TEXTAREA"].includes(target.tagName)) return; // 输入框回车由 NInput 处理
  if (isChoice.value && !lastResult.value && /^[1-4]$/.test(e.key)) {
    const idx = Number(e.key) - 1;
    const opt = currentQuestion.value.options?.[idx];
    if (opt) {
      selectedOption.value = opt;
      e.preventDefault();
    }
    return;
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

// KeepAlive 缓存下再次激活: 重新检查可恢复会话
onActivated(() => {
  checkResumable();
});
</script>

<template>
  <div class="h-full flex-col flex gap-4 overflow-auto" :class="appStore.isMobile ? 'p-2' : 'p-4'">
    <!-- 未完成会话恢复横幅 -->
    <NAlert v-if="resumable && !session" type="info" :bordered="false" class="flex items-center">
      <div class="flex items-center justify-between gap-4 w-full">
        <span>
          有一轮练习进行中 ({{ resumable.session.answered }}/{{ resumable.session.total }} 题), 要继续吗?
        </span>
        <span class="flex gap-2 shrink-0">
          <NButton size="small" type="primary" @click="resumeSession">继续练习</NButton>
          <NButton size="small" quaternary @click="dismissResumable">忽略</NButton>
        </span>
      </div>
    </NAlert>

    <!-- 配置 -->
    <NCard v-if="!session" :bordered="false" :shadow="appStore.isMobile ? false : 'sm'" title="智能练习">
      <div class="flex flex-col gap-6">
        <div>
          <div class="mb-2 text-sm font-bold">题型 (可多选)</div>
          <NCheckboxGroup v-model:value="quizTypes">
            <NCheckbox v-for="opt in typeOptions" :key="opt.value" :value="opt.value" :label="opt.label" class="mr-4" />
          </NCheckboxGroup>
        </div>
        <div class="flex flex-wrap gap-6 items-center">
          <div class="flex items-center gap-3">
            <span class="text-sm font-bold">题量</span>
            <NInputNumber v-model:value="quizSize" :min="3" :max="50" style="width: 120px" />
          </div>
          <div class="flex items-center gap-3">
            <span class="text-sm font-bold">出题范围</span>
            <NSelect v-model:value="quizSource" :options="sourceOptions" style="width: 180px" />
          </div>
          <NButton type="primary" :loading="creating" @click="startQuiz">开始练习</NButton>
        </div>
        <div class="text-xs text-gray-400">
          答错自动进错题本、答对自动消错; 答题结果会计入 FSRS 复习调度 (系统配置 quiz_fsrs_feedback 可关)。
          键盘: 数字 1-4 选择选项, Enter 提交/下一题。
        </div>
      </div>
    </NCard>

    <!-- 答题 -->
    <NCard v-else-if="currentQuestion && !showSummary" :bordered="false" :shadow="appStore.isMobile ? false : 'sm'">
      <template #header>
        <div class="flex items-center gap-4">
          <span>第 {{ qIndex + 1 }} / {{ questions.length }} 题</span>
          <NProgress type="line" :percentage="progressPercent" style="width: 160px" :show-indicator="false" />
          <NTag size="small" :bordered="false">
            {{ typeOptions.find((t) => t.value === currentQuestion.type)?.label }}
          </NTag>
        </div>
      </template>
      <template #header-extra>
        <NButton quaternary @click="restart">退出</NButton>
      </template>

      <div class="flex flex-col gap-6 py-4">
        <!-- 题面 -->
        <div class="text-center">
          <template v-if="currentQuestion.type === 'spell'">
            <div class="text-lg text-gray-500 mb-2">
              {{ isSentenceSpell ? "听例句, 拼写出句中挖空的单词" : "听音, 拼写出你听到的单词" }}
            </div>
            <!-- 有例句: 题面展示挖空句 (答案需听出来) -->
            <div v-if="isSentenceSpell" class="text-2xl font-bold mb-3">{{ currentQuestion.prompt }}</div>
            <NButton circle type="primary" size="large" @click="currentQuestion.hint && speak(currentQuestion.hint, { lang: 'en-US', rate: 0.9 })">
              <template #icon>
                <SvgIcon icon="mdi:volume-high" class="text-24px" />
              </template>
            </NButton>
          </template>
          <template v-else>
            <div class="text-2xl font-bold">{{ currentQuestion.prompt }}</div>
            <div v-if="currentQuestion.hint && currentQuestion.type !== 'translate'" class="mt-2 text-sm text-gray-400">
              {{ currentQuestion.hint }}
            </div>
          </template>
        </div>

        <!-- 选项 -->
        <div v-if="isChoice" class="grid gap-3" :class="appStore.isMobile ? 'grid-cols-1' : 'grid-cols-2'">
          <NButton
            v-for="(opt, i) in currentQuestion.options"
            :key="i"
            size="large"
            :type="selectedOption === opt ? 'primary' : 'default'"
            :disabled="!!lastResult"
            :ghost="selectedOption === opt"
            @click="selectedOption = opt"
          >
            <span class="mr-2 text-xs opacity-60 hidden md:inline">{{ i + 1 }}</span>
            {{ opt }}
          </NButton>
        </div>
        <!-- 输入 -->
        <div v-else-if="isInput">
          <NInput
            v-model:value="typedAnswer"
            :placeholder="currentQuestion.type === 'spell' ? '输入单词' : '输入英文句子'"
            :disabled="!!lastResult"
            @keyup.enter="lastResult ? nextQuestion() : submitAnswer()"
          />
        </div>

        <!-- 结果反馈 + 讲解 -->
        <NAlert v-if="lastResult" :type="lastResult.correct ? 'success' : 'error'" :bordered="false">
          <div class="font-bold">
            {{ lastResult.correct ? "✓ 回答正确" : `✗ 回答错误, 正确答案: ${lastResult.answer}` }}
          </div>
          <div v-if="lastResult.recorded" class="text-xs mt-1">已自动加入错题本</div>
          <div v-if="lastResult.decayed" class="text-xs mt-1 text-success">已消掉一条对应错题 ✓</div>
          <div v-if="lastResult.explanation" class="mt-2 text-sm whitespace-pre-line border-t border-gray-200 dark:border-gray-700 pt-2">
            {{ lastResult.explanation }}
          </div>
        </NAlert>

        <div class="flex justify-center gap-4">
          <NButton v-if="!lastResult" type="primary" size="large" :loading="submitting" @click="submitAnswer">
            提交答案
          </NButton>
          <NButton v-else type="primary" size="large" @click="nextQuestion">
            {{ qIndex + 1 >= questions.length ? "完成" : "下一题" }}
          </NButton>
        </div>
      </div>
    </NCard>

    <!-- 完成 -->
    <NCard v-else :bordered="false" :shadow="appStore.isMobile ? false : 'sm'">
      <div class="flex flex-col items-center gap-6 py-10">
        <span class="text-lg font-bold">练习完成</span>
        <div class="flex gap-10">
          <div class="flex flex-col items-center">
            <span class="text-2xl font-bold text-primary">{{ session?.correct }}</span>
            <span class="text-xs text-gray-400">正确</span>
          </div>
          <div class="flex flex-col items-center">
            <span class="text-2xl font-bold text-error">{{ (session?.total || 0) - (session?.correct || 0) }}</span>
            <span class="text-xs text-gray-400">错误</span>
          </div>
          <div class="flex flex-col items-center">
            <span class="text-2xl font-bold">
              {{ session && session.total > 0 ? Math.round((session.correct / session.total) * 100) : 0 }}%
            </span>
            <span class="text-xs text-gray-400">正确率</span>
          </div>
        </div>

        <!-- 各题型正确率 -->
        <div v-if="typeStats.length > 0" class="flex flex-wrap justify-center gap-2 w-full max-w-xl">
          <NTag v-for="ts in typeStats" :key="ts.type" size="small" :bordered="false" :type="ts.correct === ts.total ? 'success' : ts.correct === 0 ? 'error' : 'warning'">
            {{ ts.label }} {{ ts.correct }}/{{ ts.total }}
          </NTag>
        </div>

        <!-- 错题回顾 -->
        <div v-if="wrongQuestions.length > 0" class="w-full max-w-xl">
          <div class="text-sm font-bold mb-2">错题回顾 ({{ wrongQuestions.length }})</div>
          <div class="flex flex-col gap-1.5">
            <div
              v-for="q in wrongQuestions"
              :key="q.id"
              class="flex items-center justify-between gap-3 rounded border border-gray-100 dark:border-gray-800 px-3 py-1.5 text-sm"
            >
              <span class="truncate">
                <NTag size="tiny" :bordered="false" class="mr-1">{{ typeOptions.find((t) => t.value === q.type)?.label }}</NTag>
                {{ q.word || q.prompt?.slice(0, 30) }}
              </span>
              <span class="text-xs text-gray-400 truncate shrink-0 max-w-[45%]" :title="q.prompt">{{ q.prompt?.slice(0, 24) }}</span>
            </div>
          </div>
        </div>

        <div class="flex gap-4">
          <NButton type="primary" @click="restart">再来一轮</NButton>
          <NButton v-if="wrongQuestions.length > 0" @click="retryWrong">错题重练</NButton>
        </div>
      </div>
    </NCard>

    <!-- 造句批改 -->
    <NCard :bordered="false" :shadow="appStore.isMobile ? false : 'sm'" title="造句批改 (AI)">
      <div class="flex flex-col gap-4">
        <div class="flex flex-wrap gap-4 items-center">
          <NSelect
            v-model:value="spWordId"
            :options="vocabOptions"
            :loading="vocabSearching"
            filterable
            remote
            clearable
            placeholder="搜索目标词"
            style="width: 300px"
            @search="searchVocabOptions"
          />
          <NInput
            v-model:value="spSentence"
            placeholder="用这个词造一个英文句子"
            style="flex: 1; min-width: 240px"
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
        <div class="text-xs text-gray-400">得分低的造句会自动加入错题本, 方便针对复习。</div>
      </div>
    </NCard>
  </div>
</template>

<style scoped></style>
