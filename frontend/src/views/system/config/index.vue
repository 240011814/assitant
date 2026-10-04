<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useMessage } from "naive-ui";
import { request } from "@/service/request";

defineOptions({ name: "SystemConfig" });

const message = useMessage();
const loading = ref(false);
const savingRegister = ref(false);
const saving2fa = ref(false);
const savingTelegram = ref(false);
const savingTimeout = ref(false);
const savingSmtp = ref(false);
const sendingTestEmail = ref(false);
const showTestEmailModal = ref(false);
const testEmailForm = ref({ email: "", subject: "", content: "" });
const registerEnabled = ref(false);
const admin2faEnabled = ref(false);
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

      const admin2faConfig = data.find((c: any) => c.key === "admin_2fa_enabled");
      admin2faEnabled.value = admin2faConfig?.value === "true";

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
    message.error(`加载配置失败: ${err?.message || "未知错误"}`);
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
    await saveConfig("register_enabled", val ? "true" : "false", "注册功能开关");
    message.success(val ? "注册功能已开启" : "注册功能已关闭");
  } catch (err: any) {
    registerEnabled.value = !val;
    message.error(`保存失败: ${err?.message || "未知错误"}`);
  } finally {
    savingRegister.value = false;
  }
}

async function handleToggleAdmin2FA(val: boolean) {
  saving2fa.value = true;
  try {
    await saveConfig(
      "admin_2fa_enabled",
      val ? "true" : "false",
      "管理员二次验证(TOTP)开关"
    );
    message.success(val ? "管理员二次验证已开启" : "管理员二次验证已关闭");
  } catch (err: any) {
    admin2faEnabled.value = !val;
    message.error(`保存失败: ${err?.message || "未知错误"}`);
  } finally {
    saving2fa.value = false;
  }
}

async function handleToggleTelegram(val: boolean) {
  savingTelegram.value = true;
  try {
    await saveConfig("telegram_enabled", val ? "true" : "false", "Telegram Bot 开关");
    message.success(val ? "Telegram Bot 已启用" : "Telegram Bot 已禁用");
  } catch (err: any) {
    telegramEnabled.value = !val;
    message.error(`保存失败: ${err?.message || "未知错误"}`);
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
      "Telegram Webhook 回调地址 (留空使用 Long Polling 模式)"
    );
    message.success("Telegram 配置已保存，Bot 将自动重启");
  } catch (err: any) {
    message.error(`保存失败: ${err?.message || "未知错误"}`);
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
      "AI 请求超时时间（分钟）"
    );
    await saveConfig(
      "ai_tls_handshake_timeout",
      String(aiTlsHandshakeTimeout.value),
      "AI TLS 握手超时时间（秒）"
    );
    await saveConfig(
      "ai_response_header_timeout",
      String(aiResponseHeaderTimeout.value),
      "AI 响应头超时时间（秒）"
    );
    await saveConfig(
      "http_timeout_seconds",
      String(httpTimeoutSeconds.value),
      "HTTP 请求超时时间（秒）"
    );
    message.success("超时配置已保存");
  } catch (err: any) {
    message.error(`保存失败: ${err?.message || "未知错误"}`);
  } finally {
    savingTimeout.value = false;
  }
}

async function handleToggleSmtp(val: boolean) {
  savingSmtp.value = true;
  try {
    await saveConfig("smtp_enabled", val ? "true" : "false", "SMTP邮件服务开关");
    message.success(val ? "SMTP 邮件服务已启用" : "SMTP 邮件服务已禁用");
  } catch (err: any) {
    smtpEnabled.value = !val;
    message.error(`保存失败: ${err?.message || "未知错误"}`);
  } finally {
    savingSmtp.value = false;
  }
}

async function handleSaveSmtp() {
  savingSmtp.value = true;
  try {
    await saveConfig("smtp_host", smtpHost.value, "SMTP服务器地址");
    await saveConfig("smtp_port", String(smtpPort.value), "SMTP服务器端口");
    await saveConfig(
      "smtp_encryption",
      smtpEncryption.value,
      "SMTP加密方式(none/ssl/starttls)"
    );
    await saveConfig("smtp_user", smtpUser.value, "SMTP用户名");
    await saveConfig("smtp_password", smtpPassword.value, "SMTP密码");
    await saveConfig("smtp_from", smtpFrom.value, "发件人邮箱地址");
    await saveConfig("smtp_from_name", smtpFromName.value, "发件人显示名称");
    message.success("SMTP 配置已保存");
  } catch (err: any) {
    message.error(`保存失败: ${err?.message || "未知错误"}`);
  } finally {
    savingSmtp.value = false;
  }
}

async function handleToggleS3(val: boolean) {
  savingS3.value = true;
  try {
    await saveConfig("s3_enabled", val ? "true" : "false", "用户文档存储开关");
    message.success(val ? "用户文档存储已启用" : "用户文档存储已关闭");
  } catch (err: any) {
    s3Enabled.value = !val;
    message.error(`保存失败: ${err?.message || "未知错误"}`);
  } finally {
    savingS3.value = false;
  }
}

async function handleSaveS3() {
  savingS3.value = true;
  try {
    await saveConfig("s3_enabled", s3Enabled.value ? "true" : "false", "用户文档存储开关");
    await saveConfig("s3_endpoint", s3Endpoint.value, "S3 兼容存储 Endpoint");
    await saveConfig("s3_region", s3Region.value, "S3 Region (可选)");
    await saveConfig("s3_bucket", s3Bucket.value, "S3 Bucket");
    await saveConfig("s3_access_key", s3AccessKey.value, "S3 Access Key");
    await saveConfig("s3_secret_key", s3SecretKey.value, "S3 Secret Key");
    await saveConfig("s3_secure", s3Secure.value ? "true" : "false", "是否使用 HTTPS");
    await saveConfig(
      "s3_use_path_style",
      s3UsePathStyle.value ? "true" : "false",
      "Path-Style 寻址 (MinIO 等自建服务开启)"
    );
    await saveConfig("s3_max_upload_mb", String(s3MaxUploadMB.value), "单文件上传上限 MB");
    message.success("S3 存储配置已保存, 用户文档功能即时生效");
  } catch (err: any) {
    message.error(`保存失败: ${err?.message || "未知错误"}`);
  } finally {
    savingS3.value = false;
  }
}

async function handleToggleRag(val: boolean) {
  savingRag.value = true;
  try {
    await saveConfig("rag_enabled", val ? "true" : "false", "文档语义检索开关");
    message.success(val ? "文档语义检索已启用" : "文档语义检索已关闭");
  } catch (err: any) {
    ragEnabled.value = !val;
    message.error(`保存失败: ${err?.message || "未知错误"}`);
  } finally {
    savingRag.value = false;
  }
}

async function handleSaveRag() {
  savingRag.value = true;
  try {
    await saveConfig("rag_enabled", ragEnabled.value ? "true" : "false", "文档语义检索开关");
    await saveConfig(
      "rag_embedding_base_url",
      ragBaseUrl.value,
      "嵌入服务地址 (OpenAI 兼容, 含 /v1)"
    );
    await saveConfig("rag_embedding_model", ragModel.value, "嵌入模型名");
    await saveConfig("rag_embedding_api_key", ragApiKey.value, "嵌入服务 API Key (本地服务可留空)");
    await saveConfig("rag_chunk_size", String(ragChunkSize.value), "向量切块字符数");
    await saveConfig("rag_chunk_overlap", String(ragChunkOverlap.value), "相邻切块重叠字符数");
    await saveConfig("rag_top_k", String(ragTopK.value), "检索返回片段数");
    message.success("文档 RAG 配置已保存, 即时生效");
  } catch (err: any) {
    message.error(`保存失败: ${err?.message || "未知错误"}`);
  } finally {
    savingRag.value = false;
  }
}

async function handleTestEmbedding() {
  if (!ragBaseUrl.value || !ragModel.value) {
    message.warning("请先填写嵌入服务地址与模型名");
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
      message.error(error?.message || "嵌入测试失败");
      return;
    }
    message.success(data.message || "连接成功");
  } catch (err: any) {
    message.error(`嵌入测试失败: ${err?.message || "未知错误"}`);
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
      "用户画像/经历抽取开关"
    );
    message.success(val ? "画像/经历抽取已开启" : "画像/经历抽取已关闭");
  } catch (err: any) {
    memoryEnabled.value = !val;
    message.error(`保存失败: ${err?.message || "未知错误"}`);
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
      "画像/经历抽取使用的模型代码(空则用默认模型)"
    );
    await saveConfig(
      "memory_session_idle_minutes",
      String(memoryIdleMinutes.value),
      "会话静默多少分钟后触发抽取"
    );
    await saveConfig(
      "memory_min_min_user_messages",
      String(memoryMinUserMessages.value),
      "纳入抽取的最小用户消息数(大于该值)"
    );
    message.success("画像/经历抽取配置已保存");
  } catch (err: any) {
    message.error(`保存失败: ${err?.message || "未知错误"}`);
  } finally {
    savingMemory.value = false;
  }
}

function openTestEmailModal() {
  testEmailForm.value = {
    email: "",
    subject: "测试邮件",
    content:
      "这是一封 SMTP 邮件服务的测试邮件。\n\n如果您收到此邮件，说明 SMTP 配置正确。",
  };
  showTestEmailModal.value = true;
}

async function handleSendTestEmail() {
  if (!testEmailForm.value.email) {
    message.warning("请输入收件人邮箱");
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
    message.success("测试邮件已发送，请检查收件箱");
    showTestEmailModal.value = false;
  } catch (err: any) {
    message.error(`发送失败: ${err?.message || "未知错误"}`);
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
    <NCard :bordered="false" shadow="sm" title="系统配置">
      <NSpin :show="loading">
        <div class="space-y-6">
          <!-- 注册开关 -->
          <div
            class="flex items-center justify-between p-4 rounded-lg border border-gray-200 dark:border-gray-700"
          >
            <div>
              <div class="font-bold text-gray-800 dark:text-gray-200">注册功能</div>
              <div class="text-sm text-gray-500 dark:text-gray-400 mt-1">
                控制登录页面是否显示注册按钮。关闭后新用户无法自行注册。
              </div>
            </div>
            <NSwitch
              v-model:value="registerEnabled"
              :loading="savingRegister"
              @update:value="handleToggleRegister"
            >
              <template #checked>开启</template>
              <template #unchecked>关闭</template>
            </NSwitch>
          </div>

          <!-- Admin 2FA 开关 -->
          <div
            class="flex items-center justify-between p-4 rounded-lg border border-gray-200 dark:border-gray-700"
          >
            <div>
              <div class="font-bold text-gray-800 dark:text-gray-200">
                管理员二次验证 (2FA)
              </div>
              <div class="text-sm text-gray-500 dark:text-gray-400 mt-1">
                开启后，超级管理员登录时需要输入 TOTP
                动态验证码。首次开启时需扫描二维码绑定验证器。
              </div>
            </div>
            <NSwitch
              v-model:value="admin2faEnabled"
              :loading="saving2fa"
              @update:value="handleToggleAdmin2FA"
            >
              <template #checked>开启</template>
              <template #unchecked>关闭</template>
            </NSwitch>
          </div>

          <!-- Telegram Bot 配置 -->
          <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700">
            <div class="flex items-center justify-between mb-4">
              <div>
                <div class="font-bold text-gray-800 dark:text-gray-200">Telegram Bot</div>
                <div class="text-sm text-gray-500 dark:text-gray-400 mt-1">
                  启用后用户可绑定 Telegram 使用学习助手功能。配置 Webhook URL
                  后使用回调模式，留空使用 Long Polling 模式。
                </div>
              </div>
              <NSwitch
                v-model:value="telegramEnabled"
                :loading="savingTelegram"
                @update:value="handleToggleTelegram"
              >
                <template #checked>开启</template>
                <template #unchecked>关闭</template>
              </NSwitch>
            </div>
            <NForm label-placement="left" label-width="100">
              <NFormItem label="Bot Token">
                <NInput
                  v-model:value="telegramBotToken"
                  :type="showTelegramToken ? 'text' : 'password'"
                  placeholder="输入 Telegram Bot Token (从 @BotFather 获取)"
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
              <NFormItem label="Webhook URL">
                <NInput
                  v-model:value="telegramWebhookUrl"
                  placeholder="https://your-domain.com (留空使用 Long Polling)"
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
                  保存 Telegram 配置
                </NButton>
              </NFormItem>
            </NForm>
          </div>

          <!-- 超时配置 -->
          <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700">
            <div class="mb-4">
              <div class="font-bold text-gray-800 dark:text-gray-200">超时配置</div>
              <div class="text-sm text-gray-500 dark:text-gray-400 mt-1">
                配置各服务的超时时间，修改后立即生效。
              </div>
            </div>
            <NForm label-placement="left" label-width="100">
              <NGrid :cols="2" :x-gap="12" :y-gap="8">
                <NFormItemGi label="AI 请求超时">
                  <NInputNumber
                    v-model:value="aiTimeoutMinutes"
                    :min="1"
                    :max="60"
                    :disabled="savingTimeout"
                    size="small"
                  >
                    <template #suffix>分钟</template>
                  </NInputNumber>
                </NFormItemGi>
                <NFormItemGi label="AI TLS 握手">
                  <NInputNumber
                    v-model:value="aiTlsHandshakeTimeout"
                    :min="5"
                    :max="120"
                    :disabled="savingTimeout"
                    size="small"
                  >
                    <template #suffix>秒</template>
                  </NInputNumber>
                </NFormItemGi>
                <NFormItemGi label="AI 响应头超时">
                  <NInputNumber
                    v-model:value="aiResponseHeaderTimeout"
                    :min="5"
                    :max="120"
                    :disabled="savingTimeout"
                    size="small"
                  >
                    <template #suffix>秒</template>
                  </NInputNumber>
                </NFormItemGi>
                <NFormItemGi label="HTTP 请求超时">
                  <NInputNumber
                    v-model:value="httpTimeoutSeconds"
                    :min="5"
                    :max="300"
                    :disabled="savingTimeout"
                    size="small"
                  >
                    <template #suffix>秒</template>
                  </NInputNumber>
                </NFormItemGi>
              </NGrid>
              <NFormItem class="mt-4">
                <NButton
                  type="primary"
                  :loading="savingTimeout"
                  @click="handleSaveTimeout"
                >
                  保存超时配置
                </NButton>
              </NFormItem>
            </NForm>
          </div>

          <!-- 用户画像 / 经历抽取配置 -->
          <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700">
            <div class="flex items-center justify-between mb-4">
              <div>
                <div class="font-bold text-gray-800 dark:text-gray-200">
                  用户画像 / 经历抽取
                </div>
                <div class="text-sm text-gray-500 dark:text-gray-400 mt-1">
                  从聊天会话中抽取并总结用户画像与个人经历（与 Mem0 并存），修改后立即生效。
                </div>
              </div>
              <NSwitch
                v-model:value="memoryEnabled"
                :loading="savingMemory"
                @update:value="handleToggleMemory"
              >
                <template #checked>开启</template>
                <template #unchecked>关闭</template>
              </NSwitch>
            </div>
            <NForm label-placement="left" label-width="120">
              <NGrid :cols="2" :x-gap="12" :y-gap="8">
                <NFormItemGi label="抽取模型">
                  <NInput
                    v-model:value="memoryExtractionModel"
                    placeholder="留空使用默认模型"
                    :disabled="!memoryEnabled"
                  />
                </NFormItemGi>
                <NFormItemGi label="会话静默(分钟)">
                  <NInputNumber
                    v-model:value="memoryIdleMinutes"
                    :min="1"
                    :max="1440"
                    :disabled="!memoryEnabled"
                    size="small"
                  >
                    <template #suffix>分钟</template>
                  </NInputNumber>
                </NFormItemGi>
                <NFormItemGi label="最少用户消息数">
                  <NInputNumber
                    v-model:value="memoryMinUserMessages"
                    :min="1"
                    :max="100"
                    :disabled="!memoryEnabled"
                    size="small"
                  >
                    <template #suffix>条</template>
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
                  保存画像抽取配置
                </NButton>
              </NFormItem>
            </NForm>
          </div>

          <!-- SMTP 邮件配置 -->
          <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700">
            <div class="flex items-center justify-between mb-4">
              <div>
                <div class="font-bold text-gray-800 dark:text-gray-200">
                  SMTP 邮件服务
                </div>
                <div class="text-sm text-gray-500 dark:text-gray-400 mt-1">
                  配置 SMTP 邮件服务器，用于发送系统通知邮件。
                </div>
              </div>
              <NSwitch
                v-model:value="smtpEnabled"
                :loading="savingSmtp"
                @update:value="handleToggleSmtp"
              >
                <template #checked>开启</template>
                <template #unchecked>关闭</template>
              </NSwitch>
            </div>
            <NForm label-placement="left" label-width="120">
              <NGrid :cols="2" :x-gap="12" :y-gap="8">
                <NFormItemGi label="SMTP 主机" path="smtpHost">
                  <NInput
                    v-model:value="smtpHost"
                    placeholder="smtp.example.com"
                    :disabled="!smtpEnabled"
                  />
                </NFormItemGi>
                <NFormItemGi label="SMTP 端口" path="smtpPort">
                  <NInputNumber
                    v-model:value="smtpPort"
                    :min="1"
                    :max="65535"
                    :disabled="!smtpEnabled"
                    size="small"
                  />
                </NFormItemGi>
                <NFormItemGi label="加密方式" path="smtpEncryption">
                  <NSelect
                    v-model:value="smtpEncryption"
                    :options="[
                      { label: 'SSL/TLS', value: 'ssl' },
                      { label: 'STARTTLS', value: 'starttls' },
                      { label: '无加密', value: 'none' },
                    ]"
                    :disabled="!smtpEnabled"
                  />
                </NFormItemGi>
                <NFormItemGi label="用户名" path="smtpUser">
                  <NInput
                    v-model:value="smtpUser"
                    placeholder="SMTP 用户名"
                    :disabled="!smtpEnabled"
                  />
                </NFormItemGi>
                <NFormItemGi label="密码" path="smtpPassword">
                  <NInput
                    v-model:value="smtpPassword"
                    :type="showSmtpPassword ? 'text' : 'password'"
                    placeholder="SMTP 密码"
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
                <NFormItemGi label="发件人邮箱" path="smtpFrom">
                  <NInput
                    v-model:value="smtpFrom"
                    placeholder="noreply@example.com"
                    :disabled="!smtpEnabled"
                  />
                </NFormItemGi>
                <NFormItemGi label="发件人名称" path="smtpFromName">
                  <NInput
                    v-model:value="smtpFromName"
                    placeholder="系统通知"
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
                    保存 SMTP 配置
                  </NButton>
                  <NButton
                    type="info"
                    :disabled="!smtpEnabled"
                    @click="openTestEmailModal"
                  >
                    <template #icon>
                      <SvgIcon icon="mdi:email-fast-outline" />
                    </template>
                    发送测试邮件
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
                  用户文档存储 (S3 兼容)
                </div>
                <div class="text-sm text-gray-500 dark:text-gray-400 mt-1">
                  配置 S3 兼容对象存储 (MinIO / 阿里云 OSS / 腾讯 COS
                  等)，供用户上传文档供 AI 读取。保存后立即生效。
                </div>
              </div>
              <NSwitch
                v-model:value="s3Enabled"
                :loading="savingS3"
                @update:value="handleToggleS3"
              >
                <template #checked>开启</template>
                <template #unchecked>关闭</template>
              </NSwitch>
            </div>
            <NForm label-placement="left" label-width="120">
              <NGrid :cols="2" :x-gap="12" :y-gap="8">
                <NFormItemGi label="Endpoint" path="s3Endpoint">
                  <NInput
                    v-model:value="s3Endpoint"
                    placeholder="127.0.0.1:9000 或 oss-cn-hangzhou.aliyuncs.com"
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
                    placeholder="留空即可 (AWS 等需要时填写)"
                    :disabled="!s3Enabled"
                  />
                </NFormItemGi>
                <NFormItemGi label="单文件上限 (MB)" path="s3MaxUploadMB">
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
                    <template #unchecked>虚拟域名</template>
                  </NSwitch>
                </NFormItemGi>
              </NGrid>
              <NFormItem class="mt-4">
                <NButton
                  type="primary"
                  :loading="savingS3"
                  :disabled="!s3Enabled"
                  @click="handleSaveS3"
                >
                  保存 S3 存储配置
                </NButton>
              </NFormItem>
            </NForm>
          </div>

          <!-- 文档 RAG (语义检索) -->
          <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700">
            <div class="flex items-center justify-between mb-4">
              <div>
                <div class="font-bold text-gray-800 dark:text-gray-200">
                  文档 RAG (语义检索)
                </div>
                <div class="text-sm text-gray-500 dark:text-gray-400 mt-1">
                  解析后的文档切块并向量化存入 ClickHouse, AI
                  可用文档语义检索工具按相关度召回片段。嵌入走 OpenAI 兼容接口，本地
                  Ollama 填 http://127.0.0.1:11434/v1 + 模型
                  bge-m3。保存后立即生效；更换嵌入模型后需在文档管理页重建索引。
                </div>
              </div>
              <NSwitch
                v-model:value="ragEnabled"
                :loading="savingRag"
                @update:value="handleToggleRag"
              >
                <template #checked>开启</template>
                <template #unchecked>关闭</template>
              </NSwitch>
            </div>
            <NForm label-placement="left" label-width="120">
              <NGrid :cols="2" :x-gap="12" :y-gap="8">
                <NFormItemGi label="服务地址" path="ragBaseUrl">
                  <NInput
                    v-model:value="ragBaseUrl"
                    placeholder="http://127.0.0.1:11434/v1"
                    :disabled="!ragEnabled"
                  />
                </NFormItemGi>
                <NFormItemGi label="嵌入模型" path="ragModel">
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
                    placeholder="本地服务可留空"
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
                <NFormItemGi label="切块字符数" path="ragChunkSize">
                  <NInputNumber
                    v-model:value="ragChunkSize"
                    :min="100"
                    :max="2000"
                    :disabled="!ragEnabled"
                    size="small"
                  />
                </NFormItemGi>
                <NFormItemGi label="切块重叠" path="ragChunkOverlap">
                  <NInputNumber
                    v-model:value="ragChunkOverlap"
                    :min="0"
                    :max="500"
                    :disabled="!ragEnabled"
                    size="small"
                  />
                </NFormItemGi>
                <NFormItemGi label="返回片段数" path="ragTopK">
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
                    保存文档 RAG 配置
                  </NButton>
                  <NButton
                    secondary
                    :loading="testingEmbedding"
                    :disabled="!ragEnabled"
                    @click="handleTestEmbedding"
                  >
                    测试连接
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
      title="发送测试邮件"
      style="width: 500px"
    >
      <NForm label-placement="left" label-width="80">
        <NFormItem label="收件邮箱" required>
          <NInput v-model:value="testEmailForm.email" placeholder="请输入收件人邮箱" />
        </NFormItem>
        <NFormItem label="邮件主题">
          <NInput v-model:value="testEmailForm.subject" placeholder="测试邮件" />
        </NFormItem>
        <NFormItem label="邮件内容">
          <NInput
            v-model:value="testEmailForm.content"
            type="textarea"
            placeholder="请输入邮件内容"
            :rows="6"
          />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showTestEmailModal = false">取消</NButton>
          <NButton type="primary" :loading="sendingTestEmail" @click="handleSendTestEmail">
            发送
          </NButton>
        </NSpace>
      </template>
    </NModal>
  </div>
</template>
