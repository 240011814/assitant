<script setup lang="ts">
import { ref, reactive, computed, h, onMounted } from 'vue';
import { NButton, NTag } from 'naive-ui';
import { useAppStore } from '@/store/modules/app';
import { fetchAuditLogs } from '@/service/api';

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
  {
    title: '失败原因',
    key: 'errorMsg',
    minWidth: 220,
    render: (row: Api.AuditLog.OperationAuditLog) => {
      if (row.success || !row.errorMsg) return '-';
      return h(
        'span',
        { class: 'text-red-500', title: row.errorMsg },
        row.errorMsg.length > 60 ? `${row.errorMsg.slice(0, 60)}...` : row.errorMsg
      );
    }
  },
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

onMounted(() => {
  loadLogs();
});
</script>

<template>
  <div class="h-full overflow-auto" :class="isMobile ? 'p-2' : 'p-6'">
    <NCard :bordered="false" :shadow="isMobile ? false : 'sm'" title="操作审计日志">
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
  </div>
</template>
