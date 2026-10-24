<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useMessage } from "naive-ui";
import { request } from "@/service/request";
import { getWolConfig, updateWolConfig } from "@/service/api";
import { $t } from "@/locales";

defineOptions({ name: "SystemConfig" });

/** system_config 表条目 (value 恒为字符串, 布尔/数字序列化后存入) */
interface ConfigItem {
  key: string;
  value: string;
  remark?: string;
}

/** 配置条目数组转 key -> value 映射, 消除逐条 data.find 样板 */
function toConfigMap(items: ConfigItem[]): Record<string, string> {
  const map: Record<string, string> = {};
  for (const item of items) map[item.key] = item.value;
  return map;
}

/** "true" 才视为开 (register/smtp/s3/rag 等缺省关闭的开关) */
function isTrue(value: string | undefined): boolean {
  return value === "true";
}

/** 仅 "false" 视为关 (telegram/memory 等缺省开启的开关) */
function notFalse(value: string | undefined): boolean {
  return value !== "false";
}

function toNumber(value: string | undefined, fallback: number): number {
  if (value === undefined || value === "") return fallback;
  return Number(value);
}

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
const savingMqtt = ref(false);
const showMqttPassword = ref(false);
const mqttHost = ref("");
const mqttPort = ref(1883);
const mqttUsername = ref("");
const mqttPassword = ref("");

async function loadConfig() {
  loading.value = true;
  try {
    // API 失败时全局拦截器已 toast, 这里静默返回
    const { data, error } = await request<ConfigItem[]>({
      url: "/api/admin/system-config",
    });
    if (!error && data) {
      const cfg = toConfigMap(data);

      registerEnabled.value = isTrue(cfg.register_enabled);

      telegramEnabled.value = notFalse(cfg.telegram_enabled);
      telegramBotToken.value = cfg.telegram_bot_token ?? "";
      telegramWebhookUrl.value = cfg.telegram_webhook_url ?? "";

      aiTimeoutMinutes.value = toNumber(cfg.ai_timeout_minutes, 5);
      aiTlsHandshakeTimeout.value = toNumber(cfg.ai_tls_handshake_timeout, 15);
      aiResponseHeaderTimeout.value = toNumber(cfg.ai_response_header_timeout, 30);
      httpTimeoutSeconds.value = toNumber(cfg.http_timeout_seconds, 30);

      smtpEnabled.value = isTrue(cfg.smtp_enabled);
      smtpHost.value = cfg.smtp_host ?? "";
      smtpPort.value = toNumber(cfg.smtp_port, 587);
      smtpEncryption.value = cfg.smtp_encryption ?? "ssl";
      smtpUser.value = cfg.smtp_user ?? "";
      smtpPassword.value = cfg.smtp_password ?? "";
      smtpFrom.value = cfg.smtp_from ?? "";
      smtpFromName.value = cfg.smtp_from_name ?? "";

      memoryEnabled.value = notFalse(cfg.memory_extraction_enabled);
      memoryExtractionModel.value = cfg.memory_extraction_model ?? "";
      memoryIdleMinutes.value = toNumber(cfg.memory_session_idle_minutes, 15);
      memoryMinUserMessages.value = toNumber(cfg.memory_min_min_user_messages, 6);

      s3Enabled.value = isTrue(cfg.s3_enabled);
      s3Endpoint.value = cfg.s3_endpoint ?? "";
      s3Region.value = cfg.s3_region ?? "";
      s3Bucket.value = cfg.s3_bucket ?? "";
      s3AccessKey.value = cfg.s3_access_key ?? "";
      s3SecretKey.value = cfg.s3_secret_key ?? "";
      s3Secure.value = isTrue(cfg.s3_secure);
      s3UsePathStyle.value = isTrue(cfg.s3_use_path_style);
      s3MaxUploadMB.value = toNumber(cfg.s3_max_upload_mb, 20);

      ragEnabled.value = isTrue(cfg.rag_enabled);
      ragBaseUrl.value = cfg.rag_embedding_base_url ?? "";
      ragModel.value = cfg.rag_embedding_model ?? "";
      ragApiKey.value = cfg.rag_embedding_api_key ?? "";
      ragChunkSize.value = toNumber(cfg.rag_chunk_size, 500);
      ragChunkOverlap.value = toNumber(cfg.rag_chunk_overlap, 80);
      ragTopK.value = toNumber(cfg.rag_top_k, 5);
    }

    // MQTT 连接配置 (设备管理, 存 wol_mqtt_* 键, 走 wol 专用接口)
    const { data: mqttCfg, error: mqttErr } = await getWolConfig();
    if (!mqttErr && mqttCfg) {
      mqttHost.value = mqttCfg.host;
      mqttPort.value = mqttCfg.port || 1883;
      mqttUsername.value = mqttCfg.username;
    }
  } finally {
    loading.value = false;
  }
}

/** 保存单个配置项; 失败时全局拦截器已 toast, 返回 false 供调用方决定是否提示成功/回滚 */
async function saveConfig(key: string, value: string, remark: string): Promise<boolean> {
  const { error } = await request({
    url: "/api/admin/system-config",
    method: "put",
    data: { key, value, remark },
  });
  return !error;
}

/** 依次保存多个配置项, 全部成功才返回 true (沿用逐条 PUT 的既有写法) */
async function saveConfigs(entries: Array<[key: string, value: string, remark: string]>): Promise<boolean> {
  for (const [key, value, remark] of entries) {
    if (!(await saveConfig(key, value, remark))) return false;
  }
  return true;
}

async function handleToggleRegister(val: boolean) {
  savingRegister.value = true;
  try {
    const ok = await saveConfig("register_enabled", val ? "true" : "false", $t("page.system.config.remarkRegister"));
    if (ok) {
      message.success(val ? $t("page.system.config.registerEnabledMsg") : $t("page.system.config.registerDisabledMsg"));
    } else {
      registerEnabled.value = !val;
    }
  } finally {
    savingRegister.value = false;
  }
}

async function handleToggleTelegram(val: boolean) {
  savingTelegram.value = true;
  try {
    const ok = await saveConfig("telegram_enabled", val ? "true" : "false", $t("page.system.config.remarkTelegramEnabled"));
    if (ok) {
      message.success(val ? $t("page.system.config.telegramEnabledMsg") : $t("page.system.config.telegramDisabledMsg"));
    } else {
      telegramEnabled.value = !val;
    }
  } finally {
    savingTelegram.value = false;
  }
}

async function handleSaveTelegram() {
  savingTelegram.value = true;
  try {
    const ok = await saveConfigs([
      ["telegram_bot_token", telegramBotToken.value, "Telegram Bot Token"],
      ["telegram_webhook_url", telegramWebhookUrl.value, $t("page.system.config.remarkTelegramWebhook")],
    ]);
    if (ok) message.success($t("page.system.config.telegramSaved"));
  } finally {
    savingTelegram.value = false;
  }
}

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

async function handleToggleSmtp(val: boolean) {
  savingSmtp.value = true;
  try {
    const ok = await saveConfig("smtp_enabled", val ? "true" : "false", $t("page.system.config.remarkSmtpEnabled"));
    if (ok) {
      message.success(val ? $t("page.system.config.smtpEnabledMsg") : $t("page.system.config.smtpDisabledMsg"));
    } else {
      smtpEnabled.value = !val;
    }
  } finally {
    savingSmtp.value = false;
  }
}

async function handleSaveSmtp() {
  savingSmtp.value = true;
  try {
    const ok = await saveConfigs([
      ["smtp_host", smtpHost.value, $t("page.system.config.remarkSmtpHost")],
      ["smtp_port", String(smtpPort.value), $t("page.system.config.remarkSmtpPort")],
      ["smtp_encryption", smtpEncryption.value, $t("page.system.config.remarkSmtpEncryption")],
      ["smtp_user", smtpUser.value, $t("page.system.config.remarkSmtpUser")],
      ["smtp_password", smtpPassword.value, $t("page.system.config.remarkSmtpPassword")],
      ["smtp_from", smtpFrom.value, $t("page.system.config.remarkSmtpFrom")],
      ["smtp_from_name", smtpFromName.value, $t("page.system.config.remarkSmtpFromName")],
    ]);
    if (ok) message.success($t("page.system.config.smtpSaved"));
  } finally {
    savingSmtp.value = false;
  }
}

async function handleToggleS3(val: boolean) {
  savingS3.value = true;
  try {
    const ok = await saveConfig("s3_enabled", val ? "true" : "false", $t("page.system.config.remarkS3Enabled"));
    if (ok) {
      message.success(val ? $t("page.system.config.s3EnabledMsg") : $t("page.system.config.s3DisabledMsg"));
    } else {
      s3Enabled.value = !val;
    }
  } finally {
    savingS3.value = false;
  }
}

async function handleSaveS3() {
  savingS3.value = true;
  try {
    const ok = await saveConfigs([
      ["s3_enabled", s3Enabled.value ? "true" : "false", $t("page.system.config.remarkS3Enabled")],
      ["s3_endpoint", s3Endpoint.value, $t("page.system.config.remarkS3Endpoint")],
      ["s3_region", s3Region.value, $t("page.system.config.remarkS3Region")],
      ["s3_bucket", s3Bucket.value, "S3 Bucket"],
      ["s3_access_key", s3AccessKey.value, "S3 Access Key"],
      ["s3_secret_key", s3SecretKey.value, "S3 Secret Key"],
      ["s3_secure", s3Secure.value ? "true" : "false", $t("page.system.config.remarkS3Secure")],
      ["s3_use_path_style", s3UsePathStyle.value ? "true" : "false", $t("page.system.config.remarkS3PathStyle")],
      ["s3_max_upload_mb", String(s3MaxUploadMB.value), $t("page.system.config.remarkS3MaxUpload")],
    ]);
    if (ok) message.success($t("page.system.config.s3Saved"));
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
  } finally {
    testingS3.value = false;
  }
}

async function handleToggleRag(val: boolean) {
  savingRag.value = true;
  try {
    const ok = await saveConfig("rag_enabled", val ? "true" : "false", $t("page.system.config.remarkRagEnabled"));
    if (ok) {
      message.success(val ? $t("page.system.config.ragEnabledMsg") : $t("page.system.config.ragDisabledMsg"));
    } else {
      ragEnabled.value = !val;
    }
  } finally {
    savingRag.value = false;
  }
}

async function handleSaveRag() {
  savingRag.value = true;
  try {
    const ok = await saveConfigs([
      ["rag_enabled", ragEnabled.value ? "true" : "false", $t("page.system.config.remarkRagEnabled")],
      ["rag_embedding_base_url", ragBaseUrl.value, $t("page.system.config.remarkRagBaseUrl")],
      ["rag_embedding_model", ragModel.value, $t("page.system.config.remarkRagModel")],
      ["rag_embedding_api_key", ragApiKey.value, $t("page.system.config.remarkRagApiKey")],
      ["rag_chunk_size", String(ragChunkSize.value), $t("page.system.config.remarkRagChunkSize")],
      ["rag_chunk_overlap", String(ragChunkOverlap.value), $t("page.system.config.remarkRagChunkOverlap")],
      ["rag_top_k", String(ragTopK.value), $t("page.system.config.remarkRagTopK")],
    ]);
    if (ok) message.success($t("page.system.config.ragSaved"));
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
  } finally {
    testingEmbedding.value = false;
  }
}

async function handleSaveMqtt() {
  if (!mqttHost.value.trim()) {
    message.warning($t("page.system.config.mqttHostRequired"));
    return;
  }
  savingMqtt.value = true;
  try {
    const { error } = await updateWolConfig({
      host: mqttHost.value.trim(),
      port: mqttPort.value,
      username: mqttUsername.value.trim(),
      password: mqttPassword.value
    });
    if (error) {
      message.error($t("page.system.config.saveFailed", { error: error.message || $t("page.system.config.unknownError") }));
      return;
    }
    mqttPassword.value = "";
    message.success($t("page.system.config.mqttSaved"));
  } finally {
    savingMqtt.value = false;
  }
}

async function handleToggleMemory(val: boolean) {
  savingMemory.value = true;
  try {
    const ok = await saveConfig(
      "memory_extraction_enabled",
      val ? "true" : "false",
      $t("page.system.config.remarkMemoryEnabled")
    );
    if (ok) {
      message.success(val ? $t("page.system.config.memoryEnabledMsg") : $t("page.system.config.memoryDisabledMsg"));
    } else {
      memoryEnabled.value = !val;
    }
  } finally {
    savingMemory.value = false;
  }
}

async function handleSaveMemory() {
  savingMemory.value = true;
  try {
    const ok = await saveConfigs([
      ["memory_extraction_model", memoryExtractionModel.value, $t("page.system.config.remarkMemoryModel")],
      ["memory_session_idle_minutes", String(memoryIdleMinutes.value), $t("page.system.config.remarkMemoryIdle")],
      ["memory_min_min_user_messages", String(memoryMinUserMessages.value), $t("page.system.config.remarkMemoryMinMessages")],
    ]);
    if (ok) message.success($t("page.system.config.memorySaved"));
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
    const { error } = await request({
      url: "/api/admin/system-config/test-email",
      method: "post",
      data: {
        email: testEmailForm.value.email,
        subject: testEmailForm.value.subject,
        content: testEmailForm.value.content,
      },
    });
    if (error) {
      message.error($t("page.system.config.sendFailed", { error: error.message || $t("page.system.config.unknownError") }));
      return;
    }
    message.success($t("page.system.config.testEmailSent"));
    showTestEmailModal.value = false;
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
                    :placeholder="$t('page.system.config.s3AccessKeyPlaceholder')"
                    :disabled="!s3Enabled"
                  />
                </NFormItemGi>
                <NFormItemGi label="Secret Key" path="s3SecretKey">
                  <NInput
                    v-model:value="s3SecretKey"
                    :type="showS3SecretKey ? 'text' : 'password'"
                    :placeholder="$t('page.system.config.s3SecretKeyPlaceholder')"
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

        <!-- MQTT 服务器 (设备管理) -->
        <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700">
          <div class="mb-4">
            <div class="font-bold text-gray-800 dark:text-gray-200">{{ $t("page.system.config.mqtt") }}</div>
            <div class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ $t("page.system.config.mqttDesc") }}</div>
          </div>
          <NForm label-placement="left" label-width="120">
            <NGrid :cols="2" :x-gap="12" :y-gap="8">
              <NFormItemGi :label="$t('page.system.config.mqttHost')" path="mqttHost">
                <NInput
                  v-model:value="mqttHost"
                  :placeholder="$t('page.system.config.mqttHostPlaceholder')"
                  clearable
                />
              </NFormItemGi>
              <NFormItemGi :label="$t('page.system.config.mqttPort')" path="mqttPort">
                <NInputNumber v-model:value="mqttPort" :min="1" :max="65535" class="w-full" size="small" />
              </NFormItemGi>
              <NFormItemGi :label="$t('page.system.config.mqttUsername')" path="mqttUsername">
                <NInput
                  v-model:value="mqttUsername"
                  :placeholder="$t('page.system.config.mqttUsernamePlaceholder')"
                  clearable
                />
              </NFormItemGi>
              <NFormItemGi :label="$t('page.system.config.mqttPassword')" path="mqttPassword">
                <NInput
                  v-model:value="mqttPassword"
                  :type="showMqttPassword ? 'text' : 'password'"
                  :placeholder="$t('page.system.config.mqttPasswordPlaceholder')"
                >
                  <template #suffix>
                    <div
                      class="cursor-pointer text-gray-400 hover:text-gray-600"
                      :class="showMqttPassword ? 'i-mdi:eye-off' : 'i-mdi:eye'"
                      @click="showMqttPassword = !showMqttPassword"
                    />
                  </template>
                </NInput>
              </NFormItemGi>
            </NGrid>
            <NFormItem class="mt-4">
              <NButton type="primary" :loading="savingMqtt" @click="handleSaveMqtt">
                {{ $t("page.system.config.saveMqtt") }}
              </NButton>
            </NFormItem>
          </NForm>
        </div>
      </NSpin>
    </NCard>

    <!-- 发送测试邮件弹窗 -->
    <NModal
      v-model:show="showTestEmailModal"
      preset="card"
      :title="$t('page.system.config.sendTestEmailTitle')"
      style="width: 500px"
      class="max-w-[96vw]"
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
