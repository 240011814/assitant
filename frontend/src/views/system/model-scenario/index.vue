<script setup lang="ts">
import { onMounted, reactive, ref, computed } from 'vue';
import {
  NButton, NCard, NEmpty, NForm, NFormItem,
  NInput, NInputNumber, NModal, NSelect, NSpace,
  NSpin, NTabPane, NTabs, useMessage,
} from 'naive-ui';
import type { FormInst, FormRules } from 'naive-ui';
import { useAppStore } from '@/store/modules/app';
import { useAuth } from '@/hooks/business/auth';
import {
  fetchModelScenarios, fetchCreateModelScenario, fetchUpdateModelScenario, fetchDeleteModelScenario,
} from '@/service/api';
import type { ModelScenarioItem, ModelScenarioCreateRequest } from '@/service/api';
import ModelScenarioCard from './card.vue';

const message = useMessage();
const appStore = useAppStore();
const { hasAuth } = useAuth();

// ============ State ============
const activeTab = ref('model');
const loading = ref(false);
const items = ref<ModelScenarioItem[]>([]);
const expandedId = ref<number | null>(null);
const searchText = ref('');
const filterCategory = ref<string | null>(null);

const showModal = ref(false);
const editingId = ref<number | null>(null);
const formRef = ref<FormInst | null>(null);
const form = reactive<ModelScenarioCreateRequest>({
  type: 'model', name: '', summary: '', description: '', detail: '',
  category: '', sortOrder: 0,
});

// ============ Validation ============
const rules: FormRules = {
  name: { required: true, message: '请输入名称', trigger: 'blur' },
};

// ============ Computed ============
const tabItems = computed(() => items.value.filter(i => i.type === activeTab.value));

const categories = computed(() => {
  const cats = new Set(tabItems.value.map(i => i.category).filter(Boolean));
  return Array.from(cats).map(c => ({ label: c, value: c }));
});

const filtered = computed(() => {
  let list = tabItems.value;
  if (filterCategory.value) {
    list = list.filter(i => i.category === filterCategory.value);
  }
  if (searchText.value.trim()) {
    const q = searchText.value.trim().toLowerCase();
    list = list.filter(i =>
      i.name.toLowerCase().includes(q) ||
      i.summary.toLowerCase().includes(q) ||
      i.description.toLowerCase().includes(q)
    );
  }
  return list;
});

const stats = computed(() => ({
  modelCount: items.value.filter(i => i.type === 'model').length,
  scenarioCount: items.value.filter(i => i.type === 'scenario').length,
  categoryCount: new Set(items.value.map(i => i.category).filter(Boolean)).size,
}));

// ============ Load ============
async function loadData() {
  loading.value = true;
  try {
    const { data } = await fetchModelScenarios();
    if (data) items.value = data;
  } catch { /* ignore */ }
  loading.value = false;
}

// ============ CRUD ============
function resetForm() {
  Object.assign(form, {
    type: activeTab.value, name: '', summary: '', description: '', detail: '',
    category: '', sortOrder: 0,
  });
}

function openCreate() {
  resetForm();
  editingId.value = null;
  showModal.value = true;
}

function openEdit(row: ModelScenarioItem) {
  editingId.value = row.id;
  Object.assign(form, {
    type: row.type, name: row.name, summary: row.summary,
    description: row.description, detail: row.detail,
    category: row.category, sortOrder: row.sortOrder,
  });
  showModal.value = true;
}

async function handleSubmit() {
  try { await formRef.value?.validate(); } catch { return; }
  const result = editingId.value === null
    ? await fetchCreateModelScenario(form)
    : await fetchUpdateModelScenario(editingId.value, form);
  if (!result.error) {
    message.success(editingId.value === null ? '创建成功' : '更新成功');
    showModal.value = false;
    loadData();
  } else {
    message.error('操作失败');
  }
}

async function handleDelete(id: number) {
  const { error } = await fetchDeleteModelScenario(id);
  if (!error) { message.success('删除成功'); loadData(); }
  else message.error('删除失败');
}

function toggleExpand(id: number) {
  expandedId.value = expandedId.value === id ? null : id;
}

function clearFilters() {
  searchText.value = '';
  filterCategory.value = null;
}

// ============ Init ============
onMounted(loadData);
</script>

<template>
  <div class="h-full flex-col flex gap-4" :class="appStore.isMobile ? 'p-2' : 'p-4'">
    <!-- ====== Stats Summary ====== -->
    <div class="grid grid-cols-3 gap-3">
      <NCard :bordered="false" size="small" class="text-center">
        <div class="text-2xl font-bold text-blue-500">{{ stats.modelCount }}</div>
        <div class="text-xs text-gray-500 mt-1">🧠 模型</div>
      </NCard>
      <NCard :bordered="false" size="small" class="text-center">
        <div class="text-2xl font-bold text-purple-500">{{ stats.scenarioCount }}</div>
        <div class="text-xs text-gray-500 mt-1">🎬 场景</div>
      </NCard>
      <NCard :bordered="false" size="small" class="text-center">
        <div class="text-2xl font-bold text-amber-500">{{ stats.categoryCount }}</div>
        <div class="text-xs text-gray-500 mt-1">📂 分类</div>
      </NCard>
    </div>

    <!-- ====== Main Card ====== -->
    <NCard :bordered="false" :shadow="appStore.isMobile ? false : 'sm'" class="flex-1">
      <NTabs v-model:value="activeTab" type="line" @update:value="clearFilters">
        <!-- ====== Models Tab ====== -->
        <NTabPane name="model" tab="🧠 模型">
          <div class="mb-4 flex flex-wrap items-center gap-3" :class="appStore.isMobile ? 'flex-col' : 'justify-between'">
            <div class="flex flex-wrap items-center gap-2">
              <NInput
                v-model:value="searchText"
                placeholder="搜索名称/简介..."
                clearable
                :style="{ width: appStore.isMobile ? '100%' : '200px' }"
              >
                <template #prefix><SvgIcon icon="mdi:magnify" /></template>
              </NInput>
              <NSelect
                v-model:value="filterCategory"
                :options="categories"
                placeholder="分类筛选"
                clearable
                :style="{ width: '140px' }"
              />
            </div>
            <NButton v-if="hasAuth('model_scenario:create')" type="primary" @click="openCreate">
              <template #icon><SvgIcon icon="mdi:plus" /></template>
              新增模型
            </NButton>
          </div>

          <NSpin :show="loading">
            <div v-if="filtered.length > 0" class="grid gap-3" :class="appStore.isMobile ? 'grid-cols-1' : 'grid-cols-1 md:grid-cols-2 lg:grid-cols-3'">
              <ModelScenarioCard
                v-for="item in filtered"
                :key="item.id"
                :item="item"
                :expanded="expandedId === item.id"
                @toggle="toggleExpand(item.id)"
                @edit="openEdit"
                @delete="handleDelete"
              />
            </div>
            <NEmpty v-else description="暂无模型数据" class="py-12" />
          </NSpin>
        </NTabPane>

        <!-- ====== Scenarios Tab ====== -->
        <NTabPane name="scenario" tab="🎬 场景">
          <div class="mb-4 flex flex-wrap items-center gap-3" :class="appStore.isMobile ? 'flex-col' : 'justify-between'">
            <div class="flex flex-wrap items-center gap-2">
              <NInput
                v-model:value="searchText"
                placeholder="搜索名称/简介..."
                clearable
                :style="{ width: appStore.isMobile ? '100%' : '200px' }"
              >
                <template #prefix><SvgIcon icon="mdi:magnify" /></template>
              </NInput>
              <NSelect
                v-model:value="filterCategory"
                :options="categories"
                placeholder="分类筛选"
                clearable
                :style="{ width: '140px' }"
              />
            </div>
            <NButton v-if="hasAuth('model_scenario:create')" type="primary" @click="openCreate">
              <template #icon><SvgIcon icon="mdi:plus" /></template>
              新增场景
            </NButton>
          </div>

          <NSpin :show="loading">
            <div v-if="filtered.length > 0" class="grid gap-3" :class="appStore.isMobile ? 'grid-cols-1' : 'grid-cols-1 md:grid-cols-2 lg:grid-cols-3'">
              <ModelScenarioCard
                v-for="item in filtered"
                :key="item.id"
                :item="item"
                :expanded="expandedId === item.id"
                @toggle="toggleExpand(item.id)"
                @edit="openEdit"
                @delete="handleDelete"
              />
            </div>
            <NEmpty v-else description="暂无场景数据" class="py-12" />
          </NSpin>
        </NTabPane>
      </NTabs>
    </NCard>

    <!-- ====== Modal ====== -->
    <NModal
      v-model:show="showModal"
      preset="card"
      :title="editingId === null ? (activeTab === 'model' ? '新增模型' : '新增场景') : '编辑'"
      :style="{ width: appStore.isMobile ? '95vw' : '640px' }"
      :segmented="{ content: true, footer: true }"
    >
      <NForm ref="formRef" :model="form" :rules="rules" label-placement="left" label-width="80">
        <NFormItem label="名称" path="name">
          <NInput v-model:value="form.name" placeholder="名称" />
        </NFormItem>
        <NFormItem label="分类">
          <NInput v-model:value="form.category" :placeholder="form.type === 'model' ? '如：决策、沟通、思维' : '如：职场、家庭、社交'" />
        </NFormItem>
        <NFormItem label="简介">
          <NInput v-model:value="form.summary" type="textarea" placeholder="一句话简介" :rows="2" />
        </NFormItem>
        <NFormItem label="详细描述">
          <NInput v-model:value="form.description" type="textarea" placeholder="详细描述内容" :rows="5" />
        </NFormItem>
        <NFormItem :label="form.type === 'model' ? '适用场景' : '应对策略'">
          <NInput v-model:value="form.detail" type="textarea" :placeholder="form.type === 'model' ? '该模型适用于哪些场景' : '面对该场景的应对策略'" :rows="5" />
        </NFormItem>
        <NFormItem label="排序">
          <NInputNumber v-model:value="form.sortOrder" :min="0" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showModal = false">取消</NButton>
          <NButton type="primary" @click="handleSubmit">保存</NButton>
        </NSpace>
      </template>
    </NModal>
  </div>
</template>
