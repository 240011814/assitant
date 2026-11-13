<script setup lang="ts">
import { onMounted, ref } from "vue";
import { request } from "@/service/request";
import { $t } from "@/locales";
import { toConfigMap } from "./shared";
import type { ConfigItem, ConfigMap } from "./shared";
import RegisterSection from "./components/register-section.vue";
import TelegramSection from "./components/telegram-section.vue";
import TimeoutSection from "./components/timeout-section.vue";
import ApiTimeoutSection from "./components/api-timeout-section.vue";
import MemorySection from "./components/memory-section.vue";
import SmtpSection from "./components/smtp-section.vue";
import S3Section from "./components/s3-section.vue";
import RagSection from "./components/rag-section.vue";
import TtsSection from "./components/tts-section.vue";
import MqttSection from "./components/mqtt-section.vue";
import PromptsSection from "./components/prompts-section.vue";

defineOptions({ name: "SystemConfig" });

/**
 * 系统配置页 (薄壳): 只负责加载一次 system_config 并下发给各分区子组件;
 * 各分区 (注册/Telegram/超时/接口超时/记忆/SMTP/S3/RAG/TTS/MQTT/提示词) 的
 * 表单状态与保存逻辑在 components/ 下各自维护。
 */
const loading = ref(false);
const configMap = ref<ConfigMap>({});

async function loadConfig() {
  loading.value = true;
  try {
    // API 失败时全局拦截器已 toast, 这里静默返回
    const { data, error } = await request<ConfigItem[]>({
      url: "/api/admin/system-config",
    });
    if (!error && data) {
      configMap.value = toConfigMap(data);
    }
  } finally {
    loading.value = false;
  }
}

onMounted(() => {
  void loadConfig();
});
</script>

<template>
  <div class="h-full overflow-auto p-6">
    <NCard :bordered="false" shadow="sm" :title="$t('page.system.config.title')">
      <NSpin :show="loading">
        <div class="space-y-6">
          <RegisterSection :config="configMap" />
          <TelegramSection :config="configMap" />
          <TimeoutSection :config="configMap" />
          <ApiTimeoutSection />
          <MemorySection :config="configMap" />
          <SmtpSection :config="configMap" />
          <S3Section :config="configMap" />
          <RagSection :config="configMap" />
          <TtsSection :config="configMap" />
        </div>

        <MqttSection />

        <PromptsSection />
      </NSpin>
    </NCard>
  </div>
</template>
