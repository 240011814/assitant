<script setup lang="ts">
import { ref, computed, onMounted, h } from "vue";
import { NButton, NSwitch, NSpace, NTag, NPopconfirm, useMessage } from "naive-ui";
import { useAppStore } from "@/store/modules/app";
import { useAuth } from "@/hooks/business/auth";
import {
  fetchJobs,
  fetchJobTasks,
  createJob,
  updateJob,
  deleteJob,
  runJob,
  fetchJobRuns,
} from "@/service/api";

defineOptions({ name: "SystemJob" });

const message = useMessage();
const { hasAuth } = useAuth();
const appStore = useAppStore();
const isMobile = computed(() => appStore.isMobile);
const canEdit = computed(() => hasAuth("job:edit"));

const loading = ref(false);
const jobs = ref<Api.Job.JobDefinition[]>([]);
const tasks = ref<Api.Job.TaskMeta[]>([]);

const showEditModal = ref(false);
const saving = ref(false);
const editingId = ref<number | null>(null);

const showRunsModal = ref(false);
const runsLoading = ref(false);
const runsJobName = ref("");
const runs = ref<Api.Job.JobRun[]>([]);

const form = ref({
  name: "",
  taskName: "",
  cronExpr: "",
  params: "",
  enabled: false,
  maxRetries: 0,
  notifyEmail: "",
  remark: "",
});

const cronPresets = [
  { label: "每天 8:00", value: "0 8 * * *" },
  { label: "每天 17:30", value: "30 17 * * *" },
  { label: "工作日 9:00", value: "0 9 * * 1-5" },
  { label: "每小时", value: "0 * * * *" },
  { label: "每30分钟", value: "*/30 * * * *" },
];

const taskOptions = computed(() =>
  tasks.value.map((t) => ({ label: `${t.description} (${t.name})`, value: t.name }))
);

const selectedTaskExample = computed(() => {
  const task = tasks.value.find((t) => t.name === form.value.taskName);
  if (!task || !task.paramsExample) return "";
  try {
    return JSON.stringify(JSON.parse(task.paramsExample), null, 2);
  } catch {
    return task.paramsExample;
  }
});

const statusTag = {
  success: { label: "成功", type: "success" as const },
  failed: { label: "失败", type: "error" as const },
  running: { label: "运行中", type: "info" as const },
  skipped: { label: "跳过", type: "warning" as const },
};

const columns = [
  { title: "任务名称", key: "name", width: 160 },
  { title: "任务方法", key: "taskName", width: 200 },
  { title: "Cron 表达式", key: "cronExpr", width: 130 },
  {
    title: "状态",
    key: "enabled",
    width: 80,
    render: (row: Api.Job.JobDefinition) =>
      h(NSwitch, {
        value: row.enabled,
        size: "small",
        disabled: !canEdit.value,
        onUpdateValue: (val: boolean) => handleToggle(row, val),
      }),
  },
  {
    title: "下次执行",
    key: "nextRunAt",
    width: 160,
    render: (row: Api.Job.JobDefinition) =>
      formatTime(row.enabled ? row.nextRunAt : null),
  },
  { title: "备注", key: "remark", ellipsis: { tooltip: true } },
  {
    title: "操作",
    key: "actions",
    width: 300,
    fixed: "right" as const,
    render: (row: Api.Job.JobDefinition) =>
      h(
        NSpace,
        { size: "small" },
        {
          default: () =>
            [
              canEdit.value
                ? h(
                    NButton,
                    {
                      size: "small",
                      type: "primary",
                      ghost: true,
                      onClick: () => handleRun(row),
                    },
                    { default: () => "立即执行" }
                  )
                : null,
              canEdit.value
                ? h(
                    NButton,
                    { size: "small", onClick: () => openEdit(row) },
                    { default: () => "编辑" }
                  )
                : null,
              h(
                NButton,
                { size: "small", type: "info", ghost: true, onClick: () => openRuns(row) },
                { default: () => "历史" }
              ),
              canEdit.value
                ? h(
                    NPopconfirm,
                    { onPositiveClick: () => handleDelete(row) },
                    {
                      trigger: () =>
                        h(
                          NButton,
                          { size: "small", type: "error", ghost: true },
                          { default: () => "删除" }
                        ),
                      default: () => `确定删除任务「${row.name}」？`,
                    }
                  )
                : null,
            ].filter(Boolean),
        }
      ),
  },
];

const runColumns = [
  {
    title: "开始时间",
    key: "startedAt",
    width: 170,
    render: (row: Api.Job.JobRun) => formatTime(row.startedAt),
  },
  {
    title: "结束时间",
    key: "finishedAt",
    width: 170,
    render: (row: Api.Job.JobRun) => formatTime(row.finishedAt),
  },
  {
    title: "状态",
    key: "status",
    width: 90,
    render: (row: Api.Job.JobRun) => {
      const info = statusTag[row.status] || {
        label: row.status,
        type: "default" as const,
      };
      return h(NTag, { size: "small", type: info.type }, { default: () => info.label });
    },
  },
  {
    title: "触发",
    key: "triggerType",
    width: 90,
    render: (row: Api.Job.JobRun) => (row.triggerType === "manual" ? "手动" : "调度"),
  },
  { title: "尝试", key: "attempt", width: 60 },
  { title: "错误信息", key: "error", ellipsis: { tooltip: true } },
];

function formatTime(t: string | null) {
  if (!t) return "-";
  return new Date(t).toLocaleString("zh-CN", { hour12: false });
}

function jobRowKey(row: Api.Job.JobDefinition) {
  return row.id;
}

function runRowKey(row: Api.Job.JobRun) {
  return row.id;
}

async function loadJobs() {
  loading.value = true;
  try {
    const { data, error } = await fetchJobs();
    if (!error && data) {
      jobs.value = data;
    }
  } finally {
    loading.value = false;
  }
}

async function loadTasks() {
  const { data } = await fetchJobTasks();
  if (data) {
    tasks.value = data;
  }
}

function openCreate() {
  editingId.value = null;
  form.value = {
    name: "",
    taskName: "",
    cronExpr: "",
    params: "",
    enabled: false,
    maxRetries: 0,
    notifyEmail: "",
    remark: "",
  };
  showEditModal.value = true;
}

function openEdit(row: Api.Job.JobDefinition) {
  editingId.value = row.id;
  form.value = {
    name: row.name,
    taskName: row.taskName,
    cronExpr: row.cronExpr,
    params: row.params || "",
    enabled: row.enabled,
    maxRetries: row.maxRetries,
    notifyEmail: row.notifyEmail || "",
    remark: row.remark || "",
  };
  showEditModal.value = true;
}

async function handleSave() {
  if (!form.value.name || !form.value.taskName || !form.value.cronExpr) {
    message.warning("请填写任务名称、任务方法和 Cron 表达式");
    return;
  }
  if (form.value.params) {
    try {
      JSON.parse(form.value.params);
    } catch {
      message.warning("参数必须是合法 JSON");
      return;
    }
  }
  if (form.value.notifyEmail && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(form.value.notifyEmail.trim())) {
    message.warning("失败通知邮箱格式不正确");
    return;
  }

  saving.value = true;
  try {
    const payload = {
      name: form.value.name,
      taskName: form.value.taskName,
      cronExpr: form.value.cronExpr,
      params: form.value.params || null,
      enabled: form.value.enabled,
      maxRetries: form.value.maxRetries,
      notifyEmail: form.value.notifyEmail.trim(),
      remark: form.value.remark,
    };
    const { error } =
      editingId.value === null
        ? await createJob(payload)
        : await updateJob(editingId.value, payload);
    if (!error) {
      message.success(editingId.value === null ? "创建成功" : "更新成功");
      showEditModal.value = false;
      loadJobs();
    }
  } finally {
    saving.value = false;
  }
}

async function handleToggle(row: Api.Job.JobDefinition, val: boolean) {
  const { error } = await updateJob(row.id, { enabled: val });
  if (!error) {
    message.success(val ? "任务已启用" : "任务已停用");
    loadJobs();
  }
}

async function handleRun(row: Api.Job.JobDefinition) {
  const { error } = await runJob(row.id);
  if (!error) {
    message.success(`任务「${row.name}」已触发，可稍后查看执行历史`);
    setTimeout(loadJobs, 2000);
  }
}

async function handleDelete(row: Api.Job.JobDefinition) {
  const { error } = await deleteJob(row.id);
  if (!error) {
    message.success("删除成功");
    loadJobs();
  }
}

function openRuns(row: Api.Job.JobDefinition) {
  runsJobName.value = row.name;
  showRunsModal.value = true;
  loadRuns(row.id);
}

async function loadRuns(id: number) {
  runsLoading.value = true;
  try {
    const { data } = await fetchJobRuns(id);
    if (data) {
      runs.value = data;
    }
  } finally {
    runsLoading.value = false;
  }
}

onMounted(async () => {
  await Promise.all([loadJobs(), loadTasks()]);
});
</script>

<template>
  <div class="h-full flex-col flex gap-4 overflow-auto" :class="isMobile ? 'p-2' : 'p-6'">
    <NCard :bordered="false" :shadow="isMobile ? false : 'sm'" title="定时任务管理">
      <template #header-extra>
        <NButton v-if="canEdit" type="primary" @click="openCreate">新增任务</NButton>
      </template>
      <!-- PC: 表格 -->
      <NDataTable
        v-if="!isMobile"
        :columns="columns"
        :data="jobs"
        :loading="loading"
        :row-key="jobRowKey"
        :scroll-x="1140"
        size="small"
        striped
      />
      <!-- Mobile: 卡片列表 -->
      <template v-else>
        <NSpin v-if="loading" class="flex justify-center py-8" />
        <NEmpty v-else-if="!jobs.length" description="暂无定时任务" class="py-8" />
        <div v-else class="flex flex-col gap-3">
          <NCard v-for="row in jobs" :key="row.id" size="small" :bordered="true">
            <div class="flex flex-col gap-2">
              <div class="flex items-center justify-between">
                <span class="font-bold text-16px">{{ row.name }}</span>
                <NSwitch
                  :value="row.enabled"
                  size="small"
                  :disabled="!canEdit"
                  @update:value="(val: boolean) => handleToggle(row, val)"
                />
              </div>
              <div class="flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
                <span>任务方法: {{ row.taskName }}</span>
                <span>cron: {{ row.cronExpr }}</span>
              </div>
              <div class="flex items-center justify-between text-xs text-gray-400">
                <span>下次执行: {{ formatTime(row.enabled ? row.nextRunAt : null) }}</span>
                <span v-if="row.remark" class="truncate max-w-40" :title="row.remark">{{ row.remark }}</span>
              </div>
              <div class="flex flex-wrap justify-end gap-1 pt-1">
                <NButton v-if="canEdit" size="tiny" type="primary" ghost @click="handleRun(row)">立即执行</NButton>
                <NButton v-if="canEdit" size="tiny" @click="openEdit(row)">编辑</NButton>
                <NButton size="tiny" type="info" ghost @click="openRuns(row)">历史</NButton>
                <NPopconfirm v-if="canEdit" @positive-click="handleDelete(row)">
                  <template #trigger>
                    <NButton size="tiny" type="error" ghost>删除</NButton>
                  </template>
                  确定删除任务「{{ row.name }}」？
                </NPopconfirm>
              </div>
            </div>
          </NCard>
        </div>
      </template>
    </NCard>

    <!-- 新增/编辑任务 -->
    <NModal
      v-model:show="showEditModal"
      preset="card"
      :title="editingId === null ? '新增定时任务' : '编辑定时任务'"
      :style="{ width: isMobile ? '95vw' : '640px' }"
    >
      <NForm label-placement="left" :label-width="isMobile ? '80' : '100'">
        <NFormItem label="任务名称" required>
          <NInput v-model:value="form.name" placeholder="例如: 行情数据每日同步" />
        </NFormItem>
        <NFormItem label="任务方法" required>
          <NSelect
            v-model:value="form.taskName"
            :options="taskOptions"
            placeholder="选择已注册的任务方法"
            filterable
          />
        </NFormItem>
        <NFormItem label="Cron 表达式" required>
          <NInput
            v-model:value="form.cronExpr"
            placeholder="分 时 日 月 周, 例如: 0 8 * * 1-5"
          />
        </NFormItem>
        <NFormItem label="常用模板">
          <NSelect
            :options="cronPresets"
            placeholder="选择常用周期"
            @update:value="(val: string) => (form.cronExpr = val)"
          />
        </NFormItem>
        <NFormItem label="任务参数">
          <NInput
            v-model:value="form.params"
            type="textarea"
            :rows="3"
            :placeholder="selectedTaskExample || 'JSON 格式参数(可选)'"
          />
        </NFormItem>
        <NFormItem label="失败重试">
          <NInputNumber v-model:value="form.maxRetries" :min="0" :max="10" class="w-40">
            <template #suffix>次</template>
          </NInputNumber>
        </NFormItem>
        <NFormItem label="失败通知邮箱">
          <NInput
            v-model:value="form.notifyEmail"
            placeholder="任务失败(重试耗尽)时告警邮箱, 留空不发送"
          />
        </NFormItem>
        <NFormItem label="备注">
          <NInput v-model:value="form.remark" placeholder="任务说明(可选)" />
        </NFormItem>
        <NFormItem label="启用">
          <NSwitch v-model:value="form.enabled" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showEditModal = false">取消</NButton>
          <NButton type="primary" :loading="saving" @click="handleSave">保存</NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- 执行历史 -->
    <NModal
      v-model:show="showRunsModal"
      preset="card"
      :title="`执行历史 - ${runsJobName}`"
      :style="{ width: isMobile ? '95vw' : '900px' }"
    >
      <NDataTable
        :columns="runColumns"
        :data="runs"
        :loading="runsLoading"
        :row-key="runRowKey"
        :scroll-x="700"
        size="small"
        striped
        :max-height="isMobile ? 320 : 420"
      />
    </NModal>
  </div>
</template>
