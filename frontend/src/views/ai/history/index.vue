<script setup lang="ts">
import { h, onMounted, ref, computed, resolveComponent } from "vue";
import { useRouter } from "vue-router";
import {
  useMessage,
  NTag,
  NButton,
  NDrawer,
  NDrawerContent,
  NAvatar,
  NSelect,
  NDropdown,
} from "naive-ui";
import type { DataTableColumns } from "naive-ui";
import { fetchHistoryList, fetchHistoryDetail, fetchUpdateFavorite, fetchDeleteHistory, fetchGenerateShareToken, fetchRevokeShareToken } from "@/service/api";
import { $t } from "@/locales";
import { useAppStore } from "@/store/modules/app";

import { renderMarkdown } from "@/utils/markdown";

const appStore = useAppStore();

const router = useRouter();

const message = useMessage();
const loading = ref(false);
const data = ref<any[]>([]);
const title = ref("");
const favoriteFilter = ref(null);
const favoriteOptions = computed(() => [
  { label: $t("page.ai.history.all"), value: null },
  { label: $t("page.ai.history.favorited"), value: true },
  { label: $t("page.ai.history.unfavorited"), value: false },
]);
const total = ref(0);

const pagination = ref({
  page: 1,
  pageSize: 10,
  itemCount: 0,
  onChange: (page: number) => {
    pagination.value.page = page;
    // eslint-disable-next-line @typescript-eslint/no-use-before-define
    loadData();
  },
  onUpdatePageSize: (pageSize: number) => {
    pagination.value.pageSize = pageSize;
    pagination.value.page = 1;
    // eslint-disable-next-line @typescript-eslint/no-use-before-define
    loadData();
  },
});

const loadData = async () => {
  loading.value = true;
  try {
    const { data: res } = await fetchHistoryList({
      title: title.value,
      is_favorite: favoriteFilter.value !== null ? favoriteFilter.value : undefined,
      page: pagination.value.page,
      pageSize: pagination.value.pageSize,
    });
    if (res) {
      data.value = res.items;
      total.value = res.total;
      pagination.value.itemCount = res.total;
    }
  } catch (err: any) {
    message.error(
      `${$t("page.ai.history.loadFailed")}: ${err?.message || $t("common.error")}`
    );
  } finally {
    loading.value = false;
  }
};

const showDrawer = ref(false);
const currentMessages = ref<any[]>([]);

const handleView = async (row: any) => {
  try {
    const { data: detail } = await fetchHistoryDetail(row.id);
    if (detail) {
      currentMessages.value = detail.messages || [];
      showDrawer.value = true;
    }
  } catch (err: any) {
    message.error(
      `${$t("page.ai.history.parseFailed")}: ${err?.message || $t("common.error")}`
    );
  }
};

const handleContinue = (row: any) => {
  if (row.custom_training_id) {
    router.push({
      path: `/ai/custom-training/${row.custom_training_id}`,
      query: { history_id: row.id },
    });
  } else {
    message.error($t("page.ai.history.unknownType"));
  }
};

const handleToggleFavorite = async (row: any) => {
  try {
    await fetchUpdateFavorite(row.id, !row.is_favorite);
    row.is_favorite = !row.is_favorite;
    message.success(
      row.is_favorite
        ? $t("page.ai.history.favoritedSuccess")
        : $t("page.ai.history.unfavoritedSuccess")
    );
  } catch (err: any) {
    message.error(
      `${$t("page.ai.history.operationFailed")}: ${err?.message || $t("common.error")}`
    );
  }
};

const handleDelete = async (row: any) => {
  const { error } = await fetchDeleteHistory(row.id);
  if (error) {
    message.error(
      `${$t("page.ai.history.deleteFailed")}: ${error.message || $t("common.error")}`
    );
    return;
  }
  message.success($t("page.ai.history.deleteSuccess"));
  // 末页删空后回退一页, 避免出现空页
  if (data.value.length === 1 && pagination.value.page > 1) {
    pagination.value.page--;
  }
  loadData();
};

const getShareUrl = (token: string) => {
  const isHashMode = import.meta.env.VITE_ROUTER_HISTORY_MODE === 'hash';
  const basePath = isHashMode ? '/#/' : '/';
  return `${window.location.origin}${basePath}share/${token}`;
};

const handleShare = async (row: any) => {
  try {
    const { data: res } = await fetchGenerateShareToken(row.id);
    if (res?.share_token) {
      const shareUrl = getShareUrl(res.share_token);
      await navigator.clipboard.writeText(shareUrl);
      message.success($t("page.ai.history.shareSuccess"));
      loadData();
    }
  } catch (err: any) {
    message.error(
      `${$t("page.ai.history.operationFailed")}: ${err?.message || $t("common.error")}`
    );
  }
};

const handleCopyShareLink = async (row: any) => {
  if (row.share_token) {
    const shareUrl = getShareUrl(row.share_token);
    await navigator.clipboard.writeText(shareUrl);
    message.success($t("page.ai.history.shareSuccess"));
  }
};

const handleRevokeShare = async (row: any) => {
  try {
    await fetchRevokeShareToken(row.id);
    message.success($t("page.ai.history.revokeShareSuccess"));
    loadData();
  } catch (err: any) {
    message.error(
      `${$t("page.ai.history.operationFailed")}: ${err?.message || $t("common.error")}`
    );
  }
};

const getMobileDropdownOptions = (row: any) => {
  return [
    row.share_token
      ? { label: $t("page.ai.history.copyLink"), key: "copyLink" }
      : { label: $t("page.ai.history.share"), key: "share" },
    row.share_token
      ? { label: $t("page.ai.history.revokeShare"), key: "revokeShare" }
      : null,
    { type: "divider" as const, key: "d1" },
    { label: $t("common.delete"), key: "delete" },
  ].filter((item): item is NonNullable<typeof item> => item !== null);
};

const handleMobileDropdownSelect = (key: string, row: any) => {
  switch (key) {
    case "share":
      handleShare(row);
      break;
    case "copyLink":
      handleCopyShareLink(row);
      break;
    case "revokeShare":
      handleRevokeShare(row);
      break;
    case "delete":
      handleDelete(row);
      break;
  }
};

const getTrainingTypeLabel = (trainingType: string) => {
  return trainingType;
};

const columns = computed<DataTableColumns<any>>(() => {
  return [
    {
      title: $t("page.ai.history.trainingProject"),
      key: "training_type",
      width: 120,
      render(row) {
        const label = row.training_type;
        return h(
          NTag,
          { type: "info", bordered: false },
          { default: () => label }
        );
      },
    },
    {
      title: $t("page.ai.history.titleContent"),
      key: "title",
      width: 200,
      ellipsis: { tooltip: true },
    },
    {
      title: $t("page.ai.history.recentChat"),
      key: "last_message",
      minWidth: 250,
      ellipsis: { tooltip: true },
      render(row) {
        if (!row.last_message)
          return h(
            "span",
            { class: "text-gray-400" },
            $t("page.ai.history.noChatRecord")
          );

        return h("div", { class: "text-xs text-gray-500" }, [
          h("span", row.last_message),
        ]);
      },
    },
    {
      title: $t("page.ai.history.favorite"),
      key: "is_favorite",
      width: 80,
      align: "center",
      render(row) {
        const SvgIcon = resolveComponent("SvgIcon");
        return h(
          "div",
          {
            class: "flex justify-center cursor-pointer",
            onClick: () => handleToggleFavorite(row),
          },
          [
            h(SvgIcon, {
              icon: row.is_favorite ? "mdi:star" : "mdi:star-outline",
              class: row.is_favorite
                ? "text-yellow-500 text-xl"
                : "text-gray-400 text-xl",
            }),
          ]
        );
      },
    },
    {
      title: $t("page.ai.history.trainingTime"),
      key: "created_at",
      width: 160,
      render(row) {
        return h("span", { class: "text-sm" }, new Date(row.created_at).toLocaleString());
      },
    },
    {
      title: $t("page.ai.history.actions"),
      key: "actions",
      width: 180,
      fixed: "right",
      render(row) {
        const dropdownOptions = [
          row.share_token
            ? { label: $t("page.ai.history.copyLink"), key: "copyLink" }
            : { label: $t("page.ai.history.share"), key: "share" },
          row.share_token
            ? { label: $t("page.ai.history.revokeShare"), key: "revokeShare" }
            : null,
          { type: "divider" as const, key: "d1" },
          { label: $t("common.delete"), key: "delete" },
        ].filter((item): item is NonNullable<typeof item> => item !== null);

        const handleDropdownSelect = (key: string) => {
          switch (key) {
            case "share":
              handleShare(row);
              break;
            case "copyLink":
              handleCopyShareLink(row);
              break;
            case "revokeShare":
              handleRevokeShare(row);
              break;
            case "delete":
              handleDelete(row);
              break;
          }
        };

        return h("div", { class: "flex gap-1" }, [
          h(
            NButton,
            {
              size: "small",
              type: "primary",
              quaternary: true,
              onClick: () => handleView(row),
            },
            { default: () => $t("page.ai.history.view") }
          ),
          h(
            NButton,
            {
              size: "small",
              type: "success",
              quaternary: true,
              onClick: () => handleContinue(row),
            },
            { default: () => $t("page.ai.history.continueTraining") }
          ),
          h(
            NDropdown,
            {
              options: dropdownOptions,
              onSelect: handleDropdownSelect,
              trigger: "click",
            },
            {
              default: () =>
                h(
                  NButton,
                  { size: "small", quaternary: true },
                  {
                    icon: () =>
                      h(resolveComponent("SvgIcon"), {
                        icon: "mdi:dots-vertical",
                      }),
                  }
                ),
            }
          ),
        ]);
      },
    },
  ];
});

onMounted(() => {
  loadData();
});
</script>

<template>
  <div class="h-full flex-col flex gap-4" :class="appStore.isMobile ? 'p-2' : 'p-4'">
    <NCard :bordered="false" :shadow="appStore.isMobile ? false : 'sm'" class="flex-1">
      <template #header>
        <div class="flex items-center gap-4">
          <span class="font-bold" :class="appStore.isMobile ? 'text-16px' : 'text-18px'">{{ $t("page.ai.history.title") }}</span>
        </div>
      </template>
      <div class="flex flex-col h-full gap-4">
        <!-- Search and Actions -->
        <template v-if="appStore.isMobile">
          <div class="flex gap-2">
            <NInput
              v-model:value="title"
              :placeholder="$t('page.ai.history.searchPlaceholder')"
              clearable
              class="flex-1"
              @keyup.enter="loadData"
            />
            <NButton type="primary" @click="loadData">
              <template #icon>
                <IconMdiMagnify class="text-icon" />
              </template>
            </NButton>
          </div>
          <div class="flex gap-2 justify-between">
            <NSelect
              v-model:value="favoriteFilter"
              :placeholder="$t('page.ai.history.favoriteStatus')"
              clearable
              :options="(favoriteOptions as any)"
              style="flex: 1"
              @update:value="loadData"
            />
            <NButton quaternary @click="loadData">
              <template #icon>
                <SvgIcon icon="mdi:refresh" class="text-icon" />
              </template>
            </NButton>
          </div>
        </template>
        <template v-else>
          <div class="flex justify-between items-center">
            <div class="flex gap-4 items-center">
              <NInput
                v-model:value="title"
                :placeholder="$t('page.ai.history.searchPlaceholder')"
                clearable
                style="width: 240px"
                @keyup.enter="loadData"
              />
              <NSelect
                v-model:value="favoriteFilter"
                :placeholder="$t('page.ai.history.favoriteStatus')"
                clearable
                :options="(favoriteOptions as any)"
                style="width: 120px"
                @update:value="loadData"
              />
              <NButton type="primary" @click="loadData">
                <template #icon>
                  <IconMdiMagnify class="text-icon" />
                </template>
                {{ $t("common.search") }}
              </NButton>
            </div>
            <div class="flex gap-2 items-center">
              <NButton quaternary @click="loadData">
                <template #icon>
                  <SvgIcon icon="mdi:refresh" class="text-icon" />
                </template>
              </NButton>
            </div>
          </div>
        </template>

        <!-- PC: DataTable -->
        <NDataTable
          v-if="!appStore.isMobile"
          :columns="columns"
          :data="data"
          :loading="loading"
          :pagination="pagination"
          remote
          :row-key="(row) => row.id"
          flex-height
          class="flex-1"
        />

        <!-- Mobile: Card List -->
        <NSpin v-if="appStore.isMobile && loading" class="flex justify-center py-8" />
        <div v-else-if="appStore.isMobile" class="flex flex-col gap-3">
          <NCard
            v-for="row in data"
            :key="row.id"
            size="small"
            :bordered="true"
          >
            <div class="flex flex-col gap-2">
              <div class="flex items-center justify-between">
                <NTag type="info" :bordered="false" size="small">
                  {{ getTrainingTypeLabel(row.training_type) }}
                </NTag>
                <div
                  class="cursor-pointer"
                  @click="handleToggleFavorite(row)"
                >
                  <SvgIcon
                    :icon="row.is_favorite ? 'mdi:star' : 'mdi:star-outline'"
                    :class="row.is_favorite ? 'text-yellow-500 text-xl' : 'text-gray-400 text-xl'"
                  />
                </div>
              </div>
              <div class="font-bold text-base">{{ row.title }}</div>
              <div v-if="row.last_message" class="text-xs text-gray-500 line-clamp-2">
                {{ row.last_message }}
              </div>
              <div v-else class="text-xs text-gray-400">{{ $t("page.ai.history.noChatRecord") }}</div>
              <div class="flex items-center justify-between mt-1">
                <span class="text-xs text-gray-400">{{ new Date(row.created_at).toLocaleDateString() }}</span>
                <div class="flex gap-1">
                  <NButton size="tiny" type="primary" quaternary @click="handleView(row)">
                    {{ $t("page.ai.history.view") }}
                  </NButton>
                  <NButton size="tiny" type="success" quaternary @click="handleContinue(row)">
                    {{ $t("page.ai.history.continueTraining") }}
                  </NButton>
                  <NDropdown
                    trigger="click"
                    :options="getMobileDropdownOptions(row)"
                    @select="(key: string) => handleMobileDropdownSelect(key, row)"
                  >
                    <NButton size="tiny" quaternary>
                      <SvgIcon icon="mdi:dots-vertical" />
                    </NButton>
                  </NDropdown>
                </div>
              </div>
            </div>
          </NCard>
          <NEmpty v-if="data.length === 0" class="py-8" />
          <!-- Mobile Pagination -->
          <div v-if="data.length > 0" class="flex justify-center mt-2">
            <NPagination
              v-model:page="pagination.page"
              :page-size="pagination.pageSize"
              :item-count="pagination.itemCount"
              @update:page="pagination.onChange"
            />
          </div>
        </div>
      </div>
    </NCard>

    <NDrawer v-model:show="showDrawer" :width="appStore.isMobile ? '90vw' : '600'" placement="right">
      <NDrawerContent :title="$t('page.ai.history.chatDetail')">
        <div class="flex flex-col gap-4">
          <div
            v-for="(msg, index) in currentMessages"
            :key="index"
            class="flex flex-col gap-2"
          >
            <template v-if="msg.role !== 'system'">
              <div
                class="flex items-start gap-3"
                :class="msg.role === 'user' ? 'flex-row-reverse' : 'flex-row'"
              >
                <NAvatar
                  :color="msg.role === 'user' ? '#18a058' : '#2080f0'"
                  round
                  size="small"
                >
                  {{ msg.role === "user" ? "U" : "AI" }}
                </NAvatar>
                <div class="flex flex-col gap-1 max-w-[80%]">
                  <div
                    class="p-3 rounded-2xl whitespace-pre-wrap leading-relaxed shadow-sm text-sm relative"
                    :class="
                      msg.role === 'user'
                        ? 'bg-[#18a058] text-white rounded-tr-none'
                        : 'bg-gray-100 text-gray-800 rounded-tl-none dark:bg-gray-800 dark:text-gray-200'
                    "
                  >
                    <!-- eslint-disable-next-line vue/no-v-html -->
                    <div v-html="renderMarkdown(msg.content)"></div>
                  </div>
                </div>
              </div>
            </template>
          </div>
        </div>
      </NDrawerContent>
    </NDrawer>
  </div>
</template>

<style scoped></style>
