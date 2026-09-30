<script setup lang="ts">
import { onActivated, ref } from "vue";

defineOptions({
  name: 'AiOrchestration'
});
import { useRoute } from "vue-router";
import TrainingChat from "../components/training-chat.vue";
import { fetchOrchestrationChatItem } from "@/service/api";
import { useTabStore } from "@/store/modules/tab";

const route = useRoute();
const tabStore = useTabStore();
const loading = ref(true);
const orchestration = ref<{ id: number; name: string; description: string } | null>(null);
const loadedId = ref<number | null>(null);

const loadOrchestration = async (id: string | string[]) => {
  const numId = Number(id);
  if (!id || loadedId.value === numId) return;

  loading.value = true;
  try {
    const { data } = await fetchOrchestrationChatItem(numId);
    if (data) {
      orchestration.value = data;
      loadedId.value = numId;
      route.meta.title = data.name;
      tabStore.setTabLabel(data.name);
    }
  } catch (err: any) {
    console.error("加载编排失败:", err);
  } finally {
    loading.value = false;
  }
};

// 首次加载
loadOrchestration(route.params.id);

// 从 KeepAlive 恢复时，检查 id 是否变化
onActivated(() => {
  const id = route.params.id;
  if (id && Number(id) !== loadedId.value) {
    loadOrchestration(id);
  }
});
</script>

<template>
  <div v-if="loading" class="h-full flex items-center justify-center">
    <NSpin size="large" />
  </div>
  <TrainingChat
    v-else-if="orchestration"
    :orchestration-id="orchestration.id"
    :title="orchestration.name"
    :agent-id="0"
    system-prompt=""
    :initial-message="
      orchestration.description
        ? `你好！我是「${orchestration.name}」编排助手。${orchestration.description}`
        : `你好！我是「${orchestration.name}」编排助手，有什么可以帮你的？`
    "
    input-placeholder="输入消息... (回车发送，Shift + 回车换行)"
  />
  <div v-else class="h-full flex items-center justify-center text-gray-500">
    编排不存在或加载失败
  </div>
</template>