<script setup lang="ts">
import { computed } from "vue";
import { NAvatar, NTag, NSpin } from "naive-ui";
import { useAppStore } from "@/store/modules/app";
import { useAuth } from "@/hooks/business/auth";
import type { ChatMessage, ExpressionSuggestion, VocabSuggestion } from "./chat-types";

/**
 * 聊天气泡 (统一 PC/移动两套布局):
 * PC 头像+气泡水平排列, 移动端头像独占一行气泡占满; 内容块 (思考过程/正文/
 * 用量/操作按钮/智能建议) 两端共享一份模板, 仅外层布局与少量样式差异。
 */
const props = defineProps<{
  msg: ChatMessage;
  isLast: boolean;
  isGenerating: boolean;
  /** 该条 user 消息是否为最后一条 (仅最后一条 user 消息可编辑重发) */
  isLastUser: boolean;
  enableVocabulary: boolean;
  assistantColor: string;
  thinkingExpanded: boolean;
}>();

const emit = defineEmits<{
  (e: "edit"): void;
  (e: "retry"): void;
  (e: "copy", content: string): void;
  (e: "open-vocab"): void;
  (e: "open-note", content: string): void;
  (e: "apply-suggestion", vocab: VocabSuggestion): void;
  (e: "add-expression", expr: ExpressionSuggestion): void;
  (e: "select-text"): void;
  (e: "toggle-thinking"): void;
}>();

const appStore = useAppStore();
const { hasAuth } = useAuth();
const isMobile = computed(() => appStore.isMobile);
const isAssistant = computed(() => props.msg.role === "assistant");

const formatTime = (timestamp?: number) => {
  if (!timestamp) return "";
  const date = new Date(timestamp);
  return date.toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
};

const showSpinner = computed(() => props.isGenerating && props.isLast && props.msg.content === "");
const showDots = computed(() => props.isGenerating && props.isLast && props.msg.content !== "");

const bubbleClass = computed(() => [
  isMobile.value ? "p-3 text-[14px] w-full" : "p-4 text-[15px]",
  "rounded-2xl whitespace-pre-wrap leading-relaxed shadow-sm",
  props.msg.role === "user"
    ? "bg-[#e8f4fd] text-gray-800 rounded-tr-none dark:bg-blue-900/40 dark:text-gray-200"
    : "bg-white text-gray-800 rounded-tl-none dark:bg-gray-800 dark:text-gray-200",
]);

// PC 端操作按钮 hover 才显示, 移动端常显 (无 hover)
const actionsClass = computed(() =>
  isMobile.value
    ? "flex items-center gap-0.5 justify-end"
    : "flex items-center gap-0.5 mt-1 justify-end opacity-0 group-hover/btn:opacity-100 transition-all duration-200"
);

const onSelectText = () => {
  if (isAssistant.value) emit("select-text");
};
</script>

<template>
  <div
    class="flex items-start"
    :class="[msg.role === 'user' ? 'flex-row-reverse' : 'flex-row', isMobile ? 'gap-2' : 'gap-3']"
  >
    <!-- PC: 头像与气泡并排 -->
    <NAvatar
      v-if="!isMobile"
      :color="msg.role === 'user' ? '#6bb8e8' : assistantColor"
      round
      size="large"
      class="shrink-0 self-start"
    >
      {{ msg.role === "user" ? "U" : "AI" }}
    </NAvatar>

    <div class="flex flex-col gap-1" :class="isMobile ? 'w-full' : 'max-w-[80%]'">
      <!-- 移动端: 头像独占一行 -->
      <div v-if="isMobile" class="flex" :class="msg.role === 'user' ? 'justify-end' : 'justify-start'">
        <div
          class="w-8 h-8 rounded-full flex items-center justify-center text-white text-xs font-bold flex-shrink-0"
          :style="{ backgroundColor: msg.role === 'user' ? '#6bb8e8' : assistantColor }"
        >
          {{ msg.role === "user" ? "U" : "AI" }}
        </div>
      </div>

      <div class="group/btn">
        <!-- PC: 思考过程在气泡上方; 移动端: 与气泡同行 -->
        <div v-if="!isMobile && isAssistant && msg.thinkingContent" class="thinking-wrapper mb-2">
          <button class="thinking-toggle" @click="emit('toggle-thinking')">
            <span class="thinking-toggle-icon" :class="{ expanded: thinkingExpanded }">▶</span>
            <span>💭 思考过程</span>
          </button>
          <Transition name="thinking-expand">
            <div v-if="thinkingExpanded" class="thinking-body">
              <!-- eslint-disable-next-line vue/no-v-html -->
              <div class="thinking-content" v-html="msg.renderedThinking"></div>
            </div>
          </Transition>
        </div>

        <!-- 移动端的行包裹 (思考过程+气泡), PC 端为普通块容器 -->
        <div :class="isMobile ? 'flex ' + (msg.role === 'user' ? 'justify-end' : 'justify-start') : ''">
          <div v-if="isMobile && isAssistant && msg.thinkingContent" class="thinking-wrapper mb-2">
            <button class="thinking-toggle" @click="emit('toggle-thinking')">
              <span class="thinking-toggle-icon" :class="{ expanded: thinkingExpanded }">▶</span>
              <span>💭 思考过程</span>
            </button>
            <Transition name="thinking-expand">
              <div v-if="thinkingExpanded" class="thinking-body">
                <!-- eslint-disable-next-line vue/no-v-html -->
                <div class="thinking-content" v-html="msg.renderedThinking"></div>
              </div>
            </Transition>
          </div>

          <div :class="bubbleClass" @mouseup="onSelectText">
            <!-- eslint-disable-next-line vue/no-v-html -->
            <div class="msg-content" v-html="msg.renderedContent"></div>
            <span v-if="showSpinner" class="inline-block mt-1">
              <NSpin size="small" />
            </span>
            <span v-else-if="showDots" class="inline-flex items-center gap-1.5 ml-1 align-bottom">
              <span class="thinking-dot" style="animation-delay: 0s"></span>
              <span class="thinking-dot" style="animation-delay: 0.15s"></span>
              <span class="thinking-dot" style="animation-delay: 0.3s"></span>
              <span class="thinking-dot" style="animation-delay: 0.45s"></span>
              <span class="thinking-dot" style="animation-delay: 0.6s"></span>
            </span>
          </div>
        </div>

        <div class="flex items-center gap-2 justify-end" :class="isMobile ? '' : 'mt-1'">
          <span v-if="msg.usage" class="text-[11px] text-gray-400 dark:text-gray-500">
            Token: {{ msg.usage.total_tokens }}
            <span class="text-gray-300 dark:text-gray-600 mx-0.5">|</span>
            输入 {{ msg.usage.prompt_tokens }} / 输出 {{ msg.usage.completion_tokens }}
            <span class="text-gray-300 dark:text-gray-600 mx-0.5">|</span>
          </span>
          <span class="text-[11px] text-gray-400 dark:text-gray-500">
            {{ formatTime(msg.timestamp) }}
          </span>
        </div>

        <div v-if="msg.content" :class="actionsClass">
          <ButtonIcon
            v-if="msg.role === 'user' && isLastUser && !isGenerating"
            icon="mdi:pencil-outline"
            class="!h-28px !w-28px text-gray-400 hover:text-blue-500 dark:text-gray-500 dark:hover:text-blue-400"
            tooltip-content="编辑"
            @click.stop="emit('edit')"
          />
          <ButtonIcon
            v-if="isAssistant && msg.isError && !isGenerating"
            icon="mdi:refresh"
            class="!h-28px !w-28px text-gray-400 hover:text-red-500 dark:text-gray-500 dark:hover:text-red-400"
            tooltip-content="重试"
            @click.stop="emit('retry')"
          />
          <ButtonIcon
            icon="mdi:content-copy"
            class="!h-28px !w-28px text-gray-400 hover:text-gray-600 dark:text-gray-500 dark:hover:text-gray-300"
            tooltip-content="复制"
            @click.stop="emit('copy', msg.content)"
          />
          <ButtonIcon
            v-if="enableVocabulary && isAssistant"
            icon="mdi:star-outline"
            class="!h-28px !w-28px text-gray-400 hover:text-amber-500 dark:text-gray-500 dark:hover:text-amber-400"
            tooltip-content="添加到生词本"
            @click.stop="emit('open-vocab')"
          />
          <ButtonIcon
            v-if="isAssistant"
            icon="mdi:notebook-edit-outline"
            class="!h-28px !w-28px text-gray-400 hover:text-blue-500 dark:text-gray-500 dark:hover:text-blue-400"
            tooltip-content="添加笔记"
            @click.stop="emit('open-note', msg.content)"
          />
        </div>
      </div>

      <div v-if="enableVocabulary && msg.suggestions?.length" class="flex flex-wrap gap-2 mt-2">
        <span class="text-xs text-gray-400 self-center">智能建议:</span>
        <NTag
          v-for="(vocab, vocabIndex) in msg.suggestions"
          :key="vocabIndex"
          size="small"
          round
          type="info"
          check-strategy="child"
          class="cursor-pointer hover:shadow-sm transition-shadow"
          @click="emit('apply-suggestion', vocab)"
        >
          <template #icon>
            <div class="i-mdi:plus" />
          </template>
          {{ vocab.word }}
        </NTag>
      </div>

      <div v-if="hasAuth('ai:course:edit') && msg.expressions?.length" class="flex flex-wrap gap-2 mt-2">
        <span class="text-xs text-gray-400 self-center">添加到课程:</span>
        <NTag
          v-for="(expr, exprIndex) in msg.expressions"
          :key="exprIndex"
          size="small"
          round
          type="success"
          check-strategy="child"
          class="cursor-pointer hover:shadow-sm transition-shadow"
          @click="emit('add-expression', expr)"
        >
          <template #icon>
            <div class="i-mdi:plus" />
          </template>
          {{ expr.english.slice(0, 30) }}{{ expr.english.length > 30 ? '...' : '' }}
        </NTag>
      </div>
    </div>
  </div>
</template>

<style scoped>
:deep(.msg-content p) {
  margin: 0;
}
:deep(.msg-content p + p) {
  margin-top: 0.5em;
}

/* 思考过程折叠区域 */
.thinking-wrapper {
  border-left: 3px solid #d1d5db;
  border-radius: 8px;
  background: rgba(249, 250, 251, 0.8);
  overflow: hidden;
}
.dark .thinking-wrapper {
  border-left-color: #4b5563;
  background: rgba(55, 65, 81, 0.5);
}
.thinking-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 6px 12px;
  font-size: 13px;
  color: #6b7280;
  background: transparent;
  border: none;
  cursor: pointer;
  transition: color 0.2s;
}
.thinking-toggle:hover {
  color: #374151;
}
.dark .thinking-toggle {
  color: #9ca3af;
}
.dark .thinking-toggle:hover {
  color: #d1d5db;
}
.thinking-toggle-icon {
  display: inline-block;
  font-size: 10px;
  transition: transform 0.2s ease;
}
.thinking-toggle-icon.expanded {
  transform: rotate(90deg);
}
.thinking-body {
  padding: 0 12px 8px;
}
.thinking-content {
  font-size: 13px;
  color: #6b7280;
  line-height: 1.6;
  white-space: pre-wrap;
}
.dark .thinking-content {
  color: #9ca3af;
}
:deep(.thinking-content p) {
  margin: 0;
}
:deep(.thinking-content p + p) {
  margin-top: 0.5em;
}
/* 展开/收起动画 */
.thinking-expand-enter-active,
.thinking-expand-leave-active {
  transition: all 0.2s ease;
  overflow: hidden;
}
.thinking-expand-enter-from,
.thinking-expand-leave-to {
  opacity: 0;
  max-height: 0;
}
.thinking-expand-enter-to,
.thinking-expand-leave-from {
  opacity: 1;
  max-height: 500px;
}

.thinking-dot {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background-color: #60a5fa;
  animation: thinking-bounce 1.4s infinite ease-in-out both;
}

.dark .thinking-dot {
  background-color: #93c5fd;
}

@keyframes thinking-bounce {
  0%,
  80%,
  100% {
    transform: scale(0);
    opacity: 0.5;
  }
  40% {
    transform: scale(1);
    opacity: 1;
  }
}
</style>
