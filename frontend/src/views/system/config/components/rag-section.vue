<script setup lang="ts">
import { ref, watch } from "vue";
import { useMessage } from "naive-ui";
import { request } from "@/service/request";
import { $t } from "@/locales";
import { isTrue, saveConfig, saveConfigs, toNumber } from "../shared";
import type { ConfigMap } from "../shared";

defineOptions({ name: "RagSection" });

const props = defineProps<{ config: ConfigMap }>();

const message = useMessage();
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

watch(
  () => props.config,
  cfg => {
    ragEnabled.value = isTrue(cfg.rag_enabled);
    ragBaseUrl.value = cfg.rag_embedding_base_url ?? "";
    ragModel.value = cfg.rag_embedding_model ?? "";
    ragApiKey.value = cfg.rag_embedding_api_key ?? "";
    ragChunkSize.value = toNumber(cfg.rag_chunk_size, 500);
    ragChunkOverlap.value = toNumber(cfg.rag_chunk_overlap, 80);
    ragTopK.value = toNumber(cfg.rag_top_k, 5);
  },
  { immediate: true }
);

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
</script>

<template>
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
</template>
