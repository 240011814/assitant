<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import {
  NButton,
  NDatePicker,
  NEmpty,
  NForm,
  NFormItem,
  NInput,
  NModal,
  NPopconfirm,
  NRadioButton,
  NRadioGroup,
  NSelect,
  NSwitch,
  NTag,
  NTooltip,
  useMessage
} from 'naive-ui';
import {
  fetchAgentTasks,
  fetchCreateAgentTask,
  fetchDeleteAgentTask,
  fetchRunAgentTask,
  fetchUpdateAgentTask,
  type AgentTaskItem
} from '@/service/api';
import { fetchAIAgentList, fetchOrchestrationChatList } from '@/service/api';
import { $t } from '@/locales';

defineOptions({ name: 'AgentTaskPanel' });

const message = useMessage();
const loading = ref(false);
const tasks = ref<AgentTaskItem[]>([]);

// 可选执行体: 对话 Agent + 已启用编排 (同训练页的合并口径)
const chatAgents = ref<{ id: number; title: string }[]>([]);
const orchestrations = ref<{ id: number; title: string }[]>([]);
const agentOptions = computed(() => [
  ...chatAgents.value.map(a => ({ label: a.title, value: `chat:${a.id}` })),
  ...orchestrations.value.map(o => ({
    label: $t('page.tool.calendar.agentTask.orchestrationLabel', { title: o.title }),
    value: `orchestration:${o.id}`
  }))
]);

const cronPresets = [
  { label: $t('page.tool.calendar.agentTask.cronPresetDaily'), value: '0 9 * * *' },
  { label: $t('page.tool.calendar.agentTask.cronPresetWeekday'), value: '0 9 * * 1-5' },
  { label: $t('page.tool.calendar.agentTask.cronPresetMonday'), value: '0 9 * * 1' },
  { label: $t('page.tool.calendar.agentTask.cronPresetMonthly'), value: '0 9 1 * *' },
  { label: $t('page.tool.calendar.agentTask.cronPresetCustom'), value: 'custom' }
];

const form = ref({
  agentValue: '',
  input: '',
  scheduleType: 'once' as 'once' | 'cron',
  runAt: null as number | null,
  cronPreset: '0 9 * * *',
  cronExpr: '0 9 * * *',
  notifyEmail: false,
  notifyTelegram: false,
  enabled: true
});

async function loadList() {
  loading.value = true;
  try {
    const { data, error } = await fetchAgentTasks();
    if (error || !data) {
      message.error(error?.message || $t('page.tool.calendar.agentTask.loadFailed'));
      return;
    }
    tasks.value = data.items;
  } finally {
    loading.value = false;
  }
}

async function loadAgents() {
  const [agentsRes, orchRes] = await Promise.allSettled([fetchAIAgentList(), fetchOrchestrationChatList()]);
  if (agentsRes.status === 'fulfilled' && agentsRes.value.data) {
    chatAgents.value = (agentsRes.value.data as any[])
      .filter((a: any) => (a.agent_type || 'chat') === 'chat')
      .map((a: any) => ({ id: a.id, title: a.title || `Agent #${a.id}` }));
  }
  if (orchRes.status === 'fulfilled' && orchRes.value.data) {
    orchestrations.value = (orchRes.value.data as any[]).map((o: any) => ({
      id: o.id,
      title: o.title || o.name || $t('page.tool.calendar.agentTask.orchestrationFallback', { id: o.id })
    }));
  }
}

// ============ 编辑弹窗 ============
const showModal = ref(false);
const saving = ref(false);
const editingId = ref<number | null>(null);

const isCustomCron = computed(() => form.value.cronPreset === 'custom');

function openCreate() {
  editingId.value = null;
  form.value = {
    agentValue: agentOptions.value[0]?.value || '',
    input: '',
    scheduleType: 'once',
    runAt: Date.now() + 60 * 60 * 1000,
    cronPreset: '0 9 * * *',
    cronExpr: '0 9 * * *',
    notifyEmail: false,
    notifyTelegram: false,
    enabled: true
  };
  showModal.value = true;
}

function openEdit(task: AgentTaskItem) {
  editingId.value = task.id;
  form.value = {
    agentValue: `${task.params_data.agent_type}:${task.params_data.agent_id}`,
    input: task.params_data.input,
    scheduleType: task.scheduleType,
    runAt: task.runAt ? new Date(task.runAt).getTime() : null,
    cronPreset: cronPresets.some(p => p.value === task.cronExpr) ? task.cronExpr : 'custom',
    cronExpr: task.cronExpr || '0 9 * * *',
    notifyEmail: task.params_data.notify_email,
    notifyTelegram: task.params_data.notify_telegram,
    enabled: task.enabled
  };
  showModal.value = true;
}

function presetChange(v: string) {
  if (v !== 'custom') {
    form.value.cronExpr = v;
  }
}

function validate(): string | null {
  if (!form.value.agentValue) return $t('page.tool.calendar.agentTask.agentRequired');
  if (!form.value.input.trim()) return $t('page.tool.calendar.agentTask.inputRequired');
  if (form.value.scheduleType === 'once') {
    if (!form.value.runAt) return $t('page.tool.calendar.agentTask.runAtRequired');
    if (form.value.runAt <= Date.now()) return $t('page.tool.calendar.agentTask.runAtInvalid');
  } else if (!form.value.cronExpr.trim()) {
    return $t('page.tool.calendar.agentTask.cronRequired');
  }
  return null;
}

async function handleSave() {
  const err = validate();
  if (err) {
    message.warning(err);
    return;
  }
  saving.value = true;
  try {
    const [agentType, agentIdStr] = form.value.agentValue.split(':');
    const payload: any = {
      agent_id: Number(agentIdStr),
      agent_type: agentType,
      input: form.value.input.trim(),
      schedule_type: form.value.scheduleType,
      notify_email: form.value.notifyEmail,
      notify_telegram: form.value.notifyTelegram,
      enabled: form.value.enabled
    };
    if (form.value.scheduleType === 'once') {
      payload.run_at = new Date(form.value.runAt as number).toISOString();
    } else {
      payload.cron_expr = form.value.cronExpr.trim();
    }
    const { error } =
      editingId.value === null ? await fetchCreateAgentTask(payload) : await fetchUpdateAgentTask(editingId.value, payload);
    if (error) {
      message.error(error?.message || $t('page.tool.calendar.agentTask.saveFailed'));
      return;
    }
    message.success(editingId.value === null ? $t('page.tool.calendar.agentTask.created') : $t('page.tool.calendar.agentTask.updated'));
    showModal.value = false;
    await loadList();
  } finally {
    saving.value = false;
  }
}

async function handleRun(task: AgentTaskItem) {
  const { error } = await fetchRunAgentTask(task.id);
  if (error) {
    message.error(error?.message || $t('page.tool.calendar.agentTask.runFailed'));
    return;
  }
  message.success($t('page.tool.calendar.agentTask.runTriggered'));
}

async function handleToggleEnabled(task: AgentTaskItem) {
  const { error } = await fetchUpdateAgentTask(task.id, { enabled: !task.enabled });
  if (error) {
    message.error(error?.message || $t('page.tool.calendar.agentTask.operationFailed'));
    return;
  }
  message.success(task.enabled ? $t('page.tool.calendar.agentTask.disabledMsg') : $t('page.tool.calendar.agentTask.enabledMsg'));
  await loadList();
}

async function handleDelete(task: AgentTaskItem) {
  const { error } = await fetchDeleteAgentTask(task.id);
  if (error) {
    message.error(error?.message || $t('page.tool.calendar.agentTask.deleteFailed'));
    return;
  }
  message.success($t('page.tool.calendar.agentTask.deleted'));
  await loadList();
}

function agentTitle(task: AgentTaskItem): string {
  const { agent_type, agent_id } = task.params_data;
  const pool = agent_type === 'orchestration' ? orchestrations.value : chatAgents.value;
  const found = pool.find(a => a.id === agent_id);
  if (found) return found.title;
  return $t(
    agent_type === 'orchestration'
      ? 'page.tool.calendar.agentTask.deletedOrchestration'
      : 'page.tool.calendar.agentTask.deletedAgent',
    { id: agent_id }
  );
}

function scheduleLabel(task: AgentTaskItem): string {
  if (task.scheduleType === 'once') {
    return task.runAt ? new Date(task.runAt).toLocaleString('zh-CN') : $t('page.tool.calendar.agentTask.once');
  }
  return $t('page.tool.calendar.agentTask.scheduleCron', { expr: task.cronExpr });
}

function formatTime(dateStr: string | null): string {
  if (!dateStr) return '-';
  return new Date(dateStr).toLocaleString('zh-CN');
}

onMounted(() => {
  loadList();
  loadAgents();
});
</script>

<template>
  <div>
    <div class="flex items-center justify-between mb-3">
      <div class="text-13px text-gray-400">
        {{ $t('page.tool.calendar.agentTask.desc') }}
      </div>
      <NButton type="primary" size="small" @click="openCreate">
        <template #icon><SvgIcon icon="mdi:plus" /></template>
        {{ $t('page.tool.calendar.agentTask.create') }}
      </NButton>
    </div>

    <div class="min-h-48">
      <div
        v-for="task in tasks"
        :key="task.id"
        class="flex items-center gap-3 p-2 rounded border border-transparent hover:bg-gray-100 dark:hover:bg-gray-800 mb-1"
      >
        <SvgIcon icon="mdi:robot-outline" class="text-20px text-violet-500 shrink-0" />
        <div class="flex-1 min-w-0">
          <div class="text-sm font-medium truncate">
            {{ task.params_data.input }}
            <NTag size="tiny" :bordered="false" class="ml-1" :type="task.params_data.agent_type === 'orchestration' ? 'info' : 'default'">
              {{ agentTitle(task) }}
            </NTag>
            <NTag v-if="!task.enabled" size="tiny" :bordered="false" type="warning" class="ml-1">{{ $t('page.tool.calendar.agentTask.disabledTag') }}</NTag>
          </div>
          <div class="text-xs text-gray-400 flex items-center gap-2 mt-0.5 flex-wrap">
            <span>{{ scheduleLabel(task) }}</span>
            <span>{{ $t('page.tool.calendar.agentTask.nextRun', { time: task.enabled ? formatTime(task.next_run_at) : '-' }) }}</span>
            <NTag v-if="task.params_data.notify_email" size="tiny" :bordered="false">{{ $t('page.tool.calendar.agentTask.email') }}</NTag>
            <NTag v-if="task.params_data.notify_telegram" size="tiny" :bordered="false">TG</NTag>
          </div>
        </div>
        <div class="flex items-center gap-1 shrink-0">
          <NSwitch size="small" :value="task.enabled" @update:value="handleToggleEnabled(task)" />
          <NTooltip>
            <template #trigger>
              <NButton size="tiny" quaternary type="primary" @click="handleRun(task)">{{ $t('page.tool.calendar.agentTask.runNow') }}</NButton>
            </template>
            {{ $t('page.tool.calendar.agentTask.runNowTip') }}
          </NTooltip>
          <NButton size="tiny" quaternary @click="openEdit(task)">{{ $t('common.edit') }}</NButton>
          <NPopconfirm @positive-click="handleDelete(task)">
            <template #trigger>
              <NButton size="tiny" quaternary type="error">{{ $t('common.delete') }}</NButton>
            </template>
            {{ $t('page.tool.calendar.agentTask.deleteConfirm') }}
          </NPopconfirm>
        </div>
      </div>
      <NEmpty v-if="tasks.length === 0 && !loading" :description="$t('page.tool.calendar.agentTask.empty')" class="py-10" />
    </div>

    <NModal
      v-model:show="showModal"
      preset="card"
      :title="editingId === null ? $t('page.tool.calendar.agentTask.createModal') : $t('page.tool.calendar.agentTask.editModal')"
      style="width: 560px"
    >
      <NForm label-placement="left" label-width="100">
        <NFormItem :label="$t('page.tool.calendar.agentTask.executor')" required>
          <NSelect
            v-model:value="form.agentValue"
            :options="agentOptions"
            :placeholder="$t('page.tool.calendar.agentTask.executorPlaceholder')"
            filterable
          />
        </NFormItem>
        <NFormItem :label="$t('page.tool.calendar.agentTask.taskContent')" required>
          <NInput
            v-model:value="form.input"
            type="textarea"
            :rows="3"
            :placeholder="$t('page.tool.calendar.agentTask.taskContentPlaceholder')"
          />
        </NFormItem>
        <NFormItem :label="$t('page.tool.calendar.agentTask.scheduleType')" required>
          <NRadioGroup v-model:value="form.scheduleType">
            <NRadioButton value="once">{{ $t('page.tool.calendar.agentTask.once') }}</NRadioButton>
            <NRadioButton value="cron">{{ $t('page.tool.calendar.agentTask.cronRadio') }}</NRadioButton>
          </NRadioGroup>
        </NFormItem>
        <NFormItem v-if="form.scheduleType === 'once'" :label="$t('page.tool.calendar.agentTask.runAt')" required>
          <NDatePicker v-model:value="form.runAt" type="datetime" style="width: 100%" />
        </NFormItem>
        <template v-else>
          <NFormItem :label="$t('page.tool.calendar.agentTask.cronPreset')">
            <NSelect v-model:value="form.cronPreset" :options="cronPresets" @update:value="presetChange" />
          </NFormItem>
          <NFormItem v-if="isCustomCron" :label="$t('page.tool.calendar.agentTask.cronExpr')" required>
            <NInput v-model:value="form.cronExpr" :placeholder="$t('page.tool.calendar.agentTask.cronExprPlaceholder')" />
          </NFormItem>
        </template>
        <NFormItem :label="$t('page.tool.calendar.agentTask.resultNotify')">
          <div class="flex items-center gap-4">
            <div class="flex items-center gap-1">
              <NSwitch v-model:value="form.notifyEmail" size="small" />
              <span class="text-13px">{{ $t('page.tool.calendar.agentTask.email') }}</span>
            </div>
            <div class="flex items-center gap-1">
              <NSwitch v-model:value="form.notifyTelegram" size="small" />
              <span class="text-13px">Telegram</span>
            </div>
          </div>
        </NFormItem>
        <NFormItem :label="$t('page.tool.calendar.agentTask.enabled')">
          <NSwitch v-model:value="form.enabled" />
        </NFormItem>
      </NForm>
      <template #footer>
        <div class="flex justify-end gap-2">
          <NButton @click="showModal = false">{{ $t('common.cancel') }}</NButton>
          <NButton type="primary" :loading="saving" @click="handleSave">{{ $t('common.save') }}</NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>
