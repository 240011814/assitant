<script setup lang="ts">
import { ref, reactive, computed, h, onMounted } from 'vue';
import { NButton, NTag } from 'naive-ui';
import { useAppStore } from '@/store/modules/app';
import { fetchAuditLogs, fetchAdminAsyncJobs } from '@/service/api';

defineOptions({ name: 'SystemAuditLog' });

const appStore = useAppStore();
const isMobile = computed(() => appStore.isMobile);

const loading = ref(false);
const logs = ref<Api.AuditLog.OperationAuditLog[]>([]);
// 模板内联箭头函数的类型标注会触发 vue/no-undef-properties 误报, 收进 script
const rowKey = (row: Api.AuditLog.OperationAuditLog) => row.id;

const filters = reactive({
  method: null as string | null,
  path: '',
  success: null as string | null,
  userId: null as number | null
});
const timeRange = ref<[number, number] | null>(null);

const pagination = reactive({
  page: 1,
  pageSize: 20,
  itemCount: 0,
  showSizePicker: true,
  pageSizes: [20, 50, 100],
  onChange: (page: number) => {
    pagination.page = page;
    loadLogs();
  },
  onUpdatePageSize: (pageSize: number) => {
    pagination.pageSize = pageSize;
    pagination.page = 1;
    loadLogs();
  }
});

const methodOptions = [
  { label: 'POST', value: 'POST' },
  { label: 'PUT', value: 'PUT' },
  { label: 'DELETE', value: 'DELETE' },
  { label: 'PATCH', value: 'PATCH' }
];

const successOptions = [
  { label: '成功', value: 'true' },
  { label: '失败', value: 'false' }
];

const methodTagType: Record<string, 'success' | 'info' | 'warning' | 'error' | 'default'> = {
  POST: 'info',
  PUT: 'warning',
  DELETE: 'error',
  PATCH: 'success'
};

const columns = [
  { title: '时间', key: 'createdAt', width: 170, render: (row: Api.AuditLog.OperationAuditLog) => formatTime(row.createdAt) },
  {
    title: '用户',
    key: 'userName',
    width: 130,
    ellipsis: { tooltip: true },
    render: (row: Api.AuditLog.OperationAuditLog) => row.userName || (row.userId ? `#${row.userId}` : '-')
  },
  {
    title: '方法',
    key: 'method',
    width: 90,
    render: (row: Api.AuditLog.OperationAuditLog) =>
      h(NTag, { size: 'small', type: methodTagType[row.method] || 'default', bordered: false }, { default: () => row.method })
  },
  { title: '路径', key: 'path', ellipsis: { tooltip: true } },
  {
    title: '结果',
    key: 'success',
    width: 90,
    render: (row: Api.AuditLog.OperationAuditLog) =>
      h(NTag, { size: 'small', type: row.success ? 'success' : 'error', bordered: false }, { default: () => (row.success ? '成功' : '失败') })
  },
  { title: '业务码', key: 'statusCode', width: 80 },
  { title: '耗时', key: 'latencyMs', width: 90, render: (row: Api.AuditLog.OperationAuditLog) => `${row.latencyMs}ms` },
  { title: 'IP', key: 'ip', width: 140 },
  { title: '来源', key: 'userAgent', width: 160, ellipsis: { tooltip: true } },
  {
    title: '操作',
    key: 'actions',
    width: 80,
    render: (row: Api.AuditLog.OperationAuditLog) =>
      h(NButton, { size: 'small', onClick: () => openDetail(row) }, { default: () => '详情' })
  }
];

const showDetail = ref(false);
const detail = ref<Api.AuditLog.OperationAuditLog | null>(null);
const detailBody = ref('');

const prettyBody = computed(() => {
  try {
    return JSON.stringify(JSON.parse(detailBody.value), null, 2);
  } catch {
    return detailBody.value;
  }
});

function formatTime(t: string | null) {
  if (!t) return '-';
  return new Date(t).toLocaleString('zh-CN', { hour12: false });
}

function formatDateTime(ms: number) {
  const d = new Date(ms);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

function buildParams(): Api.AuditLog.AuditLogListParams {
  const params: Api.AuditLog.AuditLogListParams = {
    page: pagination.page,
    page_size: pagination.pageSize
  };
  if (filters.method) params.method = filters.method;
  if (filters.path.trim()) params.path = filters.path.trim();
  if (filters.success) params.success = filters.success;
  if (filters.userId !== null) params.userId = filters.userId;
  if (timeRange.value) {
    params.startTime = formatDateTime(timeRange.value[0]);
    params.endTime = formatDateTime(timeRange.value[1]);
  }
  return params;
}

async function loadLogs() {
  loading.value = true;
  try {
    const { data, error } = await fetchAuditLogs(buildParams());
    if (!error && data) {
      logs.value = data.list;
      pagination.itemCount = data.total;
    }
  } finally {
    loading.value = false;
  }
}

/** 移动端卡片列表的内部分页回调 */
function handleMobilePage(page: number) {
  pagination.page = page;
  loadLogs();
}

function handleSearch() {
  pagination.page = 1;
  loadLogs();
}

function handleReset() {
  filters.method = null;
  filters.path = '';
  filters.success = null;
  filters.userId = null;
  timeRange.value = null;
  handleSearch();
}

function openDetail(row: Api.AuditLog.OperationAuditLog) {
  detail.value = row;
  detailBody.value = row.requestBody || '';
  showDetail.value = true;
}

// ===================== 异步任务 Tab (async_jobs 全量用户的慢 LLM 任务) =====================
const activeTab = ref<'logs' | 'jobs'>('logs');
const jobsLoaded = ref(false);
const jobsLoading = ref(false);
const jobs = ref<Api.AsyncJob.AdminJob[]>([]);
const jobsRowKey = (row: Api.AsyncJob.AdminJob) => row.id;
const jobsFilters = reactive({
  kind: null as string | null,
  status: null as string | null,
  userId: null as number | null
});
const jobsPagination = reactive({
  page: 1,
  pageSize: 20,
  itemCount: 0,
  showSizePicker: true,
  pageSizes: [20, 50, 100],
  onChange: (page: number) => {
    jobsPagination.page = page;
    loadJobs();
  },
  onUpdatePageSize: (pageSize: number) => {
    jobsPagination.pageSize = pageSize;
    jobsPagination.page = 1;
    loadJobs();
  }
});

const kindOptions = [
  { label: 'AI 写课文', value: 'passage' },
  { label: '词汇量测试', value: 'vocab_test' },
  { label: 'RSS 生词提取', value: 'rss_annotate' },
  { label: 'RSS 中英对照', value: 'rss_translate' }
];
const jobStatusOptions = [
  { label: '运行中', value: 'running' },
  { label: '已完成', value: 'done' },
  { label: '失败', value: 'failed' }
];
const kindLabel = (kind: string) => kindOptions.find(o => o.value === kind)?.label || kind;
const jobStatusLabel = (s: string) => jobStatusOptions.find(o => o.value === s)?.label || s;
const kindTagType: Record<string, 'success' | 'info' | 'warning' | 'error' | 'primary' | 'default'> = {
  passage: 'info',
  vocab_test: 'success',
  rss_annotate: 'warning',
  rss_translate: 'primary'
};
const jobStatusTagType: Record<string, 'success' | 'info' | 'error' | 'default'> = {
  running: 'info',
  done: 'success',
  failed: 'error'
};

/** 耗时: 已结束 = 精确耗时; 运行中 = 已进行时长 (+ 号示意未结束) */
function jobDuration(row: Api.AsyncJob.AdminJob): string {
  if (!row.startedAt) return '-';
  const start = new Date(row.startedAt).getTime();
  if (Number.isNaN(start)) return '-';
  const end = row.finishedAt ? new Date(row.finishedAt).getTime() : Date.now();
  const sec = Math.max(0, (end - start) / 1000);
  const fmt = (s: number) => (s >= 60 ? `${Math.floor(s / 60)}m${Math.round(s % 60)}s` : `${s.toFixed(1)}s`);
  return row.finishedAt ? fmt(sec) : `${fmt(sec)}+`;
}

const jobColumns = [
  { title: '时间', key: 'createdAt', width: 170, render: (row: Api.AsyncJob.AdminJob) => formatTime(row.createdAt) },
  {
    title: '用户',
    key: 'userName',
    width: 130,
    ellipsis: { tooltip: true },
    render: (row: Api.AsyncJob.AdminJob) => row.userName || `#${row.userId}`
  },
  {
    title: '类型',
    key: 'kind',
    width: 130,
    render: (row: Api.AsyncJob.AdminJob) =>
      h(NTag, { size: 'small', type: kindTagType[row.kind] || 'default', bordered: false }, { default: () => kindLabel(row.kind) })
  },
  {
    title: '状态',
    key: 'status',
    width: 90,
    render: (row: Api.AsyncJob.AdminJob) =>
      h(NTag, { size: 'small', type: jobStatusTagType[row.status] || 'default', bordered: false }, { default: () => jobStatusLabel(row.status) })
  },
  { title: '进度', key: 'progress', width: 120, ellipsis: { tooltip: true }, render: (row: Api.AsyncJob.AdminJob) => row.progress || '-' },
  { title: '结果ID', key: 'resultId', width: 80, render: (row: Api.AsyncJob.AdminJob) => (row.resultId > 0 ? row.resultId : '-') },
  { title: '耗时', key: 'duration', width: 100, render: (row: Api.AsyncJob.AdminJob) => jobDuration(row) },
  {
    title: '操作',
    key: 'actions',
    width: 80,
    render: (row: Api.AsyncJob.AdminJob) =>
      h(NButton, { size: 'small', onClick: () => openJobDetail(row) }, { default: () => '详情' })
  }
];

function buildJobParams(): Api.AsyncJob.AdminListParams {
  const params: Api.AsyncJob.AdminListParams = {
    page: jobsPagination.page,
    page_size: jobsPagination.pageSize
  };
  if (jobsFilters.kind) params.kind = jobsFilters.kind;
  if (jobsFilters.status) params.status = jobsFilters.status;
  if (jobsFilters.userId !== null) params.userId = jobsFilters.userId;
  return params;
}

async function loadJobs() {
  jobsLoading.value = true;
  try {
    const { data, error } = await fetchAdminAsyncJobs(buildJobParams());
    if (!error && data) {
      jobs.value = data.list;
      jobsPagination.itemCount = data.total;
      jobsLoaded.value = true;
    }
  } finally {
    jobsLoading.value = false;
  }
}

function handleTabChange(tab: string) {
  // 首次切到异步任务 Tab 才加载 (切回不重拉, 保持滚动位置与筛选)
  if (tab === 'jobs' && !jobsLoaded.value) loadJobs();
}

function handleJobsSearch() {
  jobsPagination.page = 1;
  loadJobs();
}

function handleJobsReset() {
  jobsFilters.kind = null;
  jobsFilters.status = null;
  jobsFilters.userId = null;
  handleJobsSearch();
}

function handleJobsMobilePage(page: number) {
  jobsPagination.page = page;
  loadJobs();
}

const showJobDetail = ref(false);
const jobDetail = ref<Api.AsyncJob.AdminJob | null>(null);
const prettyJobParams = computed(() => {
  try {
    return JSON.stringify(JSON.parse(jobDetail.value?.paramsJson || ''), null, 2);
  } catch {
    return jobDetail.value?.paramsJson || '';
  }
});

function openJobDetail(row: Api.AsyncJob.AdminJob) {
  jobDetail.value = row;
  showJobDetail.value = true;
}

onMounted(() => {
  loadLogs();
});
</script>

<template>
  <div class="h-full overflow-auto" :class="isMobile ? 'p-2' : 'p-6'">
    <NCard :bordered="false" :shadow="isMobile ? false : 'sm'" title="审计日志">
      <NTabs v-model:value="activeTab" type="line" @update:value="handleTabChange">
        <!-- Tab 1: 操作日志 -->
        <NTabPane name="logs" tab="操作日志">
          <NSpace class="mb-4" align="center" wrap>
            <NSelect
              v-model:value="filters.method"
              :options="methodOptions"
              placeholder="请求方法"
              clearable
              style="width: 130px"
            />
            <NInput
              v-model:value="filters.path"
              placeholder="路径模糊搜索"
              clearable
              style="width: 220px"
              @keydown.enter="handleSearch"
            />
            <NSelect v-model:value="filters.success" :options="successOptions" placeholder="结果" clearable style="width: 120px" />
            <NInputNumber
              v-model:value="filters.userId"
              placeholder="用户ID"
              clearable
              style="width: 150px"
              :show-button="false"
            />
            <NDatePicker v-model:value="timeRange" type="datetimerange" clearable />
            <NButton type="primary" @click="handleSearch">查询</NButton>
            <NButton @click="handleReset">重置</NButton>
          </NSpace>

          <NDataTable
            v-if="!isMobile"
            :columns="columns"
            :data="logs"
            :loading="loading"
            :pagination="pagination"
            remote
            :row-key="rowKey"
            size="small"
            striped
            :scroll-x="1200"
          />

          <!-- Mobile: 卡片列表 (时间线形态) -->
          <template v-else>
            <NSpin v-if="loading" class="flex justify-center py-8" />
            <NEmpty v-else-if="!logs.length" description="暂无日志" class="py-8" />
            <template v-else>
              <div class="flex flex-col gap-3">
                <NCard v-for="row in logs" :key="row.id" size="small" :bordered="true">
                  <div class="flex flex-col gap-2">
                    <div class="flex items-center justify-between">
                      <div class="flex items-center gap-2">
                        <NTag size="small" :type="methodTagType[row.method] || 'default'" :bordered="false">{{ row.method }}</NTag>
                        <NTag size="small" :type="row.success ? 'success' : 'error'" :bordered="false">
                          {{ row.success ? '成功' : '失败' }}
                        </NTag>
                      </div>
                      <span class="text-xs text-gray-400">{{ formatTime(row.createdAt) }}</span>
                    </div>
                    <div class="break-all text-xs text-gray-600 dark:text-gray-400">{{ row.path }}</div>
                    <div class="flex items-center justify-between text-xs text-gray-400">
                      <span class="truncate" style="max-width: 60%;">{{ row.userName || (row.userId ? `#${row.userId}` : '-') }} · {{ row.ip || '-' }}</span>
                      <NButton size="tiny" type="primary" quaternary @click="openDetail(row)">详情</NButton>
                    </div>
                    <div v-if="row.statusCode != null || row.latencyMs" class="text-xs text-gray-400">
                      <span v-if="row.latencyMs">{{ row.latencyMs }}ms</span>
                      <span v-if="row.statusCode"> · 业务码 {{ row.statusCode }}</span>
                    </div>
                  </div>
                </NCard>
              </div>
              <NPagination
                v-if="pagination.itemCount > pagination.pageSize"
                class="mt-4 justify-center"
                :page="pagination.page"
                :item-count="pagination.itemCount"
                :page-size="pagination.pageSize"
                :page-slot="5"
                @update:page="handleMobilePage"
              />
            </template>
          </template>
        </NTabPane>

        <!-- Tab 2: 异步任务 -->
        <NTabPane name="jobs" tab="异步任务">
          <NSpace class="mb-4" align="center" wrap>
            <NSelect
              v-model:value="jobsFilters.kind"
              :options="kindOptions"
              placeholder="任务类型"
              clearable
              style="width: 160px"
            />
            <NSelect
              v-model:value="jobsFilters.status"
              :options="jobStatusOptions"
              placeholder="状态"
              clearable
              style="width: 120px"
            />
            <NInputNumber
              v-model:value="jobsFilters.userId"
              placeholder="用户ID"
              clearable
              style="width: 150px"
              :show-button="false"
            />
            <NButton type="primary" @click="handleJobsSearch">查询</NButton>
            <NButton @click="handleJobsReset">重置</NButton>
          </NSpace>

          <NDataTable
            v-if="!isMobile"
            :columns="jobColumns"
            :data="jobs"
            :loading="jobsLoading"
            :pagination="jobsPagination"
            remote
            :row-key="jobsRowKey"
            size="small"
            striped
            :scroll-x="900"
          />

          <!-- Mobile: 卡片列表 -->
          <template v-else>
            <NSpin v-if="jobsLoading" class="flex justify-center py-8" />
            <NEmpty v-else-if="!jobs.length" description="暂无任务" class="py-8" />
            <template v-else>
              <div class="flex flex-col gap-3">
                <NCard v-for="row in jobs" :key="row.id" size="small" :bordered="true">
                  <div class="flex flex-col gap-2">
                    <div class="flex items-center justify-between">
                      <div class="flex items-center gap-2">
                        <NTag size="small" :type="kindTagType[row.kind] || 'default'" :bordered="false">{{ kindLabel(row.kind) }}</NTag>
                        <NTag size="small" :type="jobStatusTagType[row.status] || 'default'" :bordered="false">
                          {{ jobStatusLabel(row.status) }}
                        </NTag>
                      </div>
                      <span class="text-xs text-gray-400">{{ formatTime(row.createdAt) }}</span>
                    </div>
                    <div class="text-xs text-gray-600 dark:text-gray-400">
                      {{ row.userName || `#${row.userId}` }}
                      <span v-if="row.progress"> · {{ row.progress }}</span>
                      <span v-if="row.resultId > 0"> · 结果 #{{ row.resultId }}</span>
                    </div>
                    <div class="flex items-center justify-between text-xs text-gray-400">
                      <span>耗时 {{ jobDuration(row) }}</span>
                      <NButton size="tiny" type="primary" quaternary @click="openJobDetail(row)">详情</NButton>
                    </div>
                  </div>
                </NCard>
              </div>
              <NPagination
                v-if="jobsPagination.itemCount > jobsPagination.pageSize"
                class="mt-4 justify-center"
                :page="jobsPagination.page"
                :item-count="jobsPagination.itemCount"
                :page-size="jobsPagination.pageSize"
                :page-slot="5"
                @update:page="handleJobsMobilePage"
              />
            </template>
          </template>
        </NTabPane>
      </NTabs>
    </NCard>

    <NModal v-model:show="showDetail" preset="card" title="操作详情" style="width: 720px" class="max-w-[96vw]">
      <template v-if="detail">
        <NDescriptions :column="2" size="small" bordered label-placement="left">
          <NDescriptionsItem label="时间">{{ formatTime(detail.createdAt) }}</NDescriptionsItem>
          <NDescriptionsItem label="耗时">{{ detail.latencyMs }}ms</NDescriptionsItem>
          <NDescriptionsItem label="用户">
            {{ detail.userName || '-' }}
            <span v-if="detail.userId" class="text-gray-400">(ID: {{ detail.userId }})</span>
          </NDescriptionsItem>
          <NDescriptionsItem label="IP">{{ detail.ip || '-' }}</NDescriptionsItem>
          <NDescriptionsItem label="方法">
            <NTag size="small" :type="methodTagType[detail.method] || 'default'" :bordered="false">
              {{ detail.method }}
            </NTag>
          </NDescriptionsItem>
          <NDescriptionsItem label="结果">
            <NTag size="small" :type="detail.success ? 'success' : 'error'" :bordered="false">
              {{ detail.success ? '成功' : '失败' }}
            </NTag>
            <span v-if="detail.statusCode" class="ml-2 text-gray-400">业务码: {{ detail.statusCode }}</span>
          </NDescriptionsItem>
          <NDescriptionsItem label="路径" :span="2">{{ detail.path }}</NDescriptionsItem>
          <NDescriptionsItem v-if="detail.errorMsg" label="失败原因" :span="2">
            <span class="text-red-500">{{ detail.errorMsg }}</span>
          </NDescriptionsItem>
          <NDescriptionsItem label="User-Agent" :span="2">{{ detail.userAgent || '-' }}</NDescriptionsItem>
        </NDescriptions>

        <div v-if="detailBody" class="mt-4">
          <div class="mb-1 text-gray-500">请求参数 (敏感字段已脱敏)</div>
          <NCode :code="prettyBody" language="json" word-wrap />
        </div>
      </template>
    </NModal>

    <!-- 异步任务详情 -->
    <NModal v-model:show="showJobDetail" preset="card" title="异步任务详情" style="width: 720px" class="max-w-[96vw]">
      <template v-if="jobDetail">
        <NDescriptions :column="2" size="small" bordered label-placement="left">
          <NDescriptionsItem label="任务 ID">#{{ jobDetail.id }}</NDescriptionsItem>
          <NDescriptionsItem label="创建时间">{{ formatTime(jobDetail.createdAt) }}</NDescriptionsItem>
          <NDescriptionsItem label="用户">
            {{ jobDetail.userName || '-' }}
            <span v-if="jobDetail.userId" class="text-gray-400">(ID: {{ jobDetail.userId }})</span>
          </NDescriptionsItem>
          <NDescriptionsItem label="类型">
            <NTag size="small" :type="kindTagType[jobDetail.kind] || 'default'" :bordered="false">
              {{ kindLabel(jobDetail.kind) }}
            </NTag>
            <span class="ml-2 text-gray-400">{{ jobDetail.kind }}</span>
          </NDescriptionsItem>
          <NDescriptionsItem label="状态">
            <NTag size="small" :type="jobStatusTagType[jobDetail.status] || 'default'" :bordered="false">
              {{ jobStatusLabel(jobDetail.status) }}
            </NTag>
            <span v-if="jobDetail.progress" class="ml-2 text-gray-400">{{ jobDetail.progress }}</span>
          </NDescriptionsItem>
          <NDescriptionsItem label="结果 ID">{{ jobDetail.resultId > 0 ? `#${jobDetail.resultId}` : '-' }}</NDescriptionsItem>
          <NDescriptionsItem label="开始时间">{{ formatTime(jobDetail.startedAt || null) }}</NDescriptionsItem>
          <NDescriptionsItem label="结束时间">{{ formatTime(jobDetail.finishedAt || null) }} ({{ jobDuration(jobDetail) }})</NDescriptionsItem>
          <NDescriptionsItem v-if="jobDetail.error" label="失败原因" :span="2">
            <span class="text-red-500">{{ jobDetail.error }}</span>
          </NDescriptionsItem>
        </NDescriptions>

        <div v-if="prettyJobParams" class="mt-4">
          <div class="mb-1 text-gray-500">输入快照 (创建参数)</div>
          <NCode :code="prettyJobParams" language="json" word-wrap />
        </div>
      </template>
    </NModal>
  </div>
</template>
