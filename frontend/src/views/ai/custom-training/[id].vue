<script setup lang="ts">
import { computed, onActivated, ref } from "vue";

defineOptions({
  name: 'AiCustomTraining'
});
import { useRoute } from "vue-router";
import TrainingChat from "../components/training-chat.vue";
import { fetchAIAgentDetail } from "@/service/api";
import { useTabStore } from "@/store/modules/tab";
import { useAuth } from "@/hooks/business/auth";

const route = useRoute();
const tabStore = useTabStore();
const { hasAuth } = useAuth();
const loading = ref(true);
const training = ref<any>(null);
const loadedId = ref<number | null>(null);

// 公共(内置) agent 非空 permission_code 时, 需当前用户拥有该权限码才可访问
const noPermission = computed(() => {
  const t = training.value;
  return Boolean(t?.is_public && t?.permission_code && !hasAuth(t.permission_code));
});

const loadTraining = async (id: string | string[]) => {
  const numId = Number(id);
  if (!id || loadedId.value === numId) return;

  loading.value = true;
  try {
    const { data } = await fetchAIAgentDetail(numId);
    if (data) {
      training.value = data;
      loadedId.value = numId;
      route.meta.title = data.title;
      tabStore.setTabLabel(data.title);
    }
  } catch (err: any) {
    console.error("加载训练失败:", err);
  } finally {
    loading.value = false;
  }
};

// 首次加载
loadTraining(route.params.id);

// 从 KeepAlive 恢复时，检查 id 是否变化
onActivated(() => {
  const id = route.params.id;
  if (id && Number(id) !== loadedId.value) {
    loadTraining(id);
  }
});
</script>

<template>
  <div v-if="loading" class="h-full flex items-center justify-center">
    <NSpin size="large" />
  </div>
  <TrainingChat
    v-else-if="training && !noPermission"
    :agent-id="training.id"
    :training-type="training.code || training.title"
    :custom-training-id="training.id"
    :enable-vocabulary="training.code === 'chat'"
    :system-prompt="training.system_prompt"
    :initial-message="
      training.initial_message || '你好！我是你的AI训练助手，让我们开始吧。'
    "
    :input-placeholder="
      training.input_placeholder || '输入消息... (回车发送，Shift + 回车换行)'
    "
    :assistant-color="training.color || '#2080f0'"
    :speech-lang="training.speech_lang || 'zh-CN'"
    :speech-rate="training.speech_rate || 0.95"
  />
  <div v-else class="h-full flex items-center justify-center text-gray-500">
    {{ noPermission ? '无权限访问该训练' : '训练不存在或加载失败' }}
  </div>
</template>
