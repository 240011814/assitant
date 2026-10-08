<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useMessage } from "naive-ui";
import { request } from "@/service/request";
import { $t } from "@/locales";

defineOptions({ name: "SystemConfig" });

const message = useMessage();
const loading = ref(false);
const savingRegister = ref(false);
const savingTelegram = ref(false);
const savingTimeout = ref(false);
const savingSmtp = ref(false);
const sendingTestEmail = ref(false);
const showTestEmailModal = ref(false);
const testEmailForm = ref({ email: "", subject: "", content: "" });
const registerEnabled = ref(false);
const telegramEnabled = ref(true);
const telegramBotToken = ref("");
const showTelegramToken = ref(false);
const telegramWebhookUrl = ref("");
const aiTimeoutMinutes = ref(5);
const aiTlsHandshakeTimeout = ref(15);
const aiResponseHeaderTimeout = ref(30);
const httpTimeoutSeconds = ref(30);
const smtpEnabled = ref(false);
const smtpHost = ref("");
const smtpPort = ref(587);
const smtpEncryption = ref("ssl");
const smtpUser = ref("");
const smtpPassword = ref("");
const showSmtpPassword = ref(false);
const smtpFrom = ref("");
const smtpFromName = ref("");
const savingMemory = ref(false);
const memoryEnabled = ref(true);
const memoryExtractionModel = ref("");
const memoryIdleMinutes = ref(15);
const memoryMinUserMessages = ref(6);
const savingS3 = ref(false);
const testingS3 = ref(false);
const showS3SecretKey = ref(false);
const s3Enabled = ref(false);
const s3Endpoint = ref("");
const s3Region = ref("");
const s3Bucket = ref("");
const s3AccessKey = ref("");
const s3SecretKey = ref("");
const s3Secure = ref(false);
const s3UsePathStyle = ref(false);
const s3MaxUploadMB = ref(20);
const savingRag = ref(false);
const testingEmbedding = ref(false);
const showRagApiKey = ref(false);
const ragEnabled = ref(false);
const ragBaseUrl = ref("");
const ragModel = ref("");
const ragApiKey = ref("");
const ragChunkSize = ref(500);
const ragChunkOverlap = ref(80);
const ragTopK = ref(5);

async function loadConfig() {
  loading.value = true;
  try {
    const { data } = await request<any[]>({
      url: "/api/admin/system-config",
    });
    if (data) {
      const registerConfig = data.find((c: any) => c.key === "register_enabled");
      registerEnabled.value = registerConfig?.value === "true";

      const telegramEnabledConfig = data.find((c: any) => c.key === "telegram_enabled");
      telegramEnabled.value = telegramEnabledConfig?.value !== "false";

      const telegramTokenConfig = data.find((c: any) => c.key === "telegram_bot_token");
      telegramBotToken.value = telegramTokenConfig?.value || "";

      const telegramWebhookUrlConfig = data.find(
        (c: any) => c.key === "telegram_webhook_url"
      );
      telegramWebhookUrl.value = telegramWebhookUrlConfig?.value || "";

      const aiTimeoutConfig = data.find((c: any) => c.key === "ai_timeout_minutes");
      aiTimeoutMinutes.value = aiTimeoutConfig ? Number(aiTimeoutConfig.value) : 5;

      const aiTlsConfig = data.find((c: any) => c.key === "ai_tls_handshake_timeout");
      aiTlsHandshakeTimeout.value = aiTlsConfig ? Number(aiTlsConfig.value) : 15;

      const aiResponseConfig = data.find(
        (c: any) => c.key === "ai_response_header_timeout"
      );
      aiResponseHeaderTimeout.value = aiResponseConfig
        ? Number(aiResponseConfig.value)
        : 30;

      const httpTimeoutConfig = data.find((c: any) => c.key === "http_timeout_seconds");
      httpTimeoutSeconds.value = httpTimeoutConfig ? Number(httpTimeoutConfig.value) : 30;

      const smtpEnabledConfig = data.find((c: any) => c.key === "smtp_enabled");
      smtpEnabled.value = smtpEnabledConfig?.value === "true";

      const smtpHostConfig = data.find((c: any) => c.key === "smtp_host");
      smtpHost.value = smtpHostConfig?.value || "";

      const smtpPortConfig = data.find((c: any) => c.key === "smtp_port");
      smtpPort.value = smtpPortConfig ? Number(smtpPortConfig.value) : 587;

      const smtpEncryptionConfig = data.find((c: any) => c.key === "smtp_encryption");
      smtpEncryption.value = smtpEncryptionConfig?.value || "ssl";

      const smtpUserConfig = data.find((c: any) => c.key === "smtp_user");
      smtpUser.value = smtpUserConfig?.value || "";

      const smtpPasswordConfig = data.find((c: any) => c.key === "smtp_password");
      smtpPassword.value = smtpPasswordConfig?.value || "";

      const smtpFromConfig = data.find((c: any) => c.key === "smtp_from");
      smtpFrom.value = smtpFromConfig?.value || "";

      const smtpFromNameConfig = data.find((c: any) => c.key === "smtp_from_name");
      smtpFromName.value = smtpFromNameConfig?.value || "";

      const memoryEnabledConfig = data.find(
        (c: any) => c.key === "memory_extraction_enabled"
      );
      memoryEnabled.value = memoryEnabledConfig?.value !== "false";

      const memoryModelConfig = data.find((c: any) => c.key === "memory_extraction_model");
      memoryExtractionModel.value = memoryModelConfig?.value || "";

      const memoryIdleConfig = data.find(
        (c: any) => c.key === "memory_session_idle_minutes"
      );
      memoryIdleMinutes.value = memoryIdleConfig ? Number(memoryIdleConfig.value) : 15;

      const memoryMinConfig = data.find(
        (c: any) => c.key === "memory_min_min_user_messages"
      );
      memoryMinUserMessages.value = memoryMinConfig ? Number(memoryMinConfig.value) : 6;

      const s3EnabledConfig = data.find((c: any) => c.key === "s3_enabled");
      s3Enabled.value = s3EnabledConfig?.value === "true";

      const s3EndpointConfig = data.find((c: any) => c.key === "s3_endpoint");
      s3Endpoint.value = s3EndpointConfig?.value || "";

      const s3RegionConfig = data.find((c: any) => c.key === "s3_region");
      s3Region.value = s3RegionConfig?.value || "";

      const s3BucketConfig = data.find((c: any) => c.key === "s3_bucket");
      s3Bucket.value = s3BucketConfig?.value || "";

      const s3AccessKeyConfig = data.find((c: any) => c.key === "s3_access_key");
      s3AccessKey.value = s3AccessKeyConfig?.value || "";

      const s3SecretKeyConfig = data.find((c: any) => c.key === "s3_secret_key");
      s3SecretKey.value = s3SecretKeyConfig?.value || "";

      const s3SecureConfig = data.find((c: any) => c.key === "s3_secure");
      s3Secure.value = s3SecureConfig?.value === "true";

      const s3PathStyleConfig = data.find((c: any) => c.key === "s3_use_path_style");
      s3UsePathStyle.value = s3PathStyleConfig?.value === "true";

      const s3MaxConfig = data.find((c: any) => c.key === "s3_max_upload_mb");
      s3MaxUploadMB.value = s3MaxConfig ? Number(s3MaxConfig.value) : 20;

      const ragEnabledConfig = data.find((c: any) => c.key === "rag_enabled");
      ragEnabled.value = ragEnabledConfig?.value === "true";

      const ragBaseUrlConfig = data.find((c: any) => c.key === "rag_embedding_base_url");
      ragBaseUrl.value = ragBaseUrlConfig?.value || "";

      const ragModelConfig = data.find((c: any) => c.key === "rag_embedding_model");
      ragModel.value = ragModelConfig?.value || "";

      const ragApiKeyConfig = data.find((c: any) => c.key === "rag_embedding_api_key");
      ragApiKey.value = ragApiKeyConfig?.value || "";

      const ragChunkSizeConfig = data.find((c: any) => c.key === "rag_chunk_size");
      ragChunkSize.value = ragChunkSizeConfig ? Number(ragChunkSizeConfig.value) : 500;

      const ragOverlapConfig = data.find((c: any) => c.key === "rag_chunk_overlap");
      ragChunkOverlap.value = ragOverlapConfig ? Number(ragOverlapConfig.value) : 80;

      const ragTopKConfig = data.find((c: any) => c.key === "rag_top_k");
      ragTopK.value = ragTopKConfig ? Number(ragTopKConfig.value) : 5;
    }
  } catch (err: any) {
    message.error($t("page.system.config.loadFailed", { error: err?.message || $t("page.system.config.unknownError") }));
  } finally {
    loading.value = false;
  }
}

async function saveConfig(key: string, value: string, remark: string) {
  await request({
    url: "/api/admin/system-config",
    method: "put",
    data: { key, value, remark },
  });
}

async function handleToggleRegister(val: boolean) {
  savingRegister.value = true;
  try {
    await saveConfig("register_enabled", val ? "true" : "false", $t("page.system.config.remarkRegister"));
    message.success(val ? $t("page.system.config.registerEnabledMsg") : $t("page.system.config.registerDisabledMsg"));
  } catch (err: any) {
    registerEnabled.value = !val;
    message.error($t("page.system.config.saveFailed", { error: err?.message || $t("page.system.config.unknownError") }));
  } finally {
    savingRegister.value = false;
  }
}

async function handleToggleTelegram(val: boolean) {
  savingTelegram.value = true;
  try {
    await saveConfig("telegram_enabled", val ? "true" : "false", $t("page.system.config.remarkTelegramEnabled"));
    message.success(val ? $t("page.system.config.telegramEnabledMsg") : $t("page.system.config.telegramDisabledMsg"));
  } catch (err: any) {
    telegramEnabled.value = !val;
    message.error($t("page.system.config.saveFailed", { error: err?.message || $t("page.system.config.unknownError") }));
  } finally {
    savingTelegram.value = false;
  }
}

async function handleSaveTelegram() {
  savingTelegram.value = true;
  try {
    await saveConfig("telegram_bot_token", telegramBotToken.value, "Telegram Bot Token");
    await saveConfig(
      "telegram_webhook_url",
      telegramWebhookUrl.value,
      $t("page.system.config.remarkTelegramWebhook")
    );
    message.success($t("page.system.config.telegramSaved"));
  } catch (err: any) {
    message.error($t("page.system.config.saveFailed", { error: err?.message || $t("page.system.config.unknownError") }));
  } finally {
    savingTelegram.value = false;
  }
}

async function handleSaveTimeout() {
  savingTimeout.value = true;
  try {
    await saveConfig(
      "ai_timeout_minutes",
      String(aiTimeoutMinutes.value),
      $t("page.system.config.remarkAiTimeout")
    );
    await saveConfig(
      "ai_tls_handshake_timeout",
      String(aiTlsHandshakeTimeout.value),
      $t("page.system.config.remarkAiTlsHandshake")
    );
    await saveConfig(
      "ai_response_header_timeout",
      String(aiResponseHeaderTimeout.value),
      $t("page.system.config.remarkAiResponseHeader")
    );
    await saveConfig(
      "http_timeout_seconds",
      String(httpTimeoutSeconds.value),
      $t("page.system.config.remarkHttpTimeout")
    );
    message.success($t("page.system.config.timeoutSaved"));
  } catch (err: any) {
    message.error($t("page.system.config.saveFailed", { error: err?.message || $t("page.system.config.unknownError") }));
  } finally {
    savingTimeout.value = false;
  }
}

async function handleToggleSmtp(val: boolean) {
  savingSmtp.value = true;
  try {
    await saveConfig("smtp_enabled", val ? "true" : "false", $t("page.system.config.remarkSmtpEnabled"));
    message.success(val ? $t("page.system.config.smtpEnabledMsg") : $t("page.system.config.smtpDisabledMsg"));
  } catch (err: any) {
    smtpEnabled.value = !val;
    message.error($t("page.system.config.saveFailed", { error: err?.message || $t("page.system.config.unknownError") }));
  } finally {
    savingSmtp.value = false;
  }
}

async function handleSaveSmtp() {
  savingSmtp.value = true;
  try {
    await saveConfig("smtp_host", smtpHost.value, $t("page.system.config.remarkSmtpHost"));
    await saveConfig("smtp_port", String(smtpPort.value), $t("page.system.config.remarkSmtpPort"));
    await saveConfig(
      "smtp_encryption",
      smtpEncryption.value,
      $t("page.system.config.remarkSmtpEncryption")
    );
    await saveConfig("smtp_user", smtpUser.value, $t("page.system.config.remarkSmtpUser"));
    await saveConfig("smtp_password", smtpPassword.value, $t("page.system.config.remarkSmtpPassword"));
    await saveConfig("smtp_from", smtpFrom.value, $t("page.system.config.remarkSmtpFrom"));
    await saveConfig("smtp_from_name", smtpFromName.value, $t("page.system.config.remarkSmtpFromName"));
    message.success($t("page.system.config.smtpSaved"));
  } catch (err: any) {
    message.error($t("page.system.config.saveFailed", { error: err?.message || $t("page.system.config.unknownError") }));
  } finally {
    savingSmtp.value = false;
  }
}

async function handleToggleS3(val: boolean) {
  savingS3.value = true;
  try {
    await saveConfig("s3_enabled", val ? "true" : "false", $t("page.system.config.remarkS3Enabled"));
    message.success(val ? $t("page.system.config.s3EnabledMsg") : $t("page.system.config.s3DisabledMsg"));
  } catch (err: any) {
    s3Enabled.value = !val;
    message.error($t("page.system.config.saveFailed", { error: err?.message || $t("page.system.config.unknownError") }));
  } finally {
    savingS3.value = false;
  }
}

async function handleSaveS3() {
  savingS3.value = true;
  try {
    await saveConfig("s3_enabled", s3Enabled.value ? "true" : "false", $t("page.system.config.remarkS3Enabled"));
    await saveConfig("s3_endpoint", s3Endpoint.value, $t("page.system.config.remarkS3Endpoint"));
    await saveConfig("s3_region", s3Region.value, $t("page.system.config.remarkS3Region"));
    await saveConfig("s3_bucket", s3Bucket.value, "S3 Bucket");
    await saveConfig("s3_access_key", s3AccessKey.value, "S3 Access Key");
    await saveConfig("s3_secret_key", s3SecretKey.value, "S3 Secret Key");
    await saveConfig("s3_secure", s3Secure.value ? "true" : "false", $t("page.system.config.remarkS3Secure"));
    await saveConfig(
      "s3_use_path_style",
      s3UsePathStyle.value ? "true" : "false",
      $t("page.system.config.remarkS3PathStyle")
    );
    await saveConfig("s3_max_upload_mb", String(s3MaxUploadMB.value), $t("page.system.config.remarkS3MaxUpload"));
    message.success($t("page.system.config.s3Saved"));
  } catch (err: any) {
    message.error($t("page.system.config.saveFailed", { error: err?.message || $t("page.system.config.unknownError") }));
  } finally {
    savingS3.value = false;
  }
}

async function handleTestS3() {
  if (!s3Endpoint.value || !s3Bucket.value || !s3AccessKey.value || !s3SecretKey.value) {
    message.warning($t("page.system.config.s3TestRequired"));
    return;
  }
  testingS3.value = true;
  try {
    const { data, error } = await request<{ message: string }>({
      url: "/api/admin/system-config/test-s3",
      method: "post",
      data: {
        endpoint: s3Endpoint.value,
        region: s3Region.value,
        bucket: s3Bucket.value,
        access_key: s3AccessKey.value,
        secret_key: s3SecretKey.value,
        secure: s3Secure.value,
        use_path_style: s3UsePathStyle.value
      }
    });
    if (error || !data) {
      message.error($t("page.system.config.s3TestFailed", { error: error?.message || $t("page.system.config.unknownError") }));
      return;
    }
    message.success(data.message || $t("page.system.config.s3TestSuccess"));
  } catch (err: any) {
    message.error($t("page.system.config.s3TestFailed", { error: err?.message || $t("page.system.config.unknownError") }));
  } finally {
    testingS3.value = false;
  }
}

async function handleToggleRag(val: boolean) {
  savingRag.value = true;
  try {
    await saveConfig("rag_enabled", val ? "true" : "false", $t("page.system.config.remarkRagEnabled"));
    message.success(val ? $t("page.system.config.ragEnabledMsg") : $t("page.system.config.ragDisabledMsg"));
  } catch (err: any) {
    ragEnabled.value = !val;
    message.error($t("page.system.config.saveFailed", { error: err?.message || $t("page.system.config.unknownError") }));
  } finally {
    savingRag.value = false;
  }
}

async function handleSaveRag() {
  savingRag.value = true;
  try {
    await saveConfig("rag_enabled", ragEnabled.value ? "true" : "false", $t("page.system.config.remarkRagEnabled"));
    await saveConfig(
      "rag_embedding_base_url",
      ragBaseUrl.value,
      $t("page.system.config.remarkRagBaseUrl")
    );
    await saveConfig("rag_embedding_model", ragModel.value, $t("page.system.config.remarkRagModel"));
    await saveConfig("rag_embedding_api_key", ragApiKey.value, $t("page.system.config.remarkRagApiKey"));
    await saveConfig("rag_chunk_size", String(ragChunkSize.value), $t("page.system.config.remarkRagChunkSize"));
    await saveConfig("rag_chunk_overlap", String(ragChunkOverlap.value), $t("page.system.config.remarkRagChunkOverlap"));
    await saveConfig("rag_top_k", String(ragTopK.value), $t("page.system.config.remarkRagTopK"));
    message.success($t("page.system.config.ragSaved"));
  } catch (err: any) {
    message.error($t("page.system.config.saveFailed", { error: err?.message || $t("page.system.config.unknownError") }));
  } finally {
    savingRag.value = false;
  }
}

async function handleTestEmbedding() {
  if (!ragBaseUrl.value || !ragModel.value) {
    message.warning($t("page.system.config.embeddingConfigRequired"));
    return;
  }
  testingEmbedding.value = true;
  try {
    const { data, error } = await request<{ dims: number; message: string }>({
      url: "/api/admin/system-config/test-embedding",
      method: "post",
      data: {
        base_url: ragBaseUrl.value,
        model: ragModel.value,
        api_key: ragApiKey.value
      }
    });
    if (error || !data) {
      message.error(error?.message || $t("page.system.config.embeddingTestFailed"));
      return;
    }
    message.success(data.message || $t("page.system.config.connectionSuccess"));
  } catch (err: any) {
    message.error($t("page.system.config.embeddingTestFailedWithReason", { error: err?.message || $t("page.system.config.unknownError") }));
  } finally {
    testingEmbedding.value = false;
  }
}

async function handleToggleMemory(val: boolean) {
  savingMemory.value = true;
  try {
    await saveConfig(
      "memory_extraction_enabled",
      val ? "true" : "false",
      $t("page.system.config.remarkMemoryEnabled")
    );
    message.success(val ? $t("page.system.config.memoryEnabledMsg") : $t("page.system.config.memoryDisabledMsg"));
  } catch (err: any) {
    memoryEnabled.value = !val;
    message.error($t("page.system.config.saveFailed", { error: err?.message || $t("page.system.config.unknownError") }));
  } finally {
    savingMemory.value = false;
  }
}

async function handleSaveMemory() {
  savingMemory.value = true;
  try {
    await saveConfig(
      "memory_extraction_model",
      memoryExtractionModel.value,
      $t("page.system.config.remarkMemoryModel")
    );
    await saveConfig(
      "memory_session_idle_minutes",
      String(memoryIdleMinutes.value),
      $t("page.system.config.remarkMemoryIdle")
    );
    await saveConfig(
      "memory_min_min_user_messages",
      String(memoryMinUserMessages.value),
      $t("page.system.config.remarkMemoryMinMessages")
    );
    message.success($t("page.system.config.memorySaved"));
  } catch (err: any) {
    message.error($t("page.system.config.saveFailed", { error: err?.message || $t("page.system.config.unknownError") }));
  } finally {
    savingMemory.value = false;
  }
}

function openTestEmailModal() {
  testEmailForm.value = {
    email: "",
    subject: $t("page.system.config.testEmailSubject"),
    content: $t("page.system.config.testEmailContent"),
  };
  showTestEmailModal.value = true;
}

async function handleSendTestEmail() {
  if (!testEmailForm.value.email) {
    message.warning($t("page.system.config.recipientRequired"));
    return;
  }
  sendingTestEmail.value = true;
  try {
    await request({
      url: "/api/admin/system-config/test-email",
      method: "post",
      data: {
        email: testEmailForm.value.email,
        subject: testEmailForm.value.subject,
        content: testEmailForm.value.content,
      },
    });
    message.success($t("page.system.config.testEmailSent"));
    showTestEmailModal.value = false;
  } catch (err: any) {
    message.error($t("page.system.config.sendFailed", { error: err?.message || $t("page.system.config.unknownError") }));
  } finally {
    sendingTestEmail.value = false;
  }
}

onMounted(() => {
  loadConfig();
});
</script>

<template>
  <div class="h-full overflow-auto p-6">
    <NCard :bordered="false" shadow="sm" :title="$t('page.system.config.title')">
      <NSpin :show="loading">
        <div class="space-y-6">
          <!-- 注册开关 -->
          <div
            class="flex items-center justify-between p-4 rounded-lg border border-gray-200 dark:border-gray-700"
          >
            <div>
              <div class="font-bold text-gray-800 dark:text-gray-200">{{ $t("page.system.config.register") }}</div>
              <div class="text-sm text-gray-500 dark:text-gray-400 mt-1">
                {{ $t("page.system.config.registerDesc") }}
              </div>
            </div>
            <NSwitch
              v-model:value="registerEnabled"
              :loading="savingRegister"
              @update:value="handleToggleRegister"
            >
              <template #checked>{{ $t("page.system.config.on") }}</template>
              <template #unchecked>{{ $t("page.system.config.off") }}</template>
            </NSwitch>
          </div>

          <!-- Telegram Bot 配置 -->
          <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700">
            <div class="flex items-center justify-between mb-4">
            <div>
              <div class="font-bold text-gray-800 dark:text-gray-200">{{ $t("page.system.config.telegram") }}</div>
              <div class="text-sm text-gray-500 dark:text-gray-400 mt-1">
                {{ $t("page.system.config.telegramDesc") }}
              </div>
            </div>
              <NSwitch
                v-model:value="telegramEnabled"
                :loading="savingTelegram"
                @update:value="handleToggleTelegram"
              >
                <template #checked>{{ $t("page.system.config.on") }}</template>
                <template #unchecked>{{ $t("page.system.config.off") }}</template>
              </NSwitch>
            </div>
            <NForm label-placement="left" label-width="100">
              <NFormItem :label="$t('page.system.config.botToken')">
                <NInput
                  v-model:value="telegramBotToken"
                  :type="showTelegramToken ? 'text' : 'password'"
                  :placeholder="$t('page.system.config.botTokenPlaceholder')"
                  :disabled="!telegramEnabled"
                >
                  <template #suffix>
                    <div
                      class="cursor-pointer text-gray-400 hover:text-gray-600"
                      :class="showTelegramToken ? 'i-mdi:eye-off' : 'i-mdi:eye'"
                      @click="showTelegramToken = !showTelegramToken"
                    />
                  </template>
                </NInput>
              </NFormItem>
              <NFormItem :label="$t('page.system.config.webhookUrl')">
                <NInput
                  v-model:value="telegramWebhookUrl"
                  :placeholder="$t('page.system.config.webhookPlaceholder')"
                  :disabled="!telegramEnabled"
                />
              </NFormItem>
              <NFormItem>
                <NButton
                  type="primary"
                  :loading="savingTelegram"
                  :disabled="!telegramEnabled"
                  @click="handleSaveTelegram"
                >
                  {{ $t("page.system.config.saveTelegram") }}
                </NButton>
              </NFormItem>
            </NForm>
          </div>

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

          <!-- 用户画像 / 经历抽取配置 -->
          <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700">
            <div class="flex items-center justify-between mb-4">
            <div>
              <div class="font-bold text-gray-800 dark:text-gray-200">
                {{ $t("page.system.config.memory") }}
              </div>
              <div class="text-sm text-gray-500 dark:text-gray-400 mt-1">
                {{ $t("page.system.config.memoryDesc") }}
              </div>
            </div>
              <NSwitch
                v-model:value="memoryEnabled"
                :loading="savingMemory"
                @update:value="handleToggleMemory"
              >
                <template #checked>{{ $t("page.system.config.on") }}</template>
                <template #unchecked>{{ $t("page.system.config.off") }}</template>
              </NSwitch>
            </div>
            <NForm label-placement="left" label-width="120">
              <NGrid :cols="2" :x-gap="12" :y-gap="8">
                <NFormItemGi :label="$t('page.system.config.extractionModel')">
                  <NInput
                    v-model:value="memoryExtractionModel"
                    :placeholder="$t('page.system.config.extractionModelPlaceholder')"
                    :disabled="!memoryEnabled"
                  />
                </NFormItemGi>
                <NFormItemGi :label="$t('page.system.config.sessionIdle')">
                  <NInputNumber
                    v-model:value="memoryIdleMinutes"
                    :min="1"
                    :max="1440"
                    :disabled="!memoryEnabled"
                    size="small"
                  >
                    <template #suffix>{{ $t("page.system.config.minutes") }}</template>
                  </NInputNumber>
                </NFormItemGi>
                <NFormItemGi :label="$t('page.system.config.minUserMessages')">
                  <NInputNumber
                    v-model:value="memoryMinUserMessages"
                    :min="1"
                    :max="100"
                    :disabled="!memoryEnabled"
                    size="small"
                  >
                    <template #suffix>{{ $t("page.system.config.messages") }}</template>
                  </NInputNumber>
                </NFormItemGi>
              </NGrid>
              <NFormItem class="mt-4">
                <NButton
                  type="primary"
                  :loading="savingMemory"
                  :disabled="!memoryEnabled"
                  @click="handleSaveMemory"
                >
                  {{ $t("page.system.config.saveMemory") }}
                </NButton>
              </NFormItem>
            </NForm>
          </div>

          <!-- SMTP 邮件配置 -->
          <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700">
            <div class="flex items-center justify-between mb-4">
            <div>
              <div class="font-bold text-gray-800 dark:text-gray-200">
                {{ $t("page.system.config.smtp") }}
              </div>
              <div class="text-sm text-gray-500 dark:text-gray-400 mt-1">
                {{ $t("page.system.config.smtpDesc") }}
              </div>
            </div>
              <NSwitch
                v-model:value="smtpEnabled"
                :loading="savingSmtp"
                @update:value="handleToggleSmtp"
              >
                <template #checked>{{ $t("page.system.config.on") }}</template>
                <template #unchecked>{{ $t("page.system.config.off") }}</template>
              </NSwitch>
            </div>
            <NForm label-placement="left" label-width="120">
              <NGrid :cols="2" :x-gap="12" :y-gap="8">
                <NFormItemGi :label="$t('page.system.config.smtpHost')" path="smtpHost">
                  <NInput
                    v-model:value="smtpHost"
                    placeholder="smtp.example.com"
                    :disabled="!smtpEnabled"
                  />
                </NFormItemGi>
                <NFormItemGi :label="$t('page.system.config.smtpPort')" path="smtpPort">
                  <NInputNumber
                    v-model:value="smtpPort"
                    :min="1"
                    :max="65535"
                    :disabled="!smtpEnabled"
                    size="small"
                  />
                </NFormItemGi>
                <NFormItemGi :label="$t('page.system.config.encryption')" path="smtpEncryption">
                  <NSelect
                    v-model:value="smtpEncryption"
                    :options="[
                      { label: 'SSL/TLS', value: 'ssl' },
                      { label: 'STARTTLS', value: 'starttls' },
                      { label: $t('page.system.config.noEncryption'), value: 'none' },
                    ]"
                    :disabled="!smtpEnabled"
                  />
                </NFormItemGi>
                <NFormItemGi :label="$t('page.system.config.username')" path="smtpUser">
                  <NInput
                    v-model:value="smtpUser"
                    :placeholder="$t('page.system.config.usernamePlaceholder')"
                    :disabled="!smtpEnabled"
                  />
                </NFormItemGi>
                <NFormItemGi :label="$t('page.system.config.password')" path="smtpPassword">
                  <NInput
                    v-model:value="smtpPassword"
                    :type="showSmtpPassword ? 'text' : 'password'"
                    :placeholder="$t('page.system.config.passwordPlaceholder')"
                    :disabled="!smtpEnabled"
                  >
                    <template #suffix>
                      <div
                        class="cursor-pointer text-gray-400 hover:text-gray-600"
                        :class="showSmtpPassword ? 'i-mdi:eye-off' : 'i-mdi:eye'"
                        @click="showSmtpPassword = !showSmtpPassword"
                      />
                    </template>
                  </NInput>
                </NFormItemGi>
                <NFormItemGi :label="$t('page.system.config.fromEmail')" path="smtpFrom">
                  <NInput
                    v-model:value="smtpFrom"
                    placeholder="noreply@example.com"
                    :disabled="!smtpEnabled"
                  />
                </NFormItemGi>
                <NFormItemGi :label="$t('page.system.config.fromName')" path="smtpFromName">
                  <NInput
                    v-model:value="smtpFromName"
                    :placeholder="$t('page.system.config.fromNamePlaceholder')"
                    :disabled="!smtpEnabled"
                  />
                </NFormItemGi>
              </NGrid>
              <NFormItem class="mt-4">
                <NSpace>
                  <NButton
                    type="primary"
                    :loading="savingSmtp"
                    :disabled="!smtpEnabled"
                    @click="handleSaveSmtp"
                  >
                    {{ $t("page.system.config.saveSmtp") }}
                  </NButton>
                  <NButton
                    type="info"
                    :disabled="!smtpEnabled"
                    @click="openTestEmailModal"
                  >
                    <template #icon>
                      <SvgIcon icon="mdi:email-fast-outline" />
                    </template>
                    {{ $t("page.system.config.sendTestEmail") }}
                  </NButton>
                </NSpace>
              </NFormItem>
            </NForm>
          </div>

          <!-- S3 用户文档存储 -->
          <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700">
            <div class="flex items-center justify-between mb-4">
            <div>
              <div class="font-bold text-gray-800 dark:text-gray-200">
                {{ $t("page.system.config.s3") }}
              </div>
              <div class="text-sm text-gray-500 dark:text-gray-400 mt-1">
                {{ $t("page.system.config.s3Desc") }}
              </div>
            </div>
              <NSwitch
                v-model:value="s3Enabled"
                :loading="savingS3"
                @update:value="handleToggleS3"
              >
                <template #checked>{{ $t("page.system.config.on") }}</template>
                <template #unchecked>{{ $t("page.system.config.off") }}</template>
              </NSwitch>
            </div>
            <NForm label-placement="left" label-width="120">
              <NGrid :cols="2" :x-gap="12" :y-gap="8">
                <NFormItemGi label="Endpoint" path="s3Endpoint">
                  <NInput
                    v-model:value="s3Endpoint"
                    :placeholder="$t('page.system.config.s3EndpointPlaceholder')"
                    :disabled="!s3Enabled"
                  />
                </NFormItemGi>
                <NFormItemGi label="Bucket" path="s3Bucket">
                  <NInput
                    v-model:value="s3Bucket"
                    placeholder="user-documents"
                    :disabled="!s3Enabled"
                  />
                </NFormItemGi>
                <NFormItemGi label="Access Key" path="s3AccessKey">
                  <NInput
                    v-model:value="s3AccessKey"
                    placeholder="Access Key ID"
                    :disabled="!s3Enabled"
                  />
                </NFormItemGi>
                <NFormItemGi label="Secret Key" path="s3SecretKey">
                  <NInput
                    v-model:value="s3SecretKey"
                    :type="showS3SecretKey ? 'text' : 'password'"
                    placeholder="Secret Access Key"
                    :disabled="!s3Enabled"
                  >
                    <template #suffix>
                      <div
                        class="cursor-pointer text-gray-400 hover:text-gray-600"
                        :class="showS3SecretKey ? 'i-mdi:eye-off' : 'i-mdi:eye'"
                        @click="showS3SecretKey = !showS3SecretKey"
                      />
                    </template>
                  </NInput>
                </NFormItemGi>
                <NFormItemGi label="Region" path="s3Region">
                  <NInput
                    v-model:value="s3Region"
                    :placeholder="$t('page.system.config.s3RegionPlaceholder')"
                    :disabled="!s3Enabled"
                  />
                </NFormItemGi>
                <NFormItemGi :label="$t('page.system.config.s3MaxUpload')" path="s3MaxUploadMB">
                  <NInputNumber
                    v-model:value="s3MaxUploadMB"
                    :min="1"
                    :max="512"
                    :disabled="!s3Enabled"
                    size="small"
                  />
                </NFormItemGi>
                <NFormItemGi label="HTTPS" path="s3Secure">
                  <NSwitch v-model:value="s3Secure" :disabled="!s3Enabled">
                    <template #checked>HTTPS</template>
                    <template #unchecked>HTTP</template>
                  </NSwitch>
                </NFormItemGi>
                <NFormItemGi label="Path-Style" path="s3UsePathStyle">
                  <NSwitch v-model:value="s3UsePathStyle" :disabled="!s3Enabled">
                    <template #checked>Path-Style</template>
                    <template #unchecked>{{ $t("page.system.config.virtualHostStyle") }}</template>
                  </NSwitch>
                </NFormItemGi>
              </NGrid>
              <NFormItem class="mt-4">
                <div class="flex items-center gap-3">
                  <NButton
                    type="primary"
                    :loading="savingS3"
                    :disabled="!s3Enabled"
                    @click="handleSaveS3"
                  >
                    {{ $t("page.system.config.saveS3") }}
                  </NButton>
                  <NButton
                    secondary
                    :loading="testingS3"
                    :disabled="!s3Enabled"
                    @click="handleTestS3"
                  >
                    {{ $t("page.system.config.testConnection") }}
                  </NButton>
                </div>
              </NFormItem>
            </NForm>
          </div>

          <!-- 文档 RAG (语义检索) -->
          <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700">
            <div class="flex items-center justify-between mb-4">
            <div>
              <div class="font-bold text-gray-800 dark:text-gray-200">
                {{ $t("page.system.config.rag") }}
              </div>
              <div class="text-sm text-gray-500 dark:text-gray-400 mt-1">
                {{ $t("page.system.config.ragDesc") }}
              </div>
            </div>
              <NSwitch
                v-model:value="ragEnabled"
                :loading="savingRag"
                @update:value="handleToggleRag"
              >
                <template #checked>{{ $t("page.system.config.on") }}</template>
                <template #unchecked>{{ $t("page.system.config.off") }}</template>
              </NSwitch>
            </div>
            <NForm label-placement="left" label-width="120">
              <NGrid :cols="2" :x-gap="12" :y-gap="8">
                <NFormItemGi :label="$t('page.system.config.ragBaseUrl')" path="ragBaseUrl">
                  <NInput
                    v-model:value="ragBaseUrl"
                    placeholder="http://127.0.0.1:11434/v1"
                    :disabled="!ragEnabled"
                  />
                </NFormItemGi>
                <NFormItemGi :label="$t('page.system.config.ragModel')" path="ragModel">
                  <NInput
                    v-model:value="ragModel"
                    placeholder="bge-m3"
                    :disabled="!ragEnabled"
                  />
                </NFormItemGi>
                <NFormItemGi label="API Key" path="ragApiKey">
                  <NInput
                    v-model:value="ragApiKey"
                    :type="showRagApiKey ? 'text' : 'password'"
                    :placeholder="$t('page.system.config.ragApiKeyPlaceholder')"
                    :disabled="!ragEnabled"
                  >
                    <template #suffix>
                      <div
                        class="cursor-pointer text-gray-400 hover:text-gray-600"
                        :class="showRagApiKey ? 'i-mdi:eye-off' : 'i-mdi:eye'"
                        @click="showRagApiKey = !showRagApiKey"
                      />
                    </template>
                  </NInput>
                </NFormItemGi>
                <NFormItemGi :label="$t('page.system.config.chunkSize')" path="ragChunkSize">
                  <NInputNumber
                    v-model:value="ragChunkSize"
                    :min="100"
                    :max="2000"
                    :disabled="!ragEnabled"
                    size="small"
                  />
                </NFormItemGi>
                <NFormItemGi :label="$t('page.system.config.chunkOverlap')" path="ragChunkOverlap">
                  <NInputNumber
                    v-model:value="ragChunkOverlap"
                    :min="0"
                    :max="500"
                    :disabled="!ragEnabled"
                    size="small"
                  />
                </NFormItemGi>
                <NFormItemGi :label="$t('page.system.config.topK')" path="ragTopK">
                  <NInputNumber
                    v-model:value="ragTopK"
                    :min="1"
                    :max="20"
                    :disabled="!ragEnabled"
                    size="small"
                  />
                </NFormItemGi>
              </NGrid>
              <NFormItem class="mt-4">
                <div class="flex items-center gap-3">
                  <NButton
                    type="primary"
                    :loading="savingRag"
                    :disabled="!ragEnabled"
                    @click="handleSaveRag"
                  >
                    {{ $t("page.system.config.saveRag") }}
                  </NButton>
                  <NButton
                    secondary
                    :loading="testingEmbedding"
                    :disabled="!ragEnabled"
                    @click="handleTestEmbedding"
                  >
                    {{ $t("page.system.config.testConnection") }}
                  </NButton>
                </div>
              </NFormItem>
            </NForm>
          </div>
        </div>
      </NSpin>
    </NCard>

    <!-- 发送测试邮件弹窗 -->
    <NModal
      v-model:show="showTestEmailModal"
      preset="card"
      :title="$t('page.system.config.sendTestEmailTitle')"
      style="width: 500px"
    >
      <NForm label-placement="left" label-width="80">
        <NFormItem :label="$t('page.system.config.recipient')" required>
          <NInput v-model:value="testEmailForm.email" :placeholder="$t('page.system.config.recipientPlaceholder')" />
        </NFormItem>
        <NFormItem :label="$t('page.system.config.emailSubject')">
          <NInput v-model:value="testEmailForm.subject" :placeholder="$t('page.system.config.testEmailSubject')" />
        </NFormItem>
        <NFormItem :label="$t('page.system.config.emailContent')">
          <NInput
            v-model:value="testEmailForm.content"
            type="textarea"
            :placeholder="$t('page.system.config.emailContentPlaceholder')"
            :rows="6"
          />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showTestEmailModal = false">{{ $t("common.cancel") }}</NButton>
          <NButton type="primary" :loading="sendingTestEmail" @click="handleSendTestEmail">
            {{ $t("page.system.config.send") }}
          </NButton>
        </NSpace>
      </template>
    </NModal>
  </div>
</template>
