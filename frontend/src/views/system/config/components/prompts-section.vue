<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useMessage } from "naive-ui";
import { request } from "@/service/request";
import { $t } from "@/locales";
import { saveConfig } from "../shared";

defineOptions({ name: "PromptsSection" });

/** 内置 AI 提示词条目 (system_config prompt_* 键, 接口 /api/admin/system-config/prompts) */
interface PromptItem {
  key: string;
  remark: string;
  /** 当前生效值 (无覆盖时为内置默认) */
  value: string;
  defaultValue: string;
  overridden: boolean;
}

const message = useMessage();
const prompts = ref<PromptItem[]>([]);
const savingPrompts = ref<Record<string, boolean>>({});

/** 加载内置 AI 提示词列表 (key -> remark/生效值/默认值) */
async function loadPrompts() {
  const { data, error } = await request<PromptItem[]>({
    url: "/api/admin/system-config/prompts",
  });
  if (!error && data) {
    prompts.value = data;
  }
}

/** 保存单个提示词 (复用 system-config PUT, 后端保存后自动刷新提示词缓存) */
async function handleSavePrompt(p: PromptItem) {
  savingPrompts.value = { ...savingPrompts.value, [p.key]: true };
  try {
    const ok = await saveConfig(p.key, p.value, p.remark);
    if (ok) {
      p.overridden = p.value.trim() !== "";
      message.success($t("page.system.config.promptSaved"));
    }
  } finally {
    savingPrompts.value = { ...savingPrompts.value, [p.key]: false };
  }
}

/** 恢复默认: 清空该键的覆盖值, 后端回落内置默认提示词 */
async function handleResetPrompt(p: PromptItem) {
  savingPrompts.value = { ...savingPrompts.value, [p.key]: true };
  try {
    const ok = await saveConfig(p.key, "", p.remark);
    if (ok) {
      p.value = p.defaultValue;
      p.overridden = false;
      message.success($t("page.system.config.promptResetDone"));
    }
  } finally {
    savingPrompts.value = { ...savingPrompts.value, [p.key]: false };
  }
}

onMounted(() => {
  void loadPrompts();
});
</script>

<template>
  <!-- AI 提示词 (内置 LLM 提示词, system_config prompt_* 键) -->
  <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700">
    <div class="mb-4">
      <div class="font-bold text-gray-800 dark:text-gray-200">{{ $t("page.system.config.prompts") }}</div>
      <div class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ $t("page.system.config.promptsDesc") }}</div>
    </div>
    <div class="flex flex-col gap-3">
      <div
        v-for="p in prompts"
        :key="p.key"
        class="rounded-lg border border-gray-100 dark:border-gray-800 p-3"
      >
        <div class="flex items-center justify-between mb-2 gap-2">
          <div class="flex items-center gap-2 min-w-0">
            <span class="font-medium text-gray-700 dark:text-gray-300 text-sm truncate">{{ p.remark }}</span>
            <NTag
              size="tiny"
              :bordered="false"
              :type="p.overridden ? 'warning' : 'default'"
              class="shrink-0"
            >
              {{ p.overridden ? $t("page.system.config.promptCustomized") : $t("page.system.config.promptDefault") }}
            </NTag>
          </div>
          <NSpace size="small" class="shrink-0">
            <NButton
              size="tiny"
              :disabled="!p.overridden || savingPrompts[p.key]"
              @click="handleResetPrompt(p)"
            >
              {{ $t("page.system.config.promptReset") }}
            </NButton>
            <NButton
              size="tiny"
              type="primary"
              :loading="savingPrompts[p.key]"
              @click="handleSavePrompt(p)"
            >
              {{ $t("page.system.config.promptSave") }}
            </NButton>
          </NSpace>
        </div>
        <NInput
          v-model:value="p.value"
          type="textarea"
          :rows="4"
          :placeholder="p.defaultValue"
        />
      </div>
    </div>
  </div>
</template>
