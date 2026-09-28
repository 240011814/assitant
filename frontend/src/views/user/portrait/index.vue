<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useMessage, useDialog } from 'naive-ui';
import {
  fetchCreateUserExperience,
  fetchDeleteUserExperience,
  fetchGetUserPortrait,
  fetchTriggerPortraitExtract,
  fetchUpdateUserExperience,
  fetchUpdateUserPortrait
} from '@/service/api';
import type { UserExperience, UserPortrait } from '@/service/api';
import { useAppStore } from '@/store/modules/app';
import { $t } from '@/locales';

defineOptions({ name: 'UserPortrait' });

const message = useMessage();
const dialog = useDialog();
const appStore = useAppStore();

const loading = ref(false);
const extracting = ref(false);
const savingPortrait = ref(false);
const savingExperience = ref(false);

const portrait = ref<UserPortrait | null>(null);
const experiences = ref<UserExperience[]>([]);

const categoryOptions = computed(() => [
  { label: $t('page.userPortrait.category.identity'), value: 'identity' },
  { label: $t('page.userPortrait.category.goal'), value: 'goal' },
  { label: $t('page.userPortrait.category.project'), value: 'project' },
  { label: $t('page.userPortrait.category.skill'), value: 'skill' },
  { label: $t('page.userPortrait.category.preference'), value: 'preference' },
  { label: $t('page.userPortrait.category.habit'), value: 'habit' },
  { label: $t('page.userPortrait.category.experience'), value: 'experience' },
  { label: $t('page.userPortrait.category.challenge'), value: 'challenge' },
  { label: $t('page.userPortrait.category.decision'), value: 'decision' }
]);

const categoryLabelMap = computed<Record<string, string>>(() => {
  const map: Record<string, string> = {};
  categoryOptions.value.forEach(item => {
    map[item.value] = item.label;
  });
  return map;
});

const statusOptions = computed(() => [
  { label: $t('page.userPortrait.status.ongoing'), value: 'ongoing' },
  { label: $t('page.userPortrait.status.completed'), value: 'completed' },
  { label: $t('page.userPortrait.status.abandoned'), value: 'abandoned' },
  { label: $t('page.userPortrait.status.unknown'), value: 'unknown' }
]);

const statusLabelMap = computed<Record<string, string>>(() => {
  const map: Record<string, string> = {};
  statusOptions.value.forEach(item => {
    map[item.value] = item.label;
  });
  return map;
});

const memoryLevelOptions = computed(() => [
  { label: $t('page.userPortrait.memoryLevel.core'), value: 'core' },
  { label: $t('page.userPortrait.memoryLevel.long_term'), value: 'long_term' },
  { label: $t('page.userPortrait.memoryLevel.temporary'), value: 'temporary' }
]);

const memoryLevelLabelMap = computed<Record<string, string>>(() => {
  const map: Record<string, string> = {};
  memoryLevelOptions.value.forEach(item => {
    map[item.value] = item.label;
  });
  return map;
});

const dimensionLabelMap = computed<Record<string, string>>(() => ({
  identity: $t('page.userPortrait.dimensionLabel.identity'),
  goals: $t('page.userPortrait.dimensionLabel.goals'),
  skills: $t('page.userPortrait.dimensionLabel.skills'),
  projects: $t('page.userPortrait.dimensionLabel.projects'),
  preferences: $t('page.userPortrait.dimensionLabel.preferences'),
  habits: $t('page.userPortrait.dimensionLabel.habits'),
  learning_topics: $t('page.userPortrait.dimensionLabel.learning_topics'),
  constraints: $t('page.userPortrait.dimensionLabel.constraints')
}));

function formatTime(value: string | null | undefined) {
  if (!value) return '-';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return '-';
  return d.toLocaleString();
}

type TagType = 'default' | 'error' | 'info' | 'primary' | 'success' | 'warning';

function statusTagType(status: string): TagType {
  if (status === 'completed') return 'success';
  if (status === 'ongoing') return 'info';
  if (status === 'abandoned') return 'error';
  return 'default';
}

function memoryLevelTagType(level: string): TagType {
  if (level === 'core') return 'warning';
  if (level === 'temporary') return 'default';
  return 'info';
}

async function loadData() {
  loading.value = true;
  const { data, error } = await fetchGetUserPortrait();
  if (!error && data) {
    portrait.value = data.portrait;
    experiences.value = data.experiences || [];
  }
  loading.value = false;
}

async function handleExtract() {
  extracting.value = true;
  const { data, error } = await fetchTriggerPortraitExtract();
  extracting.value = false;
  if (!error) {
    message.success($t('page.userPortrait.extractSuccess', { count: data?.extracted ?? 0 }));
    await loadData();
  }
}

// ===== 画像编辑 =====
const portraitModalVisible = ref(false);
const portraitForm = reactive({
  summary: '',
  tags: [] as string[],
  dimensions: [] as { key: string; value: string }[]
});

function openPortraitModal() {
  portraitForm.summary = portrait.value?.summary || '';
  portraitForm.tags = [...(portrait.value?.tags || [])];
  const dims = portrait.value?.dimensions || {};
  portraitForm.dimensions = Object.entries(dims).map(([key, value]) => ({ key, value }));
  portraitModalVisible.value = true;
}

function addDimension() {
  portraitForm.dimensions.push({ key: '', value: '' });
}

function removeDimension(index: number) {
  portraitForm.dimensions.splice(index, 1);
}

async function savePortrait() {
  const dimensions: Record<string, string> = {};
  portraitForm.dimensions.forEach(item => {
    if (item.key.trim()) dimensions[item.key.trim()] = item.value;
  });
  savingPortrait.value = true;
  const { error } = await fetchUpdateUserPortrait({
    summary: portraitForm.summary,
    tags: portraitForm.tags,
    dimensions
  });
  savingPortrait.value = false;
  if (!error) {
    message.success($t('page.userPortrait.saveSuccess'));
    portraitModalVisible.value = false;
    await loadData();
  }
}

// ===== 经历编辑 =====
const experienceModalVisible = ref(false);
const editingExperienceId = ref<number | null>(null);
const experienceForm = reactive({
  category: 'experience',
  title: '',
  content: '',
  timeRange: '',
  status: 'unknown',
  memoryLevel: 'long_term',
  evidence: '',
  tags: [] as string[],
  occurredAt: null as number | null
});

function openCreateExperience() {
  editingExperienceId.value = null;
  experienceForm.category = 'experience';
  experienceForm.title = '';
  experienceForm.content = '';
  experienceForm.timeRange = '';
  experienceForm.status = 'unknown';
  experienceForm.memoryLevel = 'long_term';
  experienceForm.evidence = '';
  experienceForm.tags = [];
  experienceForm.occurredAt = null;
  experienceModalVisible.value = true;
}

function openEditExperience(item: UserExperience) {
  editingExperienceId.value = item.id;
  experienceForm.category = item.category || 'experience';
  experienceForm.title = item.title;
  experienceForm.content = item.content;
  experienceForm.timeRange = item.time_range || '';
  experienceForm.status = item.status || 'unknown';
  experienceForm.memoryLevel = item.memory_level || 'long_term';
  experienceForm.evidence = item.evidence || '';
  experienceForm.tags = [...(item.tags || [])];
  experienceForm.occurredAt = item.occurred_at ? new Date(item.occurred_at).getTime() : null;
  experienceModalVisible.value = true;
}

async function saveExperience() {
  if (!experienceForm.title.trim()) {
    message.warning($t('page.userPortrait.titleRequired'));
    return;
  }
  const payload = {
    category: experienceForm.category,
    title: experienceForm.title,
    content: experienceForm.content,
    time_range: experienceForm.timeRange,
    status: experienceForm.status,
    memory_level: experienceForm.memoryLevel,
    evidence: experienceForm.evidence,
    tags: experienceForm.tags,
    occurred_at: experienceForm.occurredAt ? new Date(experienceForm.occurredAt).toISOString() : null
  };
  savingExperience.value = true;
  const { error } =
    editingExperienceId.value === null
      ? await fetchCreateUserExperience(payload)
      : await fetchUpdateUserExperience(editingExperienceId.value, payload);
  savingExperience.value = false;
  if (!error) {
    message.success($t('page.userPortrait.saveSuccess'));
    experienceModalVisible.value = false;
    await loadData();
  }
}

function handleDeleteExperience(item: UserExperience) {
  dialog.warning({
    title: $t('page.userPortrait.deleteConfirmTitle'),
    content: $t('page.userPortrait.deleteConfirm'),
    positiveText: $t('page.userPortrait.confirm'),
    negativeText: $t('page.userPortrait.cancel'),
    onPositiveClick: async () => {
      const { error } = await fetchDeleteUserExperience(item.id);
      if (!error) {
        message.success($t('page.userPortrait.deleteSuccess'));
        await loadData();
      }
    }
  });
}

onMounted(loadData);
</script>

<template>
  <div class="h-full">
    <NSpin :show="loading">
      <NGrid :x-gap="16" :y-gap="16" :cols="appStore.isMobile ? 1 : 2">
        <!-- 画像 -->
        <NGridItem>
          <NCard :title="$t('page.userPortrait.portrait')" size="small" class="h-full">
            <template #header-extra>
              <NSpace>
                <NButton size="small" :loading="extracting" @click="handleExtract">
                  {{ $t('page.userPortrait.extract') }}
                </NButton>
                <NButton size="small" type="primary" @click="openPortraitModal">
                  {{ $t('page.userPortrait.edit') }}
                </NButton>
              </NSpace>
            </template>

            <template v-if="portrait && (portrait.summary || Object.keys(portrait.dimensions || {}).length || portrait.tags.length)">
              <div class="mb-12px">
                <NTag size="small" :type="portrait.extraction_enabled ? 'success' : 'default'">
                  {{ portrait.extraction_enabled ? $t('page.userPortrait.extractionOn') : $t('page.userPortrait.extractionOff') }}
                </NTag>
                <NTag v-if="portrait.is_user_edited" size="small" class="ml-8px">
                  {{ $t('page.userPortrait.edited') }}
                </NTag>
              </div>
              <p v-if="portrait.summary" class="mb-12px whitespace-pre-wrap">{{ portrait.summary }}</p>
              <NDescriptions v-if="Object.keys(portrait.dimensions || {}).length" :column="1" label-placement="left" size="small">
                <NDescriptionsItem v-for="(value, key) in portrait.dimensions" :key="key" :label="dimensionLabelMap[String(key)] || String(key)">
                  {{ value }}
                </NDescriptionsItem>
              </NDescriptions>
              <div v-if="portrait.tags.length" class="mt-12px">
                <NTag v-for="tag in portrait.tags" :key="tag" size="small" class="mr-8px mb-8px">{{ tag }}</NTag>
              </div>
              <div class="mt-8px text-12px op-60">{{ $t('page.userPortrait.updatedAt') }}: {{ formatTime(portrait.updated_at) }}</div>
            </template>
            <NEmpty v-else :description="$t('page.userPortrait.noPortrait')" />
          </NCard>
        </NGridItem>

        <!-- 经历 -->
        <NGridItem>
          <NCard :title="$t('page.userPortrait.experiences')" size="small" class="h-full">
            <template #header-extra>
              <NButton size="small" type="primary" @click="openCreateExperience">
                {{ $t('page.userPortrait.addExperience') }}
              </NButton>
            </template>

            <NEmpty v-if="!experiences.length" :description="$t('page.userPortrait.noExperience')" />
            <NList v-else hoverable>
              <NListItem v-for="item in experiences" :key="item.id">
                <NThing>
                  <template #header>
                    <div class="flex flex-wrap items-center gap-8px">
                      <span class="font-medium">{{ item.title }}</span>
                      <NTag size="tiny" type="info">{{ categoryLabelMap[item.category] || item.category || $t('page.userPortrait.category.experience') }}</NTag>
                      <NTag v-if="item.status" size="tiny" :type="statusTagType(item.status)">
                        {{ statusLabelMap[item.status] || item.status }}
                      </NTag>
                      <NTag v-if="item.memory_level" size="tiny" :type="memoryLevelTagType(item.memory_level)">
                        {{ memoryLevelLabelMap[item.memory_level] || item.memory_level }}
                      </NTag>
                      <span v-if="item.time_range" class="text-12px op-60">{{ item.time_range }}</span>
                    </div>
                  </template>
                  <template #description>
                    <div class="whitespace-pre-wrap">{{ item.content }}</div>
                    <div v-if="item.evidence" class="mt-4px text-12px op-60">{{ $t('page.userPortrait.evidenceLabel') }}: {{ item.evidence }}</div>
                    <div class="mt-4px">
                      <NTag v-for="tag in item.tags" :key="tag" size="tiny" class="mr-4px">{{ tag }}</NTag>
                    </div>
                  </template>
                  <template #action>
                    <NSpace>
                      <span class="text-12px op-60">{{ formatTime(item.occurred_at) }}</span>
                      <NButton size="tiny" text @click="openEditExperience(item)">{{ $t('page.userPortrait.edit') }}</NButton>
                      <NButton size="tiny" text type="error" @click="handleDeleteExperience(item)">{{ $t('page.userPortrait.delete') }}</NButton>
                    </NSpace>
                  </template>
                </NThing>
              </NListItem>
            </NList>
          </NCard>
        </NGridItem>
      </NGrid>
    </NSpin>

    <!-- 画像编辑弹窗 -->
    <NModal
      v-model:show="portraitModalVisible"
      preset="card"
      :title="$t('page.userPortrait.editPortrait')"
      class="w-600px max-w-90vw"
    >
      <NForm label-placement="top">
        <NFormItem :label="$t('page.userPortrait.summary')">
          <NInput v-model:value="portraitForm.summary" type="textarea" :rows="3" :placeholder="$t('page.userPortrait.summaryPlaceholder')" />
        </NFormItem>
        <NFormItem :label="$t('page.userPortrait.tags')">
          <NSelect v-model:value="portraitForm.tags" multiple filterable tag :options="[]" :placeholder="$t('page.userPortrait.tagsPlaceholder')" />
        </NFormItem>
        <NFormItem :label="$t('page.userPortrait.dimensions')">
          <div class="w-full">
            <div v-for="(dim, index) in portraitForm.dimensions" :key="index" class="mb-8px flex items-center gap-8px">
              <NInput v-model:value="dim.key" :placeholder="$t('page.userPortrait.keyPlaceholder')" />
              <NInput v-model:value="dim.value" :placeholder="$t('page.userPortrait.valuePlaceholder')" />
              <NButton text type="error" @click="removeDimension(index)">{{ $t('page.userPortrait.delete') }}</NButton>
            </div>
            <NButton size="small" dashed @click="addDimension">{{ $t('page.userPortrait.addDimension') }}</NButton>
          </div>
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="portraitModalVisible = false">{{ $t('page.userPortrait.cancel') }}</NButton>
          <NButton type="primary" :loading="savingPortrait" @click="savePortrait">{{ $t('page.userPortrait.save') }}</NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- 经历编辑弹窗 -->
    <NModal
      v-model:show="experienceModalVisible"
      preset="card"
      :title="editingExperienceId === null ? $t('page.userPortrait.addExperience') : $t('page.userPortrait.editExperience')"
      class="w-600px max-w-90vw"
    >
      <NForm label-placement="top">
        <NFormItem :label="$t('page.userPortrait.categoryLabel')">
          <NSelect v-model:value="experienceForm.category" :options="categoryOptions" />
        </NFormItem>
        <NFormItem :label="$t('page.userPortrait.statusLabel')">
          <NSelect v-model:value="experienceForm.status" :options="statusOptions" />
        </NFormItem>
        <NFormItem :label="$t('page.userPortrait.memoryLevelLabel')">
          <NSelect v-model:value="experienceForm.memoryLevel" :options="memoryLevelOptions" />
        </NFormItem>
        <NFormItem :label="$t('page.userPortrait.titleField')">
          <NInput v-model:value="experienceForm.title" :placeholder="$t('page.userPortrait.titlePlaceholder')" />
        </NFormItem>
        <NFormItem :label="$t('page.userPortrait.content')">
          <NInput v-model:value="experienceForm.content" type="textarea" :rows="4" />
        </NFormItem>
        <NFormItem :label="$t('page.userPortrait.timeRange')">
          <NInput v-model:value="experienceForm.timeRange" :placeholder="$t('page.userPortrait.timeRangePlaceholder')" />
        </NFormItem>
        <NFormItem :label="$t('page.userPortrait.evidenceLabel')">
          <NInput v-model:value="experienceForm.evidence" type="textarea" :rows="2" />
        </NFormItem>
        <NFormItem :label="$t('page.userPortrait.tags')">
          <NSelect v-model:value="experienceForm.tags" multiple filterable tag :options="[]" />
        </NFormItem>
        <NFormItem :label="$t('page.userPortrait.occurredAt')">
          <NDatePicker v-model:value="experienceForm.occurredAt" type="datetime" clearable class="w-full" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="experienceModalVisible = false">{{ $t('page.userPortrait.cancel') }}</NButton>
          <NButton type="primary" :loading="savingExperience" @click="saveExperience">{{ $t('page.userPortrait.save') }}</NButton>
        </NSpace>
      </template>
    </NModal>
  </div>
</template>