<script setup lang="ts">
import { ref, watch } from "vue";
import { useMessage } from "naive-ui";
import { $t } from "@/locales";
import { saveConfigs, toNumber } from "../shared";
import type { ConfigMap } from "../shared";

defineOptions({ name: "TimeoutSection" });

const props = defineProps<{ config: ConfigMap }>();

const message = useMessage();
const savingTimeout = ref(false);
const aiTimeoutMinutes = ref(5);
const aiTlsHandshakeTimeout = ref(15);
const aiResponseHeaderTimeout = ref(30);
const httpTimeoutSeconds = ref(30);

watch(
  () => props.config,
  cfg => {
    aiTimeoutMinutes.value = toNumber(cfg.ai_timeout_minutes, 5);
    aiTlsHandshakeTimeout.value = toNumber(cfg.ai_tls_handshake_timeout, 15);
    aiResponseHeaderTimeout.value = toNumber(cfg.ai_response_header_timeout, 30);
    httpTimeoutSeconds.value = toNumber(cfg.http_timeout_seconds, 30);
  },
  { immediate: true }
);

async function handleSaveTimeout() {
  savingTimeout.value = true;
  try {
    const ok = await saveConfigs([
      ["ai_timeout_minutes", String(aiTimeoutMinutes.value), $t("page.system.config.remarkAiTimeout")],
      ["ai_tls_handshake_timeout", String(aiTlsHandshakeTimeout.value), $t("page.system.config.remarkAiTlsHandshake")],
      ["ai_response_header_timeout", String(aiResponseHeaderTimeout.value), $t("page.system.config.remarkAiResponseHeader")],
      ["http_timeout_seconds", String(httpTimeoutSeconds.value), $t("page.system.config.remarkHttpTimeout")],
    ]);
    if (ok) message.success($t("page.system.config.timeoutSaved"));
  } finally {
    savingTimeout.value = false;
  }
}
</script>

<template>
  <!-- 超时配置 -->
  <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700">
    <div class="mb-4">
      <div class="font-bold text-gray-800 dark:text-gray-200">{{ $t("page.system.config.timeout") }}</div>
      <div class="text-sm text-gray-500 dark:text-gray-400 mt-1">
        {{ $t("page.system.config.timeoutDesc") }}
      </div>
    </div>
    <NForm label-placement="left" label-width="100">
      <NGrid :cols="2" :x-gap="12" :y-gap="8">
        <NFormItemGi :label="$t('page.system.config.aiTimeout')">
          <NInputNumber
            v-model:value="aiTimeoutMinutes"
            :min="1"
            :max="60"
            :disabled="savingTimeout"
            size="small"
          >
            <template #suffix>{{ $t("page.system.config.minutes") }}</template>
          </NInputNumber>
        </NFormItemGi>
        <NFormItemGi :label="$t('page.system.config.aiTlsHandshake')">
          <NInputNumber
            v-model:value="aiTlsHandshakeTimeout"
            :min="5"
            :max="120"
            :disabled="savingTimeout"
            size="small"
          >
            <template #suffix>{{ $t("page.system.config.seconds") }}</template>
          </NInputNumber>
        </NFormItemGi>
        <NFormItemGi :label="$t('page.system.config.aiResponseHeader')">
          <NInputNumber
            v-model:value="aiResponseHeaderTimeout"
            :min="5"
            :max="120"
            :disabled="savingTimeout"
            size="small"
          >
            <template #suffix>{{ $t("page.system.config.seconds") }}</template>
          </NInputNumber>
        </NFormItemGi>
        <NFormItemGi :label="$t('page.system.config.httpTimeout')">
          <NInputNumber
            v-model:value="httpTimeoutSeconds"
            :min="5"
            :max="300"
            :disabled="savingTimeout"
            size="small"
          >
            <template #suffix>{{ $t("page.system.config.seconds") }}</template>
          </NInputNumber>
        </NFormItemGi>
      </NGrid>
      <NFormItem class="mt-4">
        <NButton
          type="primary"
          :loading="savingTimeout"
          @click="handleSaveTimeout"
        >
          {{ $t("page.system.config.saveTimeout") }}
        </NButton>
      </NFormItem>
    </NForm>
  </div>
</template>
