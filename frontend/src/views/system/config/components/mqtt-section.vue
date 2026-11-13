<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useMessage } from "naive-ui";
import { getWolConfig, updateWolConfig } from "@/service/api";
import { $t } from "@/locales";

defineOptions({ name: "MqttSection" });

const message = useMessage();
const savingMqtt = ref(false);
const showMqttPassword = ref(false);
const mqttHost = ref("");
const mqttPort = ref(1883);
const mqttUsername = ref("");
const mqttPassword = ref("");

async function loadMqttConfig() {
  // MQTT 连接配置 (设备管理, 存 wol_mqtt_* 键, 走 wol 专用接口)
  const { data, error } = await getWolConfig();
  if (!error && data) {
    mqttHost.value = data.host;
    mqttPort.value = data.port || 1883;
    mqttUsername.value = data.username;
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

onMounted(() => {
  void loadMqttConfig();
});
</script>

<template>
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
</template>
