<script setup lang="ts">
import { computed, h, onBeforeUnmount, ref } from 'vue';
import { NButton, NDataTable, NInput, NScrollbar, NSpace, NTag, useMessage } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { fetchOrchestrationDebugRun } from '@/service/api';

// 节点调试摘要 (与后端 OrchNodeTrace 对应)
export interface NodeTrace {
  key: string;
  name: string;
  comp: string;
  status: 'running' | 'success' | 'error';
  ms: number;
  content?: string;
  tokens?: { prompt_tokens: number; completion_tokens: number; total_tokens: number } | null;
  tool_calls?: { name: string; ms: number }[];
  error?: string;
  /** 子Agent 节点专用: 是否被主 Agent 委派过 / 所属主 Agent / 委派任务原文 */
  delegated?: boolean;
  owner?: string;
  task?: string;
}

interface DebugSummary {
  mode: string;
  output: string;
  nodes: NodeTrace[];
  total_ms: number;
  tokens?: { prompt_tokens: number; completion_tokens: number; total_tokens: number } | null;
}

interface DebugEvent {
  time: string;
  kind: string;
  text: string;
  type: 'info' | 'error' | 'success';
}

const props = defineProps<{
  orchestrationId: number | null;
  getDefinition: () => string;
  hasNodes: boolean;
}>();

const emit = defineEmits<{
  'running-change': [running: boolean];
  'traces-change': [traces: Record<string, NodeTrace>];
}>();

const message = useMessage();

const input = ref('');
const running = ref(false);
const output = ref('');
// 思考过程 (模型 reasoning 增量) 与正文分开: 默认折叠, 有内容时可展开
const reasoning = ref('');
const showReasoning = ref(true);
const events = ref<DebugEvent[]>([]);
const summary = ref<DebugSummary | null>(null);
const liveTraces = ref<Record<string, NodeTrace>>({});

let abortController: AbortController | null = null;

const TRACE_EVENT_CAP = 300;

function nowTime(): string {
  return new Date().toLocaleTimeString('zh-CN', { hour12: false });
}

function pushEvent(kind: string, text: string, type: DebugEvent['type'] = 'info') {
  events.value.push({ time: nowTime(), kind, text, type });
  if (events.value.length > TRACE_EVENT_CAP) {
    events.value.splice(0, events.value.length - TRACE_EVENT_CAP);
  }
}

// 实时归并节点事件到画布状态: 节点自身 end (key===owner) 更新状态,
// 内部事件 (react 模型/工具) 追加工具调用记录; token 用量在 summary 阶段补全
function applyNodeEvent(payload: any) {
  // 子Agent 委派事件: key 是子Agent 节点 id, owner 才是触发委派的主 Agent,
  // 不能按 owner 归并 (否则子Agent 的状态会写到主 Agent 上)
  if (payload.delegated) {
    const key = String(payload.key || '');
    if (!key) return;
    const sub: NodeTrace = liveTraces.value[key] || {
      key,
      name: String(payload.name || key),
      comp: 'DelegateTool',
      status: 'running',
      ms: 0
    };
    sub.delegated = true;
    sub.owner = String(payload.owner || sub.owner || '');
    if (payload.task) sub.task = String(payload.task);
    if (payload.content) sub.content = String(payload.content);
    if (payload.error) {
      sub.status = 'error';
      sub.error = String(payload.error);
    } else if (payload.status) {
      sub.status = payload.status;
    }
    liveTraces.value = { ...liveTraces.value, [key]: sub };
    emit('traces-change', liveTraces.value);
    return;
  }
  const owner = String(payload.owner || payload.key || '');
  if (!owner) return;
  const trace: NodeTrace = liveTraces.value[owner] || { key: owner, name: owner, comp: '', status: 'running', ms: 0 };
  if (payload.kind === 'start') {
    if (payload.key === owner && payload.name) trace.name = payload.name;
    trace.status = 'running';
  } else if (payload.key === owner) {
    trace.name = payload.name || trace.name;
    trace.ms = payload.ms ?? trace.ms;
    if (payload.content) trace.content = payload.content;
    if (payload.error) {
      trace.status = 'error';
      trace.error = payload.error;
    } else {
      trace.status = 'success';
    }
  } else if (payload.tool) {
    trace.tool_calls = [...(trace.tool_calls || []), { name: payload.tool, ms: payload.ms ?? 0 }];
  }
  liveTraces.value = { ...liveTraces.value, [owner]: trace };
  emit('traces-change', liveTraces.value);
}

function buildTraces(result: DebugSummary): Record<string, NodeTrace> {
  const map: Record<string, NodeTrace> = {};
  for (const node of result.nodes || []) {
    map[node.key] = node;
  }
  return map;
}

async function handleRun() {
  if (!props.hasNodes) {
    message.warning('画布为空, 请先添加节点');
    return;
  }
  if (!input.value.trim()) {
    message.warning('请输入调试输入');
    return;
  }
  running.value = true;
  emit('running-change', true);
  output.value = '';
  reasoning.value = '';
  events.value = [];
  summary.value = null;
  liveTraces.value = {};
  emit('traces-change', {});
  abortController = new AbortController();

  try {
    const response = await fetchOrchestrationDebugRun({
      id: props.orchestrationId ?? undefined,
      definition: props.getDefinition(),
      input: input.value,
      signal: abortController.signal
    });
    if (!response.ok || !response.body) {
      const text = await response.text().catch(() => '');
      message.error(`调试请求失败 (${response.status}) ${text.slice(0, 200)}`);
      return;
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder('utf-8');
    let buffer = '';

    const handlePayload = (eventName: string, dataStr: string) => {
      let payload: any = null;
      try {
        payload = dataStr ? JSON.parse(dataStr) : null;
      } catch {
        payload = { raw: dataStr };
      }
      switch (eventName) {
        case 'start':
          pushEvent('run', `开始执行 (${payload?.mode} 编排, ${payload?.node_count} 个节点)`);
          break;
        case 'node': {
          if (payload?.delegated) {
            // 子Agent 委派: 用可读文案替代 owner/ms 形式, 便于看清"谁委派给谁做什么"
            const task = payload?.task ? ` 「${String(payload.task).slice(0, 60)}」` : '';
            const okText = payload?.status === 'success' ? '完成' : payload?.status === 'error' ? '失败' : '开始';
            pushEvent(
              'delegate',
              `⇢ ${payload?.owner || '?'} 委派 ${payload?.name || payload?.key}${task} · ${okText}`,
              payload?.status === 'error' ? 'error' : 'success'
            );
            applyNodeEvent(payload);
            break;
          }
          if (payload?.kind === 'start') {
            pushEvent('node', `▶ ${payload.key} (${payload.comp})`, 'info');
          } else {
            const toolText = payload?.tool ? ` 工具 ${payload.tool}` : '';
            const errorText = payload?.error ? ` 错误: ${payload.error}` : '';
            pushEvent('node', `■ ${payload?.owner || payload?.key}${toolText} ${payload?.ms ?? 0}ms${errorText}`, payload?.error ? 'error' : 'info');
            applyNodeEvent(payload);
          }
          break;
        }
        case 'delta':
          output.value += payload?.content || '';
          break;
        case 'reasoning':
          // 思考增量: 单独累计并展示, 不混进最终输出
          reasoning.value += payload?.content || '';
          break;
        case 'summary':
          summary.value = payload as DebugSummary;
          emit('traces-change', buildTraces(summary.value));
          pushEvent('done', `执行完成 (${payload?.mode}, ${payload?.total_ms ?? 0}ms)`, 'success');
          break;
        case 'error':
          if (payload?.errors?.length) {
            payload.errors.forEach((e: string) => pushEvent('error', e, 'error'));
          } else {
            pushEvent('error', payload?.message || '执行出错', 'error');
          }
          break;
        case 'done':
          break;
        default:
          break;
      }
    };

    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const blocks = buffer.split(/\n\n/);
      buffer = blocks.pop() || '';
      for (const block of blocks) {
        let eventName = 'message';
        const dataLines: string[] = [];
        for (const line of block.split(/\r?\n/)) {
          if (line.startsWith('event:')) {
            eventName = line.replace(/^event:\s*/, '').trim();
          } else if (line.startsWith('data:')) {
            dataLines.push(line.replace(/^data:\s*/, ''));
          }
        }
        if (dataLines.length > 0) {
          handlePayload(eventName, dataLines.join('\n'));
        }
      }
    }
  } catch (err: any) {
    if (err?.name === 'AbortError') {
      pushEvent('abort', '已手动中断', 'error');
    } else {
      pushEvent('error', err?.message || '调试请求异常', 'error');
    }
  } finally {
    running.value = false;
    emit('running-change', false);
    abortController = null;
  }
}

function handleAbort() {
  abortController?.abort();
}

const traceColumns: DataTableColumns<NodeTrace> = [
  {
    title: '节点',
    key: 'name',
    width: 110,
    ellipsis: { tooltip: true },
    render: row => h('span', { class: 'font-medium' }, row.name || row.key)
  },
  {
    title: '状态',
    key: 'status',
    width: 72,
    render: row =>
      h(
        NTag,
        { size: 'small', type: row.status === 'error' ? 'error' : row.status === 'success' ? 'success' : 'info', bordered: false },
        { default: () => (row.status === 'error' ? '失败' : row.status === 'success' ? '成功' : '运行中') }
      )
  },
  {
    // 子Agent 节点不参与主流, 只有"是否被委派"这一个有意义的标记
    title: '委派',
    key: 'delegated',
    width: 96,
    render: row => (row.delegated ? h('span', { class: 'text-xs' }, row.owner ? `← ${row.owner}` : '已委派') : '-')
  },
  { title: '耗时', key: 'ms', width: 76, render: row => `${row.ms}ms` },
  {
    title: 'Tokens',
    key: 'tokens',
    width: 84,
    render: row => (row.tokens?.total_tokens ? String(row.tokens.total_tokens) : '-')
  },
  {
    title: '工具调用',
    key: 'tool_calls',
    width: 110,
    render: row =>
      row.tool_calls?.length
        ? h('span', { class: 'text-xs' }, row.tool_calls.map(tc => tc.name).join(', '))
        : '-'
  },
  {
    title: '输出',
    key: 'content',
    ellipsis: { tooltip: true },
    render: row => row.content || row.error || '-'
  }
];

const totalTokens = computed(() => summary.value?.tokens?.total_tokens || 0);

onBeforeUnmount(() => {
  abortController?.abort();
});
</script>

<template>
  <div class="h-full flex flex-col gap-2 min-h-0">
    <!-- 输入与控制 -->
    <div class="flex gap-2 shrink-0">
      <NInput
        v-model:value="input"
        type="textarea"
        :rows="2"
        placeholder="调试输入 (将作为编排入口的用户消息)"
        class="flex-1"
      />
      <NSpace vertical size="small" class="shrink-0">
        <NButton type="primary" size="small" :loading="running" :disabled="!hasNodes" @click="handleRun">
          <template #icon><SvgIcon icon="mdi:play" /></template>
          运行
        </NButton>
        <NButton v-if="running" size="small" type="error" secondary @click="handleAbort">中断</NButton>
      </NSpace>
    </div>

    <!-- 输出 + 事件流 -->
    <div class="flex-1 min-h-0 flex gap-3" :class="'flex-col md:flex-row'">
      <div class="flex-1 min-h-0 flex flex-col gap-1">
        <div class="text-xs text-gray-500 flex items-center gap-2">
          <span>最终输出</span>
          <NTag v-if="summary" size="small" type="info" :bordered="false">{{ summary.mode }} · {{ summary.total_ms }}ms · {{ totalTokens }} tokens</NTag>
          <NButton v-if="reasoning" size="tiny" quaternary @click="showReasoning = !showReasoning">
            {{ showReasoning ? '隐藏思考' : '显示思考' }}
          </NButton>
        </div>
        <!-- 思考过程: 模型 reasoning 增量, 边生成边追加 -->
        <div v-if="reasoning && showReasoning" class="shrink-0 max-h-40 flex flex-col">
          <div class="text-11px text-gray-400 mb-0.5">思考过程</div>
          <NScrollbar class="min-h-0">
            <pre class="text-xs whitespace-pre-wrap break-all m-0 p-2 rounded min-h-8 bg-amber-50 dark:bg-amber-900/20 text-gray-600 dark:text-gray-300">{{ reasoning }}</pre>
          </NScrollbar>
        </div>
        <NScrollbar class="flex-1">
          <pre class="text-xs whitespace-pre-wrap break-all m-0 p-2 bg-gray-50 dark:bg-gray-800 rounded min-h-12">{{ output || '—' }}</pre>
        </NScrollbar>
      </div>
      <div class="flex-1 min-h-0 flex flex-col gap-1">
        <div class="text-xs text-gray-500">事件流 (最近 {{ TRACE_EVENT_CAP }} 条)</div>
        <NScrollbar class="flex-1">
          <div class="text-xs leading-5 p-2 bg-gray-50 dark:bg-gray-800 rounded">
            <div v-for="(ev, i) in [...events].reverse()" :key="i" class="flex gap-2">
              <span class="text-gray-400 shrink-0">{{ ev.time }}</span>
              <span class="truncate" :class="ev.type === 'error' ? 'text-red-500' : ev.type === 'success' ? 'text-green-600' : ''">{{ ev.text }}</span>
            </div>
            <div v-if="events.length === 0" class="text-gray-400">尚无事件</div>
          </div>
        </NScrollbar>
      </div>
    </div>

    <!-- 节点摘要表 -->
    <div v-if="summary" class="shrink-0">
      <NDataTable :columns="traceColumns" :data="summary.nodes" :row-key="(r: NodeTrace) => r.key" size="small" :max-height="120" />
    </div>
  </div>
</template>
