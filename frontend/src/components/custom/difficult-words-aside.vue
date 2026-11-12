<script setup lang="ts">
import { computed, ref } from "vue";
import { useMessage } from "naive-ui";
import { breakpointsTailwind, useBreakpoints } from "@vueuse/core";
import { fetchAddVocabulary } from "@/service/api";
import type { DifficultWord } from "@/service/api";
import { $t } from "@/locales";

/**
 * DifficultWordsAside 生词侧栏 (分级阅读详情与 RSS 阅读弹窗共用):
 * 桌面右侧内联 / 移动端折叠为紧凑入口 + 底部抽屉 (正文下方不再拖一长列);
 * 点击查词 (emit lookup), 一键全部加入生词本 (带来源回链, 4 路并发)。
 * words 为父级响应式数组, 加入成功直接写回 w.known 同步高亮与列表状态。
 */
defineOptions({
  name: "DifficultWordsAside"
});

const props = withDefaults(
  defineProps<{
    words: DifficultWord[];
    /** 标题前缀 (如 "标注生词" / "本文生词"), 不传用默认文案 */
    heading?: string;
    /** 生词回链来源: reading=分级阅读 / rss=RSS 条目 */
    sourceKind?: string;
    sourceRef?: number;
    sourceContext?: string;
  }>(),
  {
    heading: "",
    sourceKind: "",
    sourceRef: 0,
    sourceContext: ""
  }
);

const emit = defineEmits<{
  lookup: [word: string];
}>();

const message = useMessage();
const headingLabel = computed(() => props.heading || $t("page.ai.reader.thisWords"));
// 与 aside 布局的 md: 断点对齐 (isMobile 是 sm 断点, 中间宽度仍按桌面内联渲染)
const breakpoints = useBreakpoints(breakpointsTailwind);
const isDesktop = breakpoints.greaterOrEqual("md");

const drawerOpen = ref(false);

// 一键全部加入生词本 (已存在/失败的逐个跳过), 4 路并发替代逐个串行
const addingAll = ref(false);
const addAll = async () => {
  if (addingAll.value) return;
  const targets = props.words.filter((w) => !w.known);
  if (targets.length === 0) {
    message.info($t("page.ai.reader.allAdded"));
    return;
  }
  addingAll.value = true;
  let added = 0;
  let cursor = 0;
  const workers = Array.from({ length: Math.min(4, targets.length) }, async () => {
    while (cursor < targets.length) {
      const w = targets[cursor];
      cursor += 1;
      const { error } = await fetchAddVocabulary({
        word: w.word,
        definition: w.meaning,
        sourceContext: props.sourceContext || "阅读",
        sourceKind: props.sourceKind || undefined,
        sourceRef: props.sourceRef || undefined
      });
      if (!error) {
        added += 1;
        w.known = true;
      }
    }
  });
  await Promise.all(workers);
  addingAll.value = false;
  message.success(
    `${$t("page.ai.reader.addedCount", { count: added })}${targets.length - added > 0 ? `, ${$t("page.ai.reader.skippedCount", { count: targets.length - added })}` : ""}`
  );
};
</script>

<template>
  <!-- 桌面: 内联侧栏 -->
  <aside
    v-if="isDesktop"
    class="w-full md:w-64 md:shrink-0 md:sticky md:top-4 md:self-start max-h-[calc(100vh-220px)] overflow-auto"
  >
    <div class="flex items-center justify-between mb-2">
      <span class="font-bold text-15px text-gray-900 dark:text-gray-50">{{ headingLabel }} ({{ words.length }})</span>
      <NButton size="tiny" type="primary" :loading="addingAll" @click="addAll">{{ $t('page.ai.reader.addAll') }}</NButton>
    </div>
    <div class="flex flex-col gap-2">
      <div
        v-for="w in words"
        :key="w.word"
        role="button"
        tabindex="0"
        class="rounded-lg border border-gray-200 dark:border-gray-700 p-2 text-sm cursor-pointer hover:border-primary focus:outline-none focus:border-primary"
        @click="emit('lookup', w.word)"
        @keydown.enter.prevent="emit('lookup', w.word)"
        @keydown.space.prevent="emit('lookup', w.word)"
      >
        <div class="flex items-center justify-between">
          <span class="font-bold text-primary">{{ w.word }}</span>
          <NTag size="tiny" :type="w.known ? 'default' : 'warning'" :bordered="false">
            {{ w.known ? $t('page.ai.reader.collected') : $t('page.ai.reader.newWord') }}
          </NTag>
        </div>
        <div class="text-xs text-gray-500 mt-1">{{ w.meaning }}</div>
      </div>
    </div>
  </aside>

  <!-- 移动端: 紧凑入口 + 底部抽屉 -->
  <div v-else class="w-full">
    <div
      role="button"
      tabindex="0"
      class="w-full flex items-center justify-between rounded-lg border border-gray-200 dark:border-gray-700 px-3 py-2.5 text-sm cursor-pointer active:bg-gray-50 dark:active:bg-gray-800"
      @click="drawerOpen = true"
      @keydown.enter.prevent="drawerOpen = true"
    >
      <span class="font-bold text-gray-900 dark:text-gray-50">{{ headingLabel }} ({{ words.length }})</span>
      <span class="flex items-center gap-2 text-gray-400">
        <SvgIcon icon="mdi:chevron-up" class="text-16px" />
      </span>
    </div>
    <NDrawer v-model:show="drawerOpen" placement="bottom" height="62%">
      <NDrawerContent :title="`${headingLabel} (${words.length})`" closable>
        <div class="flex flex-col gap-2">
          <div
            v-for="w in words"
            :key="w.word"
            role="button"
            tabindex="0"
            class="rounded-lg border border-gray-200 dark:border-gray-700 p-2.5 text-sm cursor-pointer active:bg-gray-50 dark:active:bg-gray-800"
            @click="emit('lookup', w.word); drawerOpen = false"
            @keydown.enter.prevent="emit('lookup', w.word)"
          >
            <div class="flex items-center justify-between">
              <span class="font-bold text-primary">{{ w.word }}</span>
              <NTag size="tiny" :type="w.known ? 'default' : 'warning'" :bordered="false">
                {{ w.known ? $t('page.ai.reader.collected') : $t('page.ai.reader.newWord') }}
              </NTag>
            </div>
            <div class="text-xs text-gray-500 mt-1">{{ w.meaning }}</div>
          </div>
        </div>
        <template #footer>
          <NButton size="small" type="primary" :loading="addingAll" @click="addAll">{{ $t('page.ai.reader.addAll') }}</NButton>
        </template>
      </NDrawerContent>
    </NDrawer>
  </div>
</template>
