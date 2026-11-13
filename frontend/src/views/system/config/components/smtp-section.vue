<script setup lang="ts">
import { reactive, ref, watch } from "vue";
import { useMessage } from "naive-ui";
import { request } from "@/service/request";
import { $t } from "@/locales";
import { isTrue, saveConfig, saveConfigs, toNumber } from "../shared";
import type { ConfigMap } from "../shared";

defineOptions({ name: "SmtpSection" });

const props = defineProps<{ config: ConfigMap }>();

const message = useMessage();
const savingSmtp = ref(false);
const smtpEnabled = ref(false);
const smtpHost = ref("");
const smtpPort = ref(587);
const smtpEncryption = ref("ssl");
const smtpUser = ref("");
const smtpPassword = ref("");
const showSmtpPassword = ref(false);
const smtpFrom = ref("");
const smtpFromName = ref("");
const sendingTestEmail = ref(false);
const showTestEmailModal = ref(false);
const testEmailForm = reactive({ email: "", subject: "", content: "" });

watch(
  () => props.config,
  cfg => {
    smtpEnabled.value = isTrue(cfg.smtp_enabled);
    smtpHost.value = cfg.smtp_host ?? "";
    smtpPort.value = toNumber(cfg.smtp_port, 587);
    smtpEncryption.value = cfg.smtp_encryption ?? "ssl";
    smtpUser.value = cfg.smtp_user ?? "";
    smtpPassword.value = cfg.smtp_password ?? "";
    smtpFrom.value = cfg.smtp_from ?? "";
    smtpFromName.value = cfg.smtp_from_name ?? "";
  },
  { immediate: true }
);

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

function openTestEmailModal() {
  testEmailForm.email = "";
  testEmailForm.subject = $t("page.system.config.testEmailSubject");
  testEmailForm.content = $t("page.system.config.testEmailContent");
  showTestEmailModal.value = true;
}

async function handleSendTestEmail() {
  if (!testEmailForm.email) {
    message.warning($t("page.system.config.recipientRequired"));
    return;
  }
  sendingTestEmail.value = true;
  try {
    const { error } = await request({
      url: "/api/admin/system-config/test-email",
      method: "post",
      data: {
        email: testEmailForm.email,
        subject: testEmailForm.subject,
        content: testEmailForm.content,
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
</script>

<template>
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
</template>
