<script setup lang="ts">
import { ref, watch } from "vue";
import { useMessage } from "naive-ui";
import { $t } from "@/locales";
import { isTrue, saveConfig } from "../shared";
import type { ConfigMap } from "../shared";

defineOptions({ name: "RegisterSection" });

const props = defineProps<{ config: ConfigMap }>();

const message = useMessage();
const savingRegister = ref(false);
const registerEnabled = ref(false);

watch(
  () => props.config,
  cfg => {
    registerEnabled.value = isTrue(cfg.register_enabled);
  },
  { immediate: true }
);

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
</script>

<template>
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
</template>
