<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useMessage } from "naive-ui";
import { useAuth } from "@/hooks/business/auth";
import {
  fetchAIAgentList,
  fetchAIAgentDetail,
  fetchCreateAIAgent,
  fetchUpdateAIAgent,
  fetchDeleteAIAgent,
} from "@/service/api";
import type { AIAgent } from "@/service/api";
import { $t } from "@/locales";

const router = useRouter();
const message = useMessage();
const { hasAuth } = useAuth();

const agents = ref<AIAgent[]>([]);
const loading = ref(false);

const showModal = ref(false);
const isEdit = ref(false);
const editId = ref(0);
const submitting = ref(false);

const form = ref({
  title: "",
  description: "",
  code: "",
  system_prompt: "",
  icon: "mdi:robot-outline",
  color: "#2080f0",
  initial_message: "",
  input_placeholder: "",
  speech_lang: "zh-CN",
  speech_rate: 0.95,
});

const iconOptions = [
  { label: "机器人", value: "mdi:robot-outline" },
  { label: "对话", value: "mdi:chat-outline" },
  { label: "大脑", value: "mdi:brain" },
  { label: "灯泡", value: "mdi:lightbulb-outline" },
  { label: "书本", value: "mdi:book-open-outline" },
  { label: "铅笔", value: "mdi:pencil-outline" },
  { label: "星星", value: "mdi:star-outline" },
  { label: "爱心", value: "mdi:heart-outline" },
  { label: "火箭", value: "mdi:rocket-launch-outline" },
  { label: "齿轮", value: "mdi:cog-outline" },
];

const colorOptions = [
  { label: "蓝色", value: "#2080f0" },
  { label: "绿色", value: "#18a058" },
  { label: "红色", value: "#d9534f" },
  { label: "橙色", value: "#f0a020" },
  { label: "紫色", value: "#7c3aed" },
  { label: "棕色", value: "#8a6d3b" },
  { label: "灰色", value: "#666666" },
  { label: "青色", value: "#20c997" },
];

const resetForm = () => {
  form.value = {
    title: "",
    description: "",
    code: "",
    system_prompt: "",
    icon: "mdi:robot-outline",
    color: "#2080f0",
    initial_message: "",
    input_placeholder: "",
    speech_lang: "zh-CN",
    speech_rate: 0.95,
  };
};

const loadAgents = async () => {
  loading.value = true;
  try {
    const { data } = await fetchAIAgentList();
    if (data) {
      // 公共(内置) agent 非空 permission_code 时, 需当前用户拥有该权限码才可见/可访问
      agents.value = data.filter(canAccessAgent);
    }
  } catch (err: any) {
    console.error("loadAgents error:", err);
  } finally {
    loading.value = false;
  }
};

function canAccessAgent(agent: AIAgent) {
  return !agent.is_public || !agent.permission_code || hasAuth(agent.permission_code);
}

function goToAgent(id: number) {
  router.push(`/ai/custom-training/${id}`);
}

function handleCreate() {
  isEdit.value = false;
  editId.value = 0;
  resetForm();
  showModal.value = true;
}

async function handleEdit(id: number) {
  isEdit.value = true;
  editId.value = id;
  try {
    const { data } = await fetchAIAgentDetail(id);
    if (data) {
      form.value = {
        title: data.title,
        description: data.description,
        code: data.code || "",
        system_prompt: data.system_prompt,
        icon: data.icon || "mdi:robot-outline",
        color: data.color || "#2080f0",
        initial_message: data.initial_message,
        input_placeholder: data.input_placeholder || "",
        speech_lang: data.speech_lang || "zh-CN",
        speech_rate: data.speech_rate || 0.95,
      };
    }
  } catch (err: any) {
    message.error(
      `${$t("page.ai.training.loadFailed")}: ${err?.message || $t("common.error")}`
    );
    return;
  }
  showModal.value = true;
}

async function handleSubmit() {
  if (!form.value.title.trim()) {
    message.warning($t("page.ai.training.titleRequired"));
    return;
  }
  if (!form.value.system_prompt.trim()) {
    message.warning($t("page.ai.training.promptRequired"));
    return;
  }

  submitting.value = true;
  try {
    if (isEdit.value) {
      await fetchUpdateAIAgent(editId.value, form.value);
      message.success($t("page.ai.training.updateSuccess"));
    } else {
      await fetchCreateAIAgent(form.value);
      message.success($t("page.ai.training.createSuccess"));
    }
    showModal.value = false;
    loadAgents();
  } catch (err: any) {
    message.error(
      `${$t("page.ai.training.operationFailed")}: ${err?.message || $t("common.error")}`
    );
  } finally {
    submitting.value = false;
  }
}

async function handleDeleteAgent(id: number) {
  try {
    await fetchDeleteAIAgent(id);
    message.success($t("page.ai.training.deleteSuccess"));
    loadAgents();
  } catch (err: any) {
    message.error(
      `${$t("page.ai.training.deleteFailed")}: ${err?.message || $t("common.error")}`
    );
  }
}

onMounted(() => {
  loadAgents();
});
</script>

<template>
  <div class="h-full overflow-auto p-6">
    <div class="mx-auto max-w-4xl">
      <div class="mb-8 text-center">
        <h1 class="text-3xl font-bold text-gray-800 dark:text-gray-200">
          {{ $t("page.ai.training.title") }}
        </h1>
        <p class="mt-2 text-gray-500 dark:text-gray-400">
          {{ $t("page.ai.training.subtitle") }}
        </p>
      </div>

      <div class="mb-6 flex items-center justify-between">
        <h2 class="text-xl font-bold text-gray-800 dark:text-gray-200">
          {{ $t("page.ai.training.customTraining") }}
        </h2>
        <NButton
          v-if="hasAuth('ai:custom-training:create')"
          type="primary"
          @click="handleCreate"
        >
          <template #icon>
            <SvgIcon icon="mdi:plus" />
          </template>
          {{ $t("page.ai.training.addTraining") }}
        </NButton>
      </div>

      <div v-if="loading" class="flex justify-center py-8">
        <NSpin size="large" />
      </div>

      <div
        v-else-if="agents.length === 0"
        class="rounded-xl border border-dashed border-gray-300 p-8 text-center dark:border-gray-600"
      >
        <SvgIcon icon="mdi:robot-outline" class="text-4xl text-gray-400 mb-2" />
        <p class="text-gray-500 dark:text-gray-400">
          {{ $t("page.ai.training.noCustomTraining") }}
        </p>
        <p class="text-sm text-gray-400 dark:text-gray-500 mt-1">
          {{ $t("page.ai.training.noCustomTrainingTip") }}
        </p>
      </div>

      <div v-else class="grid grid-cols-1 gap-6 md:grid-cols-2">
        <div
          v-for="item in agents"
          :key="item.id"
          class="group relative cursor-pointer rounded-xl border border-gray-200 bg-white p-6 shadow-sm transition-all hover:shadow-md dark:border-gray-700 dark:bg-gray-800"
          @click="goToAgent(item.id)"
        >
          <div class="flex items-start gap-4">
            <div
              class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-lg"
              :style="{
                backgroundColor: (item.color || '#2080f0') + '15',
                color: item.color || '#2080f0',
              }"
            >
              <SvgIcon :icon="item.icon || 'mdi:robot-outline'" class="text-2xl" />
            </div>
            <div class="flex-1">
              <div class="flex items-center gap-2">
                <h3 class="text-lg font-semibold text-gray-800 dark:text-gray-200">
                  {{ item.title }}
                </h3>
                <NTag
                  v-if="item.is_public"
                  size="small"
                  type="success"
                  :bordered="false"
                >
                  公共
                </NTag>
              </div>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400 line-clamp-2">
                {{ item.description || $t("page.ai.training.noDescription") }}
              </p>
            </div>
            <SvgIcon
              icon="mdi:chevron-right"
              class="text-xl text-gray-400 transition-transform group-hover:translate-x-1"
            />
          </div>
          <div
            v-if="!item.is_public && (hasAuth('ai:custom-training:edit') || hasAuth('ai:custom-training:delete'))"
            class="absolute bottom-3 right-3 flex gap-1 opacity-0 transition-opacity group-hover:opacity-100"
            @click.stop
          >
            <NButton
              v-if="hasAuth('ai:custom-training:edit')"
              size="small"
              quaternary
              @click="handleEdit(item.id)"
            >
              <template #icon>
                <SvgIcon icon="mdi:pencil-outline" />
              </template>
            </NButton>
            <NPopconfirm
              v-if="hasAuth('ai:custom-training:delete')"
              @positive-click="handleDeleteAgent(item.id)"
            >
              <template #trigger>
                <NButton size="small" quaternary type="error">
                  <template #icon>
                    <SvgIcon icon="mdi:delete-outline" />
                  </template>
                </NButton>
              </template>
              {{ $t("page.ai.training.deleteConfirm") }}
            </NPopconfirm>
          </div>
        </div>
      </div>
    </div>

    <!-- 新增/编辑弹窗 -->
    <NModal
      v-model:show="showModal"
      preset="card"
      :title="
        isEdit
          ? $t('page.ai.training.editTraining')
          : $t('page.ai.training.createTraining')
      "
      style="width: 600px"
      :segmented="{ content: true, footer: true }"
    >
      <NForm :model="form" label-placement="left" label-width="80">
        <NFormItem :label="$t('page.ai.training.titleLabel')" path="title">
          <NInput
            v-model:value="form.title"
            :placeholder="$t('page.ai.training.titlePlaceholder')"
            maxlength="50"
            show-count
          />
        </NFormItem>
        <NFormItem :label="$t('page.ai.training.descLabel')" path="description">
          <NInput
            v-model:value="form.description"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 3 }"
            :placeholder="$t('page.ai.training.descPlaceholder')"
            maxlength="200"
            show-count
          />
        </NFormItem>
        <NFormItem label="标识码" path="code">
          <NInput
            v-model:value="form.code"
            placeholder="唯一标识，如 english_chat（创建后不可修改）"
            :disabled="isEdit"
            maxlength="50"
          />
        </NFormItem>
        <NFormItem :label="$t('page.ai.training.promptLabel')" path="system_prompt">
          <NInput
            v-model:value="form.system_prompt"
            type="textarea"
            :autosize="{ minRows: 4, maxRows: 8 }"
            :placeholder="$t('page.ai.training.promptPlaceholder')"
          />
        </NFormItem>
        <NFormItem :label="$t('page.ai.training.welcomeLabel')" path="initial_message">
          <NInput
            v-model:value="form.initial_message"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 3 }"
            :placeholder="$t('page.ai.training.welcomePlaceholder')"
          />
        </NFormItem>
        <div class="grid grid-cols-2 gap-4">
          <NFormItem :label="$t('page.ai.training.iconLabel')" path="icon">
            <NSelect v-model:value="form.icon" :options="iconOptions" />
          </NFormItem>
          <NFormItem :label="$t('page.ai.training.colorLabel')" path="color">
            <NSelect v-model:value="form.color" :options="colorOptions" />
          </NFormItem>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <NFormItem :label="$t('page.ai.training.langLabel')" path="speech_lang">
            <NSelect
              v-model:value="form.speech_lang"
              :options="[
                { label: '中文', value: 'zh-CN' },
                { label: 'English', value: 'en-US' },
                { label: '日本語', value: 'ja-JP' },
              ]"
            />
          </NFormItem>
          <NFormItem :label="$t('page.ai.training.speedLabel')" path="speech_rate">
            <NSlider
              v-model:value="form.speech_rate"
              :min="0.5"
              :max="1.5"
              :step="0.05"
            />
          </NFormItem>
        </div>
      </NForm>
      <template #footer>
        <div class="flex justify-end gap-3">
          <NButton @click="showModal = false">{{ $t("common.cancel") }}</NButton>
          <NButton type="primary" :loading="submitting" @click="handleSubmit">
            {{ isEdit ? $t("common.save") : $t("common.confirm") }}
          </NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>
