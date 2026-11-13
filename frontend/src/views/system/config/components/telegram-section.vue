<script setup lang="ts">
import { ref, watch } from "vue";
import { useMessage } from "naive-ui";
import { $t } from "@/locales";
import { notFalse, saveConfig, saveConfigs } from "../shared";
import type { ConfigMap } from "../shared";

defineOptions({ name: "TelegramSection" });

const props = defineProps<{ config: ConfigMap }>();

const message = useMessage();
const savingTelegram = ref(false);
const telegramEnabled = ref(true);
const telegramBotToken = ref("");
const showTelegramToken = ref(false);
const telegramWebhookUrl = ref("");

watch(
  () => props.config,
  cfg => {
    telegramEnabled.value = notFalse(cfg.telegram_enabled);
    telegramBotToken.value = cfg.telegram_bot_token ?? "";
    telegramWebhookUrl.value = cfg.telegram_webhook_url ?? "";
  },
  { immediate: true }
);

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
</script>

<template>
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
</template>
