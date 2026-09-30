<script setup lang="ts">
import { computed, watch } from 'vue';
import { useMessage } from 'naive-ui';
import { useVueFlow, VueFlow, type Connection, type Edge as FlowEdge, type EdgeMouseEvent, type Node as FlowNode, type NodeMouseEvent } from '@vue-flow/core';
import { Background } from '@vue-flow/background';
import { MiniMap } from '@vue-flow/minimap';
import '@vue-flow/core/dist/style.css';
import '@vue-flow/core/dist/theme-default.css';
import '@vue-flow/minimap/dist/style.css';
import type { NodeTrace } from './debug-panel.vue';

const NODE_META: Record<string, { label: string; icon: string; color: string }> = {
  agent: { label: 'Agent', icon: 'mdi:robot-outline', color: '#2080f0' },
  tool: { label: '工具', icon: 'mdi:wrench-outline', color: '#18a058' },
  template: { label: '模板', icon: 'mdi:text-box-edit-outline', color: '#f0a020' },
  branch: { label: '分支', icon: 'mdi:source-branch', color: '#d03050' },
  merge: { label: '合并', icon: 'mdi:call-merge', color: '#8a2be2' },
  end: { label: '结束', icon: 'mdi:check-circle-outline', color: '#666' }
};

const props = defineProps<{
  nodeTraces: Record<string, NodeTrace>;
  selectedId: string | null;
  debugRunning: boolean;
  refitKey?: boolean;
}>();

const nodes = defineModel<FlowNode[]>('nodes', { required: true });
const edges = defineModel<FlowEdge[]>('edges', { required: true });

const emit = defineEmits<{
  'select-node': [id: string | null];
  'delete-edge': [id: string];
}>();

const message = useMessage();

const { fitView } = useVueFlow();
// 布局变化 (调试面板展开/收起、配置面板开合) 后重新适配视口
function refit() {
  setTimeout(() => {
    fitView({ padding: 0.15, duration: 200 });
  }, 260);
}
watch(
  () => props.refitKey,
  () => refit()
);
watch(
  () => props.selectedId,
  () => refit()
);

const traceOf = computed(() => (id: string) => props.nodeTraces[id]);

function nodeSummary(data: { nodeType: string; config?: Record<string, any> }): string {
  const cfg = data.config || {};
  switch (data.nodeType) {
    case 'agent':
      return [cfg.model || '默认模型', (cfg.tools || []).length ? `工具×${(cfg.tools || []).length}` : '无工具'].filter(Boolean).join(' · ');
    case 'tool':
      return cfg.tool || '未选择工具';
    case 'template':
      return cfg.template ? '模板已配置' : '未配置模板';
    case 'branch':
      return `条件×${(cfg.cases || []).length}`;
    case 'merge':
      return '多路合并';
    default:
      return '最终输出';
  }
}

function onConnect(connection: Connection) {
  const { source, target } = connection;
  if (!source || !target || source === target) return;
  // 与后端校验规则一致的轻量前端校验
  const sourceNode = nodes.value.find(n => n.id === source);
  const targetNode = nodes.value.find(n => n.id === target);
  if (!sourceNode || !targetNode) return;
  const inEdges = edges.value.filter(e => e.target === target);
  const outEdges = edges.value.filter(e => e.source === source);
  if (inEdges.length >= 1 && targetNode.data.nodeType !== 'merge') {
    message.warning('该节点已有一条入边 (多路合并请使用合并节点)');
    return;
  }
  if (outEdges.length >= 1 && sourceNode.data.nodeType !== 'branch') {
    message.warning('该节点已有一条出边 (多路分发请使用分支节点)');
    return;
  }
  if (edges.value.some(e => e.source === source && e.target === target)) return;
  edges.value = [...edges.value, { id: `e_${Date.now().toString(36)}`, source, target }];
}

function onNodeClick(_e: NodeMouseEvent) {
  const node = _e.node;
  emit('select-node', node.id);
}

function onPaneClick() {
  emit('select-node', null);
}

function onEdgeClick(_e: EdgeMouseEvent) {
  const edge = _e.edge;
  emit('delete-edge', edge.id);
  message.info('已删除连线');
}

function nodeHeaderColor(nodeType: string, trace?: NodeTrace): string {
  if (trace) {
    if (trace.status === 'error') return '#d03050';
    if (trace.status === 'success') return '#18a058';
  }
  return NODE_META[nodeType]?.color || '#2080f0';
}
</script>

<template>
  <div class="h-full w-full">
    <VueFlow
      v-model:nodes="nodes"
      v-model:edges="edges"
      :min-zoom="0.3"
      :max-zoom="1.8"
      fit-view-on-init
      @connect="onConnect"
      @node-click="onNodeClick"
      @pane-click="onPaneClick"
      @edge-click="onEdgeClick"
    >
      <Background :gap="16" />
      <MiniMap pannable zoomable />
      <template #node-orch="nodeProps">
        <div
          class="orch-node"
          :class="{
            selected: nodeProps.id === selectedId,
            running: traceOf(nodeProps.id)?.status === 'running'
          }"
          :style="{ borderColor: nodeHeaderColor(nodeProps.data.nodeType, traceOf(nodeProps.id)) }"
        >
          <div
            class="orch-node-header"
            :style="{ background: nodeHeaderColor(nodeProps.data.nodeType, traceOf(nodeProps.id)) }"
          >
            <SvgIcon :icon="NODE_META[nodeProps.data.nodeType]?.icon || 'mdi:circle-outline'" class="text-14px" />
            <span class="truncate">{{ nodeProps.data.name || NODE_META[nodeProps.data.nodeType]?.label }}</span>
          </div>
          <div class="orch-node-body">
            <span class="text-11px text-gray-500 truncate block">{{ nodeSummary(nodeProps.data) }}</span>
          </div>
        </div>
      </template>
    </VueFlow>
  </div>
</template>

<style scoped>
.orch-node {
  width: 180px;
  border: 2px solid #2080f0;
  border-radius: 8px;
  background: #fff;
  overflow: hidden;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.08);
  transition: box-shadow 0.2s;
}
.dark .orch-node {
  background: #1e1e20;
}
.orch-node.selected {
  box-shadow: 0 0 0 3px rgba(32, 128, 240, 0.25);
}
.orch-node.running {
  box-shadow: 0 0 0 4px rgba(24, 160, 88, 0.35);
  animation: orch-pulse 1.2s ease-in-out infinite;
}
@keyframes orch-pulse {
  0%, 100% { box-shadow: 0 0 0 3px rgba(24, 160, 88, 0.35); }
  50% { box-shadow: 0 0 0 6px rgba(24, 160, 88, 0.15); }
}
.orch-node-header {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #fff;
  font-size: 12px;
  font-weight: 600;
  padding: 4px 8px;
}
.orch-node-body {
  padding: 4px 8px;
  min-height: 22px;
}
</style>
