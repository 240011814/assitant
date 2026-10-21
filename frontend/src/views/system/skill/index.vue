<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from 'vue';
import {
  NAlert, NButton, NCard, NDataTable, NDescriptions, NDescriptionsItem,
  NEmpty, NForm, NFormItem, NInput, NList, NListItem, NModal, NPopconfirm,
  NScrollbar, NSelect, NSpace, NSwitch, NTag, useMessage,
} from 'naive-ui';
import type { DataTableColumns, FormInst, FormRules } from 'naive-ui';
import { useAppStore } from '@/store/modules/app';
import { useAuth } from '@/hooks/business/auth';
import {
  fetchSkills, fetchCreateSkill, fetchUpdateSkill, fetchDeleteSkill,
  fetchDiscoverGithubSkills, fetchGithubSkillsCache,
} from '@/service/api';
import type { AISkillItem, AISkillCreateRequest, DiscoveredSkill } from '@/service/api';

const message = useMessage();
const appStore = useAppStore();
const { hasAuth } = useAuth();

const DEFAULT_DISCOVER_REPO = 'https://github.com/ComposioHQ/awesome-claude-skills';
const DEFAULT_DISCOVER_REF = 'master';

const loading = ref(false);
const items = ref<AISkillItem[]>([]);
const searchText = ref('');

const showModal = ref(false);
const editingId = ref<number | null>(null);
const formRef = ref<FormInst | null>(null);
const form = reactive<AISkillCreateRequest>({
  name: '', description: '', context: 'inline', agent: '', model: '', content: '', enabled: true,
});

const showDiscoverModal = ref(false);
const discovering = ref(false);
const discoverForm = reactive({ repo: '', ref: '', path: '' });
const discovered = ref<DiscoveredSkill[]>([]);
const discoverSearch = ref('');
const discoverTotal = ref(0);
const discoverCached = ref(false);
const discoverWarning = ref('');
const showViewModal = ref(false);
const viewingSkill = ref<DiscoveredSkill | null>(null);

const filteredDiscovered = computed(() => {
  const q = discoverSearch.value.trim().toLowerCase();
  if (!q) return discovered.value;
  return discovered.value.filter(i =>
    i.name.toLowerCase().includes(q) ||
    i.description.toLowerCase().includes(q) ||
    i.path.toLowerCase().includes(q)
  );
});

const contextOptions = [
  { label: 'inline（内联返回）', value: 'inline' },
  { label: 'fork（独立子 Agent）', value: 'fork' },
  { label: 'fork_with_context（携带上下文子 Agent）', value: 'fork_with_context' },
];

const contextTypeMap: Record<string, 'default' | 'info' | 'warning'> = {
  inline: 'default',
  fork: 'info',
  fork_with_context: 'warning',
};

const contextLabelMap: Record<string, string> = {
  inline: 'inline',
  fork: 'fork',
  fork_with_context: 'fork+ctx',
};

const rules: FormRules = {
  name: { required: true, message: '请输入 Skill 名称', trigger: 'blur' },
  content: { required: true, message: '请输入 Skill 内容', trigger: 'blur' },
};

const filtered = computed(() => {
  const q = searchText.value.trim().toLowerCase();
  if (!q) return items.value;
  return items.value.filter(i =>
    i.name.toLowerCase().includes(q) ||
    i.description.toLowerCase().includes(q) ||
    i.model.toLowerCase().includes(q)
  );
});

const stats = computed(() => ({
  total: items.value.length,
  enabled: items.value.filter(i => i.enabled).length,
}));

function formatTime(value: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString('zh-CN', { hour12: false });
}

async function loadData() {
  loading.value = true;
  try {
    const { data } = await fetchSkills();
    if (data) items.value = data;
  } catch { /* ignore */ }
  loading.value = false;
}

function openCreate() {
  editingId.value = null;
  Object.assign(form, {
    name: '', description: '', context: 'inline', agent: '', model: '', content: '', enabled: true,
  });
  showModal.value = true;
}

async function openDiscover() {
  discoverForm.repo = DEFAULT_DISCOVER_REPO;
  discoverForm.ref = DEFAULT_DISCOVER_REF;
  discoverForm.path = '';
  discovered.value = [];
  discoverSearch.value = '';
  discoverTotal.value = 0;
  discoverCached.value = false;
  discoverWarning.value = '';
  showDiscoverModal.value = true;
  await loadCachedDiscovery();
}

// 打开弹窗时优先展示上次扫描成功的内存缓存, 避免再次打开为空
async function loadCachedDiscovery() {
  if (!discoverForm.repo.trim()) return;
  try {
    const { data } = await fetchGithubSkillsCache({
      repo: discoverForm.repo.trim(),
      ref: discoverForm.ref.trim() || undefined,
      path: discoverForm.path.trim() || undefined,
    });
    if (data) {
      discovered.value = data.skills || [];
      discoverTotal.value = data.total || 0;
      discoverCached.value = true;
      discoverWarning.value = data.warning || '';
    }
  } catch { /* ignore */ }
}

async function runDiscover() {
  if (!discoverForm.repo.trim()) {
    message.warning('请输入 GitHub 仓库地址');
    return;
  }
  discovering.value = true;
  discoverWarning.value = '';
  try {
    const { data, error } = await fetchDiscoverGithubSkills({
      repo: discoverForm.repo.trim(),
      ref: discoverForm.ref.trim() || undefined,
      path: discoverForm.path.trim() || undefined,
    });
    if (error) {
      message.error(error.message || '扫描失败');
      return;
    }
    discovered.value = data?.skills || [];
    discoverTotal.value = data?.total || 0;
    discoverCached.value = data?.cached || false;
    discoverWarning.value = data?.warning || '';
    if (discoverWarning.value) message.warning(discoverWarning.value);
    else if (discoverTotal.value === 0) message.info('未在仓库中找到 SKILL.md');
    else message.success(`共发现 ${discoverTotal.value} 个 Skill`);
  } finally {
    discovering.value = false;
  }
}

// 查看扫描到的 Skill 详情
function viewDiscovered(item: DiscoveredSkill) {
  viewingSkill.value = item;
  showViewModal.value = true;
}

// 点击导入: 关闭发现弹窗, 把解析结果预填到新增表单, 由用户补充后保存
function applyDiscovered(item: DiscoveredSkill) {
  editingId.value = null;
  Object.assign(form, {
    name: item.name,
    description: item.description,
    context: item.context || 'inline',
    agent: item.agent,
    model: item.model,
    content: item.content,
    enabled: true,
  });
  showDiscoverModal.value = false;
  showViewModal.value = false;
  showModal.value = true;
}

function openEdit(row: AISkillItem) {
  editingId.value = row.id;
  Object.assign(form, {
    name: row.name,
    description: row.description,
    context: row.context || 'inline',
    agent: row.agent,
    model: row.model,
    content: row.content,
    enabled: row.enabled,
  });
  showModal.value = true;
}

async function handleSubmit() {
  try { await formRef.value?.validate(); } catch { return; }
  const result = editingId.value === null
    ? await fetchCreateSkill(form)
    : await fetchUpdateSkill(editingId.value, form);
  if (!result.error) {
    message.success(editingId.value === null ? '创建成功' : '更新成功');
    showModal.value = false;
    loadData();
  } else {
    message.error(result.error.message || '操作失败');
  }
}

async function handleToggle(row: AISkillItem, enabled: boolean) {
  const { error } = await fetchUpdateSkill(row.id, { enabled });
  if (error) {
    message.error('更新失败');
    return;
  }
  row.enabled = enabled;
  message.success(enabled ? '已启用' : '已停用');
}

async function handleDelete(id: number) {
  const { error } = await fetchDeleteSkill(id);
  if (!error) {
    message.success('删除成功');
    loadData();
  } else {
    message.error('删除失败');
  }
}

const columns: DataTableColumns<AISkillItem> = [
  { title: '名称', key: 'name', width: 160, ellipsis: { tooltip: true } },
  { title: '描述', key: 'description', ellipsis: { tooltip: true } },
  {
    title: '模式', key: 'context', width: 110,
    render: row => h(NTag, { type: contextTypeMap[row.context] || 'default', size: 'small' },
      { default: () => contextLabelMap[row.context] || row.context || 'inline' }),
  },
  {
    title: '模型', key: 'model', width: 120,
    render: row => row.model || '-',
  },
  {
    title: '状态', key: 'enabled', width: 90,
    render: row => h(NSwitch, {
      value: row.enabled,
      size: 'small',
      disabled: !hasAuth('system:skill:update'),
      'onUpdate:value': (val: boolean) => handleToggle(row, val),
    }),
  },
  {
    title: '更新时间', key: 'updated_at', width: 170,
    render: row => formatTime(row.updated_at),
  },
  {
    title: '操作', key: 'actions', width: 140,
    render: row => h(NSpace, { size: 'small' }, {
      default: () => [
        hasAuth('system:skill:update')
          ? h(NButton, { size: 'small', quaternary: true, type: 'primary', onClick: () => openEdit(row) }, { default: () => '编辑' })
          : null,
        hasAuth('system:skill:delete')
          ? h(NPopconfirm, { onPositiveClick: () => handleDelete(row.id) }, {
              trigger: () => h(NButton, { size: 'small', quaternary: true, type: 'error' }, { default: () => '删除' }),
              default: () => `确认删除 Skill「${row.name}」？`,
            })
          : null,
      ],
    }),
  },
];

onMounted(loadData);
</script>

<template>
  <div class="h-full flex-col flex gap-4" :class="appStore.isMobile ? 'p-2' : 'p-4'">
    <!-- ====== 统计 ====== -->
    <div class="grid grid-cols-2 gap-3">
      <NCard :bordered="false" size="small" class="text-center">
        <div class="text-2xl font-bold text-blue-500">{{ stats.total }}</div>
        <div class="text-xs text-gray-500 mt-1">Skill 总数</div>
      </NCard>
      <NCard :bordered="false" size="small" class="text-center">
        <div class="text-2xl font-bold text-green-500">{{ stats.enabled }}</div>
        <div class="text-xs text-gray-500 mt-1">已启用</div>
      </NCard>
    </div>

    <!-- ====== 主卡片 ====== -->
    <NCard :bordered="false" :shadow="appStore.isMobile ? false : 'sm'" class="flex-1">
      <div class="mb-4 flex flex-wrap items-center gap-3" :class="appStore.isMobile ? 'flex-col' : 'justify-between'">
        <NInput
          v-model:value="searchText"
          placeholder="搜索名称/描述/模型..."
          clearable
          :style="{ width: appStore.isMobile ? '100%' : '240px' }"
        >
          <template #prefix><SvgIcon icon="mdi:magnify" /></template>
        </NInput>
        <NSpace>
          <NButton v-if="hasAuth('system:skill:create')" secondary @click="openDiscover">
            <template #icon><SvgIcon icon="mdi:github" /></template>
            从 GitHub 导入
          </NButton>
          <NButton v-if="hasAuth('system:skill:create')" type="primary" @click="openCreate">
            <template #icon><SvgIcon icon="mdi:plus" /></template>
            新增 Skill
          </NButton>
        </NSpace>
      </div>

      <!-- PC: 表格 -->
      <template v-if="!appStore.isMobile">
        <NDataTable
          v-if="filtered.length > 0"
          :columns="columns"
          :data="filtered"
          :loading="loading"
          :scroll-x="1000"
          :row-key="row => row.id"
          size="small"
        />
        <NEmpty v-else description="暂无 Skill, 点击右上角新增" class="py-12" />
      </template>
      <!-- Mobile: 卡片列表 -->
      <template v-else>
        <NSpin v-if="loading" class="flex justify-center py-8" />
        <NEmpty v-else-if="filtered.length === 0" description="暂无 Skill, 点击右上角新增" class="py-12" />
        <div v-else class="flex flex-col gap-3">
          <NCard v-for="row in filtered" :key="row.id" size="small" :bordered="true">
            <div class="flex flex-col gap-2">
              <div class="flex items-center justify-between">
                <span class="font-bold text-16px">{{ row.name }}</span>
                <NSwitch
                  :value="row.enabled"
                  size="small"
                  :disabled="!hasAuth('system:skill:update')"
                  @update:value="(val: boolean) => handleToggle(row, val)"
                />
              </div>
              <div v-if="row.description" class="text-sm text-gray-600 dark:text-gray-400">
                {{ row.description }}
              </div>
              <div class="flex flex-wrap items-center gap-2 text-xs">
                <NTag :type="contextTypeMap[row.context] || 'default'" size="small" :bordered="false">
                  {{ contextLabelMap[row.context] || row.context || 'inline' }}
                </NTag>
                <span v-if="row.model" class="text-gray-400">{{ row.model }}</span>
                <span v-else class="text-gray-400">默认模型</span>
              </div>
              <div class="flex items-center justify-between mt-1">
                <span class="text-xs text-gray-400">{{ formatTime(row.updated_at) }}</span>
                <div class="flex gap-1">
                  <NButton
                    v-if="hasAuth('system:skill:update')"
                    size="tiny"
                    quaternary
                    type="primary"
                    @click="openEdit(row)"
                  >
                    编辑
                  </NButton>
                  <NPopconfirm v-if="hasAuth('system:skill:delete')" @positive-click="handleDelete(row.id)">
                    <template #trigger>
                      <NButton size="tiny" quaternary type="error">删除</NButton>
                    </template>
                    确认删除 Skill「{{ row.name }}」？
                  </NPopconfirm>
                </div>
              </div>
            </div>
          </NCard>
        </div>
      </template>
    </NCard>

    <!-- ====== 编辑弹窗 ====== -->
    <NModal
      v-model:show="showModal"
      preset="card"
      :title="editingId === null ? '新增 Skill' : '编辑 Skill'"
      :style="{ width: appStore.isMobile ? '95vw' : '720px' }"
      :segmented="{ content: true, footer: true }"
    >
      <NForm ref="formRef" :model="form" :rules="rules" label-placement="left" label-width="90">
        <NFormItem label="名称" path="name">
          <NInput v-model:value="form.name" placeholder="唯一标识, 如 pdf / xlsx" />
        </NFormItem>
        <NFormItem label="描述">
          <NInput v-model:value="form.description" type="textarea" :rows="2" placeholder="描述该 Skill 的能力与使用场景" />
        </NFormItem>
        <NFormItem label="执行模式">
          <NSelect v-model:value="form.context" :options="contextOptions" />
        </NFormItem>
        <NFormItem v-if="form.context !== 'inline'" label="子 Agent">
          <NInput v-model:value="form.agent" placeholder="fork 使用的子 Agent 名称(留空用默认)" />
        </NFormItem>
        <NFormItem label="模型">
          <NInput v-model:value="form.model" placeholder="留空使用默认模型" />
        </NFormItem>
        <NFormItem label="内容" path="content">
          <NInput
            v-model:value="form.content"
            type="textarea"
            :rows="10"
            placeholder="SKILL.md 正文 (frontmatter 之后的 markdown 指令)"
          />
        </NFormItem>
        <NFormItem label="启用">
          <NSwitch v-model:value="form.enabled" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showModal = false">取消</NButton>
          <NButton type="primary" @click="handleSubmit">保存</NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- ====== 从 GitHub 发现弹窗 ====== -->
    <NModal
      v-model:show="showDiscoverModal"
      preset="card"
      title="从 GitHub 仓库导入 Skill"
      :style="{ width: appStore.isMobile ? '95vw' : '720px' }"
      :segmented="{ content: true, footer: true }"
    >
      <NForm label-placement="left" label-width="90">
        <NFormItem label="仓库地址">
          <NInput v-model:value="discoverForm.repo" placeholder="owner/repo 或 https://github.com/owner/repo" />
        </NFormItem>
        <NFormItem label="分支/标签">
          <NInput v-model:value="discoverForm.ref" placeholder="留空使用默认分支, 如 main" />
        </NFormItem>
        <NFormItem label="子目录">
          <NInput v-model:value="discoverForm.path" placeholder="留空扫描全仓库, 如 skills" />
        </NFormItem>
      </NForm>
      <NSpace justify="end" class="mb-3">
        <NButton type="primary" :loading="discovering" @click="runDiscover">
          <template #icon><SvgIcon icon="mdi:magnify" /></template>
          扫描 SKILL.md
        </NButton>
      </NSpace>

      <NAlert v-if="discoverWarning" type="warning" class="mb-3" :show-icon="true">
        {{ discoverWarning }}
      </NAlert>

      <template v-if="discovered.length > 0">
        <div class="mb-2 flex items-center gap-3 flex-wrap">
          <span class="text-sm">
            共发现 <span class="font-bold text-blue-500">{{ discoverTotal }}</span> 个 Skill
          </span>
          <NTag v-if="discoverCached" size="small" type="warning">缓存结果</NTag>
          <NInput
            v-model:value="discoverSearch"
            size="small"
            placeholder="搜索名称/描述/路径..."
            clearable
            class="flex-1"
            :style="{ minWidth: '200px' }"
          >
            <template #prefix><SvgIcon icon="mdi:magnify" /></template>
          </NInput>
        </div>
        <NScrollbar style="max-height: 360px">
          <NList bordered>
            <NListItem v-for="item in filteredDiscovered" :key="item.path">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <div class="font-medium truncate">
                    {{ item.name }}
                    <NTag v-if="item.model" size="small" class="ml-2">{{ item.model }}</NTag>
                  </div>
                  <div class="text-xs text-gray-500 mt-1 line-clamp-2">{{ item.description || '无描述' }}</div>
                  <div class="text-xs text-gray-400 mt-1 truncate">{{ item.path }}</div>
                </div>
                <div class="flex gap-2 shrink-0">
                  <NButton size="small" @click="viewDiscovered(item)">查看</NButton>
                  <NButton size="small" type="primary" @click="applyDiscovered(item)">导入</NButton>
                </div>
              </div>
            </NListItem>
          </NList>
        </NScrollbar>
      </template>
      <NEmpty v-else-if="!discovering" description="输入仓库地址并点击扫描" class="py-8" />

      <template #footer>
        <NSpace justify="end">
          <NButton @click="showDiscoverModal = false">关闭</NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- ====== Skill 详情弹窗 ====== -->
    <NModal
      v-model:show="showViewModal"
      preset="card"
      :title="viewingSkill?.name || 'Skill 详情'"
      :style="{ width: appStore.isMobile ? '95vw' : '760px' }"
      :segmented="{ content: true, footer: true }"
    >
      <template v-if="viewingSkill">
        <NDescriptions :column="1" label-placement="left" bordered size="small">
          <NDescriptionsItem label="名称">{{ viewingSkill.name }}</NDescriptionsItem>
          <NDescriptionsItem label="描述">{{ viewingSkill.description || '-' }}</NDescriptionsItem>
          <NDescriptionsItem label="执行模式">{{ viewingSkill.context || 'inline' }}</NDescriptionsItem>
          <NDescriptionsItem label="模型">{{ viewingSkill.model || '-' }}</NDescriptionsItem>
          <NDescriptionsItem label="子 Agent">{{ viewingSkill.agent || '-' }}</NDescriptionsItem>
          <NDescriptionsItem label="路径">{{ viewingSkill.path }}</NDescriptionsItem>
        </NDescriptions>
        <div class="mt-4">
          <div class="text-sm font-medium mb-2">内容</div>
          <NScrollbar style="max-height: 320px">
            <pre class="text-xs whitespace-pre-wrap break-all p-2 bg-gray-50 rounded">{{ viewingSkill.content || '（无内容）' }}</pre>
          </NScrollbar>
        </div>
      </template>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showViewModal = false">关闭</NButton>
          <NButton v-if="hasAuth('system:skill:create')" type="primary" @click="viewingSkill && applyDiscovered(viewingSkill)">导入</NButton>
        </NSpace>
      </template>
    </NModal>
  </div>
</template>