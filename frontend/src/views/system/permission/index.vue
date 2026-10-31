<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useMessage, useDialog } from "naive-ui";
import {
  fetchGetPermissions,
  fetchGetRolePermissions,
  fetchGetRoles,
  fetchUpdateRolePermissions,
  fetchCreateRole,
  fetchUpdateRole,
  fetchDeleteRole,
  fetchCreatePermission,
  fetchUpdatePermission,
  fetchDeletePermission,
} from "@/service/api";

import { $t } from "@/locales";

const message = useMessage();
const dialog = useDialog();

const roles = ref<Api.Admin.Role[]>([]);
const permissions = ref<Api.Admin.Permission[]>([]);
const selectedRole = ref("");
const checkedPermissions = ref<string[]>([]);
const loading = ref(false);
const saving = ref(false);

const showAddRoleModal = ref(false);
const addRoleLoading = ref(false);
const newRole = ref({
  code: "",
  name: "",
  description: "",
});

// 角色编辑 (名称/描述; 内置角色编码锁死, 自定义角色可改码)
const BUILT_IN_ROLE_CODES = ["R_SUPER", "R_ADMIN", "R_USER"];
const isBuiltInCode = (code: string) => BUILT_IN_ROLE_CODES.includes(code);
const showEditRoleModal = ref(false);
const editRoleSaving = ref(false);
const editRoleForm = ref({ originalCode: "", code: "", name: "", description: "" });

function openEditRole(role: Api.Admin.Role) {
  editRoleForm.value = { originalCode: role.code, code: role.code, name: role.name, description: role.description || "" };
  showEditRoleModal.value = true;
}

async function handleUpdateRole() {
  if (!editRoleForm.value.name.trim()) {
    message.warning($t("page.system.permission.roleInfoRequired"));
    return;
  }
  const form = editRoleForm.value;
  const codeChanged = form.code !== form.originalCode && !isBuiltInCode(form.originalCode);
  editRoleSaving.value = true;
  const { error } = await fetchUpdateRole(form.originalCode, {
    name: form.name.trim(),
    description: form.description.trim(),
    // 编码仅在自定义角色改码时下发
    ...(codeChanged ? { code: form.code.trim() } : {}),
  });
  editRoleSaving.value = false;
  if (!error) {
    message.success($t("page.system.permission.roleEditSuccess"));
    // 当前选中的角色若已改码, 选中态跟随新编码
    if (selectedRole.value === form.originalCode && codeChanged) {
      selectedRole.value = form.code.trim();
    }
    showEditRoleModal.value = false;
    await loadBaseData();
  }
}

const showPermissionModal = ref(false);
const permissionLoading = ref(false);
const isEditPermission = ref(false);
const permissionForm = ref({
  id: 0,
  code: "",
  name: "",
  groupName: "",
});

const groupedPermissions = computed(() => {
  const groups = new Map<string, Api.Admin.Permission[]>();
  permissions.value.forEach((permission) => {
    const items = groups.get(permission.groupName) || [];
    items.push(permission);
    groups.set(permission.groupName, items);
  });
  return Array.from(groups.entries()).map(([name, items]) => ({ name, items }));
});

async function loadBaseData() {
  const [roleResult, permissionResult] = await Promise.all([
    fetchGetRoles(),
    fetchGetPermissions(),
  ]);
  if (!roleResult.error) {
    roles.value = roleResult.data;
    if (roles.value.length > 0 && !selectedRole.value) {
      selectedRole.value = roles.value[0].code;
    }
  }
  if (!permissionResult.error) {
    permissions.value = permissionResult.data;
  }
}

async function loadRolePermissions() {
  if (!selectedRole.value) return;
  loading.value = true;
  const { data, error } = await fetchGetRolePermissions(selectedRole.value);
  if (!error) {
    checkedPermissions.value = data;
  }
  loading.value = false;
}

async function handleSaveRolePermissions() {
  if (!selectedRole.value) return;
  saving.value = true;
  const { error } = await fetchUpdateRolePermissions(
    selectedRole.value,
    checkedPermissions.value
  );
  if (!error) {
    message.success($t("page.system.permission.permissionSaveSuccess"));
  }
  saving.value = false;
}

async function handleAddRole() {
  if (!newRole.value.code || !newRole.value.name) {
    message.warning($t("page.system.permission.roleInfoRequired"));
    return;
  }
  addRoleLoading.value = true;
  const { error } = await fetchCreateRole(newRole.value);
  if (!error) {
    message.success($t("page.system.permission.roleCreateSuccess"));
    showAddRoleModal.value = false;
    newRole.value = { code: "", name: "", description: "" };
    await loadBaseData();
  }
  addRoleLoading.value = false;
}

function confirmDeleteRole(role: Api.Admin.Role) {
  dialog.warning({
    title: $t("page.system.permission.deleteConfirm"),
    content: $t("page.system.permission.deleteRoleConfirm", { name: role.name }),
    positiveText: $t("common.confirm"),
    negativeText: $t("common.cancel"),
    onPositiveClick: async () => {
      const { error } = await fetchDeleteRole(role.code);
      if (!error) {
        message.success($t("page.system.permission.roleDeleteSuccess"));
        if (selectedRole.value === role.code) {
          selectedRole.value = "";
        }
        await loadBaseData();
      }
    },
  });
}

function handleAddPermission(groupName?: string) {
  isEditPermission.value = false;
  permissionForm.value = { id: 0, code: "", name: "", groupName: groupName || "" };
  showPermissionModal.value = true;
}

function handleEditPermission(item: Api.Admin.Permission) {
  isEditPermission.value = true;
  permissionForm.value = { ...item };
  showPermissionModal.value = true;
}

async function handleSubmitPermission() {
  permissionLoading.value = true;
  let error;
  if (isEditPermission.value) {
    ({ error } = await fetchUpdatePermission(
      permissionForm.value.id,
      permissionForm.value
    ));
  } else {
    ({ error } = await fetchCreatePermission(permissionForm.value));
  }

  if (!error) {
    message.success(
      isEditPermission.value
        ? $t("page.system.permission.permissionUpdateSuccess")
        : $t("page.system.permission.permissionCreateSuccess")
    );
    showPermissionModal.value = false;
    await loadBaseData();
    if (selectedRole.value) {
      await loadRolePermissions();
    }
  }
  permissionLoading.value = false;
}

function confirmDeletePermission(item: Api.Admin.Permission) {
  dialog.warning({
    title: $t("page.system.permission.deleteConfirm"),
    content: $t("page.system.permission.deletePermissionConfirm", {
      name: item.name,
      code: item.code,
    }),
    positiveText: $t("common.confirm"),
    negativeText: $t("common.cancel"),
    onPositiveClick: async () => {
      const { error } = await fetchDeletePermission(item.id);
      if (!error) {
        message.success($t("page.system.permission.permissionDeleteSuccess"));
        await loadBaseData();
        if (selectedRole.value) {
          await loadRolePermissions();
        }
      }
    },
  });
}

const currentRoleName = computed(() => {
  return roles.value.find((r) => r.code === selectedRole.value)?.name || "";
});

watch(selectedRole, loadRolePermissions);

onMounted(async () => {
  await loadBaseData();
  if (selectedRole.value) {
    loadRolePermissions();
  }
});
</script>

<template>
  <div class="h-full flex gap-4 p-4">
    <!-- Role List Sidebar -->
    <NCard
      :bordered="false"
      shadow="sm"
      class="w-280px flex-shrink-0 flex flex-col"
      content-style="padding: 0; display: flex; flex-direction: column;"
    >
      <template #header>
        <div class="flex items-center justify-between">
          <span class="text-16px font-bold">{{
            $t("page.system.permission.roleList")
          }}</span>
          <NButton
            size="small"
            type="primary"
            quaternary
            circle
            @click="showAddRoleModal = true"
          >
            <template #icon>
              <SvgIcon icon="mdi:plus" />
            </template>
          </NButton>
        </div>
      </template>

      <NScrollbar class="flex-1 px-2 py-2">
        <div class="flex flex-col gap-1">
          <div
            v-for="role in roles"
            :key="role.code"
            class="group relative flex cursor-pointer items-center justify-between rounded-8px px-4 py-3 transition-all hover:bg-primary/5"
            :class="
              selectedRole === role.code
                ? 'bg-primary/10 text-primary shadow-sm'
                : 'text-gray-600'
            "
            @click="selectedRole = role.code"
          >
            <div class="flex flex-col overflow-hidden">
              <span class="truncate font-medium">{{ role.name }}</span>
              <span class="text-12px opacity-60">{{ role.code }}</span>
            </div>

            <div class="flex items-center gap-1">
              <NButton
                size="tiny"
                type="primary"
                quaternary
                circle
                class="opacity-0 group-hover:opacity-100"
                :title="$t('page.system.permission.roleEdit')"
                @click.stop="openEditRole(role)"
              >
                <template #icon>
                  <SvgIcon icon="mdi:pencil-outline" />
                </template>
              </NButton>
              <NButton
                v-if="
                  role.code !== 'R_SUPER' &&
                    role.code !== 'R_ADMIN' &&
                    role.code !== 'R_USER'
                "
                size="tiny"
                type="error"
                quaternary
                circle
                class="opacity-0 group-hover:opacity-100"
                @click.stop="confirmDeleteRole(role)"
              >
                <template #icon>
                  <SvgIcon icon="mdi:trash-can-outline" />
                </template>
              </NButton>
            </div>
          </div>
        </div>
      </NScrollbar>
    </NCard>

    <!-- Permission Area -->
    <NCard
      :bordered="false"
      shadow="sm"
      class="flex-1"
      content-style="display: flex; flex-direction: column;"
    >
      <template #header>
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <span class="text-18px font-bold">{{
              $t("page.system.permission.permissionConfig")
            }}</span>
            <NTag v-if="selectedRole" type="primary" round size="small">
              {{ currentRoleName }}
            </NTag>
          </div>
          <div class="flex gap-2">
            <NButton type="info" secondary @click="handleAddPermission()">
              <template #icon>
                <SvgIcon icon="mdi:plus-circle-outline" />
              </template>
              {{ $t("page.system.permission.addPermission") }}
            </NButton>
            <NButton
              type="primary"
              :loading="saving"
              :disabled="!selectedRole || selectedRole === 'R_SUPER'"
              @click="handleSaveRolePermissions"
            >
              <template #icon>
                <SvgIcon icon="mdi:content-save-outline" />
              </template>
              {{ $t("page.system.permission.saveConfig") }}
            </NButton>
          </div>
        </div>
      </template>

      <div
        v-if="!selectedRole"
        class="flex flex-1 items-center justify-center text-gray-400"
      >
        {{ $t("page.system.permission.selectRoleTip") }}
      </div>

      <div
        v-else-if="selectedRole === 'R_SUPER'"
        class="flex flex-1 flex-col items-center justify-center text-gray-400 gap-2"
      >
        <SvgIcon icon="mdi:shield-check" class="text-48px text-primary/40" />
        <span>{{ $t("page.system.permission.superAdminTip") }}</span>
      </div>

      <NSpin v-else :show="loading" class="flex-1">
        <NScrollbar class="pr-2">
          <div class="grid grid-cols-1 gap-4 lg:grid-cols-2 pb-4">
            <NCard
              v-for="group in groupedPermissions"
              :key="group.name"
              size="small"
              :bordered="true"
              class="rounded-12px border-gray-100 transition-shadow hover:shadow-md"
            >
              <template #header>
                <div class="flex items-center justify-between w-full">
                  <span class="font-bold">{{ group.name }}</span>
                  <NButton
                    size="tiny"
                    quaternary
                    type="primary"
                    @click="handleAddPermission(group.name)"
                  >
                    <template #icon><SvgIcon icon="mdi:plus" /></template>
                  </NButton>
                </div>
              </template>

              <NCheckboxGroup v-model:value="checkedPermissions">
                <div class="grid grid-cols-1 gap-x-4 gap-y-3 sm:grid-cols-1">
                  <div
                    v-for="item in group.items"
                    :key="item.code"
                    class="group/item flex items-center justify-between py-1 px-2 rounded hover:bg-gray-50"
                  >
                    <NCheckbox :value="item.code" class="flex-1">
                      <div class="flex flex-col">
                        <span class="group-hover/item:text-primary transition-colors">{{
                          item.name
                        }}</span>
                        <span class="text-12px text-gray-400 font-mono">{{
                          item.code
                        }}</span>
                      </div>
                    </NCheckbox>
                    <div
                      class="flex opacity-0 group-hover/item:opacity-100 transition-opacity"
                    >
                      <NButton
                        size="tiny"
                        quaternary
                        type="info"
                        @click.stop="handleEditPermission(item)"
                      >
                        <template #icon><SvgIcon icon="mdi:pencil-outline" /></template>
                      </NButton>
                      <NButton
                        size="tiny"
                        quaternary
                        type="error"
                        @click.stop="confirmDeletePermission(item)"
                      >
                        <template #icon>
                          <SvgIcon icon="mdi:trash-can-outline" />
                        </template>
                      </NButton>
                    </div>
                  </div>
                </div>
              </NCheckboxGroup>
            </NCard>
          </div>
        </NScrollbar>
      </NSpin>
    </NCard>

    <!-- Add Role Modal -->
    <NModal
      v-model:show="showAddRoleModal"
      preset="card"
      :title="$t('page.system.permission.addRole')"
      class="w-450px"
      :segmented="{ content: true, footer: true }"
    >
      <NForm>
        <NFormItem :label="$t('page.system.permission.roleName')" path="name">
          <NInput
            v-model:value="newRole.name"
            :placeholder="$t('page.system.permission.roleNamePlaceholder')"
          />
        </NFormItem>
        <NFormItem :label="$t('page.system.permission.roleCode')" path="code">
          <NInput
            v-model:value="newRole.code"
            :placeholder="$t('page.system.permission.roleCodePlaceholder')"
          />
        </NFormItem>
        <NFormItem :label="$t('page.system.permission.description')" path="description">
          <NInput
            v-model:value="newRole.description"
            type="textarea"
            :placeholder="$t('page.system.permission.descriptionPlaceholder')"
          />
        </NFormItem>
      </NForm>
      <template #footer>
        <div class="flex justify-end gap-3">
          <NButton @click="showAddRoleModal = false">{{ $t("common.cancel") }}</NButton>
          <NButton type="primary" :loading="addRoleLoading" @click="handleAddRole">
            {{ $t("common.confirm") }}
          </NButton>
        </div>
      </template>
    </NModal>

    <!-- Edit Role Modal (编码不可改) -->
    <NModal
      v-model:show="showEditRoleModal"
      preset="card"
      :title="$t('page.system.permission.roleEdit')"
      class="w-450px"
      :segmented="{ content: true, footer: true }"
    >
      <NForm>
        <NFormItem :label="$t('page.system.permission.roleCode')" path="code">
          <NInput
            v-model:value="editRoleForm.code"
            :disabled="isBuiltInCode(editRoleForm.code)"
          />
        </NFormItem>
        <NFormItem :label="$t('page.system.permission.roleName')" path="name">
          <NInput
            v-model:value="editRoleForm.name"
            :placeholder="$t('page.system.permission.roleNamePlaceholder')"
          />
        </NFormItem>
        <NFormItem :label="$t('page.system.permission.description')" path="description">
          <NInput
            v-model:value="editRoleForm.description"
            type="textarea"
            :placeholder="$t('page.system.permission.descriptionPlaceholder')"
          />
        </NFormItem>
      </NForm>
      <template #footer>
        <div class="flex justify-end gap-3">
          <NButton @click="showEditRoleModal = false">{{ $t("common.cancel") }}</NButton>
          <NButton type="primary" :loading="editRoleSaving" @click="handleUpdateRole">
            {{ $t("common.confirm") }}
          </NButton>
        </div>
      </template>
    </NModal>

    <!-- Add/Edit Permission Modal -->
    <NModal
      v-model:show="showPermissionModal"
      preset="card"
      :title="
        isEditPermission
          ? $t('page.system.permission.addPermission')
          : $t('page.system.permission.addPermission')
      "
      class="w-450px"
      :segmented="{ content: true, footer: true }"
    >
      <NForm>
        <NFormItem :label="$t('page.system.permission.permissionName')" path="name">
          <NInput
            v-model:value="permissionForm.name"
            :placeholder="$t('page.system.permission.permissionNamePlaceholder')"
          />
        </NFormItem>
        <NFormItem :label="$t('page.system.permission.permissionCode')" path="code">
          <NInput
            v-model:value="permissionForm.code"
            :disabled="isEditPermission"
            :placeholder="$t('page.system.permission.permissionCodePlaceholder')"
          />
        </NFormItem>
        <NFormItem :label="$t('page.system.permission.groupName')" path="groupName">
          <NInput
            v-model:value="permissionForm.groupName"
            :placeholder="$t('page.system.permission.groupNamePlaceholder')"
          />
        </NFormItem>
      </NForm>
      <template #footer>
        <div class="flex justify-end gap-3">
          <NButton @click="showPermissionModal = false">
            {{ $t("common.cancel") }}
          </NButton>
          <NButton
            type="primary"
            :loading="permissionLoading"
            @click="handleSubmitPermission"
          >
            {{ $t("common.confirm") }}
          </NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>

<style scoped>
:deep(.n-card-header__main) {
  width: 100%;
}
</style>
