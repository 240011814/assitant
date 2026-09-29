<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useMessage, NSwitch } from "naive-ui";
import {
  fetchGetVocabularyList,
  fetchUpdateVocabulary,
  fetchCourseDetail,
  fetchGetErrorBookForPractice,
  fetchIncrementTrainingCount,
} from "@/service/api";
import { fetchAddErrorBook, fetchUpdateErrorBook } from "@/service/api/error-book";
import { speak } from "@/utils/tts";

import typingSound from "@/assets/sound/typing.mp3";
import errorSound from "@/assets/sound/error.mp3";

const route = useRoute();
const router = useRouter();
const message = useMessage();

// --- Sound Engine ---
const playClick = () => {
  try {
    new Audio(typingSound).play().catch(() => {});
  } catch {}
};

const playError = () => {
  try {
    new Audio(errorSound).play().catch(() => {});
  } catch {}
};

// --- State ---
const isStarted = ref(false);
const loading = ref(false);
const rawWords = ref<any[]>([]);
const currentSentenceIndex = ref(0);
const isFinished = ref(false);
const isPlaying = ref(false);

const activeWordIndex = ref(0);
const currentInput = ref("");
const wordResults = ref<{ typed: string; status: "pending" | "correct" | "error" }[]>([]);
const errorCounts = ref<number[]>([]);
const autoPlayThreeTimes = ref(true);

const elapsedSeconds = ref(0);
let timerInterval: ReturnType<typeof setInterval> | null = null;
// 全部答对后的延迟推进句柄, 组件卸载时清理, 避免离开页面后仍朗读/跳句
let sentenceAdvanceTimer: ReturnType<typeof setTimeout> | null = null;

const formatTime = (seconds: number) => {
  const mins = Math.floor(seconds / 60);
  const secs = seconds % 60;
  return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
};

const startTimer = () => {
  elapsedSeconds.value = 0;
  timerInterval = setInterval(() => {
    elapsedSeconds.value++;
  }, 1000);
};

const stopTimer = () => {
  if (timerInterval) {
    clearInterval(timerInterval);
    timerInterval = null;
  }
};

// --- Computed ---
const currentItem = computed(() => rawWords.value[currentSentenceIndex.value] || null);
const targetSentence = computed(
  () => currentItem.value?.example || currentItem.value?.english_sentence || ""
);
const targetWords = computed(() => {
  if (!targetSentence.value) return [];
  return targetSentence.value.trim().split(/\s+/);
});

const progress = computed(() => {
  if (rawWords.value.length === 0) return 0;
  return Math.round((currentSentenceIndex.value / rawWords.value.length) * 100);
});

// --- Error Book ---
const addToErrorBook = async () => {
  try {
    const item = currentItem.value;
    const isCourseMode = !!route.query.courseId;

    // 都记录例句，contentType 区分来源
    const contentType = isCourseMode ? "sentence" : "word";
    const content = targetSentence.value;
    const translation = item?._translation || item?.chinese_translation || "";
    const sourceType = isCourseMode ? "course" : "vocabulary";
    const sourceId = item?.id || 0;

    await fetchAddErrorBook({
      contentType,
      content,
      translation,
      sourceType,
      sourceId,
    });
  } catch (err: any) {
    console.error("添加错题本失败", err);
  }
};

const markAsMastered = async () => {
  const item = currentItem.value;
  if (!item?.id) {
    message.warning("无法标记：当前题目没有ID");
    return;
  }

  try {
    const mode = route.query.mode as string;
    if (mode === "error-book") {
      await fetchUpdateErrorBook(item.id, { isMastered: true });
      message.success("已标记为掌握");
    } else {
      await fetchUpdateVocabulary(item.id, { isMastered: true });
      message.success("已标记为掌握");
    }
  } catch (err: any) {
    console.error("标记已掌握失败", err);
    message.error("标记失败");
  }
};

// --- Methods ---
const loadData = async () => {
  loading.value = true;
  try {
    const courseId = route.query.courseId as string;
    const ids = route.query.ids as string;
    const mode = route.query.mode as string;
    const type = route.query.type as string;

    // 课程包模式
    if (courseId) {
      const { data } = await fetchCourseDetail(Number(courseId));
      if (data) {
        rawWords.value = data.items.filter((item) => item.english_sentence);
        if (rawWords.value.length === 0) {
          message.warning("课程中没有可练习的句子");
          router.push({ name: "ai_course" });
        }
      }
      return;
    }

    // 错题本模式
    if (mode === "error-book") {
      if (ids) {
        // 指定IDs模式（随机练习）
        const idList = ids.split(",").map(Number);
        const { data: res } = await fetchGetErrorBookForPractice({
          contentType: type || undefined,
        });
        if (res) {
          rawWords.value = res
            .filter((item) => idList.includes(item.id))
            .map((item) => ({
              id: item.id,
              example: item.content,
              _translation: item.translation || "",
              word: item.content,
            }));
        }
      } else {
        // 全量模式（错题练习）
        const { data: res } = await fetchGetErrorBookForPractice({
          contentType: type || undefined,
        });
        if (res) {
          rawWords.value = res.map((item) => ({
            id: item.id,
            example: item.content,
            _translation: item.translation || "",
            word: item.content,
          }));
        }
      }
      if (rawWords.value.length === 0) {
        message.warning("没有待练习的错题");
        router.push({ name: "ai_error-book" });
      }
      return;
    }

    // 生词本模式
    const { data: res } = await fetchGetVocabularyList({ isMastered: false });
    if (res) {
      if (mode === "all") {
        rawWords.value = res.filter((item) => item.example);
      } else if (ids) {
        const { data: allRes } = await fetchGetVocabularyList();
        if (allRes) {
          const idList = ids.split(",").map(Number);
          rawWords.value = allRes.filter(
            (item) => idList.includes(item.id) && item.example
          );
        }
      } else {
        rawWords.value = res.filter((item) => item.example);
      }

      if (rawWords.value.length === 0) {
        message.warning("没找到含有例句的可练习单词");
        router.push({ name: "ai_vocabulary" });
      } else {
        rawWords.value.forEach((item) => {
          const match = item.example.match(/\s*\(([^)]*[一-龥][^)]*)\)\s*$/);
          item._translation = match ? match[1] : "";
          item.example = item.example.replace(/\s*\([^)]*[一-龥][^)]*\)\s*$/, "");
        });
      }
    }
  } catch {
    message.error("加载数据失败");
  } finally {
    loading.value = false;
  }
};

const initSentence = () => {
  activeWordIndex.value = 0;
  currentInput.value = "";
  wordResults.value = targetWords.value.map(() => ({ typed: "", status: "pending" }));
  errorCounts.value = targetWords.value.map(() => 0);
  // eslint-disable-next-line @typescript-eslint/no-use-before-define
  if (autoPlayThreeTimes.value) playCurrent(3);
};

const playCurrent = async (times = 3) => {
  isPlaying.value = true;
  for (let i = 0; i < times; i++) {
    if (isFinished.value) break;
    try {
      await speak(targetSentence.value, { lang: "en-US", rate: 0.9 });
    } catch (err: any) {
      message.error("语音播放失败 - " + err.message);
      console.error(err);
      break;
    }
  }
  isPlaying.value = false;
};

const selectWord = (index: number) => {
  if (activeWordIndex.value === index) return;
  playClick();

  // 离开当前单词前进行校验
  // eslint-disable-next-line @typescript-eslint/no-use-before-define
  validateWord(activeWordIndex.value, currentInput.value);

  // 切换到目标单词
  activeWordIndex.value = index;
  currentInput.value = wordResults.value[index].typed;
  // 重新进入编辑模式
  wordResults.value[index].status = "pending";
};

const handleGlobalKeydown = (e: KeyboardEvent) => {
  if (!isStarted.value || isFinished.value) return;

  // 几乎所有按键都触发点击声
  if (
    e.key.length === 1 ||
    ["Backspace", "Enter", "ArrowLeft", "ArrowRight", " "].includes(e.key)
  ) {
    playClick();
  }

  // 下箭头：显示当前单词提示
  if (e.key === "ArrowDown") {
    e.preventDefault();
    const word = targetWords.value[activeWordIndex.value];
    if (word) {
      message.info(`提示：${word}`, { duration: 3000 });
      addToErrorBook();
    }
    return;
  }

  // 上箭头：跳过当前单词，自动填入正确答案并移到下一个
  if (e.key === "ArrowUp") {
    e.preventDefault();
    const idx = activeWordIndex.value;
    const target = targetWords.value[idx];
    if (target) {
      addToErrorBook();
      wordResults.value[idx] = { typed: target, status: "correct" };
      errorCounts.value[idx] = 0;
      if (idx < targetWords.value.length - 1) {
        activeWordIndex.value = idx + 1;
        currentInput.value = wordResults.value[activeWordIndex.value].typed;
      } else {
        // eslint-disable-next-line @typescript-eslint/no-use-before-define
        validateWord(idx, target);
      }
    }
    return;
  }

  // 方向键导航：切换前校验
  if (e.key === "ArrowLeft") {
    if (activeWordIndex.value > 0) {
      selectWord(activeWordIndex.value - 1);
    }
    return;
  }
  if (e.key === "ArrowRight") {
    if (activeWordIndex.value < targetWords.value.length - 1) {
      selectWord(activeWordIndex.value + 1);
    }
    return;
  }

  // 重播/提交
  if (e.key === "Enter") {
    // eslint-disable-next-line @typescript-eslint/no-use-before-define
    validateWord(activeWordIndex.value, currentInput.value);
    // 如果没全对，可以触发重播
    if (!wordResults.value.every((r) => r.status === "correct")) {
      playCurrent(1);
    }
    return;
  }

  // 空格：校验并尝试跳转到下一个待输入
  if (e.key === " ") {
    e.preventDefault();
    const oldIndex = activeWordIndex.value;
    // eslint-disable-next-line @typescript-eslint/no-use-before-define
    validateWord(oldIndex, currentInput.value);

    // 如果还没全对，且刚才校验的是当前焦点词，则移动到下一个
    if (
      !wordResults.value.every((r) => r.status === "correct") &&
      oldIndex < targetWords.value.length - 1
    ) {
      activeWordIndex.value = oldIndex + 1;
      currentInput.value = wordResults.value[activeWordIndex.value].typed;
    }
    return;
  }

  // 退格
  if (e.key === "Backspace") {
    if (currentInput.value === "" && activeWordIndex.value > 0) {
      // 回退时不强制校验当前（因为是空的），但回退后的词进入pending
      activeWordIndex.value--;
      currentInput.value = wordResults.value[activeWordIndex.value].typed;
      wordResults.value[activeWordIndex.value].status = "pending";
    } else {
      currentInput.value = currentInput.value.slice(0, -1);
    }
    return;
  }

  // Ctrl+M：标记当前题目为已掌握（仅在生词本和错题本模式下可用）
  if (e.key === "m" && (e.ctrlKey || e.metaKey)) {
    const mode = route.query.mode as string;
    const courseId = route.query.courseId as string;
    if (mode === "error-book" || !courseId) {
      e.preventDefault();
      markAsMastered();
    }
    return;
  }

  // 字母与符号
  if (e.key.length === 1) {
    currentInput.value += e.key;
  }
};

const validateWord = (index: number, typedValue: string) => {
  if (index < 0 || index >= targetWords.value.length) return;

  const target = targetWords.value[index];
  const typed = typedValue.trim();

  const normalize = (s: string) => s.toLowerCase().replace(/[.,!?;:]/g, "");
  const isCorrect = normalize(target) === normalize(typed);

  if (!isCorrect) {
    // 只有在非空或者用户确实离开时才判定错误
    playError();
    errorCounts.value[index]++;
    if (errorCounts.value[index] >= 6) {
      message.info(`提示：${target}`, { duration: 5000 });
      addToErrorBook();
    }
  } else {
    errorCounts.value[index] = 0;
  }

  wordResults.value[index] = {
    typed: typed,
    status: isCorrect ? "correct" : "error",
  };

  // 检查整句是否全部正确
  const allCorrect = wordResults.value.every((r) => r.status === "correct");
  if (allCorrect) {
    if (sentenceAdvanceTimer) {
      clearTimeout(sentenceAdvanceTimer);
    }
    sentenceAdvanceTimer = setTimeout(() => {
      if (wordResults.value.every((r) => r.status === "correct")) {
        if (currentSentenceIndex.value < rawWords.value.length - 1) {
          currentSentenceIndex.value++;
          initSentence();
        } else {
          isFinished.value = true;
          stopTimer();
          const courseId = route.query.courseId as string;
          if (courseId) {
            fetchIncrementTrainingCount(Number(courseId)).catch(() => {});
          }
        }
      }
    }, 800);
  }
};

const shuffleArray = (arr: any[]) => {
  for (let i = arr.length - 1; i > 0; i--) {
    const j = Math.floor(Math.random() * (i + 1));
    [arr[i], arr[j]] = [arr[j], arr[i]];
  }
};

const startPractice = () => {
  shuffleArray(rawWords.value);
  isStarted.value = true;
  startTimer();
  initSentence();
};

const goBack = () => {
  const courseId = route.query.courseId as string;
  const mode = route.query.mode as string;
  if (courseId) {
    router.push({ name: "ai_course-detail", params: { id: courseId } });
  } else if (mode === "error-book") {
    router.push({ name: "ai_error-book" });
  } else {
    router.push({ name: "ai_vocabulary" });
  }
};

onMounted(() => {
  loadData();
  window.addEventListener("keydown", handleGlobalKeydown);
  window.speechSynthesis.getVoices();
});

onUnmounted(() => {
  window.speechSynthesis.cancel();
  window.removeEventListener("keydown", handleGlobalKeydown);
  stopTimer();
  if (sentenceAdvanceTimer) {
    clearTimeout(sentenceAdvanceTimer);
    sentenceAdvanceTimer = null;
  }
});

watch(
  () => route.query,
  () => {
    if (!isStarted.value) {
      loadData();
    }
  }
);
</script>

<template>
  <div
    class="h-full flex flex-col p-4 gap-4 bg-white dark:bg-dark-bg transition-colors duration-300"
  >
    <!-- Start Screen -->
    <div v-if="!isStarted && !loading" class="flex-1 flex-center">
      <div class="max-w-md w-full text-center space-y-10 animate-in fade-in duration-700">
        <div class="space-y-4">
          <h2 class="text-4xl font-bold text-gray-800 dark:text-gray-100 tracking-tight">
            沉浸式听写训练
          </h2>
          <p class="text-lg text-gray-500">
            听发音，还原地道例句，开启听觉与肌肉记忆之旅
          </p>
        </div>
        <div class="flex items-center justify-center gap-3 py-2">
          <NSwitch v-model:value="autoPlayThreeTimes" />
          <span class="text-sm text-gray-500">自动播放三遍</span>
        </div>
        <NButton
          type="primary"
          size="large"
          class="w-full h-16 text-xl rd-2xl shadow-xl hover:scale-105 transition-transform"
          @click="startPractice"
        >
          开始练习
        </NButton>
        <div
          class="text-xs text-gray-400 flex justify-center gap-6 uppercase tracking-widest"
        >
          <span>Backspace 回退</span>
          <span>Space 跳转</span>
          <span>Enter 重播</span>
          <span>↓ 提示</span>
          <span>↑ 跳过</span>
          <span v-if="route.query.mode === 'error-book' || !route.query.courseId">Ctrl+M 标记掌握</span>
        </div>
      </div>
    </div>

    <!-- Main Content -->
    <NCard
      v-else-if="!isFinished"
      class="flex-1 flex flex-col items-center justify-center relative border-none shadow-none bg-transparent"
    >
      <div class="absolute top-0 left-0 w-full flex items-center gap-2">
        <NProgress
          type="line"
          :percentage="progress"
          :show-indicator="false"
          class="flex-1"
          processing
          :height="3"
          color="#2080f0"
        />
        <span class="text-xs text-gray-400 pr-2">{{ currentSentenceIndex + 1 }}/{{ rawWords.length }}</span>
        <span class="text-xs text-gray-400 pr-2">{{ formatTime(elapsedSeconds) }}</span>
      </div>

      <div v-if="loading" class="flex-center h-full">
        <NSpin size="large" />
      </div>

      <div
        v-else-if="currentItem"
        class="max-w-5xl w-full flex flex-col items-center gap-24 py-12"
      >
        <div class="text-center">
          <div
            class="text-2xl text-gray-400 dark:text-gray-500 font-medium tracking-wide"
          >
            {{ currentItem.definition }}
          </div>
          <div
            v-if="currentItem._translation || currentItem.chinese_translation"
            class="text-2xl text-gray-400 dark:text-gray-500 font-medium tracking-wide mt-2"
          >
            {{ currentItem._translation || currentItem.chinese_translation }}
          </div>
        </div>

        <!-- Word Blocks -->
        <div class="w-full px-8">
          <div
            class="flex flex-wrap justify-center gap-x-8 gap-y-16 font-mono text-4xl leading-none text-center min-h-[240px]"
          >
            <div
              v-for="(word, idx) in targetWords"
              :key="idx"
              class="relative border-b-4 pb-3 transition-all duration-300 flex justify-center items-end cursor-pointer group whitespace-nowrap"
              :style="{ minWidth: (word.length || 1) + 'ch' }"
              :class="[
                activeWordIndex === Number(idx)
                  ? 'border-primary scale-105'
                  : wordResults[Number(idx)].status === 'error'
                    ? 'border-red-500'
                    : wordResults[Number(idx)].status === 'correct'
                      ? 'border-transparent'
                      : 'border-gray-100 dark:border-gray-800',
              ]"
              @click="selectWord(Number(idx))"
            >
              <span v-if="activeWordIndex === Number(idx)" class="text-primary">
                {{ currentInput }}
                <span class="animate-pulse border-r-3 border-primary ml-1 h-10"></span>
              </span>
              <span
                v-else
                class="transition-all duration-300"
                :class="
                  wordResults[Number(idx)].status === 'error'
                    ? 'text-red-500'
                    : 'text-gray-800 dark:text-gray-100'
                "
              >
                {{ wordResults[Number(idx)].typed || "&nbsp;" }}
              </span>
            </div>
          </div>
        </div>

        <!-- Status & Help -->
        <div
          class="mt-12 flex flex-col items-center gap-6 text-gray-300 dark:text-gray-600 select-none"
        >
          <div
            v-if="isPlaying"
            class="flex items-center gap-2 animate-bounce text-primary"
          >
            <IconMdiVolumeHigh class="text-24px" />
            <span class="text-sm font-bold tracking-tighter">Listening...</span>
          </div>
          <div
            v-else-if="wordResults.some((r) => r.status === 'error')"
            class="flex items-center gap-2 text-red-400 font-medium"
          >
            <IconMdiAlertCircle class="text-20px" />
            <span>请修正标红的拼写错误</span>
          </div>
          <div class="flex gap-10 text-xs uppercase tracking-widest font-bold">
            <span class="flex items-center gap-2"><NTag size="small" :bordered="false" round>← →</NTag> 切换单词</span>
            <span class="flex items-center gap-2"><NTag size="small" :bordered="false" round>Space</NTag> 下一个</span>
            <span class="flex items-center gap-2"><NTag size="small" :bordered="false" round>Enter</NTag> 重播</span>
            <span class="flex items-center gap-2"><NTag size="small" :bordered="false" round>↓</NTag> 提示</span>
            <span class="flex items-center gap-2"><NTag size="small" :bordered="false" round>↑</NTag> 跳过</span>
            <span v-if="route.query.mode === 'error-book' || !route.query.courseId" class="flex items-center gap-2"><NTag size="small" :bordered="false" round>Ctrl+M</NTag> 标记掌握</span>
          </div>
        </div>
      </div>
    </NCard>

    <!-- Result -->
    <NCard v-else class="flex-1 flex-center border-none shadow-none bg-transparent">
      <NResult
        status="success"
        title="训练达成！"
        description="例句听写训练已全部完成，您的语感得到了显著提升。"
        class="py-12"
      >
        <template #footer>
          <NSpace justify="center" :size="32">
            <NButton
              type="primary"
              size="large"
              secondary
              class="px-12 rd-lg"
              @click="goBack"
            >
              {{ route.query.courseId ? "返回课程" : route.query.mode === "error-book" ? "返回错题本" : "返回生词本" }}
            </NButton>
            <NButton
              size="large"
              quaternary
              class="px-12 rd-lg"
              @click="
                () => {
                  currentSentenceIndex = 0;
                  isFinished = false;
                  isStarted = false;
                  loadData();
                }
              "
            >
              再练一遍
            </NButton>
          </NSpace>
        </template>
      </NResult>
    </NCard>
  </div>
</template>

<style scoped>
.flex-center {
  display: flex;
  align-items: center;
  justify-content: center;
}
</style>

<style scoped></style>
