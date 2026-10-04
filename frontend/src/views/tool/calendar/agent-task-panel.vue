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

defineOptions({ name: 'AgentTaskPanel' });

const message = useMessage();
const loading = ref(false);
const tasks = ref<AgentTaskItem[]>([]);

// 可选执行体: 对话 Agent + 已启用编排 (同训练页的合并口径)
const chatAgents = ref<{ id: number; title: string }[]>([]);
const orchestrations = ref<{ id: number; title: string }[]>([]);
const agentOptions = computed(() => [
  ...chatAgents.value.map(a => ({ label: a.title, value: `chat:${a.id}` })),
  ...orchestrations.value.map(o => ({ label: `${o.title}（编排）`, value: `orchestration:${o.id}` }))
]);

const cronPresets = [
  { label: '每天 09:00', value: '0 9 * * *' },
  { label: '工作日 09:00', value: '0 9 * * 1-5' },
  { label: '每周一 09:00', value: '0 9 * * 1' },
  { label: '每月 1 日 09:00', value: '0 9 1 * *' },
  { label: '自定义…', value: 'custom' }
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
      message.error(error?.message || '获取任务列表失败');
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
    orchestrations.value = (orchRes.value.data as any[]).map((o: any) => ({ id: o.id, title: o.title || o.name || `编排 #${o.id}` }));
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
  if (!form.value.agentValue) return '请选择要执行的 Agent';
  if (!form.value.input.trim()) return '请输入任务内容';
  if (form.value.scheduleType === 'once') {
    if (!form.value.runAt) return '请选择执行时间';
    if (form.value.runAt <= Date.now()) return '执行时间必须晚于当前时间';
  } else if (!form.value.cronExpr.trim()) {
    return '请填写 cron 表达式';
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
      message.error(error?.message || '保存失败');
      return;
    }
    message.success(editingId.value === null ? '任务已创建' : '任务已更新');
    showModal.value = false;
    await loadList();
  } finally {
    saving.value = false;
  }
}

async function handleRun(task: AgentTaskItem) {
  const { error } = await fetchRunAgentTask(task.id);
  if (error) {
    message.error(error?.message || '触发失败');
    return;
  }
  message.success('已触发执行, 结果将写入训练历史');
}

async function handleToggleEnabled(task: AgentTaskItem) {
  const { error } = await fetchUpdateAgentTask(task.id, { enabled: !task.enabled });
  if (error) {
    message.error(error?.message || '操作失败');
    return;
  }
  message.success(task.enabled ? '任务已停用' : '任务已启用');
  await loadList();
}

async function handleDelete(task: AgentTaskItem) {
  const { error } = await fetchDeleteAgentTask(task.id);
  if (error) {
    message.error(error?.message || '删除失败');
    return;
  }
  message.success('任务已删除');
  await loadList();
}

function agentTitle(task: AgentTaskItem): string {
  const { agent_type, agent_id } = task.params_data;
  const pool = agent_type === 'orchestration' ? orchestrations.value : chatAgents.value;
  const found = pool.find(a => a.id === agent_id);
  if (found) return found.title;
  return `${agent_type === 'orchestration' ? '编排' : 'Agent'} #${agent_id} (已删除)`;
}

function scheduleLabel(task: AgentTaskItem): string {
  if (task.scheduleType === 'once') {
    return task.runAt ? new Date(task.runAt).toLocaleString('zh-CN') : '单次';
  }
  return `cron: ${task.cronExpr}`;
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
        到点让选定的 Agent / 编排带着输入文本完整执行（含工具调用），结果写入训练历史；可推送邮件 / Telegram。
      </div>
      <NButton type="primary" size="small" @click="openCreate">
        <template #icon><SvgIcon icon="mdi:plus" /></template>
        新建任务
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
            <NTag v-if="!task.enabled" size="tiny" :bordered="false" type="warning" class="ml-1">已停用</NTag>
          </div>
          <div class="text-xs text-gray-400 flex items-center gap-2 mt-0.5 flex-wrap">
            <span>{{ scheduleLabel(task) }}</span>
            <span>下次: {{ task.enabled ? formatTime(task.next_run_at) : '-' }}</span>
            <NTag v-if="task.params_data.notify_email" size="tiny" :bordered="false">邮件</NTag>
            <NTag v-if="task.params_data.notify_telegram" size="tiny" :bordered="false">TG</NTag>
          </div>
        </div>
        <div class="flex items-center gap-1 shrink-0">
          <NSwitch size="small" :value="task.enabled" @update:value="handleToggleEnabled(task)" />
          <NTooltip>
            <template #trigger>
              <NButton size="tiny" quaternary type="primary" @click="handleRun(task)">立即运行</NButton>
            </template>
            手动执行一次, 不影响调度
          </NTooltip>
          <NButton size="tiny" quaternary @click="openEdit(task)">编辑</NButton>
          <NPopconfirm @positive-click="handleDelete(task)">
            <template #trigger>
              <NButton size="tiny" quaternary type="error">删除</NButton>
            </template>
            确认删除该任务？
          </NPopconfirm>
        </div>
      </div>
      <NEmpty v-if="tasks.length === 0 && !loading" description="暂无定时 Agent 任务" class="py-10" />
    </div>

    <NModal v-model:show="showModal" preset="card" :title="editingId === null ? '新建 Agent 任务' : '编辑 Agent 任务'" style="width: 560px">
      <NForm label-placement="left" label-width="100">
        <NFormItem label="执行者" required>
          <NSelect v-model:value="form.agentValue" :options="agentOptions" placeholder="选择 Agent 或编排" filterable />
        </NFormItem>
        <NFormItem label="任务内容" required>
          <NInput
            v-model:value="form.input"
            type="textarea"
            :rows="3"
            placeholder="到点发给 Agent 的指令, 如: 搜索今天 AI 行业的新闻, 总结成 5 条要点"
          />
        </NFormItem>
        <NFormItem label="执行方式" required>
          <NRadioGroup v-model:value="form.scheduleType">
            <NRadioButton value="once">单次</NRadioButton>
            <NRadioButton value="cron">周期 (cron)</NRadioButton>
          </NRadioGroup>
        </NFormItem>
        <NFormItem v-if="form.scheduleType === 'once'" label="执行时间" required>
          <NDatePicker v-model:value="form.runAt" type="datetime" style="width: 100%" />
        </NFormItem>
        <template v-else>
          <NFormItem label="快捷预设">
            <NSelect v-model:value="form.cronPreset" :options="cronPresets" @update:value="presetChange" />
          </NFormItem>
          <NFormItem v-if="isCustomCron" label="cron 表达式" required>
            <NInput v-model:value="form.cronExpr" placeholder="分 时 日 月 周, 如 30 8 * * 1-5" />
          </NFormItem>
        </template>
        <NFormItem label="结果通知">
          <div class="flex items-center gap-4">
            <div class="flex items-center gap-1">
              <NSwitch v-model:value="form.notifyEmail" size="small" />
              <span class="text-13px">邮件</span>
            </div>
            <div class="flex items-center gap-1">
              <NSwitch v-model:value="form.notifyTelegram" size="small" />
              <span class="text-13px">Telegram</span>
            </div>
          </div>
        </NFormItem>
        <NFormItem label="启用">
          <NSwitch v-model:value="form.enabled" />
        </NFormItem>
      </NForm>
      <template #footer>
        <div class="flex justify-end gap-2">
          <NButton @click="showModal = false">取消</NButton>
          <NButton type="primary" :loading="saving" @click="handleSave">保存</NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>
