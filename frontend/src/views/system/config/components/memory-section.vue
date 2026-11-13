<script setup lang="ts">
import { ref, watch } from "vue";
import { useMessage } from "naive-ui";
import { $t } from "@/locales";
import { notFalse, saveConfig, saveConfigs, toNumber } from "../shared";
import type { ConfigMap } from "../shared";

defineOptions({ name: "MemorySection" });

const props = defineProps<{ config: ConfigMap }>();

const message = useMessage();
const savingMemory = ref(false);
const memoryEnabled = ref(true);
const memoryExtractionModel = ref("");
const memoryIdleMinutes = ref(15);
const memoryMinUserMessages = ref(6);

watch(
  () => props.config,
  cfg => {
    memoryEnabled.value = notFalse(cfg.memory_extraction_enabled);
    memoryExtractionModel.value = cfg.memory_extraction_model ?? "";
    memoryIdleMinutes.value = toNumber(cfg.memory_session_idle_minutes, 15);
    memoryMinUserMessages.value = toNumber(cfg.memory_min_min_user_messages, 6);
  },
  { immediate: true }
);

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
</script>

<template>
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
</template>
