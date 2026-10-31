<script setup lang="ts">
import { h, onMounted, ref, resolveComponent, computed } from "vue";
import { NButton, NPopconfirm, NModal, NInputNumber, useMessage, useDialog } from "naive-ui";
import type { DataTableColumns, DataTableRowKey } from "naive-ui";
import { useRouterPush } from "@/hooks/common/router";
import {
  fetchDeleteVocabulary,
  fetchGetVocabularyList,
  fetchUpdateVocabulary,
  fetchGetRandomVocabulary,
  fetchGetReviewSession,
  fetchGetVocabularyReviewStats,
  fetchSubmitReviewFSRS,
  fetchAssistVocabulary,
  fetchImportVocabularyFile,
  fetchStartAdaptiveVocabTest,
  fetchGetAsyncJob,
  fetchSubmitAdaptiveRound,
  fetchVocabBehavior,
  fetchLatestVocabTest,
} from "@/service/api";
import type { AdaptiveQuestion, AdaptiveVocabResult, VocabBehaviorEstimate } from "@/service/api";
import { onKeyStroke } from "@vueuse/core";
import { speak } from "@/utils/tts";
import { $t } from "@/locales";
import { useAppStore } from "@/store/modules/app";

const appStore = useAppStore();

const message = useMessage();
const dialog = useDialog();
const { routerPushByKey } = useRouterPush();
const loading = ref(false);
const data = ref<any[]>([]);
const keyword = ref("");
const checkedRowKeys = ref<DataTableRowKey[]>([]);
const isSelectionMode = ref(false);
const activeTab = ref<"new" | "mastered">("new");
const showRandomPractice = ref(false);
const practiceCount = ref(10);

// 回到原文: RSS 条目 → RSS 页并自动打开该文章; 分级阅读文档 → 详情页
const goSourceArticle = (row: any) => {
  if (row.sourceKind === "rss" && row.sourceRef) {
    localStorage.setItem("rss_open_item", String(row.sourceRef));
    routerPushByKey("ai_rss");
    return;
  }
  if (row.sourceKind === "reading" && row.sourceRef) {
    routerPushByKey("ai_reading-detail", { params: { id: String(row.sourceRef) } });
  }
};

// ===================== 自适应词汇量测试 (锚定词库 + 二分探针) =====================
const showVocabTest = ref(false);
const vocabTestLoading = ref(false);
const vocabTestJobId = ref(0);
// 已提交的轮次历史 (服务端无状态重放二分路径的依据)
const vocabTestRounds = ref<{ questionId: string; choice: number }[][]>([]);
const currentQuestions = ref<AdaptiveQuestion[]>([]);
const currentChoices = ref<Record<string, number>>({});
const vocabTestResult = ref<AdaptiveVocabResult | null>(null);
const vocabTestBehavior = ref<VocabBehaviorEstimate | null>(null);
const vocabTestLatest = ref<{ cefr: string; estSize: number; createdAt: string } | null>(null);
let vocabTestStartedAt = 0;
let vocabTestPollTimer: number | null = null;

const stopVocabTestPoll = () => {
  if (vocabTestPollTimer !== null) {
    window.clearInterval(vocabTestPollTimer);
    vocabTestPollTimer = null;
  }
};

const roundNumber = computed(() => vocabTestRounds.value.length + 1);
const currentAnswered = computed(() => currentQuestions.value.filter(q => currentChoices.value[q.id] !== undefined).length);

const loadVocabBehavior = async () => {
  const { data: behavior } = await fetchVocabBehavior();
  if (behavior) vocabTestBehavior.value = behavior;
};

const openVocabTest = async () => {
  showVocabTest.value = true;
  vocabTestResult.value = null;
  currentQuestions.value = [];
  currentChoices.value = {};
  vocabTestRounds.value = [];
  if (!vocabTestLatest.value) {
    const { data: latest } = await fetchLatestVocabTest();
    vocabTestLatest.value = latest || null;
  }
  loadVocabBehavior();
};

const startVocabTest = async () => {
  vocabTestLoading.value = true;
  const { data: job, error } = await fetchStartAdaptiveVocabTest();
  if (error || !job) {
    vocabTestLoading.value = false;
    message.error(`启动测试失败: ${(error as any)?.message || "未知错误"}`);
    return;
  }
  vocabTestJobId.value = job.id;
  vocabTestStartedAt = Date.now();
  vocabTestPollTimer = window.setInterval(async () => {
    const { data: j, error: jErr } = await fetchGetAsyncJob(job.id);
    if (jErr || !j) {
      stopVocabTestPoll();
      vocabTestLoading.value = false;
      return;
    }
    if (j.status === "running") return;
    stopVocabTestPoll();
    vocabTestLoading.value = false;
    if (j.status === "failed") {
      message.error(`出题失败: ${j.error || "未知错误"}`);
      return;
    }
    try {
      const parsed = JSON.parse(j.resultJson || "{}");
      // 首轮探针固定 B1 档 (区间 [-1,6] 的中点), 题目池已含全部档位
      currentQuestions.value = (parsed.questions || []).filter((q: AdaptiveQuestion) => q.band === "B1");
      currentChoices.value = {};
      vocabTestRounds.value = [];
    } catch {
      message.error("测试题数据异常");
    }
  }, 2500);
};

const submitRound = async () => {
  if (currentAnswered.value < currentQuestions.value.length) {
    message.warning(`还有 ${currentQuestions.value.length - currentAnswered.value} 题未作答`);
    return;
  }
  vocabTestLoading.value = true;
  const rounds = [
    ...vocabTestRounds.value,
    currentQuestions.value.map(q => ({ questionId: q.id, choice: currentChoices.value[q.id] }))
  ];
  const { data: resp, error } = await fetchSubmitAdaptiveRound({
    jobId: vocabTestJobId.value,
    durationMs: Date.now() - vocabTestStartedAt,
    rounds
  });
  vocabTestLoading.value = false;
  if (error || !resp) {
    message.error(`提交失败: ${(error as any)?.message || "未知错误"}`);
    return;
  }
  if (resp.finished && resp.result) {
    vocabTestResult.value = resp.result;
    vocabTestLatest.value = { cefr: resp.result.cefr, estSize: resp.result.estSize, createdAt: new Date().toISOString() };
    loadVocabBehavior();
    return;
  }
  vocabTestRounds.value = rounds;
  currentQuestions.value = resp.questions || [];
  currentChoices.value = {};
};

const columns = computed<DataTableColumns<any>>(() => {
  const cols: DataTableColumns<any> = [
    {
      title: $t("page.ai.vocabulary.word"),
      key: "word",
      width: 150,
      render(row) {
        return h("span", { class: "text-lg font-bold text-primary" }, row.word);
      },
    },
    {
      title: $t("page.ai.vocabulary.phonetic"),
      key: "play",
      width: 80,
      render(row) {
        return h(
          NButton,
          {
            circle: true,
            size: "small",
            quaternary: true,
            type: "primary",
            // eslint-disable-next-line @typescript-eslint/no-use-before-define
            onClick: () => handlePlay(row.word),
          },
          {
            icon: () =>
              h(resolveComponent("SvgIcon"), {
                icon: "mdi:volume-high",
                class: "text-22px text-primary",
              }),
          }
        );
      },
    },
    { title: $t("page.ai.vocabulary.phoneticSymbol"), key: "phonetic", width: 100 },
    { title: $t("page.ai.vocabulary.definition"), key: "definition", minWidth: 200 },
    { title: $t("page.ai.vocabulary.example"), key: "example", minWidth: 200 },
    { title: $t("page.ai.vocabulary.confusing"), key: "confusingWords", minWidth: 150 },
    {
      title: $t("page.ai.vocabulary.addedAt"),
      key: "createdAt",
      width: 180,
      render(row) {
        return h("span", new Date(row.createdAt).toLocaleString());
      },
    },
    {
      title: $t("page.ai.vocabulary.actions"),
      key: "actions",
      width: 150,
      fixed: "right",
      render(row) {
        const actions = [
          row.sourceKind === "rss" || row.sourceKind === "reading"
            ? h(
                NButton,
                {
                  size: "small",
                  quaternary: true,
                  title: "回到原文",
                  onClick: () => goSourceArticle(row),
                },
                {
                  icon: () =>
                    h(resolveComponent("SvgIcon"), {
                      icon: "mdi:book-open-outline",
                      class: "text-18px text-primary",
                    }),
                }
              )
            : null,
          h(
            NButton,
            {
              size: "small",
              type: "info",
              quaternary: true,
              title: "AI 助记",
              // eslint-disable-next-line @typescript-eslint/no-use-before-define
              onClick: () => handleAssist(row),
            },
            {
              icon: () =>
                h(resolveComponent("SvgIcon"), {
                  icon: "mdi:lightbulb-on-outline",
                  class: "text-18px text-info",
                }),
            }
          ),
          h(
            NButton,
            {
              size: "small",
              type: activeTab.value === "new" ? "success" : "warning",
              quaternary: true,
              // eslint-disable-next-line @typescript-eslint/no-use-before-define
              onClick: () => handleToggleMastered(row),
            },
            {
              default: () =>
                activeTab.value === "new"
                  ? $t("page.ai.vocabulary.masteredBtn")
                  : $t("page.ai.vocabulary.moveBackBtn"),
            }
          ),
          h(
            NPopconfirm,
            {
              // eslint-disable-next-line @typescript-eslint/no-use-before-define
              onPositiveClick: () => handleDelete(row.id),
              trigger: "click",
            },
            {
              trigger: () =>
                h(
                  NButton,
                  { size: "small", type: "error", quaternary: true },
                  { default: () => $t("common.delete") }
                ),
              default: () => $t("page.ai.vocabulary.deleteConfirm"),
            }
          ),
        ];
        return h("div", { class: "flex gap-1" }, actions);
      },
    },
  ];

  if (isSelectionMode.value) {
    cols.unshift({ type: "selection" });
  }

  return cols;
});

const loadData = async () => {
  loading.value = true;
  try {
    const { data: res } = await fetchGetVocabularyList({
      keyword: keyword.value,
      isMastered: activeTab.value === "mastered",
    });
    if (res) {
      data.value = res;
    }
  } catch (err: any) {
    message.error(
      `${$t("page.ai.vocabulary.loadFailed")}: ${err?.message || $t("common.error")}`
    );
  } finally {
    loading.value = false;
  }
};

const handleDelete = async (id: number) => {
  try {
    await fetchDeleteVocabulary(id);
    message.success($t("page.ai.vocabulary.deleteSuccess"));
    loadData();
  } catch (err: any) {
    message.error(
      `${$t("page.ai.vocabulary.deleteFailed")}: ${err?.message || $t("common.error")}`
    );
  }
};

const handleToggleMastered = async (row: any) => {
  try {
    const newStatus = !row.isMastered;
    await fetchUpdateVocabulary(row.id, { isMastered: newStatus });
    message.success(
      newStatus
        ? $t("page.ai.vocabulary.movedToMastered")
        : $t("page.ai.vocabulary.movedBack")
    );
    loadData();
  } catch (err: any) {
    message.error(
      `${$t("page.ai.vocabulary.operationFailed")}: ${err?.message || $t("common.error")}`
    );
  }
};

const handleBatchMastered = async () => {
  if (checkedRowKeys.value.length === 0) {
    message.warning($t("page.ai.vocabulary.selectWordFirst"));
    return;
  }
  try {
    const isToMastered = activeTab.value === "new";
    await Promise.all(
      checkedRowKeys.value.map((id) =>
        fetchUpdateVocabulary(id as number, { isMastered: isToMastered })
      )
    );
    message.success(
      isToMastered
        ? $t("page.ai.vocabulary.batchMasteredSuccess")
        : $t("page.ai.vocabulary.batchRestoreSuccess")
    );
    checkedRowKeys.value = [];
    loadData();
  } catch (err: any) {
    message.error(
      `${$t("page.ai.vocabulary.batchFailed")}: ${err?.message || $t("common.error")}`
    );
  }
};

onKeyStroke(["m", "M"], (e) => {
  if (e.ctrlKey) {
    e.preventDefault();
    handleBatchMastered();
  }
});

const handlePlay = (text: string) => {
  speak(text, {
    lang: "en-US",
    rate: 0.9,
  }).catch((err) => {
    message.error($t("page.ai.vocabulary.speechFailed"));
    console.error(err);
  });
};

const handleStartExercise = () => {
  const selectedIds = checkedRowKeys.value;
  if (selectedIds.length === 0) {
    dialog.info({
      title: $t("page.ai.vocabulary.trainAll"),
      content: $t("page.ai.vocabulary.trainAllConfirm"),
      positiveText: $t("common.confirm"),
      negativeText: $t("common.cancel"),
      onPositiveClick: () => {
        routerPushByKey("ai_exercise", { query: { mode: "all" } });
      },
    });
  } else {
    routerPushByKey("ai_exercise", { query: { ids: selectedIds.join(",") } });
  }
};

const handleRandomPractice = async () => {
  if (practiceCount.value <= 0) {
    message.warning("请输入有效的练习数量");
    return;
  }
  try {
    const { data: res } = await fetchGetRandomVocabulary({
      count: practiceCount.value,
      isMastered: activeTab.value === "mastered",
    });
    if (res && res.length > 0) {
      const ids = res.map((item: any) => item.id).join(",");
      routerPushByKey("ai_exercise", { query: { ids } });
    } else {
      message.warning("没有可练习的词汇");
    }
  } catch (err: any) {
    message.error(`获取随机词汇失败: ${err?.message || "未知错误"}`);
  }
};

// ===================== FSRS 今日复习 (双向卡 + 四级评分) =====================
const reviewStats = ref<Api.Vocabulary.ReviewStats | null>(null);
const showReviewModal = ref(false);
const reviewLoading = ref(false);
const reviewSubmitting = ref(false);
const reviewCards = ref<Api.Vocabulary.ReviewCard[]>([]);
const reviewIndex = ref(0);
const reviewFlipped = ref(false);
const reviewDone = ref(false);
const reviewGradeCounts = ref([0, 0, 0, 0]); // Again/Hard/Good/Easy
const lastGraded = ref<{ word: string; next: string } | null>(null);
const cardShownAt = ref(0);

const currentCard = computed(() => reviewCards.value[reviewIndex.value] || null);
// 正面显示英文的卡: en2zh (再认) / listen (听音辨词) / context (原句挖空); zh2en 正面显示中文释义 (主动回忆)
const isFrontEnglish = computed(() => currentCard.value?.direction !== "zh2en");
const directionLabel = computed(() => {
  switch (currentCard.value?.direction) {
    case "zh2en":
      return "中 → 英";
    case "listen":
      return "听音辨词";
    case "context":
      return "原文语境";
    default:
      return "英 → 中";
  }
});
// 语境方向: 原句挖空 (目标词替换为 ____) 供正面显示
const contextBlank = computed(() => {
  const w = currentCard.value?.word;
  if (!w?.contextSentence) return "";
  const re = new RegExp(`\\b${w.word.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}\\b`, "gi");
  return w.contextSentence.replace(re, "____");
});

const loadReviewStats = async () => {
  const { data: resp, error } = await fetchGetVocabularyReviewStats();
  if (error) {
    // 静默失败: 徽标保持不展示
    return;
  }
  reviewStats.value = resp;
};

const startReview = async () => {
  showReviewModal.value = true;
  reviewDone.value = false;
  reviewIndex.value = 0;
  reviewFlipped.value = false;
  reviewGradeCounts.value = [0, 0, 0, 0];
  lastGraded.value = null;
  reviewLoading.value = true;
  const { data: resp, error } = await fetchGetReviewSession({ limit: 20 });
  reviewLoading.value = false;
  if (error) {
    message.error($t("page.ai.vocabulary.reviewLoadFailed"));
    return;
  }
  reviewCards.value = [...(resp?.reviews || []), ...(resp?.newWords || [])];
  cardShownAt.value = Date.now();
  // 听音辨词卡正面自动朗读
  if (reviewCards.value[0]?.direction === "listen") {
    handlePlay(reviewCards.value[0].word.word);
  }
};

const handleReviewGrade = async (rating: 1 | 2 | 3 | 4) => {
  const current = currentCard.value;
  if (!current || reviewSubmitting.value) return;
  reviewSubmitting.value = true;
  const durationMs = Date.now() - cardShownAt.value;
  const { data: resp, error } = await fetchSubmitReviewFSRS(current.word.id, rating, current.direction, durationMs);
  reviewSubmitting.value = false;
  if (error) {
    message.error($t("page.ai.vocabulary.reviewSubmitFailed"));
    return;
  }
  reviewGradeCounts.value[rating - 1] += 1;
  if (resp) {
    lastGraded.value = {
      word: resp.word,
      next: resp.nextReviewAt ? new Date(resp.nextReviewAt).toLocaleString() : "已毕业",
    };
  }
  if (reviewIndex.value + 1 >= reviewCards.value.length) {
    reviewDone.value = true;
    loadReviewStats();
    loadData();
  } else {
    reviewIndex.value += 1;
    reviewFlipped.value = false;
    cardShownAt.value = Date.now();
    if (reviewCards.value[reviewIndex.value]?.direction === "listen") {
      handlePlay(reviewCards.value[reviewIndex.value].word.word);
    }
  }
};

// ===================== 遗忘干预 (AI 助记) =====================
const showAssistModal = ref(false);
const assistLoading = ref(false);
const assistWord = ref<Api.Vocabulary.Item | null>(null);

const handleAssist = async (row: any) => {
  showAssistModal.value = true;
  assistLoading.value = true;
  assistWord.value = null;
  const { data: resp, error } = await fetchAssistVocabulary(row.id);
  assistLoading.value = false;
  if (error || !resp) {
    message.error("生成记忆辅助失败");
    showAssistModal.value = false;
    return;
  }
  assistWord.value = resp;
};

// ===================== 词书批量导入 =====================
const showImportModal = ref(false);
const importLoading = ref(false);
const importSourceTag = ref("词书导入");
const importFileRef = ref<HTMLInputElement | null>(null);

const handleImportFile = async (e: Event) => {
  const input = e.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) return;
  importLoading.value = true;
  const { data: resp, error } = await fetchImportVocabularyFile(file, importSourceTag.value);
  importLoading.value = false;
  input.value = "";
  if (error || !resp) {
    message.error("导入失败");
    return;
  }
  message.success(`导入完成: 新增 ${resp.added}, 跳过(已存在) ${resp.skipped}, 失败 ${resp.failed.length}`);
  loadData();
  loadReviewStats();
};

onMounted(() => {
  loadData();
  loadReviewStats();
});
</script>

<template>
  <div class="h-full flex-col flex gap-4" :class="appStore.isMobile ? 'p-2' : 'p-4'">
    <NCard :bordered="false" :shadow="appStore.isMobile ? false : 'sm'" class="flex-1">
      <template #header>
        <div class="flex items-center gap-2" :class="appStore.isMobile ? 'flex-col items-start' : 'gap-4'">
          <span class="font-bold" :class="appStore.isMobile ? 'text-16px' : 'text-18px'">{{ $t("page.ai.vocabulary.title") }}</span>
          <NTabs
            v-model:value="activeTab"
            type="segment"
            :style="{ width: appStore.isMobile ? '100%' : '280px' }"
            @update:value="loadData"
          >
            <NTab name="new">
              <div class="flex items-center gap-2">
                <IconMdiBookOpenVariant class="text-lg" />
                <span>{{ $t("page.ai.vocabulary.wordBook") }}</span>
              </div>
            </NTab>
            <NTab name="mastered">
              <div class="flex items-center gap-2">
                <IconMdiCheckDecagram class="text-lg text-success" />
                <span>{{ $t("page.ai.vocabulary.mastered") }}</span>
              </div>
            </NTab>
          </NTabs>
          <NButton size="small" @click="openVocabTest">
            <template #icon>
              <SvgIcon icon="mdi:chart-bar" />
            </template>
            词汇量测试
          </NButton>
        </div>
      </template>
      <div class="flex flex-col h-full gap-4">
        <!-- Search and Actions -->
        <template v-if="appStore.isMobile">
          <div class="flex gap-2">
            <NInput
              v-model:value="keyword"
              :placeholder="$t('page.ai.vocabulary.searchPlaceholder')"
              clearable
              class="flex-1"
              @keyup.enter="loadData"
            />
            <NButton type="primary" @click="loadData">
              <template #icon>
                <IconMdiMagnify class="text-icon" />
              </template>
            </NButton>
          </div>
          <div class="flex gap-2 justify-between">
            <div class="flex gap-2">
              <NButton
                size="small"
                :type="isSelectionMode ? 'primary' : 'default'"
                @click="isSelectionMode = !isSelectionMode"
              >
                <template #icon>
                  <IconMdiCheckboxMultipleMarkedOutline class="text-icon" />
                </template>
              </NButton>
              <NButton
                type="warning"
                size="small"
                @click="showRandomPractice = true"
              >
                <template #icon>
                  <IconMdiDiceMultipleOutline class="text-icon" />
                </template>
              </NButton>
              <NButton
                type="info"
                size="small"
                @click="handleStartExercise"
              >
                <template #icon>
                  <IconMdiPlayCircleOutline class="text-icon" />
                </template>
              </NButton>
              <NBadge
                :value="reviewStats?.dueCount ?? 0"
                :show="!!reviewStats && reviewStats.dueCount > 0"
              >
                <NButton type="warning" size="small" @click="startReview">
                  <template #icon>
                    <IconMdiRestart class="text-icon" />
                  </template>
                </NButton>
              </NBadge>
              <ButtonIcon icon="mdi:upload" tooltip-content="导入词书" @click="showImportModal = true" />
            </div>
            <ButtonIcon
              icon="mdi:refresh"
              :tooltip-content="$t('common.refresh')"
              @click="loadData"
            />
          </div>
        </template>
        <template v-else>
          <div class="flex justify-between items-center">
            <div class="flex gap-4 items-center">
              <NInput
                v-model:value="keyword"
                :placeholder="$t('page.ai.vocabulary.searchPlaceholder')"
                clearable
                style="width: 260px"
                @keyup.enter="loadData"
              />
              <NButton type="primary" @click="loadData">
                <template #icon>
                  <IconMdiMagnify class="text-icon" />
                </template>
                {{ $t("common.search") }}
              </NButton>
            </div>
            <div class="flex gap-2 items-center">
              <NButton
                :type="isSelectionMode ? 'primary' : 'default'"
                @click="isSelectionMode = !isSelectionMode"
              >
                <template #icon>
                  <IconMdiCheckboxMultipleMarkedOutline class="text-icon" />
                </template>
                {{
                  isSelectionMode
                    ? $t("page.ai.vocabulary.selectMode")
                    : $t("page.ai.vocabulary.selectModeOn")
                }}
              </NButton>
              <NButton type="warning" @click="showRandomPractice = true">
                <template #icon>
                  <IconMdiDiceMultipleOutline class="text-icon" />
                </template>
                随机练习
              </NButton>
              <NButton type="info" @click="handleStartExercise">
                <template #icon>
                  <IconMdiPlayCircleOutline class="text-icon" />
                </template>
                {{ $t("page.ai.vocabulary.startPractice") }}
              </NButton>
              <NBadge
                :value="reviewStats?.dueCount ?? 0"
                :show="!!reviewStats && reviewStats.dueCount > 0"
              >
                <NButton type="warning" @click="startReview">
                  <template #icon>
                    <IconMdiRestart class="text-icon" />
                  </template>
                  {{ $t("page.ai.vocabulary.todayReview") }}
                </NButton>
              </NBadge>
              <ButtonIcon icon="mdi:upload" tooltip-content="导入词书" @click="showImportModal = true" />
              <ButtonIcon
                icon="mdi:refresh"
                :tooltip-content="$t('common.refresh')"
                @click="loadData"
              />
            </div>
          </div>
        </template>

        <!-- PC: DataTable -->
        <NDataTable
          v-if="!appStore.isMobile"
          v-model:checked-row-keys="checkedRowKeys"
          :columns="columns"
          :data="data"
          :loading="loading"
          :pagination="{ pageSize: 10 }"
          :row-key="(row) => row.id"
          flex-height
          class="flex-1"
        />

        <!-- Mobile: Card List -->
        <NSpin v-if="appStore.isMobile && loading" class="flex justify-center py-8" />
        <div v-else-if="appStore.isMobile" class="flex flex-col gap-3">
          <NCard
            v-for="row in data"
            :key="row.id"
            size="small"
            :bordered="true"
          >
            <div class="flex flex-col gap-2">
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-2">
                  <span class="text-lg font-bold text-primary">{{ row.word }}</span>
                  <NButton
                    circle
                    size="tiny"
                    quaternary
                    type="primary"
                    @click="handlePlay(row.word)"
                  >
                    <template #icon>
                      <SvgIcon icon="mdi:volume-high" class="text-18px" />
                    </template>
                  </NButton>
                </div>
                <span class="text-xs text-gray-400">{{ row.phonetic }}</span>
              </div>
              <div class="text-sm text-gray-600 dark:text-gray-400">{{ row.definition }}</div>
              <div v-if="row.example" class="text-xs text-gray-500 italic">{{ row.example }}</div>
              <div v-if="row.confusingWords" class="text-xs text-gray-400">
                {{ $t("page.ai.vocabulary.confusing") }}: {{ row.confusingWords }}
              </div>
              <div class="flex items-center justify-between mt-1">
                <span class="text-xs text-gray-400">{{ new Date(row.createdAt).toLocaleDateString() }}</span>
                <div class="flex gap-1">
                  <NButton
                    size="tiny"
                    :type="activeTab === 'new' ? 'success' : 'warning'"
                    quaternary
                    @click="handleToggleMastered(row)"
                  >
                    {{ activeTab === 'new' ? $t("page.ai.vocabulary.masteredBtn") : $t("page.ai.vocabulary.moveBackBtn") }}
                  </NButton>
                  <NPopconfirm @positive-click="handleDelete(row.id)">
                    <template #trigger>
                      <NButton size="tiny" type="error" quaternary>
                        {{ $t("common.delete") }}
                      </NButton>
                    </template>
                    {{ $t("page.ai.vocabulary.deleteConfirm") }}
                  </NPopconfirm>
                </div>
              </div>
            </div>
          </NCard>
          <NEmpty v-if="data.length === 0" class="py-8" />
        </div>
      </div>
    </NCard>

    <!-- Random Practice Dialog -->
    <NModal v-model:show="showRandomPractice" preset="dialog" title="指定数量随机练习" :style="{ width: appStore.isMobile ? '90%' : '400px' }">
      <div class="flex flex-col gap-4 py-4">
        <div class="text-gray-500">
          输入要练习的词汇数量，系统将随机选取对应数量的词汇进行练习。
        </div>
        <div class="flex items-center gap-4">
          <span class="w-20">练习数量：</span>
          <NInputNumber v-model:value="practiceCount" :min="1" :max="100" placeholder="请输入数量" class="flex-1" />
        </div>
      </div>
      <template #action>
        <NButton @click="showRandomPractice = false">取消</NButton>
        <NButton type="primary" @click="showRandomPractice = false; handleRandomPractice()">开始练习</NButton>
      </template>
    </NModal>

    <!-- FSRS Daily Review Dialog -->
    <NModal
      v-model:show="showReviewModal"
      preset="card"
      :title="$t('page.ai.vocabulary.reviewTitle')"
      :style="{ width: appStore.isMobile ? '92%' : '520px' }"
    >
      <NSpin :show="reviewLoading">
        <!-- 全部完成: 本次统计 -->
        <div v-if="reviewDone" class="flex flex-col items-center gap-6 py-10">
          <span class="text-lg font-bold">{{ $t('page.ai.vocabulary.reviewComplete') }}</span>
          <div class="flex gap-8">
            <div v-for="(label, idx) in ['忘记', '困难', '良好', '简单']" :key="label" class="flex flex-col items-center gap-1">
              <span
                class="text-2xl font-bold"
                :class="['text-error', 'text-warning', 'text-success', 'text-info'][idx]"
              >
                {{ reviewGradeCounts[idx] }}
              </span>
              <span class="text-xs text-gray-400">{{ label }}</span>
            </div>
          </div>
          <div v-if="lastGraded" class="text-xs text-gray-400">
            {{ lastGraded.word }} → 下次复习: {{ lastGraded.next }}
          </div>
        </div>

        <!-- 无卡片 -->
        <NEmpty v-else-if="!currentCard" class="py-10" :description="$t('page.ai.vocabulary.reviewNoDue')" />

        <!-- 复习卡片 -->
        <div v-else class="flex flex-col gap-4">
          <div class="flex items-center justify-between text-xs text-gray-400">
            <span>{{ reviewIndex + 1 }} / {{ reviewCards.length }}</span>
            <div class="flex items-center gap-2">
              <NTag size="small" :bordered="false" type="info">{{ directionLabel }}</NTag>
              <NTag
                v-if="currentCard.word.fsrsState === 0"
                size="small"
                :bordered="false"
                type="success"
              >
                新词
              </NTag>
              <NTag
                v-else-if="currentCard.word.fsrsLapses >= 3"
                size="small"
                :bordered="false"
                type="error"
              >
                易忘 ×{{ currentCard.word.fsrsLapses }}
              </NTag>
            </div>
          </div>
          <div v-if="lastGraded" class="text-center text-12px text-success">
            {{ lastGraded.word }} → 下次复习: {{ lastGraded.next }}
          </div>
          <div
            class="min-h-200px rounded-lg border border-gray-200 p-6 flex flex-col items-center justify-center cursor-pointer select-none dark:border-gray-700"
            @click="reviewFlipped = !reviewFlipped"
          >
            <!-- 正面 -->
            <template v-if="!reviewFlipped">
              <template v-if="currentCard.direction === 'context' && contextBlank">
                <div class="text-lg text-gray-700 dark:text-gray-200 leading-relaxed max-w-xl text-center">
                  {{ contextBlank }}
                </div>
                <div class="mt-2 text-xs text-gray-400">回想 ____ 处的单词</div>
              </template>
              <template v-else-if="isFrontEnglish">
                <div class="flex items-center gap-2">
                  <span v-if="currentCard.direction !== 'listen'" class="text-3xl font-bold text-primary">
                    {{ currentCard.word.word }}
                  </span>
                  <span v-else class="text-xl text-gray-400">🔊 点击喇叭听音辨词</span>
                  <NButton circle size="small" quaternary type="primary" @click.stop="handlePlay(currentCard.word.word)">
                    <template #icon>
                      <SvgIcon icon="mdi:volume-high" class="text-20px" />
                    </template>
                  </NButton>
                </div>
                <div v-if="currentCard.direction !== 'listen'" class="mt-2 text-sm text-gray-400">
                  {{ currentCard.word.phonetic }}
                </div>
              </template>
              <template v-else>
                <div class="text-lg text-gray-700 dark:text-gray-200">{{ currentCard.word.definition }}</div>
                <div class="mt-2 text-xs text-gray-400">回想英文单词</div>
              </template>
            </template>
            <!-- 背面 -->
            <template v-else>
              <div class="flex items-center gap-2">
                <span class="text-3xl font-bold text-primary">{{ currentCard.word.word }}</span>
                <NButton circle size="small" quaternary type="primary" @click.stop="handlePlay(currentCard.word.word)">
                  <template #icon>
                    <SvgIcon icon="mdi:volume-high" class="text-20px" />
                  </template>
                </NButton>
              </div>
              <div class="mt-1 text-sm text-gray-400">{{ currentCard.word.phonetic }}</div>
              <div class="mt-3 text-lg text-gray-700 dark:text-gray-200">{{ currentCard.word.definition }}</div>
              <div v-if="currentCard.word.example" class="mt-3 text-sm text-gray-500 italic">
                {{ currentCard.word.example }}
              </div>
              <div v-if="currentCard.word.contextSentence" class="mt-2 text-xs text-gray-400 max-w-xl text-center">
                📖 原文: {{ currentCard.word.contextSentence }}
              </div>
              <div v-if="currentCard.word.assistMnemonic" class="mt-2 text-xs text-warning">
                💡 {{ currentCard.word.assistMnemonic }}
              </div>
            </template>
            <div class="mt-4 text-xs text-gray-300">{{ $t('page.ai.vocabulary.reviewFlipHint') }}</div>
          </div>
          <div class="flex justify-center gap-3">
            <NButton type="error" size="large" :loading="reviewSubmitting" @click="handleReviewGrade(1)">
              忘记
            </NButton>
            <NButton type="warning" size="large" :loading="reviewSubmitting" @click="handleReviewGrade(2)">
              困难
            </NButton>
            <NButton type="success" size="large" :loading="reviewSubmitting" @click="handleReviewGrade(3)">
              良好
            </NButton>
            <NButton type="info" size="large" :loading="reviewSubmitting" @click="handleReviewGrade(4)">
              简单
            </NButton>
          </div>
        </div>
      </NSpin>
    </NModal>

    <!-- 词书批量导入 -->
    <NModal
      v-model:show="showImportModal"
      preset="card"
      title="导入词书"
      :style="{ width: appStore.isMobile ? '92%' : '460px' }"
    >
      <div class="flex flex-col gap-4 py-2">
        <div class="text-sm text-gray-500">
          支持 txt / csv 词书文件: 每行一个词, 制表符或逗号分隔 <code>word, 音标, 释义, 例句</code>。已存在的词自动跳过。
        </div>
        <NInput v-model:value="importSourceTag" placeholder="来源标签" />
        <input ref="importFileRef" type="file" accept=".txt,.csv,.tsv" class="hidden" @change="handleImportFile" />
        <NButton
          type="primary"
          block
          :loading="importLoading"
          @click="importFileRef?.click()"
        >
          选择文件并导入
        </NButton>
      </div>
    </NModal>

    <!-- 遗忘干预: AI 助记 -->
    <NModal
      v-model:show="showAssistModal"
      preset="card"
      title="AI 记忆辅助"
      :style="{ width: appStore.isMobile ? '92%' : '480px' }"
    >
      <NSpin :show="assistLoading">
        <div v-if="assistWord" class="flex flex-col gap-4 py-2">
          <div class="flex items-center gap-2">
            <span class="text-2xl font-bold text-primary">{{ assistWord.word }}</span>
            <span class="text-sm text-gray-400">{{ assistWord.phonetic }}</span>
            <NTag size="small" type="error" :bordered="false">已遗忘 {{ assistWord.fsrsLapses }} 次</NTag>
          </div>
          <div class="text-sm text-gray-600 dark:text-gray-300">{{ assistWord.definition }}</div>
          <NAlert type="info" :bordered="false">
            <div class="font-bold mb-1">💡 助记</div>
            {{ assistWord.assistMnemonic || '生成中...' }}
          </NAlert>
          <div v-if="assistWord.assistExample" class="rounded-lg bg-gray-50 dark:bg-gray-800 p-3">
            <div class="text-sm italic">{{ assistWord.assistExample }}</div>
            <NButton size="tiny" quaternary type="primary" class="mt-1" @click="handlePlay(assistWord.assistExample)">
              朗读例句
            </NButton>
          </div>
        </div>
      </NSpin>
    </NModal>

    <!-- 词汇量测试 (自适应: 锚定词库 + 二分探针) -->
    <NModal v-model:show="showVocabTest" preset="card" title="词汇量测试" :style="{ width: 'min(94vw, 640px)' }">
      <NSpin :show="vocabTestLoading">
        <!-- 结果页 -->
        <div v-if="vocabTestResult" class="flex flex-col gap-4 py-2">
          <div class="text-center">
            <div class="text-4xl font-bold text-primary">{{ vocabTestResult.cefr }}</div>
            <div class="mt-2 text-gray-500">
              估算词汇量 <span class="font-bold text-gray-700 dark:text-gray-200">{{ vocabTestResult.estLow }} ~ {{ vocabTestResult.estHigh }}</span> 词
              <span class="ml-1">(中位 ≈ {{ vocabTestResult.estSize }})</span>
            </div>
          </div>
          <div class="flex justify-center gap-2 flex-wrap">
            <NTag v-for="(v, band) in vocabTestResult.bands" :key="band" size="small" :bordered="false">
              {{ band }}: {{ v.correct }}/{{ v.asked }}
            </NTag>
          </div>
          <NAlert v-if="vocabTestResult.suspicious" type="warning" :bordered="false">
            检测到作答速度异常偏快, 结果可能不准, 建议重新测试。
          </NAlert>
          <div v-if="vocabTestBehavior && vocabTestBehavior.taggedCount > 0" class="rounded-lg border border-gray-200 dark:border-gray-700 p-3 text-sm">
            <div class="font-bold mb-1">阅读行为校准 (持续推断, 无需测试)</div>
            <div class="text-gray-500 dark:text-gray-400">
              你正在攻克 <span class="font-bold text-primary">{{ vocabTestBehavior.frontierBand }}</span> 档词汇
              (生词本锚定 {{ vocabTestBehavior.taggedCount }} 词), 推断当前水平约
              <span class="font-bold">{{ vocabTestBehavior.estCefr }}</span>
              <NTag size="tiny" :bordered="false" class="ml-1" :type="vocabTestBehavior.confidence === 'high' ? 'success' : vocabTestBehavior.confidence === 'mid' ? 'info' : 'warning'">
                可信度{{ vocabTestBehavior.confidence === 'high' ? '高' : vocabTestBehavior.confidence === 'mid' ? '中' : '低' }}
              </NTag>
            </div>
            <div class="text-xs text-gray-400 mt-1">{{ vocabTestBehavior.note }}</div>
          </div>
          <div class="text-xs text-gray-400 text-center">
            结果会用于 AI 课文难度校准与文章难度预估。隔段时间可重新测试。
          </div>
          <NButton block @click="showVocabTest = false">完成</NButton>
        </div>
        <!-- 答题页 (每轮 4 题, 服务端按表现自适应调整下一轮难度) -->
        <div v-else-if="currentQuestions.length > 0" class="flex flex-col gap-3 py-2 max-h-[70vh] overflow-auto">
          <div class="text-xs text-gray-400">
            第 {{ roundNumber }} 轮 · 每轮 4 题, 难度根据你的表现自动调整 (约 3~4 轮)
          </div>
          <div
            v-for="(q, qi) in currentQuestions"
            :key="q.id"
            class="rounded-lg border border-gray-200 dark:border-gray-700 p-3"
          >
            <div class="flex items-center gap-2">
              <NTag size="tiny" :bordered="false">{{ qi + 1 }}</NTag>
              <span class="font-bold">{{ q.word }}</span>
            </div>
            <NRadioGroup v-model:value="currentChoices[q.id]" class="mt-2" size="small">
              <div class="flex flex-col gap-1.5">
                <NRadio v-for="(opt, oi) in q.options" :key="oi" :value="oi" :label="opt" />
              </div>
            </NRadioGroup>
          </div>
          <NButton type="primary" block @click="submitRound">
            提交本轮 ({{ currentAnswered }}/{{ currentQuestions.length }})
          </NButton>
        </div>
        <!-- 首屏 -->
        <div v-else class="flex flex-col gap-3 py-2">
          <div class="text-sm text-gray-500">
            自适应测试: 考词取自分级锚定词库, 每轮 4 题, 难度根据你的表现自动调整, 约 3~4 轮即可定位水平。
            结果给出词汇量区间与 CEFR 等级, 用于 AI 课文难度校准。
          </div>
          <div v-if="vocabTestBehavior && vocabTestBehavior.taggedCount > 0" class="rounded-lg border border-gray-200 dark:border-gray-700 p-3 text-sm">
            <div class="font-bold mb-1">阅读行为校准 (测试前的粗略参考)</div>
            <div class="text-gray-500 dark:text-gray-400">
              你正在攻克 <span class="font-bold text-primary">{{ vocabTestBehavior.frontierBand }}</span> 档词汇,
              推断当前水平约 <span class="font-bold">{{ vocabTestBehavior.estCefr }}</span>
              (生词本锚定 {{ vocabTestBehavior.taggedCount }} 词,
              可信度{{ vocabTestBehavior.confidence === 'high' ? '高' : vocabTestBehavior.confidence === 'mid' ? '中' : '低' }})
            </div>
            <div class="text-xs text-gray-400 mt-1">{{ vocabTestBehavior.note }}</div>
          </div>
          <div v-if="vocabTestLatest" class="text-xs text-gray-400">
            上次测试: {{ vocabTestLatest.cefr }} ≈ {{ vocabTestLatest.estSize }} 词
            ({{ new Date(vocabTestLatest.createdAt).toLocaleDateString() }})
          </div>
          <NButton type="primary" block :loading="vocabTestLoading" @click="startVocabTest">
            {{ vocabTestLoading ? '生成题目中...' : '开始测试' }}
          </NButton>
        </div>
      </NSpin>
    </NModal>
  </div>
</template>

<style scoped></style>
