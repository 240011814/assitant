<script setup lang="ts">
import { h, onMounted, ref, computed, resolveComponent } from "vue";
import { NButton, NDropdown, useMessage, NTag } from "naive-ui";
import type { DataTableColumns } from "naive-ui";
import { fetchDeleteNote, fetchGetNoteList, fetchUpdateNote } from "@/service/api";
import { renderMarkdown } from "@/utils/markdown";
import { $t } from "@/locales";
import { useAppStore } from "@/store/modules/app";

const appStore = useAppStore();

const message = useMessage();
const loading = ref(false);
const data = ref<any[]>([]);
const keyword = ref("");
const category = ref("");
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
    const { data: res } = await fetchGetNoteList({
      content: keyword.value,
      category: category.value,
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
      `${$t("page.ai.note.loadFailed")}: ${err?.message || $t("common.error")}`
    );
  } finally {
    loading.value = false;
  }
};

const handleDelete = async (id: number) => {
  try {
    await fetchDeleteNote(id);
    message.success($t("page.ai.note.deleteSuccess"));
    if (data.value.length === 1 && pagination.value.page > 1) {
      pagination.value.page--;
    }
    loadData();
  } catch (err: any) {
    message.error(
      `${$t("page.ai.note.deleteFailed")}: ${err?.message || $t("common.error")}`
    );
  }
};

const showViewModal = ref(false);
const currentNoteContent = ref("");

const showEditModal = ref(false);
const isEdit = ref(false);
const editLoading = ref(false);
const editForm = ref({ id: 0, title: "", category: "", content: "" });

const handleAdd = () => {
  isEdit.value = false;
  editForm.value = { id: 0, title: "", category: "", content: "" };
  showEditModal.value = true;
};

const handleView = (row: any) => {
  currentNoteContent.value = row.content || "";
  showViewModal.value = true;
};

const handleEdit = (row: any) => {
  isEdit.value = true;
  editForm.value = {
    id: row.id,
    title: row.title,
    category: row.category,
    content: row.content,
  };
  showEditModal.value = true;
};

const submitEdit = async () => {
  if (
    !editForm.value.title.trim() ||
    !editForm.value.category.trim() ||
    !editForm.value.content.trim()
  ) {
    message.warning($t("page.ai.note.fieldsRequired"));
    return;
  }
  editLoading.value = true;
  try {
    if (isEdit.value) {
      await fetchUpdateNote(editForm.value.id, {
        title: editForm.value.title,
        category: editForm.value.category,
        content: editForm.value.content,
      });
      message.success($t("page.ai.note.updateSuccess"));
    } else {
      const { fetchAddNote } = await import("@/service/api");
      await fetchAddNote({
        title: editForm.value.title,
        category: editForm.value.category,
        content: editForm.value.content,
      });
      message.success($t("page.ai.note.addSuccess"));
    }
    showEditModal.value = false;
    loadData();
  } catch (err: any) {
    message.error(
      `${$t("page.ai.note.operationFailed")}: ${err?.message || $t("common.error")}`
    );
  } finally {
    editLoading.value = false;
  }
};

const getMobileDropdownOptions = () => {
  return [
    { label: $t("page.ai.note.edit"), key: "edit" },
    { type: "divider" as const, key: "d1" },
    { label: $t("common.delete"), key: "delete" },
  ];
};

const handleMobileDropdownSelect = (key: string, row: any) => {
  switch (key) {
    case "edit":
      handleEdit(row);
      break;
    case "delete":
      handleDelete(row.id);
      break;
  }
};

const columns = computed<DataTableColumns<any>>(() => {
  return [
    {
      title: $t("page.ai.note.noteTitle"),
      key: "title",
      width: 180,
      ellipsis: { tooltip: true },
      render(row) {
        return h("span", { class: "font-bold" }, row.title || $t("page.ai.note.noTitle"));
      },
    },
    {
      title: $t("page.ai.note.category"),
      key: "category",
      width: 100,
      render(row) {
        return h(
          NTag,
          { type: "info", bordered: false, size: "small" },
          { default: () => row.category }
        );
      },
    },
    {
      title: $t("page.ai.note.content"),
      key: "content",
      minWidth: 200,
      ellipsis: { tooltip: true },
      render(row) {
        const text = (row.content || "").replace(/[#*`\n]/g, " ").slice(0, 100);
        return h("span", { class: "text-sm text-gray-600 dark:text-gray-400" }, text);
      },
    },
    {
      title: $t("page.ai.note.createdAt"),
      key: "createdAt",
      width: 150,
      render(row) {
        return h("span", { class: "text-sm" }, new Date(row.createdAt).toLocaleString());
      },
    },
    {
      title: $t("page.ai.note.actions"),
      key: "actions",
      width: 120,
      fixed: "right",
      render(row) {
        const dropdownOptions = [
          { label: $t("page.ai.note.edit"), key: "edit" },
          { type: "divider" as const, key: "d1" },
          { label: $t("common.delete"), key: "delete" },
        ];

        const handleDropdownSelect = (key: string) => {
          switch (key) {
            case "edit":
              handleEdit(row);
              break;
            case "delete":
              handleDelete(row.id);
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
            { default: () => $t("page.ai.note.view") }
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
          <span class="font-bold" :class="appStore.isMobile ? 'text-16px' : 'text-18px'">{{ $t("page.ai.note.title") }}</span>
        </div>
      </template>
      <div class="flex flex-col h-full gap-4">
        <!-- Search and Actions -->
        <template v-if="appStore.isMobile">
          <div class="flex gap-2">
            <NInput
              v-model:value="keyword"
              :placeholder="$t('page.ai.note.searchTitlePlaceholder')"
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
          <div class="flex gap-2">
            <NInput
              v-model:value="category"
              :placeholder="$t('page.ai.note.searchCategoryPlaceholder')"
              clearable
              class="flex-1"
              @keyup.enter="loadData"
            />
            <ButtonIcon
              icon="mdi:refresh"
              :tooltip-content="$t('common.refresh')"
              @click="loadData"
            />
            <NButton type="primary" size="small" @click="handleAdd">
              <template #icon>
                <IconMdiPlus class="text-icon" />
              </template>
            </NButton>
          </div>
        </template>
        <template v-else>
          <div class="flex justify-between items-center">
            <div class="flex gap-4 items-center">
              <NInput
                v-model:value="keyword"
                :placeholder="$t('page.ai.note.searchTitlePlaceholder')"
                clearable
                style="width: 260px"
                @keyup.enter="loadData"
              />
              <NInput
                v-model:value="category"
                :placeholder="$t('page.ai.note.searchCategoryPlaceholder')"
                clearable
                style="width: 200px"
                @keyup.enter="loadData"
              />
              <NButton type="primary" @click="loadData">
                <template #icon>
                  <IconMdiMagnify class="text-icon" />
                </template>
                {{ $t("common.search") }}
              </NButton>
            </div>
            <div class="flex gap-2 items-center">
              <ButtonIcon
                icon="mdi:refresh"
                :tooltip-content="$t('common.refresh')"
                @click="loadData"
              />
              <NButton type="primary" @click="handleAdd">
                <template #icon>
                  <IconMdiPlus class="text-icon" />
                </template>
                {{ $t("page.ai.note.addNote") }}
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
          <div
            v-for="row in data"
            :key="row.id"
            class="bg-white dark:bg-gray-800 rounded-lg border border-gray-200 dark:border-gray-700 p-3"
          >
            <div class="flex flex-col gap-1.5">
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-2">
                  <span class="font-bold text-sm">{{ row.title || $t("page.ai.note.noTitle") }}</span>
                  <NTag type="info" :bordered="false" size="small">{{ row.category }}</NTag>
                </div>
                <span class="text-xs text-gray-400">{{ new Date(row.createdAt).toLocaleDateString() }}</span>
              </div>
              <div class="text-xs text-gray-500 line-clamp-2">
                {{ (row.content || "").replace(/[#*`\n]/g, " ").slice(0, 80) }}
              </div>
              <div class="flex gap-1 justify-end mt-1">
                <NButton size="tiny" type="primary" quaternary @click="handleView(row)">
                  {{ $t("page.ai.note.view") }}
                </NButton>
                <NDropdown
                  trigger="click"
                  :options="getMobileDropdownOptions()"
                  @select="(key: string) => handleMobileDropdownSelect(key, row)"
                >
                  <NButton size="tiny" quaternary>
                    <SvgIcon icon="mdi:dots-vertical" />
                  </NButton>
                </NDropdown>
              </div>
            </div>
          </div>
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

    <!-- 查看模态框 -->
    <NModal
      v-model:show="showViewModal"
      preset="card"
      :title="$t('page.ai.note.viewNote')"
      :style="{ width: appStore.isMobile ? '95vw' : '800px' }"
      :segmented="{ content: 'soft' }"
    >
      <!-- eslint-disable-next-line vue/no-v-html -->
      <div
        class="prose dark:prose-invert max-w-none overflow-y-auto p-4 bg-gray-50/50 dark:bg-dark-100 rounded-md text-sm leading-relaxed"
        style="max-height: 70vh"
        v-html="renderMarkdown(currentNoteContent)"
      ></div>
    </NModal>

    <!-- 编辑模态框 -->
    <NModal
      v-model:show="showEditModal"
      preset="card"
      :title="isEdit ? $t('page.ai.note.editNote') : $t('page.ai.note.addNote')"
      :style="{ width: appStore.isMobile ? '95vw' : '800px' }"
      :segmented="{ content: 'soft' }"
    >
      <NForm :model="editForm" label-placement="left" :label-width="appStore.isMobile ? '60' : '80'">
        <div :class="appStore.isMobile ? 'flex flex-col gap-2' : 'flex gap-4'">
          <NFormItem :label="$t('page.ai.note.noteTitle')" path="title" class="flex-1">
            <NInput
              v-model:value="editForm.title"
              :placeholder="$t('page.ai.note.noteTitlePlaceholder')"
            />
          </NFormItem>
          <NFormItem
            :label="$t('page.ai.note.category')"
            path="category"
            :style="appStore.isMobile ? {} : { width: '240px' }"
          >
            <NInput
              v-model:value="editForm.category"
              :placeholder="$t('page.ai.note.searchCategoryPlaceholder')"
            />
          </NFormItem>
        </div>
        <NFormItem :label="$t('page.ai.note.content')" path="content">
          <div :class="appStore.isMobile ? 'flex flex-col gap-4 w-full' : 'grid grid-cols-2 gap-4 w-full'">
            <NInput
              v-model:value="editForm.content"
              type="textarea"
              :autosize="appStore.isMobile ? { minRows: 6, maxRows: 10 } : { minRows: 12, maxRows: 15 }"
              :placeholder="$t('page.ai.note.noteContentPlaceholder')"
            />
            <!-- eslint-disable-next-line vue/no-v-html -->
            <div
              class="prose dark:prose-invert max-w-none overflow-y-auto p-4 border border-gray-200 dark:border-gray-700 rounded-md bg-gray-50/50 dark:bg-dark-100 text-sm leading-relaxed"
              :style="appStore.isMobile ? { maxHeight: '200px' } : { height: '100%', maxHeight: '350px' }"
              v-html="renderMarkdown(editForm.content)"
            ></div>
          </div>
        </NFormItem>
      </NForm>
      <template #footer>
        <div class="flex justify-end gap-3">
          <NButton @click="showEditModal = false">{{ $t("common.cancel") }}</NButton>
          <NButton type="primary" :loading="editLoading" @click="submitEdit">
            {{ $t("common.confirm") }}
          </NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>

<style scoped></style>
