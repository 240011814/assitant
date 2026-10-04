<script setup lang="ts">
import { ref, computed, watch, onMounted, h } from 'vue';
import { useMessage, useDialog, NButton, NSpace, NTabPane, NTabs } from 'naive-ui';
import type { FormInst } from 'naive-ui';
import {
  fetchGetReminders,
  fetchCreateReminder,
  fetchUpdateReminder,
  fetchDeleteReminder,
} from '@/service/api';
import type { Reminder, CreateReminderParams, UpdateReminderParams } from '@/service/api';
import AgentTaskPanel from './agent-task-panel.vue';

defineOptions({ name: 'ToolCalendar' });

const message = useMessage();
const dialog = useDialog();

const loading = ref(false);
const reminders = ref<Reminder[]>([]);
const selectedDate = ref(new Date());
const showModal = ref(false);
const editingId = ref<number | null>(null);
// 编辑的备忘原本是否设置了重复结束时间 (用于提交时区分"清除"与"保持为空")
const editingHadRepeatEndAt = ref(false);
const formRef = ref<FormInst | null>(null);

const currentDate = ref(new Date());
const currentYear = computed(() => currentDate.value.getFullYear());
const currentMonth = computed(() => currentDate.value.getMonth() + 1);

const form = ref({
  title: '',
  content: '',
  remindAt: null as number | null,
  advanceMinutes: 3,
  repeatType: 'none',
  repeatInterval: 1,
  repeatEndAt: null as number | null,
});

const repeatOptions = [
  { label: '不重复', value: 'none' },
  { label: '每天', value: 'daily' },
  { label: '每周', value: 'weekly' },
  { label: '每月', value: 'monthly' },
  { label: '每年', value: 'yearly' },
];

const rules = {
  title: { required: true, message: '请输入标题', trigger: ['blur', 'input'] },
};

watch(currentDate, () => {
  loadReminders();
});

async function loadReminders(year = currentYear.value, month = currentMonth.value) {
  loading.value = true;
  const { data, error } = await fetchGetReminders({ year, month });
  if (!error) {
    reminders.value = data || [];
  }
  loading.value = false;
}

function handlePanelChange(info: { year: number; month: number }) {
  loadReminders(info.year, info.month);
}

function handleDateChange(date: Date) {
  selectedDate.value = date;
}

const selectedDayReminders = computed(() => {
  const d = selectedDate.value;
  return reminders.value.filter((r) => {
    const rd = new Date(r.scheduledAt);
    return (
      rd.getFullYear() === d.getFullYear() &&
      rd.getMonth() === d.getMonth() &&
      rd.getDate() === d.getDate()
    );
  });
});

function getDefaultRemindAt() {
  const now = new Date();
  const d = selectedDate.value;
  const isToday =
    d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth() && d.getDate() === now.getDate();
  const base = isToday
    ? now
    : new Date(d.getFullYear(), d.getMonth(), d.getDate(), now.getHours(), now.getMinutes(), now.getSeconds());
  const t = new Date(base.getTime());
  t.setSeconds(0, 0);
  const remainder = t.getMinutes() % 30;
  if (remainder !== 0) {
    t.setMinutes(t.getMinutes() + (30 - remainder));
  }
  if (isToday && t.getTime() <= now.getTime()) {
    t.setMinutes(t.getMinutes() + 30);
  }
  return t.getTime();
}

function openCreateModal() {
  editingId.value = null;
  editingHadRepeatEndAt.value = false;
  form.value = {
    title: '',
    content: '',
    remindAt: getDefaultRemindAt(),
    advanceMinutes: 3,
    repeatType: 'none',
    repeatInterval: 1,
    repeatEndAt: null,
  };
  showModal.value = true;
}

function openEditModal(reminder: Reminder) {
  editingId.value = reminder.id;
  editingHadRepeatEndAt.value = !!reminder.repeatEndAt;
  form.value = {
    title: reminder.params.title,
    content: reminder.params.content,
    remindAt: new Date(reminder.scheduledAt).getTime(),
    advanceMinutes: reminder.advanceMinutes || 0,
    repeatType: reminder.repeatType,
    repeatInterval: reminder.repeatInterval,
    repeatEndAt: reminder.repeatEndAt ? new Date(reminder.repeatEndAt).getTime() : null,
  };
  showModal.value = true;
}

async function handleSubmit() {
  if (!formRef.value) return;
  try {
    await formRef.value.validate();
  } catch {
    return;
  }

  // 编辑时区分三种语义: 有值传 ISO; 原来有值现在清空传 repeatEndAtClear;
  // 原来就没有也保持空 (不带字段), 避免后端把已有的 repeat_end_at 清成 NULL
  const data: CreateReminderParams & UpdateReminderParams = {
    title: form.value.title,
    content: form.value.content,
    remindAt: new Date(form.value.remindAt as number).toISOString(),
    advanceMinutes: form.value.advanceMinutes,
    repeatType: form.value.repeatType,
    repeatInterval: form.value.repeatInterval,
  };
  if (form.value.repeatEndAt) {
    data.repeatEndAt = new Date(form.value.repeatEndAt as number).toISOString();
  } else if (editingId.value && editingHadRepeatEndAt.value) {
    data.repeatEndAt = null;
    data.repeatEndAtClear = true;
  }

  if (editingId.value) {
    const { error } = await fetchUpdateReminder(editingId.value, data);
    if (!error) {
      message.success('备忘已更新');
      showModal.value = false;
      loadReminders();
    }
  } else {
    const { error } = await fetchCreateReminder(data);
    if (!error) {
      message.success('备忘已创建');
      showModal.value = false;
      loadReminders();
    }
  }
}

async function doDelete(id: number, scope: 'this' | 'all') {
  const { error } = await fetchDeleteReminder(id, scope);
  if (!error) {
    message.success('删除成功');
    loadReminders();
  }
}

function handleDelete(reminder: Reminder) {
  if (reminder.repeatType === 'none') {
    dialog.warning({
      title: '确认删除',
      content: `确定要删除备忘「${reminder.params.title}」吗？`,
      positiveText: '删除',
      negativeText: '取消',
      onPositiveClick: () => doDelete(reminder.id, 'this')
    });
    return;
  }

  const d = dialog.warning({
    title: '确认删除',
    content: `「${reminder.params.title}」是重复提醒，请选择删除范围：`,
    action: () =>
      h(NSpace, { justify: 'end' }, {
        default: () => [
          h(
            NButton,
            { size: 'small', onClick: () => d.destroy() },
            { default: () => '取消' }
          ),
          h(
            NButton,
            {
              size: 'small',
              type: 'primary',
              onClick: () => {
                d.destroy();
                doDelete(reminder.id, 'this');
              }
            },
            { default: () => '仅删除此条' }
          ),
          h(
            NButton,
            {
              size: 'small',
              type: 'error',
              onClick: () => {
                d.destroy();
                doDelete(reminder.id, 'all');
              }
            },
            { default: () => '删除全部' }
          )
        ]
      })
  });
}

function formatTime(dateStr: string) {
  return new Date(dateStr).toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function getRepeatLabel(type: string) {
  return repeatOptions.find((o) => o.value === type)?.label || '不重复';
}

onMounted(() => {
  loadReminders();
});
</script>

<template>
  <div class="h-full p-4 overflow-auto">
    <NTabs type="line" animated>
      <!-- 日历备忘 (原有功能) -->
      <NTabPane name="reminder" tab="日历备忘">
        <NCard size="small">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-3">
              <div class="w-40px h-40px rounded-lg bg-primary/10 flex items-center justify-center">
                <SvgIcon icon="mdi:calendar-month" class="text-24px text-primary" />
              </div>
              <div>
                <div class="text-16px font-bold">日历备忘</div>
                <div class="text-13px text-gray-400">管理您的备忘提醒，支持重复提醒</div>
              </div>
            </div>
            <NButton type="primary" @click="openCreateModal">
              <template #icon>
                <SvgIcon icon="mdi:plus" />
              </template>
              新增备忘
            </NButton>
          </div>
        </NCard>

        <div class="flex gap-4">
      <NCard size="small" class="flex-[7]">
        <NCalendar
          :value="currentDate.getTime()"
          :is-date-disabled="() => false"
          @update:value="(v: number) => { currentDate = new Date(v); handleDateChange(new Date(v)); }"
          @panel-change="handlePanelChange"
        >
          <template #default="{ year, month, date }">
            <div class="relative flex items-center justify-center">
              <div
                v-if="reminders.some(r => { const d = new Date(r.scheduledAt); return d.getFullYear() === year && d.getMonth() + 1 === month && d.getDate() === date; })"
                class="w-6px h-6px rounded-full bg-primary"
              />
            </div>
          </template>
        </NCalendar>
      </NCard>

      <NCard size="small" class="flex-[3]">
        <template #header>
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <SvgIcon icon="mdi:format-list-bulleted" class="text-18px text-primary" />
              <span class="font-bold">
                {{ selectedDate.toLocaleDateString('zh-CN', { month: 'long', day: 'numeric' }) }} 备忘
              </span>
              <NTag size="tiny" :bordered="false" type="info">
                {{ selectedDayReminders.length }} 条
              </NTag>
            </div>
          </div>
        </template>

        <NSpin :show="loading">
          <div v-if="selectedDayReminders.length === 0" class="py-12 flex flex-col items-center justify-center text-gray-400">
            <SvgIcon icon="mdi:calendar-blank-outline" class="text-48px mb-2 opacity-30" />
            <div class="text-14px">暂无备忘</div>
            <div class="text-12px mt-1">点击上方按钮添加新备忘</div>
          </div>
          <div v-else class="space-y-3">
            <div
              v-for="item in selectedDayReminders"
              :key="item.id"
              class="p-3 rounded-lg border border-gray-100 dark:border-gray-700 hover:border-primary/50 transition-colors"
              :class="{ 'opacity-50 bg-gray-50 dark:bg-gray-800/50': item.status === 'completed', 'bg-white dark:bg-gray-800': item.status !== 'completed' }"
            >
              <div class="flex items-start justify-between gap-3">
                <div class="flex-1 min-w-0">
                  <div class="flex items-center gap-2">
                    <span class="font-bold text-15px truncate">{{ item.params.title }}</span>
                    <NTag v-if="item.status === 'completed'" size="tiny" type="success" :bordered="false">
                      已通知
                    </NTag>
                  </div>
                  <div v-if="item.params.content" class="text-gray-500 mt-1 text-13px line-clamp-2">{{ item.params.content }}</div>
                  <div class="flex flex-wrap gap-1 mt-2">
                    <NTag size="tiny" :bordered="false" type="warning">
                      <template #icon>
                        <SvgIcon icon="mdi:clock-outline" class="text-12px" />
                      </template>
                      {{ formatTime(item.scheduledAt) }}
                    </NTag>
                    <NTag v-if="item.repeatType !== 'none'" size="tiny" type="info" :bordered="false">
                      <template #icon>
                        <SvgIcon icon="mdi:repeat" class="text-12px" />
                      </template>
                      {{ getRepeatLabel(item.repeatType) }}{{ item.repeatInterval > 1 ? ` ×${item.repeatInterval}` : '' }}
                    </NTag>
                  </div>
                </div>
                <div class="flex items-center gap-1 flex-shrink-0">
                  <NButton text type="primary" size="small" @click="openEditModal(item)">
                    <SvgIcon icon="mdi:pencil-outline" />
                  </NButton>
                  <NButton text type="error" size="small" @click="handleDelete(item)">
                    <SvgIcon icon="mdi:delete-outline" />
                  </NButton>
                </div>
              </div>
            </div>
          </div>
        </NSpin>
      </NCard>
      </div>
      </NTabPane>

      <!-- 定时 Agent 任务 -->
      <NTabPane name="agent-task" tab="Agent 任务">
        <AgentTaskPanel />
      </NTabPane>
    </NTabs>

    <NModal v-model:show="showModal" preset="card" :title="editingId ? '编辑备忘' : '新增备忘'" style="width: 520px">
      <NForm ref="formRef" :model="form" :rules="rules" label-placement="left" label-width="80">
        <NFormItem label="标题" path="title">
          <NInput v-model:value="form.title" placeholder="请输入标题" />
        </NFormItem>
        <NFormItem label="内容" path="content">
          <NInput v-model:value="form.content" type="textarea" placeholder="请输入内容（可选）" :rows="3" />
        </NFormItem>
        <NFormItem label="提前通知">
          <div class="w-full">
            <NInputNumber v-model:value="form.advanceMinutes" :min="0" :max="1440" style="width: 100%">
              <template #suffix>分钟</template>
            </NInputNumber>
          </div>
        </NFormItem>
        <NFormItem label="提醒时间" path="remindAt">
          <NDatePicker v-model:value="form.remindAt" type="datetime" style="width: 100%" />
        </NFormItem>
        <NFormItem label="重复">
          <NSpace>
            <NSelect v-model:value="form.repeatType" :options="repeatOptions" style="width: 120px" />
            <NInputNumber
              v-if="form.repeatType !== 'none'"
              v-model:value="form.repeatInterval"
              :min="1"
              :max="99"
              style="width: 80px"
            />
          </NSpace>
        </NFormItem>
        <NFormItem v-if="form.repeatType !== 'none'" label="重复结束">
          <NDatePicker v-model:value="form.repeatEndAt" type="datetime" style="width: 100%" clearable placeholder="不设置则永不过期" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showModal = false">取消</NButton>
          <NButton type="primary" @click="handleSubmit">确定</NButton>
        </NSpace>
      </template>
    </NModal>
  </div>
</template>