<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useMessage } from "naive-ui";
import { request } from "@/service/request";
import { ensureTimeoutConfigLoaded } from "@/service/request/timeout";
import { $t } from "@/locales";

defineOptions({ name: "ApiTimeoutSection" });

// ===================== 接口超时 (前端请求超时: api_timeout_default + 按路由覆盖) =====================
interface ApiRouteTimeout {
  method: string;
  path: string;
  /** 生效超时毫秒 (无覆盖 = 默认值) */
  timeoutMs: number;
  custom: boolean;
  streaming: boolean;
}

const message = useMessage();
const apiTimeoutRoutes = ref<ApiRouteTimeout[]>([]);
const apiTimeoutKeyword = ref("");
const apiTimeoutDefaultSec = ref(10);
const loadingApiTimeouts = ref(false);
const savingApiTimeouts = ref(false);
const savingApiTimeoutDefault = ref(false);
/** 未保存的行编辑: "METHOD path" -> 秒 (0 = 清除覆盖恢复默认) */
const apiTimeoutEdits = ref<Record<string, number>>({});

const apiTimeoutFiltered = computed(() => {
  const kw = apiTimeoutKeyword.value.trim().toLowerCase();
  if (!kw) return apiTimeoutRoutes.value;
  return apiTimeoutRoutes.value.filter(r => r.path.toLowerCase().includes(kw) || r.method.toLowerCase().includes(kw));
});
const apiTimeoutCustomCount = computed(() => apiTimeoutRoutes.value.filter(r => r.custom).length);
const apiTimeoutDirtyCount = computed(() => Object.keys(apiTimeoutEdits.value).length);

async function loadApiTimeouts() {
  loadingApiTimeouts.value = true;
  try {
    const { data, error } = await request<{ defaultMs: number; routes: ApiRouteTimeout[] }>({
      url: "/api/admin/system-config/api-timeouts",
    });
    if (!error && data) {
      apiTimeoutDefaultSec.value = Math.max(1, Math.round(data.defaultMs / 1000));
      apiTimeoutRoutes.value = data.routes || [];
      apiTimeoutEdits.value = {};
    }
  } finally {
    loadingApiTimeouts.value = false;
  }
}

const apiTimeoutKey = (r: ApiRouteTimeout) => `${r.method} ${r.path}`;

function rowSec(r: ApiRouteTimeout): number {
  const edited = apiTimeoutEdits.value[apiTimeoutKey(r)];
  if (edited !== undefined) return edited;
  return Math.max(1, Math.round((r.timeoutMs || 0) / 1000));
}

function setRowSec(r: ApiRouteTimeout, v: number | null) {
  apiTimeoutEdits.value[apiTimeoutKey(r)] = v ?? 0;
}

function clearRowOverride(r: ApiRouteTimeout) {
  apiTimeoutEdits.value[apiTimeoutKey(r)] = 0;
}

/** 按路由覆盖列表: 已覆盖项 + 本次编辑将新增的覆盖项 (编辑为 0 = 待恢复默认, 不算覆盖) */
const apiTimeoutOverrideRows = computed(() =>
  apiTimeoutRoutes.value.filter(r => r.custom || (apiTimeoutEdits.value[apiTimeoutKey(r)] ?? 0) > 0)
);

/** 该行是否有未保存的修改 (覆盖列表与全量表共用) */
function rowPending(r: ApiRouteTimeout): boolean {
  return apiTimeoutEdits.value[apiTimeoutKey(r)] !== undefined;
}

async function saveApiTimeoutDefault() {
  savingApiTimeoutDefault.value = true;
  try {
    const { error } = await request({
      url: "/api/admin/system-config/api-timeouts/default",
      method: "put",
      data: { timeoutMs: Math.round(apiTimeoutDefaultSec.value * 1000) },
    });
    if (!error) {
      message.success($t("page.system.config.apiTimeoutSaved"));
      ensureTimeoutConfigLoaded(true); // 本页后续请求立即生效
    }
  } finally {
    savingApiTimeoutDefault.value = false;
  }
}

async function saveApiTimeoutRows() {
  const entries = Object.entries(apiTimeoutEdits.value);
  if (entries.length === 0) return;
  savingApiTimeouts.value = true;
  try {
    let failed = 0;
    for (const [key, sec] of entries) {
      const idx = key.indexOf(" ");
      const { error } = await request({
        url: "/api/admin/system-config/api-timeouts/route",
        method: "put",
        data: { method: key.slice(0, idx), path: key.slice(idx + 1), timeoutMs: Math.round((sec || 0) * 1000) },
      });
      if (error) failed += 1;
    }
    if (failed === 0) {
      message.success($t("page.system.config.apiTimeoutSaved"));
      ensureTimeoutConfigLoaded(true);
    } else {
      message.error(`${failed} 条保存失败, 已刷新当前值`);
    }
    await loadApiTimeouts();
  } finally {
    savingApiTimeouts.value = false;
  }
}

onMounted(() => {
  void loadApiTimeouts();
});
</script>

<template>
  <!-- 接口超时 (前端请求超时: 默认值 + 按路由覆盖, 全量路由由后端枚举) -->
  <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700">
    <div class="mb-4 flex items-start justify-between gap-2 flex-wrap">
      <div>
        <div class="font-bold text-gray-800 dark:text-gray-200">{{ $t("page.system.config.apiTimeout") }}</div>
        <div class="text-sm text-gray-500 dark:text-gray-400 mt-1">
          {{ $t("page.system.config.apiTimeoutDesc") }}
        </div>
      </div>
      <div class="flex items-center gap-2">
        <NInput
          v-model:value="apiTimeoutKeyword"
          size="small"
          clearable
          :placeholder="$t('page.system.config.apiTimeoutSearch')"
          class="w-56"
        />
        <NButton size="small" :loading="loadingApiTimeouts" @click="loadApiTimeouts">
          {{ $t("common.refresh") }}
        </NButton>
      </div>
    </div>
    <NForm label-placement="left" label-width="100">
      <NFormItem :label="$t('page.system.config.apiTimeoutDefaultLabel')">
        <NInputNumber
          v-model:value="apiTimeoutDefaultSec"
          :min="1"
          :max="600"
          size="small"
          class="w-36"
        >
          <template #suffix>{{ $t("page.system.config.seconds") }}</template>
        </NInputNumber>
        <NButton size="small" class="ml-2" :loading="savingApiTimeoutDefault" @click="saveApiTimeoutDefault">
          {{ $t("common.save") }}
        </NButton>
        <span class="ml-3 text-xs text-gray-400">
          {{ $t("page.system.config.apiTimeoutCustomCount", { n: apiTimeoutCustomCount }) }}
        </span>
      </NFormItem>
    </NForm>
    <!-- 按路由覆盖 (只列非默认项: 已保存的覆盖 + 本次编辑将新增的覆盖) -->
    <div class="mt-2 mb-4">
      <div class="text-sm font-bold text-gray-700 dark:text-gray-300 mb-2">
        {{ $t("page.system.config.apiTimeoutOverridesTitle") }} ({{ apiTimeoutOverrideRows.length }})
      </div>
      <div v-if="apiTimeoutOverrideRows.length > 0" class="rounded border border-gray-100 dark:border-gray-800">
        <table class="w-full text-sm">
          <tbody>
            <tr
              v-for="r in apiTimeoutOverrideRows"
              :key="r.method + r.path"
              class="border-t border-gray-100 dark:border-gray-800 first:border-t-0 align-middle"
            >
              <td class="px-3 py-1.5 w-20">
                <NTag
                  size="tiny"
                  :type="r.method === 'GET' ? 'success' : r.method === 'POST' ? 'info' : 'warning'"
                  :bordered="false"
                >
                  {{ r.method }}
                </NTag>
              </td>
              <td class="px-3 py-1.5 font-mono text-xs break-all">
                {{ r.path }}
                <NTag v-if="rowPending(r)" size="tiny" type="warning" :bordered="false" class="ml-1">
                  {{ $t("page.system.config.apiTimeoutPending") }}
                </NTag>
              </td>
              <td class="px-3 py-1.5 w-48">
                <div v-if="!r.streaming" class="flex items-center gap-1">
                  <NInputNumber
                    :value="rowSec(r)"
                    size="tiny"
                    :min="0"
                    :max="600"
                    :show-button="false"
                    class="w-24"
                    @update:value="(v: number | null) => setRowSec(r, v)"
                  />
                  <span class="text-xs text-gray-400">{{ $t("page.system.config.seconds") }}</span>
                  <NButton size="tiny" quaternary :title="$t('page.system.config.apiTimeoutRowHint')" @click="clearRowOverride(r)">
                    ↺
                  </NButton>
                </div>
                <span v-else class="text-xs text-gray-400">—</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-else class="text-xs text-gray-400 py-2">{{ $t("page.system.config.apiTimeoutOverridesEmpty") }}</div>
    </div>
    <!-- 全部路由 (搜索定位, 修改某行并保存即生成覆盖) -->
    <div class="text-sm font-bold text-gray-700 dark:text-gray-300 mb-2">
      {{ $t("page.system.config.apiTimeoutAllRoutes") }} ({{ apiTimeoutRoutes.length }})
    </div>
    <div class="max-h-96 overflow-auto rounded border border-gray-100 dark:border-gray-800">
      <table class="w-full text-sm">
        <thead class="sticky top-0 bg-gray-50 dark:bg-gray-800 text-gray-500 dark:text-gray-400">
          <tr>
            <th class="text-left px-3 py-2 w-20">{{ $t("page.system.config.apiTimeoutMethod") }}</th>
            <th class="text-left px-3 py-2">{{ $t("page.system.config.apiTimeoutPath") }}</th>
            <th class="text-left px-3 py-2 w-44">{{ $t("page.system.config.apiTimeoutValue") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="r in apiTimeoutFiltered"
            :key="r.method + r.path"
            class="border-t border-gray-100 dark:border-gray-800 align-middle"
          >
            <td class="px-3 py-1.5">
              <NTag
                size="tiny"
                :type="r.method === 'GET' ? 'success' : r.method === 'POST' ? 'info' : 'warning'"
                :bordered="false"
              >
                {{ r.method }}
              </NTag>
            </td>
            <td class="px-3 py-1.5 font-mono text-xs break-all">
              {{ r.path }}
              <NTag v-if="r.streaming" size="tiny" :bordered="false" class="ml-1">SSE</NTag>
              <NTag v-if="r.custom" size="tiny" type="primary" :bordered="false" class="ml-1">
                {{ $t("page.system.config.custom") }}
              </NTag>
              <NTag v-if="rowPending(r)" size="tiny" type="warning" :bordered="false" class="ml-1">
                {{ $t("page.system.config.apiTimeoutPending") }}
              </NTag>
            </td>
            <td class="px-3 py-1.5">
              <div v-if="!r.streaming" class="flex items-center gap-1">
                <NInputNumber
                  :value="rowSec(r)"
                  size="tiny"
                  :min="0"
                  :max="600"
                  :show-button="false"
                  class="w-24"
                  @update:value="(v: number | null) => setRowSec(r, v)"
                />
                <NButton size="tiny" quaternary :title="$t('page.system.config.apiTimeoutRowHint')" @click="clearRowOverride(r)">
                  ↺
                </NButton>
              </div>
              <span v-else class="text-xs text-gray-400">—</span>
            </td>
          </tr>
        </tbody>
      </table>
      <NEmpty v-if="apiTimeoutFiltered.length === 0" size="small" class="py-6" />
    </div>
    <div class="mt-3 flex items-center gap-2 flex-wrap">
      <NButton
        type="primary"
        size="small"
        :loading="savingApiTimeouts"
        :disabled="apiTimeoutDirtyCount === 0"
        @click="saveApiTimeoutRows"
      >
        {{ $t("page.system.config.apiTimeoutSaveRows") }} ({{ apiTimeoutDirtyCount }})
      </NButton>
      <NButton v-if="apiTimeoutDirtyCount > 0" size="small" @click="apiTimeoutEdits = {}">
        {{ $t("common.cancel") }}
      </NButton>
      <span class="text-xs text-gray-400">{{ $t("page.system.config.apiTimeoutRowHint") }}</span>
    </div>
  </div>
</template>
