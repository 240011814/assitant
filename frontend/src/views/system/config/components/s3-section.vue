<script setup lang="ts">
import { ref, watch } from "vue";
import { useMessage } from "naive-ui";
import { request } from "@/service/request";
import { $t } from "@/locales";
import { isTrue, saveConfig, saveConfigs, toNumber } from "../shared";
import type { ConfigMap } from "../shared";

defineOptions({ name: "S3Section" });

const props = defineProps<{ config: ConfigMap }>();

const message = useMessage();
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

watch(
  () => props.config,
  cfg => {
    s3Enabled.value = isTrue(cfg.s3_enabled);
    s3Endpoint.value = cfg.s3_endpoint ?? "";
    s3Region.value = cfg.s3_region ?? "";
    s3Bucket.value = cfg.s3_bucket ?? "";
    s3AccessKey.value = cfg.s3_access_key ?? "";
    s3SecretKey.value = cfg.s3_secret_key ?? "";
    s3Secure.value = isTrue(cfg.s3_secure);
    s3UsePathStyle.value = isTrue(cfg.s3_use_path_style);
    s3MaxUploadMB.value = toNumber(cfg.s3_max_upload_mb, 20);
  },
  { immediate: true }
);

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
</script>

<template>
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
</template>
