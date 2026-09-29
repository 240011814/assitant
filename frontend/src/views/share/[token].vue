<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useRoute } from "vue-router";
import { NResult, NTag, NSpin, NButton } from "naive-ui";
import { fetchSharedHistory } from "@/service/api";
import type { TrainingHistory } from "@/service/api";
import { $t } from "@/locales";
import { renderMarkdown as renderMarkdownRaw } from "@/utils/markdown";

const route = useRoute();
const token = route.params.token as string;

/** 移除 vocabs/expressions 标签和尾部空白，渲染 markdown */
const renderMarkdown = (content: string) => {
  let cleaned = content.replace(/<vocabs>[\s\S]*?<\/vocabs>/g, "");
  cleaned = cleaned.replace(/<expressions>[\s\S]*?<\/expressions>/g, "");
  return renderMarkdownRaw(cleaned.trim());
};

/** 获取纯文本内容（去除 vocabs/expressions 标签） */
const getPlainContent = (content: string) => {
  let cleaned = content.replace(/<vocabs>[\s\S]*?<\/vocabs>/g, "");
  cleaned = cleaned.replace(/<expressions>[\s\S]*?<\/expressions>/g, "");
  return cleaned.trim();
};

const loading = ref(true);
const error = ref(false);
const history = ref<TrainingHistory | null>(null);

// 思考过程展开状态管理
const expandedThinking = ref<Set<number>>(new Set());
const toggleThinking = (index: number) => {
  if (expandedThinking.value.has(index)) {
    expandedThinking.value.delete(index);
  } else {
    expandedThinking.value.add(index);
  }
};

const hasAnyThinkingContent = computed(() => history.value?.messages?.some((msg) => msg.thinking_content) ?? false);

const allThinkingExpanded = computed(() => {
  if (!history.value?.messages) return false;
  const thinkingIndices = history.value.messages
    .map((msg, idx) => (msg.thinking_content ? idx : -1))
    .filter((idx) => idx !== -1);
  return thinkingIndices.length > 0 && thinkingIndices.every((idx) => expandedThinking.value.has(idx));
});

const toggleAllThinking = () => {
  if (!history.value?.messages) return;
  if (allThinkingExpanded.value) {
    expandedThinking.value.clear();
  } else {
    history.value.messages.forEach((msg, idx) => {
      if (msg.thinking_content) {
        expandedThinking.value.add(idx);
      }
    });
  }
};

const typeMap: Record<string, string> = {};

const loadHistory = async () => {
  loading.value = true;
  error.value = false;
  try {
    const { data } = await fetchSharedHistory(token);
    if (data) {
      history.value = data;
      // 历史记录中的思考过程默认展开
      data.messages?.forEach((msg, idx) => {
        if (msg.thinking_content) {
          expandedThinking.value.add(idx);
        }
      });
    } else {
      error.value = true;
    }
  } catch {
    error.value = true;
  } finally {
    loading.value = false;
  }
};

const copyToClipboard = async (content: string) => {
  try {
    await navigator.clipboard.writeText(getPlainContent(content));
    window.$message?.success("已复制");
  } catch {
    window.$message?.error("复制失败");
  }
};

onMounted(() => {
  loadHistory();
});
</script>

<template>
  <div class="min-h-screen bg-gray-50 dark:bg-gray-900">
    <div class="max-w-3xl mx-auto py-6 px-4">
      <NSpin :show="loading">
        <template v-if="error">
          <NResult status="404" :title="$t('page.share.notFound')" class="mt-20" />
        </template>
        <template v-else-if="history">
          <!-- Header -->
          <div class="mb-6">
            <div class="flex items-start justify-between">
              <div>
                <h1 class="text-xl font-bold text-gray-800 dark:text-gray-100 mb-2">
                  {{ history.title }}
                </h1>
                <NTag type="info" :bordered="false" size="small">
                  {{ typeMap[history.training_type] || history.training_type }}
                </NTag>
              </div>
              <NButton
                v-if="hasAnyThinkingContent"
                quaternary
                size="small"
                @click="toggleAllThinking"
              >
                {{ allThinkingExpanded ? "折叠全部" : "展开全部" }}
              </NButton>
            </div>
          </div>

          <!-- Messages -->
          <div class="flex flex-col gap-6">
            <template v-for="(msg, index) in history.messages" :key="index">
              <div v-if="msg.role !== 'system'" class="flex flex-col gap-2">
                <!-- User: avatar right, bubble right -->
                <template v-if="msg.role === 'user'">
                  <div class="flex justify-end">
                    <div
                      class="w-8 h-8 rounded-full bg-[#18a058] flex items-center justify-center text-white text-xs font-bold"
                    >
                      U
                    </div>
                  </div>
                  <div class="flex flex-col items-end gap-1">
                    <div
                      class="p-3 rounded-2xl rounded-tr-none leading-relaxed shadow-sm text-sm bg-[#18a058] text-white"
                    >
                      <!-- eslint-disable-next-line vue/no-v-html -->
                      <div class="msg-content" v-html="renderMarkdown(msg.content)"></div>
                    </div>
                    <NButton
                      quaternary
                      size="tiny"
                      class="copy-btn"
                      @click="copyToClipboard(msg.content)"
                    >
                      <template #icon>
                        <SvgIcon icon="mdi:content-copy" class="text-xs" />
                      </template>
                    </NButton>
                  </div>
                </template>
                <!-- AI: avatar left, bubble left -->
                <template v-else>
                  <div class="flex justify-start">
                    <div
                      class="w-8 h-8 rounded-full bg-[#2080f0] flex items-center justify-center text-white text-xs font-bold"
                    >
                      AI
                    </div>
                  </div>
                  <div class="flex flex-col gap-1">
                    <!-- 思考过程 -->
                    <div v-if="msg.thinking_content" class="thinking-wrapper">
                      <button
                        class="thinking-toggle"
                        @click="toggleThinking(index)"
                      >
                        <span class="thinking-toggle-icon" :class="{ expanded: expandedThinking.has(index) }">▶</span>
                        <span>💭 思考过程</span>
                      </button>
                      <Transition name="thinking-expand">
                        <div v-if="expandedThinking.has(index)" class="thinking-body">
                          <!-- eslint-disable-next-line vue/no-v-html -->
                          <div class="thinking-content" v-html="renderMarkdown(msg.thinking_content)"></div>
                        </div>
                      </Transition>
                    </div>
                    <div
                      class="p-3 rounded-2xl rounded-tl-none leading-relaxed shadow-sm text-sm bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200"
                    >
                      <!-- eslint-disable-next-line vue/no-v-html -->
                      <div class="msg-content" v-html="renderMarkdown(msg.content)"></div>
                    </div>
                    <div class="flex justify-end">
                      <NButton
                        quaternary
                        size="tiny"
                        class="copy-btn"
                        @click="copyToClipboard(msg.content)"
                      >
                        <template #icon>
                          <SvgIcon icon="mdi:content-copy" class="text-xs" />
                        </template>
                      </NButton>
                    </div>
                  </div>
                </template>
              </div>
            </template>
          </div>
        </template>
      </NSpin>
    </div>
  </div>
</template>

<style scoped>
:deep(.msg-content p) {
  margin: 0;
}
:deep(.msg-content p + p) {
  margin-top: 1em;
}
/* 思考过程折叠区域 */
.thinking-wrapper {
  border-left: 3px solid #d1d5db;
  border-radius: 8px;
  background: rgba(249, 250, 251, 0.8);
  overflow: hidden;
  margin-bottom: 0.5rem;
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
.copy-btn {
  opacity: 0;
  transition: opacity 0.2s;
}
div:hover > .copy-btn {
  opacity: 1;
}
</style>
