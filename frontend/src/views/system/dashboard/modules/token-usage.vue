<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from 'vue';
import { NButton, NProgress, NTag } from 'naive-ui';
import { useEcharts } from '@/hooks/common/echarts';
import { fetchTokenUsageRecords, fetchTokenUsageStats } from '@/service/api';

defineOptions({ name: 'TokenUsagePanel' });

const loading = ref(false);
const stats = ref<Api.TokenUsage.Stats | null>(null);

// ---------- 筛选 ----------
const granularity = ref<Api.TokenUsage.Granularity>('day');
const model = ref<string | null>(null);
const userId = ref<number | null>(null);
const timeRange = ref<[number, number] | null>(null);

const granularityOptions = [
  { label: '按小时', value: 'hour' },
  { label: '按日', value: 'day' },
  { label: '按月', value: 'month' },
  { label: '按年', value: 'year' }
];

// 模型下拉: 取全年不分模型的分布作为候选 (空串 = 默认模型)
const modelOptions = ref<{ label: string; value: string }[]>([]);

async function loadModelOptions() {
  const yearAgo = new Date();
  yearAgo.setFullYear(yearAgo.getFullYear() - 1);
  const { data, error } = await fetchTokenUsageStats({
    granularity: 'year',
    startTime: formatDateTime(yearAgo.getTime()),
    endTime: formatDateTime(Date.now())
  });
  if (!error && data) {
    modelOptions.value = data.by_model.map(m => ({
      label: m.model ? m.model : '默认模型',
      value: m.model
    }));
  }
}

function formatDateTime(ms: number) {
  const d = new Date(ms);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

function buildParams() {
  const params: Api.TokenUsage.StatsParams = { granularity: granularity.value };
  if (model.value) params.model = model.value;
  if (userId.value !== null) params.userId = userId.value;
  if (timeRange.value) {
    params.startTime = formatDateTime(timeRange.value[0]);
    params.endTime = formatDateTime(timeRange.value[1]);
  }
  return params;
}

// ---------- 汇总卡片 ----------
const summaryCards = computed(() => {
  const s = stats.value?.summary;
  return [
    { label: '总 Token', value: fmt(s?.total_tokens) },
    { label: '输入 Token', value: fmt(s?.prompt_tokens) },
    { label: '输出 Token', value: fmt(s?.completion_tokens) },
    { label: '模型调用次数', value: fmt(s?.calls) },
    { label: '使用用户数', value: fmt(s?.users) }
  ];
});

function fmt(n?: number) {
  if (n === undefined || n === null) return '-';
  return n.toLocaleString('zh-CN');
}

function formatTime(t: string | null) {
  if (!t) return '-';
  return new Date(t).toLocaleString('zh-CN', { hour12: false });
}

// ---------- 趋势图 ----------
const { domRef: trendRef, updateOptions: updateTrend } = useEcharts(() => ({
  tooltip: { trigger: 'axis' },
  legend: { data: ['Token 用量', '调用次数'] },
  grid: { left: '3%', right: '4%', bottom: '3%', top: '18%', containLabel: true },
  xAxis: { type: 'category', data: [] as string[] },
  yAxis: [
    { type: 'value', name: 'Token' },
    { type: 'value', name: '次数', minInterval: 1, splitLine: { show: false } }
  ],
  series: [
    {
      name: 'Token 用量',
      type: 'bar',
      data: [] as number[],
      itemStyle: { color: '#5b8ff9' }
    },
    {
      name: '调用次数',
      type: 'line',
      yAxisIndex: 1,
      smooth: true,
      data: [] as number[],
      itemStyle: { color: '#5ad8a6' }
    }
  ]
}));

// ---------- 模型分布图 ----------
const { domRef: modelRef, updateOptions: updateModel } = useEcharts(() => ({
  tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
  grid: { left: '3%', right: '8%', bottom: '3%', top: '8%', containLabel: true },
  xAxis: { type: 'value', minInterval: 1 },
  yAxis: { type: 'category', data: [] as string[] },
  series: [
    {
      name: 'Token 用量',
      type: 'bar',
      data: [] as number[],
      itemStyle: { color: '#8e9dff' },
      label: { show: true, position: 'right', formatter: (p: any) => Number(p.value).toLocaleString('zh-CN') }
    }
  ]
}));

// ---------- 用户排行 ----------
const userColumns = [
  { title: '用户', key: 'username', width: 140, ellipsis: { tooltip: true }, render: (row: Api.TokenUsage.UserItem) => row.nickname || row.username },
  { title: 'Token', key: 'total_tokens', width: 110, render: (row: Api.TokenUsage.UserItem) => fmt(row.total_tokens) },
  { title: '调用', key: 'calls', width: 80, render: (row: Api.TokenUsage.UserItem) => fmt(row.calls) },
  {
    title: '月度限额',
    key: 'quota_month',
    render: (row: Api.TokenUsage.UserItem) => renderQuota(row)
  }
];

function renderQuota(row: Api.TokenUsage.UserItem) {
  if (!row.quota_month || row.quota_month <= 0) {
    return h(NTag, { size: 'small', bordered: false }, { default: () => '不限' });
  }
  const percent = Math.min(100, Math.round((row.total_tokens / row.quota_month) * 100));
  const status = percent >= 100 ? 'error' : percent >= 80 ? 'warning' : 'success';
  const text = `${fmt(row.total_tokens)} / ${fmt(row.quota_month)}`;
  return h('div', { class: 'flex items-center gap-2' }, [
    h(NProgress, {
      type: 'line',
      percentage: percent,
      status,
      showIndicator: false,
      style: 'width: 110px'
    }),
    h('span', { class: 'text-xs whitespace-nowrap', style: percent >= 100 ? 'color:#d03050' : '' }, text)
  ]);
}

// ---------- 明细 ----------
const records = ref<Api.TokenUsage.UsageRecord[]>([]);
const pagination = reactive({
  page: 1,
  pageSize: 20,
  itemCount: 0,
  showSizePicker: true,
  pageSizes: [20, 50, 100],
  onChange: (page: number) => {
    pagination.page = page;
    loadRecords();
  },
  onUpdatePageSize: (pageSize: number) => {
    pagination.pageSize = pageSize;
    pagination.page = 1;
    loadRecords();
  }
});

const sourceLabels: Record<string, string> = {
  chat: '对话',
  orchestration_debug: '编排调试',
  orchestration_chat: '编排对话'
};

const recordColumns = [
  { title: '时间', key: 'created_at', width: 165, render: (row: Api.TokenUsage.UsageRecord) => formatTime(row.created_at) },
  { title: '用户', key: 'username', width: 130, ellipsis: { tooltip: true }, render: (row: Api.TokenUsage.UsageRecord) => row.nickname || row.username },
  { title: '模型', key: 'model', width: 170, ellipsis: { tooltip: true }, render: (row: Api.TokenUsage.UsageRecord) => row.model || '默认模型' },
  {
    title: '来源',
    key: 'source',
    width: 100,
    render: (row: Api.TokenUsage.UsageRecord) =>
      h(NTag, { size: 'small', bordered: false }, { default: () => sourceLabels[row.source] || row.source || '-' })
  },
  { title: '输入', key: 'prompt_tokens', width: 90, render: (row: Api.TokenUsage.UsageRecord) => fmt(row.prompt_tokens) },
  { title: '输出', key: 'completion_tokens', width: 90, render: (row: Api.TokenUsage.UsageRecord) => fmt(row.completion_tokens) },
  { title: '合计', key: 'total_tokens', width: 100, render: (row: Api.TokenUsage.UsageRecord) => fmt(row.total_tokens) }
];

// ---------- 加载 ----------
async function loadStats() {
  const { data, error } = await fetchTokenUsageStats(buildParams());
  if (!error && data) {
    stats.value = data;
    updateTrend(opts => {
      opts.xAxis.data = data.trend.map(i => i.bucket);
      opts.series[0].data = data.trend.map(i => i.total_tokens);
      opts.series[1].data = data.trend.map(i => i.calls);
      return opts;
    });
    updateModel(opts => {
      const items = [...data.by_model].reverse(); // 横向条形图从上到下按用量降序
      opts.yAxis.data = items.map(m => (m.model ? m.model : '默认模型'));
      opts.series[0].data = items.map(m => m.total_tokens);
      return opts;
    });
  }
}

async function loadRecords() {
  const { data, error } = await fetchTokenUsageRecords({
    ...buildParams(),
    page: pagination.page,
    page_size: pagination.pageSize
  });
  if (!error && data) {
    records.value = data.list;
    pagination.itemCount = data.total;
  }
}

async function loadData() {
  loading.value = true;
  try {
    await Promise.all([loadStats(), loadRecords()]);
  } finally {
    loading.value = false;
  }
}

function handleSearch() {
  pagination.page = 1;
  loadData();
}

function handleReset() {
  granularity.value = 'day';
  model.value = null;
  userId.value = null;
  timeRange.value = null;
  handleSearch();
}

onMounted(() => {
  loadModelOptions();
  handleSearch();
});

const exportLoading = ref(false);

async function handleExportCsv() {
  // 导出当前筛选的明细 (最多取 5000 行, 由前端拼 CSV)
  exportLoading.value = true;
  try {
    const { data, error } = await fetchTokenUsageRecords({
      ...buildParams(),
      page: 1,
      page_size: 100
    });
    if (error || !data) return;
    const rows: Api.TokenUsage.UsageRecord[] = [...data.list];
    const pages = Math.min(50, Math.ceil(data.total / data.page_size)); // 上限 50 页 = 5000 行
    for (let p = 2; p <= pages; p += 1) {
      const next = await fetchTokenUsageRecords({ ...buildParams(), page: p, page_size: 100 });
      if (!next.error && next.data) rows.push(...next.data.list);
    }
    const header = ['时间', '用户', '模型', '来源', '输入', '输出', '合计'];
    const lines = rows.map(r =>
      [
        formatTime(r.created_at),
        r.nickname || r.username,
        r.model || '默认模型',
        sourceLabels[r.source] || r.source,
        r.prompt_tokens,
        r.completion_tokens,
        r.total_tokens
      ].join(',')
    );
    const csv = `\uFEFF${header.join(',')}\n${lines.join('\n')}`;
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `token-usage-${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  } finally {
    exportLoading.value = false;
  }
}

const showExport = computed(() => records.value.length > 0);
</script>

<template>
  <div>
    <NCard :bordered="false" shadow="sm" title="Token 用量统计">
      <NSpace class="mb-4" align="center" wrap>
        <NSelect v-model:value="granularity" :options="granularityOptions" style="width: 110px" />
        <NSelect
          v-model:value="model"
          :options="modelOptions"
          placeholder="全部模型"
          clearable
          filterable
          style="width: 200px"
        />
        <NInputNumber v-model:value="userId" placeholder="用户ID" clearable :show-button="false" style="width: 130px" />
        <NDatePicker v-model:value="timeRange" type="datetimerange" clearable />
        <NButton type="primary" :loading="loading" @click="handleSearch">查询</NButton>
        <NButton @click="handleReset">重置</NButton>
        <NButton v-if="showExport" :loading="exportLoading" @click="handleExportCsv">导出明细</NButton>
      </NSpace>

      <NSpin :show="loading">
        <NGrid :x-gap="16" :y-gap="16" cols="2 s:3 m:5" responsive="screen">
          <NGi v-for="card in summaryCards" :key="card.label">
            <NCard size="small" :bordered="false" shadow="sm" class="bg-gray-50">
              <div class="text-sm text-gray-400">{{ card.label }}</div>
              <div class="mt-2 text-2xl font-semibold">{{ card.value }}</div>
            </NCard>
          </NGi>
        </NGrid>

        <NCard :bordered="false" shadow="sm" title="用量趋势" class="mt-4">
          <div ref="trendRef" class="h-320px" />
        </NCard>

        <NGrid :x-gap="16" :y-gap="16" cols="1 m:2" responsive="screen" class="mt-4">
          <NGi>
            <NCard :bordered="false" shadow="sm" title="模型分布">
              <div ref="modelRef" class="h-300px" />
            </NCard>
          </NGi>
          <NGi>
            <NCard :bordered="false" shadow="sm" title="用户排行 (Top 20)">
              <NDataTable
                :columns="userColumns"
                :data="stats?.by_user || []"
                size="small"
                striped
                :row-key="(row: Api.TokenUsage.UserItem) => row.user_id"
                :max-height="300"
              />
            </NCard>
          </NGi>
        </NGrid>

        <NCard :bordered="false" shadow="sm" title="调用明细" class="mt-4">
          <NDataTable
            :columns="recordColumns"
            :data="records"
            :loading="loading"
            :pagination="pagination"
            remote
            :row-key="(row: Api.TokenUsage.UsageRecord) => row.id"
            size="small"
            striped
            :scroll-x="900"
          />
        </NCard>
      </NSpin>
    </NCard>
  </div>
</template>
