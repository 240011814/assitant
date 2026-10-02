<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from "vue";
import { NAvatar, NButton, NPopconfirm, NSpace, NTag, useMessage } from "naive-ui";
import type { DataTableColumns, FormInst, FormRules } from "naive-ui";
import {
  fetchCreateUser,
  fetchDeleteUser,
  fetchGetRoles,
  fetchGetUsers,
  fetchProxyLogin,
  fetchUpdateUser,
} from "@/service/api";
import { useAuth } from "@/hooks/business/auth";
import { useAuthStore } from "@/store/modules/auth";
import { useRouteStore } from "@/store/modules/route";
import { useRouterPush } from "@/hooks/common/router";
import { useAppStore } from "@/store/modules/app";
import { $t } from "@/locales";

const message = useMessage();
const { hasAuth } = useAuth();
const authStore = useAuthStore();
const routeStore = useRouteStore();
const appStore = useAppStore();
const { routerPushByKey } = useRouterPush(false);

const isSuperAdmin = computed(() => authStore.userInfo.roles.includes("R_SUPER"));
const loading = ref(false);
const users = ref<Api.Admin.User[]>([]);
const roles = ref<Api.Admin.Role[]>([]);
const keyword = ref("");
const role = ref<string | null>(null);
const showModal = ref(false);
const editingUserId = ref<number | null>(null);
const formRef = ref<FormInst | null>(null);

const form = reactive({
  userName: "",
  password: "",
  nickname: "",
  role: "R_USER",
});

const roleOptions = computed(() =>
  roles.value.map(item => ({ label: `${item.name} (${item.code})`, value: item.code }))
);
const roleNameMap = computed(() => new Map(roles.value.map(item => [item.code, item.name])));

const rules: FormRules = {
  userName: [
    {
      required: true,
      message: $t("page.system.user.userNameRequired"),
      trigger: ["blur", "input"],
    },
  ],
  password: [
    {
      validator() {
        if (editingUserId.value === null && !form.password) {
          return new Error($t("page.system.user.passwordRequired"));
        }
        return true;
      },
      trigger: ["blur", "input"],
    },
  ],
  role: [
    {
      required: true,
      message: $t("page.system.user.roleRequired"),
      trigger: ["change", "blur"],
    },
  ],
};

const hasRolePermission = computed(() => hasAuth("system:role:list"));

const roleTagType = (roleCode: string) => {
  const map: Record<string, "error" | "warning" | "info" | "success"> = {
    R_SUPER: "error",
    R_ADMIN: "warning",
    R_USER: "info",
  };
  return map[roleCode] || "info";
};

const userStats = computed(() => {
  const total = users.value.length;
  const superCount = users.value.filter(u => u.role === "R_SUPER").length;
  const adminCount = users.value.filter(u => u.role === "R_ADMIN").length;
  const userCount = users.value.filter(u => u.role === "R_USER").length;
  return { total, superCount, adminCount, userCount };
});

const columns = computed<DataTableColumns<Api.Admin.User>>(() => [
  {
    title: $t("page.system.user.userName"),
    key: "userName",
    minWidth: 160,
    render(row) {
      return h("div", { class: "flex items-center gap-3" }, [
        h(NAvatar, { size: 32, round: true, color: "#f0f0f0" }, { default: () => row.userName.charAt(0).toUpperCase() }),
        h("div", { class: "flex flex-col" }, [
          h("span", { class: "font-medium text-gray-800 dark:text-gray-200" }, row.userName),
          h("span", { class: "text-xs text-gray-400" }, `ID: ${row.userId}`),
        ]),
      ]);
    },
  },
  { title: $t("page.system.user.nickname"), key: "nickname", minWidth: 120 },
  ...(hasRolePermission.value
    ? [
        {
          title: $t("page.system.user.role"),
          key: "role",
          width: 140,
          render(row: Api.Admin.User) {
            return h(
              NTag,
              { type: roleTagType(row.role), bordered: false, round: true, size: "small" },
              { default: () => roleNameMap.value.get(row.role) || row.role }
            );
          },
        },
      ]
    : []),
  {
    title: $t("page.system.user.createdAt"),
    key: "createdAt",
    width: 180,
    render(row) {
      return h("span", { class: "text-sm text-gray-500 dark:text-gray-400" }, new Date(row.createdAt).toLocaleString());
    },
  },
  {
    title: $t("page.system.user.actions"),
    key: "actions",
    render(row) {
      return h(
        NSpace,
        { size: "small" },
        {
          default: () => {
            const actions = [];
            if (isSuperAdmin.value && String(row.userId) !== authStore.userInfo.userId) {
              actions.push(
                h(
                  NButton,
                  { size: "small", type: "warning", ghost: true, onClick: () => handleProxyLogin(row) },
                  { default: () => $t("page.system.user.proxyLogin") }
                )
              );
            }
            if (hasAuth("system:user:update")) {
              actions.push(
                h(
                  NButton,
                  { size: "small", onClick: () => openEdit(row) },
                  { default: () => $t("common.edit") }
                )
              );
            }
            if (hasAuth("system:user:delete")) {
              actions.push(
                h(
                  NPopconfirm,
                  { onPositiveClick: () => handleDelete(row.userId) },
                  {
                    default: () => $t("page.system.user.deleteUserConfirm"),
                    trigger: () =>
                      h(
                        NButton,
                        { size: "small", type: "error", ghost: true },
                        { default: () => $t("common.delete") }
                      ),
                  }
                )
              );
            }
            return actions;
          },
        }
      );
    },
  },
]);

async function loadRoles() {
  const { data, error } = await fetchGetRoles();
  if (!error) {
    roles.value = data;
  }
}

async function loadUsers() {
  loading.value = true;
  const { data, error } = await fetchGetUsers({
    keyword: keyword.value,
    role: role.value || undefined,
    page: pagination.page,
    page_size: pagination.pageSize,
  });
  if (!error && data) {
    users.value = data.list;
    pagination.itemCount = data.total;
  }
  loading.value = false;
}

function onPageChange(page: number) {
  pagination.page = page;
  loadUsers();
}

function onPageSizeChange(pageSize: number) {
  pagination.pageSize = pageSize;
  pagination.page = 1;
  loadUsers();
}

function handleSearch() {
  pagination.page = 1;
  loadUsers();
}

const pagination = reactive({
  page: 1,
  pageSize: 10,
  itemCount: 0,
  showSizePicker: true,
  pageSizes: [10, 20, 50],
  onChange: onPageChange,
  onUpdatePageSize: onPageSizeChange,
});

function resetForm() {
  editingUserId.value = null;
  Object.assign(form, { userName: "", password: "", nickname: "", role: "R_USER" });
}

function openCreate() {
  resetForm();
  showModal.value = true;
}

function openEdit(row: Api.Admin.User) {
  editingUserId.value = row.userId;
  Object.assign(form, {
    userName: row.userName,
    password: "",
    nickname: row.nickname,
    role: row.role,
  });
  showModal.value = true;
}

async function handleSubmit() {
  if (!formRef.value) return;
  try {
    await formRef.value.validate();
  } catch {
    // 校验失败直接返回, 避免产生 unhandled rejection
    return;
  }
  const payload = {
    password: form.password || undefined,
    nickname: form.nickname,
    role: form.role,
  };
  const result =
    editingUserId.value === null
      ? await fetchCreateUser({ userName: form.userName, password: form.password, nickname: form.nickname, role: form.role })
      : await fetchUpdateUser(editingUserId.value, payload);
  if (!result.error) {
    message.success(editingUserId.value === null ? $t("page.system.user.createSuccess") : $t("page.system.user.updateSuccess"));
    showModal.value = false;
    loadUsers();
  }
}

async function handleDelete(userId: number) {
  const { error } = await fetchDeleteUser(userId);
  if (!error) {
    message.success($t("page.system.user.deleteSuccess"));
    loadUsers();
  }
}

async function handleProxyLogin(row: Api.Admin.User) {
  const { data, error } = await fetchProxyLogin(row.userId);
  if (!error) {
    await routeStore.resetStore();
    authStore.proxyMode = true;
    const pass = await authStore.loginByToken(data);
    if (pass) {
      await routeStore.initAuthRoute();
      message.success($t("page.system.user.proxyLoginSuccess"));
      routerPushByKey("root");
    }
  }
}

onMounted(async () => {
  if (hasRolePermission.value) {
    await loadRoles();
  }
  loadUsers();
});
</script>

<template>
  <div class="h-full flex-col flex gap-4" :class="appStore.isMobile ? 'p-2' : 'p-4'">
    <!-- Stats Cards -->
    <div class="grid gap-3" :class="appStore.isMobile ? 'grid-cols-2' : 'grid-cols-4'">
      <NCard :bordered="false" :shadow="appStore.isMobile ? false : 'sm'" size="small">
        <div class="flex items-center gap-3">
          <div class="i-mdi:account-group text-3xl text-primary" />
          <div>
            <div class="text-2xl font-bold text-gray-800 dark:text-gray-100">{{ userStats.total }}</div>
            <div class="text-xs text-gray-400">{{ $t("page.system.user.title") }}</div>
          </div>
        </div>
      </NCard>
      <NCard :bordered="false" :shadow="appStore.isMobile ? false : 'sm'" size="small">
        <div class="flex items-center gap-3">
          <div class="i-mdi:shield-crown text-3xl text-red-500" />
          <div>
            <div class="text-2xl font-bold text-gray-800 dark:text-gray-100">{{ userStats.superCount }}</div>
            <div class="text-xs text-gray-400">超级管理员</div>
          </div>
        </div>
      </NCard>
      <NCard :bordered="false" :shadow="appStore.isMobile ? false : 'sm'" size="small">
        <div class="flex items-center gap-3">
          <div class="i-mdi:shield-account text-3xl text-orange-500" />
          <div>
            <div class="text-2xl font-bold text-gray-800 dark:text-gray-100">{{ userStats.adminCount }}</div>
            <div class="text-xs text-gray-400">管理员</div>
          </div>
        </div>
      </NCard>
      <NCard :bordered="false" :shadow="appStore.isMobile ? false : 'sm'" size="small">
        <div class="flex items-center gap-3">
          <div class="i-mdi:account text-3xl text-blue-500" />
          <div>
            <div class="text-2xl font-bold text-gray-800 dark:text-gray-100">{{ userStats.userCount }}</div>
            <div class="text-xs text-gray-400">普通用户</div>
          </div>
        </div>
      </NCard>
    </div>

    <!-- Main Content -->
    <NCard :bordered="false" :shadow="appStore.isMobile ? false : 'sm'" class="flex-1">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-bold" :class="appStore.isMobile ? 'text-16px' : 'text-18px'">
            {{ $t("page.system.user.title") }}
          </span>
          <NButton v-if="hasAuth('system:user:create')" type="primary" @click="openCreate">
            <template #icon>
              <SvgIcon icon="mdi:account-plus-outline" />
            </template>
            <span v-if="!appStore.isMobile">{{ $t("page.system.user.addUser") }}</span>
          </NButton>
        </div>
      </template>

      <div class="flex flex-col h-full gap-4">
        <!-- Search Bar -->
        <template v-if="!appStore.isMobile">
          <div class="flex justify-between items-center">
            <div class="flex gap-3 items-center">
              <NInput
                v-model:value="keyword"
                clearable
                :placeholder="$t('page.system.user.searchPlaceholder')"
                style="width: 260px"
                @keyup.enter="handleSearch"
              />
              <NSelect
                v-if="hasRolePermission"
                v-model:value="role"
                clearable
                :options="roleOptions"
                :placeholder="$t('page.system.user.role')"
                style="width: 180px"
                @update:value="handleSearch"
              />
              <NButton type="primary" @click="handleSearch">
                <template #icon>
                  <SvgIcon icon="mdi:magnify" />
                </template>
                {{ $t("common.search") }}
              </NButton>
            </div>
            <NButton quaternary size="small" @click="loadUsers">
              <template #icon>
                <SvgIcon icon="mdi:refresh" />
              </template>
            </NButton>
          </div>
        </template>
        <template v-else>
          <div class="flex flex-col gap-2">
            <div class="flex gap-2">
              <NInput
                v-model:value="keyword"
                clearable
                class="flex-1"
                :placeholder="$t('page.system.user.searchPlaceholder')"
                @keyup.enter="handleSearch"
              />
              <NButton type="primary" @click="handleSearch">
                <template #icon>
                  <SvgIcon icon="mdi:magnify" />
                </template>
              </NButton>
            </div>
            <NSelect
              v-if="hasRolePermission"
              v-model:value="role"
              clearable
              :options="roleOptions"
              :placeholder="$t('page.system.user.role')"
              @update:value="handleSearch"
            />
          </div>
        </template>

        <!-- Desktop Table -->
        <template v-if="!appStore.isMobile">
          <NDataTable
            :columns="columns"
            :data="users"
            :loading="loading"
            :pagination="pagination"
            remote
            :row-key="row => row.userId"
            flex-height
            class="flex-1"
          />
        </template>

        <!-- Mobile Card List -->
        <template v-else>
          <NSpin v-if="loading" class="flex justify-center py-8" />
          <div v-else-if="users.length" class="flex flex-col gap-3">
            <NCard v-for="row in users" :key="row.userId" size="small" :bordered="true">
              <div class="flex items-start justify-between">
                <div class="flex items-center gap-3">
                  <NAvatar :size="36" round color="#f0f0f0">
                    {{ row.userName.charAt(0).toUpperCase() }}
                  </NAvatar>
                  <div class="flex flex-col">
                    <span class="font-medium text-gray-800 dark:text-gray-200">{{ row.userName }}</span>
                    <span class="text-xs text-gray-400">{{ row.nickname }}</span>
                  </div>
                </div>
                <NTag
                  v-if="hasRolePermission"
                  :type="roleTagType(row.role)"
                  bordered
                  round
                  size="small"
                >
                  {{ roleNameMap.get(row.role) || row.role }}
                </NTag>
              </div>
              <div class="flex items-center justify-between mt-3 pt-3 border-t border-gray-100 dark:border-gray-700">
                <span class="text-xs text-gray-400">{{ new Date(row.createdAt).toLocaleDateString() }}</span>
                <div class="flex gap-1">
                  <NButton
                    v-if="isSuperAdmin && String(row.userId) !== authStore.userInfo.userId"
                    size="tiny"
                    type="warning"
                    quaternary
                    @click="handleProxyLogin(row)"
                  >
                    {{ $t("page.system.user.proxyLogin") }}
                  </NButton>
                  <NButton
                    v-if="hasAuth('system:user:update')"
                    size="tiny"
                    type="primary"
                    quaternary
                    @click="openEdit(row)"
                  >
                    {{ $t("common.edit") }}
                  </NButton>
                  <NPopconfirm v-if="hasAuth('system:user:delete')" @positive-click="handleDelete(row.userId)">
                    <template #trigger>
                      <NButton size="tiny" type="error" quaternary>
                        {{ $t("common.delete") }}
                      </NButton>
                    </template>
                    {{ $t("page.system.user.deleteUserConfirm") }}
                  </NPopconfirm>
                </div>
              </div>
            </NCard>
            <div class="flex justify-center">
              <NPagination
                :page="pagination.page"
                :page-size="pagination.pageSize"
                :item-count="pagination.itemCount"
                :page-sizes="pagination.pageSizes"
                show-size-picker
                @update:page="onPageChange"
                @update:page-size="onPageSizeChange"
              />
            </div>
          </div>
          <NEmpty v-else class="py-8" />
        </template>
      </div>
    </NCard>

    <!-- Create/Edit Modal -->
    <NModal
      v-model:show="showModal"
      preset="card"
      :title="editingUserId === null ? $t('page.system.user.addUser') : $t('page.system.user.editUser')"
      :style="{ width: appStore.isMobile ? '95vw' : '520px' }"
      :segmented="{ content: true, footer: true }"
    >
      <NForm
        ref="formRef"
        :model="form"
        :rules="rules"
        label-placement="left"
        :label-width="appStore.isMobile ? '70' : '90'"
      >
        <NFormItem :label="$t('page.system.user.userName')" path="userName">
          <NInput
            v-model:value="form.userName"
            :disabled="editingUserId !== null"
            :placeholder="$t('page.system.user.userNameRequired')"
          >
            <template #prefix>
              <SvgIcon icon="mdi:account-outline" class="text-gray-400" />
            </template>
          </NInput>
        </NFormItem>
        <NFormItem :label="$t('page.system.user.passwordPlaceholder')" path="password">
          <NInput
            v-model:value="form.password"
            type="password"
            show-password-on="click"
            :placeholder="
              editingUserId === null
                ? $t('page.system.user.passwordPlaceholder')
                : $t('page.system.user.passwordEditPlaceholder')
            "
          >
            <template #prefix>
              <SvgIcon icon="mdi:lock-outline" class="text-gray-400" />
            </template>
          </NInput>
        </NFormItem>
        <NFormItem :label="$t('page.system.user.nickname')" path="nickname">
          <NInput
            v-model:value="form.nickname"
            :placeholder="$t('page.system.user.nicknamePlaceholder')"
          >
            <template #prefix>
              <SvgIcon icon="mdi:card-account-details-outline" class="text-gray-400" />
            </template>
          </NInput>
        </NFormItem>
        <NFormItem v-if="hasRolePermission" :label="$t('page.system.user.role')" path="role">
          <NSelect v-model:value="form.role" :options="roleOptions" />
        </NFormItem>
      </NForm>
      <template #footer>
        <div class="flex justify-end gap-3">
          <NButton @click="showModal = false">{{ $t("common.cancel") }}</NButton>
          <NButton type="primary" @click="handleSubmit">
            {{ $t("common.confirm") }}
          </NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>
