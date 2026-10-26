<script setup lang="ts">
import { computed, h, onMounted, ref } from "vue";
import { useAppStore } from "@/store/modules/app";
import {
  NAlert,
  NButton,
  NCard,
  NDataTable,
  NForm,
  NFormItem,
  NInput,
  NModal,
  NPopconfirm,
  NSelect,
  NSpace,
  NSwitch,
  NTabPane,
  NTabs,
  NTag,
  useMessage,
} from "naive-ui";
import type { DataTableColumns } from "naive-ui";
import {
  fetchCreateAIModel,
  fetchCreateAIProvider,
  fetchDeleteAIModel,
  fetchDeleteAIProvider,
  fetchGetAIProviders,
  fetchUpdateAIModel,
  fetchUpdateAIProvider,
  fetchTestAIConnection,
  fetchGetAITools,
  fetchCreateAITool,
  fetchUpdateAITool,
  fetchDeleteAITool,
  fetchGetAIToolMetas,
} from "@/service/api/admin";
import { $t } from "@/locales";

const message = useMessage();
const loading = ref(false);
const activeTab = ref("providers");
const appStore = useAppStore();
const isMobile = computed(() => appStore.isMobile);
const providers = ref<Api.Admin.AIProvider[]>([]);
const tools = ref<Api.Admin.AITool[]>([]);
const toolMetas = ref<Api.Admin.AIToolMeta[]>([]);

// Tool Modal
const showToolModal = ref(false);
const toolModalTitle = ref("");
const toolModalMode = ref<"select" | "config">("select");
const selectedToolMeta = ref<Api.Admin.AIToolMeta | null>(null);
const toolConfigValues = ref<Record<string, string>>({});
const toolForm = ref<Partial<Api.Admin.AITool>>({
  name: "",
  display_name: "",
  description: "",
  enabled: false,
  confirm_required: false,
  config_json: "{}",
});

// Provider Modal
const showProviderModal = ref(false);
const providerModalTitle = ref("");
const providerForm = ref<Partial<Api.Admin.AIProvider>>({
  name: "",
  api_key: "",
  base_url: "",
  is_active: false,
});

// Model Modal
const showModelModal = ref(false);
const modelModalTitle = ref("");
const modelModalMode = ref<"list" | "form">("list");
const modelDialogTitle = computed(() =>
  modelModalMode.value === "list"
    ? $t("page.system.aiConfig.manageModel")
    : modelModalTitle.value
);
const currentProviderId = ref<number | null>(null);
const modelForm = ref<Partial<Api.Admin.AIModel>>({
  model_code: "",
  display_name: "",
  model_type: "chat",
  is_default: false,
  config_json: "{}",
});

const modelTypeOptions = computed(() => [
  { label: $t("page.system.aiConfig.modelTypeChat"), value: "chat" },
  { label: $t("page.system.aiConfig.modelTypeTts"), value: "tts" },
  { label: $t("page.system.aiConfig.modelTypeAsr"), value: "asr" },
  { label: $t("page.system.aiConfig.modelTypeEmbedding"), value: "embedding" }
]);

const modelTypeLabel = (type?: string) => {
  switch (type) {
    case "tts":
      return $t("page.system.aiConfig.modelTypeTts");
    case "asr":
      return $t("page.system.aiConfig.modelTypeAsr");
    case "embedding":
      return $t("page.system.aiConfig.modelTypeEmbedding");
    default:
      return $t("page.system.aiConfig.modelTypeChat");
  }
};

const columns = computed<DataTableColumns<Api.Admin.AIProvider>>(() => [
  { title: $t("page.system.aiConfig.providerName"), key: "name", width: 120 },
  {
    title: "API Key",
    key: "api_key",
    render(row) {
      // 后端只返回掩码形式的 masked_api_key, 不再返回明文
      if (!row.masked_api_key) return "-";
      return row.masked_api_key;
    },
  },
  { title: "Base URL", key: "base_url" },
  {
    title: $t("page.system.aiConfig.status"),
    key: "is_active",
    render(row) {
      return h(NSwitch, {
        value: row.is_active,
        onUpdateValue: (val: boolean) => handleToggleProviderStatus(row, val),
      });
    },
  },
  {
    title: $t("page.system.aiConfig.actions"),
    key: "actions",
    render(row) {
      return h(
        NSpace,
        {},
        {
          default: () => [
            h(
              NButton,
              { size: "small", onClick: () => handleEditProvider(row) },
              { default: () => $t("common.edit") }
            ),
            h(
              NButton,
              {
                size: "small",
                type: "primary",
                ghost: true,
                onClick: () => handleManageModels(row),
              },
              { default: () => $t("page.system.aiConfig.manageModel") }
            ),
            h(
              NPopconfirm,
              { onPositiveClick: () => handleDeleteProvider(row.id) },
              {
                default: () => $t("page.system.aiConfig.deleteProviderConfirm"),
                trigger: () =>
                  h(
                    NButton,
                    { size: "small", type: "error", ghost: true },
                    { default: () => $t("common.delete") }
                  ),
              }
            ),
          ],
        }
      );
    },
  },
]);

async function getProviders() {
  loading.value = true;
  const { data } = await fetchGetAIProviders();
  if (data) {
    providers.value = data;
  }
  loading.value = false;
}

async function getTools() {
  const { data } = await fetchGetAITools();
  if (data) {
    tools.value = data;
  }
}

async function getToolMetas() {
  const { data } = await fetchGetAIToolMetas();
  if (data) {
    toolMetas.value = data;
  }
}

function handleAddTool() {
  toolModalTitle.value = $t("page.system.aiConfig.addTool");
  toolModalMode.value = "select";
  selectedToolMeta.value = null;
  toolForm.value = {
    name: "",
    display_name: "",
    description: "",
    enabled: true,
    config_json: "{}",
  };
  showToolModal.value = true;
}

function handleSelectTool(meta: Api.Admin.AIToolMeta) {
  selectedToolMeta.value = meta;
  const values: Record<string, string> = {};
  meta.params?.forEach((p) => {
    values[p.name] = p.default || "";
  });
  toolConfigValues.value = values;
  toolForm.value = {
    name: meta.name,
    display_name: meta.display_name,
    description: meta.description,
    enabled: true,
    config_json: JSON.stringify(values, null, 2),
  };
  toolModalMode.value = "config";
}

function handleBackToSelect() {
  toolModalMode.value = "select";
  selectedToolMeta.value = null;
}

function handleEditTool(row: Api.Admin.AITool) {
  toolModalTitle.value = $t("page.system.aiConfig.editTool");
  toolModalMode.value = "config";
  const meta = toolMetas.value.find((m) => m.name === row.name);
  selectedToolMeta.value = meta || null;
  const values: Record<string, string> = {};
  try {
    const parsed = JSON.parse(row.config_json || "{}");
    Object.keys(parsed).forEach((k) => {
      values[k] = String(parsed[k]);
    });
  } catch (err) {
    // 配置 JSON 损坏时降级为空表单, 但留痕便于排查脏数据
    console.warn("工具配置 JSON 解析失败:", row.name, err);
  }
  if (meta) {
    meta.params?.forEach((p) => {
      if (!(p.name in values)) values[p.name] = p.default || "";
    });
  }
  toolConfigValues.value = values;
  toolForm.value = { ...row };
  showToolModal.value = true;
}

async function handleSaveTool() {
  toolForm.value.config_json = JSON.stringify(toolConfigValues.value);
  if (toolForm.value.id) {
    const { error } = await fetchUpdateAITool(toolForm.value.id, toolForm.value);
    if (error) return;
    message.success($t("page.system.aiConfig.updateSuccess"));
  } else {
    const { error } = await fetchCreateAITool(toolForm.value);
    if (error) return;
    message.success($t("page.system.aiConfig.createSuccess"));
  }
  showToolModal.value = false;
  getTools();
}

async function handleDeleteTool(id: number) {
  const { error } = await fetchDeleteAITool(id);
  if (error) return;
  message.success($t("page.system.aiConfig.deleteSuccess"));
  getTools();
}

async function handleToggleToolStatus(row: Api.Admin.AITool, val: boolean) {
  const { error } = await fetchUpdateAITool(row.id, {
    name: row.name,
    display_name: row.display_name,
    description: row.description,
    config_json: row.config_json,
    enabled: val,
  });
  if (error) return;
  message.success(val ? $t("page.system.aiConfig.enabledStatus") : $t("page.system.aiConfig.disabledStatus"));
  getTools();
}


function _getToolConfigPlaceholder(meta: Api.Admin.AIToolMeta): string {
  if (!meta.params?.length) return "{}";
  const obj: Record<string, string> = {};
  meta.params.forEach((p) => {
    obj[p.name] = p.default || "";
  });
  return JSON.stringify(obj, null, 2);
}

const availableToolMetas = computed(() => {
  const savedNames = new Set(tools.value.map((t) => t.name));
  return toolMetas.value.filter((m) => !savedNames.has(m.name));
});

const toolColumns = computed<DataTableColumns<Api.Admin.AITool>>(() => [
  { title: $t("page.system.aiConfig.toolName"), key: "name", width: 120 },
  { title: $t("page.system.aiConfig.displayName"), key: "display_name", width: 120 },
  { title: $t("page.system.aiConfig.toolDescription"), key: "description", ellipsis: { tooltip: true } },
  {
    title: $t("page.system.aiConfig.status"),
    key: "enabled",
    width: 80,
    render(row) {
      return h(NSwitch, {
        value: row.enabled,
        onUpdateValue: (val: boolean) => handleToggleToolStatus(row, val),
      });
    },
  },
  {
    title: $t("page.system.aiConfig.toolConfirmRequired"),
    key: "confirm_required",
    width: 80,
    render(row) {
      return row.confirm_required ? $t("common.yesOrNo.yes") : $t("common.yesOrNo.no");
    },
  },
  {
    title: $t("page.system.aiConfig.actions"),
    key: "actions",
    width: 150,
    render(row) {
      return h(
        NSpace,
        {},
        {
          default: () => [
            h(
              NButton,
              { size: "small", onClick: () => handleEditTool(row) },
              { default: () => $t("common.edit") }
            ),
            h(
              NPopconfirm,
              { onPositiveClick: () => handleDeleteTool(row.id) },
              {
                default: () => $t("page.system.aiConfig.deleteToolConfirm"),
                trigger: () =>
                  h(
                    NButton,
                    { size: "small", type: "error", ghost: true },
                    { default: () => $t("common.delete") }
                  ),
              }
            ),
          ],
        }
      );
    },
  },
]);

function handleAddProvider() {
  providerModalTitle.value = $t("page.system.aiConfig.addProvider");
  providerForm.value = { name: "", api_key: "", base_url: "", is_active: true };
  showProviderModal.value = true;
}

function handleEditProvider(row: Api.Admin.AIProvider) {
  providerModalTitle.value = $t("page.system.aiConfig.editProvider");
  // 后端不再返回明文 api_key, 编辑时留空表示不修改
  providerForm.value = { ...row, api_key: "" };
  showProviderModal.value = true;
}

async function handleSaveProvider() {
  if (providerForm.value.id) {
    const { error } = await fetchUpdateAIProvider(providerForm.value.id, providerForm.value);
    if (error) return;
    message.success($t("page.system.aiConfig.updateSuccess"));
  } else {
    const { error } = await fetchCreateAIProvider(providerForm.value);
    if (error) return;
    message.success($t("page.system.aiConfig.createSuccess"));
  }
  showProviderModal.value = false;
  getProviders();
}

async function handleToggleProviderStatus(row: Api.Admin.AIProvider, val: boolean) {
  const { error } = await fetchUpdateAIProvider(row.id, { ...row, is_active: val });
  if (error) return;
  message.success(
    val
      ? $t("page.system.aiConfig.enabledStatus")
      : $t("page.system.aiConfig.disabledStatus")
  );
  getProviders();
}

async function handleDeleteProvider(id: number) {
  const { error } = await fetchDeleteAIProvider(id);
  if (error) return;
  message.success($t("page.system.aiConfig.deleteSuccess"));
  getProviders();
}

// Model Management
const activeModels = ref<Api.Admin.AIModel[]>([]);
const testingModelId = ref<number | null>(null);
const modelColumns = computed<DataTableColumns<Api.Admin.AIModel>>(() => [
  { title: $t("page.system.aiConfig.modelCode"), key: "model_code" },
  { title: $t("page.system.aiConfig.displayName"), key: "display_name" },
  {
    title: $t("page.system.aiConfig.modelType"),
    key: "model_type",
    width: 130,
    render(row) {
      return h(
        NTag,
        { type: row.model_type === "chat" ? "default" : "info" },
        { default: () => modelTypeLabel(row.model_type) }
      );
    },
  },
  {
    title: $t("page.system.aiConfig.default"),
    key: "is_default",
    render(row) {
      return row.is_default
        ? h(
            NTag,
            { type: "success" },
            { default: () => $t("page.system.aiConfig.default") }
          )
        : null;
    },
  },
  {
    title: $t("page.system.aiConfig.defaultTts"),
    key: "is_default_tts",
    render(row) {
      return row.is_default_tts
        ? h(
            NTag,
            { type: "warning" },
            { default: () => $t("page.system.aiConfig.defaultTts") }
          )
        : null;
    },
  },
  {
    title: $t("page.system.aiConfig.actions"),
    key: "actions",
    width: 240,
    render(row) {
      return h(
        NSpace,
        { wrap: false },
        {
          default: () => [
            h(
              NButton,
              { size: "small", onClick: () => handleEditModel(row) },
              { default: () => $t("common.edit") }
            ),
            h(
              NButton,
              {
                size: "small",
                type: "info",
                loading: testingModelId.value === row.id,
                onClick: () => handleTestSingleModel(row),
              },
              { default: () => $t("page.system.aiConfig.testConnection") }
            ),
            h(
              NPopconfirm,
              { onPositiveClick: () => handleDeleteModel(row) },
              {
                default: () => $t("page.system.aiConfig.deleteModelConfirm"),
                trigger: () =>
                  h(
                    NButton,
                    { size: "small", type: "error", ghost: true },
                    { default: () => $t("common.delete") }
                  ),
              }
            ),
          ],
        }
      );
    },
  },
]);

function handleManageModels(row: Api.Admin.AIProvider) {
  currentProviderId.value = row.id;
  activeModels.value = row.models || [];
  modelModalMode.value = "list";
  showModelModal.value = true;
}

function handleAddModel() {
  modelModalTitle.value = $t("page.system.aiConfig.addModel");
  modelForm.value = {
    provider_id: currentProviderId.value!,
    model_code: "",
    display_name: "",
    model_type: "chat",
    is_default: activeModels.value.length === 0,
    is_default_tts: false,
    config_json: "{}",
  };
  modelModalMode.value = "form";
  showModelModal.value = true;
}

function handleEditModel(row: Api.Admin.AIModel) {
  modelModalTitle.value = $t("page.system.aiConfig.editModel");
  modelForm.value = { ...row };
  modelModalMode.value = "form";
  showModelModal.value = true;
}

function closeModelForm() {
  modelModalMode.value = "list";
}

async function handleSaveModel() {
  if (modelForm.value.id) {
    const { error } = await fetchUpdateAIModel(modelForm.value.id, modelForm.value);
    if (error) return;
    message.success($t("page.system.aiConfig.updateSuccess"));
  } else {
    const { error } = await fetchCreateAIModel(modelForm.value as Api.Admin.AIModel);
    if (error) return;
    message.success($t("page.system.aiConfig.createSuccess"));
  }
  // Refresh data
  const { data } = await fetchGetAIProviders();
  if (data) {
    providers.value = data;
    const current = data.find((p) => p.id === currentProviderId.value);
    if (current) {
      activeModels.value = current.models || [];
    }
  }
  modelModalMode.value = "list";
}

async function handleDeleteModel(row: Api.Admin.AIModel) {
  const { error } = await fetchDeleteAIModel(row.id);
  if (error) return;
  message.success($t("page.system.aiConfig.deleteSuccess"));
  // Refresh data
  const { data } = await fetchGetAIProviders();
  if (data) {
    providers.value = data;
    const current = data.find((p) => p.id === currentProviderId.value);
    if (current) {
      activeModels.value = current.models || [];
    }
  }
}

async function handleTestSingleModel(row: Api.Admin.AIModel) {
  testingModelId.value = row.id;
  try {
    // 列表接口不返回明文 API Key，后端按 model_id 读取已保存的 Provider 配置
    const { error } = await fetchTestAIConnection({ model_id: row.id });
    if (!error) {
      message.success($t("page.system.aiConfig.testSuccess"));
    } else {
      message.error($t("page.system.aiConfig.testFailed"));
    }
  } catch (e) {
    message.error($t("page.system.aiConfig.testFailed"));
    console.error(e);
  } finally {
    testingModelId.value = null;
  }
}

onMounted(() => {
  getProviders();
  getTools();
  getToolMetas();
});
</script>

<template>
  <div class="h-full" :class="isMobile ? 'p-2' : 'p-4'">
    <NCard
      :bordered="false"
      class="h-full rounded-16px shadow-sm"
    >
      <NTabs v-model:value="activeTab" type="line" animated>
        <template #suffix>
          <NButton v-if="activeTab === 'providers'" type="primary" size="small" @click="handleAddProvider">
            {{ $t("page.system.aiConfig.addProvider") }}
          </NButton>
          <NButton v-else type="primary" size="small" @click="handleAddTool">
            {{ $t("page.system.aiConfig.addTool") }}
          </NButton>
        </template>
        <NTabPane name="providers" :tab="$t('page.system.aiConfig.title')">
          <!-- PC: 表格 -->
          <NDataTable
            v-if="!isMobile"
            class="mt-4"
            :columns="columns"
            :data="providers"
            :loading="loading"
            :pagination="false"
          />
          <!-- Mobile: 卡片列表 -->
          <template v-else>
            <NSpin v-if="loading" class="mt-4 flex justify-center py-8" />
            <NEmpty v-else-if="!providers.length" class="mt-4 py-8" />
            <div v-else class="mt-4 flex flex-col gap-3">
              <NCard v-for="row in providers" :key="row.id" size="small" :bordered="true">
                <div class="flex flex-col gap-2">
                  <div class="flex items-center justify-between">
                    <span class="font-bold text-16px">{{ row.name }}</span>
                    <NSwitch
                      :value="row.is_active"
                      size="small"
                      @update:value="(val: boolean) => handleToggleProviderStatus(row, val)"
                    />
                  </div>
                  <div class="break-all text-xs text-gray-500 dark:text-gray-400">
                    Base URL: {{ row.base_url || "-" }}
                  </div>
                  <div class="text-xs text-gray-400">API Key: {{ row.masked_api_key || "-" }}</div>
                  <div class="flex flex-wrap justify-end gap-1 pt-1">
                    <NButton size="tiny" @click="handleEditProvider(row)">{{ $t("common.edit") }}</NButton>
                    <NButton size="tiny" type="primary" ghost @click="handleManageModels(row)">
                      {{ $t("page.system.aiConfig.manageModel") }}
                    </NButton>
                    <NPopconfirm @positive-click="handleDeleteProvider(row.id)">
                      <template #trigger>
                        <NButton size="tiny" type="error" ghost>{{ $t("common.delete") }}</NButton>
                      </template>
                      {{ $t("page.system.aiConfig.deleteProviderConfirm") }}
                    </NPopconfirm>
                  </div>
                </div>
              </NCard>
            </div>
          </template>
        </NTabPane>

        <NTabPane name="tools" :tab="$t('page.system.aiConfig.toolManagement')">
          <!-- PC: 表格 -->
          <NDataTable
            v-if="!isMobile"
            class="mt-4"
            :columns="toolColumns"
            :data="tools"
            :pagination="false"
            :scroll-x="560"
          />
          <!-- Mobile: 卡片列表 -->
          <template v-else>
            <NSpin v-if="loading" class="mt-4 flex justify-center py-8" />
            <NEmpty v-else-if="!tools.length" class="mt-4 py-8" />
            <div v-else class="mt-4 flex flex-col gap-3">
              <NCard v-for="row in tools" :key="row.id" size="small" :bordered="true">
                <div class="flex flex-col gap-2">
                  <div class="flex items-center justify-between">
                    <div class="flex items-center gap-2">
                      <span class="font-bold text-16px">{{ row.display_name || row.name }}</span>
                      <span class="text-xs text-gray-400">{{ row.name }}</span>
                    </div>
                    <NSwitch
                      :value="row.enabled"
                      size="small"
                      @update:value="(val: boolean) => handleToggleToolStatus(row, val)"
                    />
                  </div>
                  <div v-if="row.description" class="text-sm text-gray-600 dark:text-gray-400">{{ row.description }}</div>
                  <div class="flex items-center justify-between">
                    <NTag size="small" :bordered="false" :type="row.confirm_required ? 'warning' : 'default'">
                      {{ $t('page.system.aiConfig.toolConfirmRequired') }}: {{ row.confirm_required ? $t('common.yesOrNo.yes') : $t('common.yesOrNo.no') }}
                    </NTag>
                    <div class="flex gap-1">
                      <NButton size="tiny" @click="handleEditTool(row)">{{ $t("common.edit") }}</NButton>
                      <NPopconfirm @positive-click="handleDeleteTool(row.id)">
                        <template #trigger>
                          <NButton size="tiny" type="error" ghost>{{ $t("common.delete") }}</NButton>
                        </template>
                        {{ $t("page.system.aiConfig.deleteToolConfirm") }}
                      </NPopconfirm>
                    </div>
                  </div>
                </div>
              </NCard>
            </div>
          </template>
        </NTabPane>
      </NTabs>
    </NCard>

    <!-- Provider Modal -->
    <NModal
      v-model:show="showProviderModal"
      :title="providerModalTitle"
      preset="card"
      class="w-500px max-w-[96vw]"
    >
      <NForm :model="providerForm" label-placement="left" :label-width="100">
        <NFormItem :label="$t('page.system.aiConfig.providerName')" path="name">
          <NInput
            v-model:value="providerForm.name"
            :placeholder="$t('page.system.aiConfig.providerNamePlaceholder')"
          />
        </NFormItem>
        <NFormItem label="API Key" path="api_key">
          <NInput
            v-model:value="providerForm.api_key"
            type="password"
            show-password-on="click"
            :placeholder="$t('page.system.aiConfig.apiKeyEditPlaceholder')"
          />
        </NFormItem>
        <NFormItem label="Base URL" path="base_url">
          <NInput
            v-model:value="providerForm.base_url"
            :placeholder="$t('page.system.aiConfig.baseUrlPlaceholder')"
          />
        </NFormItem>
        <NFormItem :label="$t('page.system.aiConfig.enabled')" path="is_active">
          <NSwitch v-model:value="providerForm.is_active" />
        </NFormItem>
        <div class="flex justify-end gap-2">
          <NButton @click="showProviderModal = false">{{ $t("common.cancel") }}</NButton>
          <NButton type="primary" @click="handleSaveProvider">
            {{ $t("common.confirm") }}
          </NButton>
        </div>
      </NForm>
    </NModal>

    <!-- Models Modal -->
    <NModal
      v-model:show="showModelModal"
      :title="modelDialogTitle"
      preset="card"
      :class="modelModalMode === 'list' ? 'w-920px max-w-[96vw]' : 'w-500px max-w-[96vw]'"
    >
      <div v-if="modelModalMode === 'list'">
        <div class="mb-4 flex justify-end">
          <NButton type="primary" size="small" @click="handleAddModel">
            {{ $t("page.system.aiConfig.addModel") }}
          </NButton>
        </div>
        <NDataTable :columns="modelColumns" :data="activeModels" :scroll-x="860" />
      </div>

      <NForm v-else :model="modelForm" label-placement="left" :label-width="100">
        <NFormItem :label="$t('page.system.aiConfig.modelCode')" path="model_code">
          <NInput
            v-model:value="modelForm.model_code"
            :placeholder="$t('page.system.aiConfig.modelCodePlaceholder')"
          />
        </NFormItem>
        <NFormItem :label="$t('page.system.aiConfig.displayName')" path="display_name">
          <NInput
            v-model:value="modelForm.display_name"
            :placeholder="$t('page.system.aiConfig.displayNamePlaceholder')"
          />
        </NFormItem>
        <NFormItem :label="$t('page.system.aiConfig.modelType')" path="model_type">
          <NSelect v-model:value="modelForm.model_type" :options="modelTypeOptions" />
        </NFormItem>
        <NFormItem :label="$t('page.system.aiConfig.setDefault')" path="is_default">
          <NSwitch v-model:value="modelForm.is_default" />
        </NFormItem>
        <NFormItem :label="$t('page.system.aiConfig.setDefaultTts')" path="is_default_tts">
          <NSwitch v-model:value="modelForm.is_default_tts" />
        </NFormItem>
        <NAlert v-if="modelForm.model_type === 'tts'" type="info" :bordered="false" class="mb-3">
          {{ $t("page.system.aiConfig.modelTypeTtsHint") }}
        </NAlert>
        <NFormItem :label="$t('page.system.aiConfig.runParams')" path="config_json">
          <NInput
            v-model:value="modelForm.config_json"
            type="textarea"
            placeholder="{&quot;temperature&quot;: 0.7}"
          />
        </NFormItem>
        <div class="flex justify-end gap-2">
          <NButton @click="closeModelForm">{{ $t("common.cancel") }}</NButton>
          <NButton type="primary" @click="handleSaveModel">
            {{ $t("common.confirm") }}
          </NButton>
        </div>
      </NForm>
    </NModal>

    <!-- Tool Modal -->
    <NModal
      v-model:show="showToolModal"
      :title="toolModalTitle"
      preset="card"
      :class="toolModalMode === 'select' ? 'w-700px' : 'w-500px'"
    >
      <!-- Step 1: Select Tool -->
      <div v-if="toolModalMode === 'select'">
        <div class="mb-4 text-gray-500 text-sm">{{ $t("page.system.aiConfig.selectToolType") }}</div>
        <div class="grid grid-cols-2 gap-3">
          <div
            v-for="meta in availableToolMetas"
            :key="meta.name"
            class="p-3 border border-gray-200 dark:border-gray-700 rounded-lg cursor-pointer hover:border-primary hover:shadow-sm transition-all"
            @click="handleSelectTool(meta)"
          >
            <div class="font-bold text-gray-800 dark:text-gray-200">{{ meta.display_name }}</div>
            <div class="text-xs text-gray-500 mt-1">{{ meta.description }}</div>
            <div class="flex flex-wrap gap-1 mt-2">
              <NTag v-for="param in meta.params" :key="param.name" size="tiny" type="info">
                {{ param.name }}
              </NTag>
            </div>
          </div>
        </div>
        <div v-if="availableToolMetas.length === 0" class="text-center text-gray-400 py-8">
          {{ toolMetas.length === 0 ? $t("page.system.aiConfig.noTools") : $t("page.system.aiConfig.allToolsAdded") }}
        </div>
      </div>

      <!-- Step 2: Configure Tool -->
      <div v-else>
        <div class="mb-4 flex items-center gap-2">
          <NButton text @click="handleBackToSelect">
            {{ $t("page.system.aiConfig.backToSelect") }}
          </NButton>
          <span class="text-gray-500 text-sm">{{ $t("page.system.aiConfig.configuring", { name: selectedToolMeta?.display_name }) }}</span>
        </div>
        <NForm :model="toolForm" label-placement="left" :label-width="100">
          <NFormItem :label="$t('page.system.aiConfig.toolName')" path="name">
            <NInput v-model:value="toolForm.name" disabled />
          </NFormItem>
          <NFormItem :label="$t('page.system.aiConfig.displayName')" path="display_name">
            <NInput v-model:value="toolForm.display_name" />
          </NFormItem>
          <NFormItem :label="$t('page.system.aiConfig.toolDescription')" path="description">
            <NInput v-model:value="toolForm.description" type="textarea" />
          </NFormItem>

          <div v-if="selectedToolMeta?.params?.length" class="mb-4">
            <div class="text-sm font-bold text-gray-700 dark:text-gray-300 mb-2 ml-24">{{ $t("page.system.aiConfig.toolParams") }}</div>
            <NFormItem
              v-for="param in selectedToolMeta.params"
              :key="param.name"
              :label="param.description || param.name"
              :required="param.required"
            >
              <NInput
                v-model:value="toolConfigValues[param.name]"
                :type="param.name.includes('key') || param.name.includes('secret') ? 'password' : 'text'"
                :show-password-on="param.name.includes('key') || param.name.includes('secret') ? 'click' : undefined"
                :placeholder="param.type + (param.default ? $t('page.system.aiConfig.paramDefaultSuffix', { value: param.default }) : '')"
              />
            </NFormItem>
          </div>

          <NFormItem :label="$t('page.system.aiConfig.toolEnabled')" path="enabled">
            <NSwitch v-model:value="toolForm.enabled" />
          </NFormItem>
          <NFormItem :label="$t('page.system.aiConfig.toolConfirmRequired')" path="confirm_required">
            <NSwitch v-model:value="toolForm.confirm_required" />
          </NFormItem>
          <div class="flex justify-end gap-2">
            <NButton @click="showToolModal = false">{{ $t("common.cancel") }}</NButton>
            <NButton type="primary" @click="handleSaveTool">{{ $t("common.confirm") }}</NButton>
          </div>
        </NForm>
      </div>
    </NModal>
  </div>
</template>

<style scoped></style>
