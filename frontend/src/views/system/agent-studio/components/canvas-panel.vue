<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useMessage } from 'naive-ui';
import { useVueFlow, VueFlow, type Connection, type Edge as FlowEdge, type EdgeMouseEvent, type Node as FlowNode, type NodeMouseEvent } from '@vue-flow/core';
import { Background } from '@vue-flow/background';
import { MiniMap } from '@vue-flow/minimap';
import '@vue-flow/core/dist/style.css';
import '@vue-flow/core/dist/theme-default.css';
import '@vue-flow/minimap/dist/style.css';
import type { NodeTrace } from './debug-panel.vue';
import type { OrchestrationResource } from '@/service/api';
import { NODE_META } from '../nodes/registry';

const props = defineProps<{
  nodeTraces: Record<string, NodeTrace>;
  selectedId: string | null;
  debugRunning: boolean;
  refitKey?: boolean;
  /** resources 画布可用资源 (仅用于把子Agent 显示成被引用的 Agent 标题) */
  resources?: OrchestrationResource | null;
}>();

const nodes = defineModel<FlowNode[]>('nodes', { required: true });
const edges = defineModel<FlowEdge[]>('edges', { required: true });

const emit = defineEmits<{
  'select-node': [id: string | null];
  'delete-node': [id: string];
  'duplicate-node': [id: string];
  'add-node': [type: string, position: { x: number; y: number }];
  'delete-edge': [id: string];
}>();

const message = useMessage();
const { fitView, screenToFlowCoordinate } = useVueFlow();

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
// 每种节点类型的组件由注册表提供, 画布只按类型动态渲染
const nodeComponent = (type: string) => NODE_META[type as keyof typeof NODE_META]?.component;

/** 子Agent 节点被引用 Agent 的标题: 节点名默认都是「子Agent」, 画布上靠它区分 */
function refAgentTitle(data: any): string {
  const agentId = Number(data?.config?.agent_id || 0);
  if (!agentId) return '';
  return props.resources?.agents?.find(a => a.id === agentId)?.title || '';
}

// ---------- 连线 ----------
// 与后端校验规则一致: 非 merge 节点单入边、非 branch 节点单出边 (委派边除外);
// Agent -> 子Agent 为委派边: 一个主 Agent 可委派多个子Agent, 子Agent 不可向外连线
function nodeTypeOf(id?: string): string {
  return nodes.value.find(n => n.id === id)?.data.nodeType || '';
}

function flowOutCount(source: string): number {
  return edges.value.filter(e => e.source === source && nodeTypeOf(e.target) !== 'subagent').length;
}

function isValidConnection(connection: Connection): boolean {
  const { source, target } = connection;
  if (!source || !target || source === target) return false;
  const sourceType = nodeTypeOf(source);
  const targetType = nodeTypeOf(target);
  if (sourceType === 'subagent') return false;
  if (targetType === 'subagent') {
    return sourceType === 'agent' && !edges.value.some(e => e.target === target);
  }
  if (edges.value.some(e => e.source === source && e.target === target)) return false;
  if (targetType !== 'merge' && edges.value.some(e => e.target === target)) return false;
  if (sourceType !== 'branch' && flowOutCount(source) >= 1) return false;
  return true;
}

function onConnect(connection: Connection) {
  if (!isValidConnection(connection)) {
    const { source, target } = connection;
    const sourceType = nodeTypeOf(source);
    const targetType = nodeTypeOf(target);
    if (sourceType === 'subagent') {
      message.warning('子Agent 不能向外连线 (委派方向: 主 Agent -> 子Agent)');
    } else if (targetType === 'subagent') {
      if (sourceType !== 'agent') message.warning('子Agent 只能由 Agent 节点委派');
      else message.warning('该子Agent 已有主 Agent');
    } else if (targetType !== 'merge' && edges.value.some(e => e.target === target)) {
      message.warning('该节点已有一条入边 (多路合并请使用合并节点)');
    } else if (sourceType !== 'branch' && flowOutCount(source) >= 1) {
      message.warning('该节点已有一条出边 (多路分发请使用分支节点)');
    } else {
      message.warning('不允许的连线');
    }
    return;
  }
  edges.value = [...edges.value, {
    id: `e_${Date.now().toString(36)}`,
    source: connection.source!,
    target: connection.target!,
    sourceHandle: connection.sourceHandle || 'out',
    targetHandle: connection.targetHandle || 'in'
  }];
}

function onNodeClick(e: NodeMouseEvent) {
  closeCtxMenu();
  emit('select-node', e.node.id);
}

function onPaneClick() {
  closeCtxMenu();
  emit('select-node', null);
}

function onEdgeClick(_e: EdgeMouseEvent) {
  closeCtxMenu();
  emit('select-node', null);
}

// ---------- 右键菜单 ----------
interface CtxMenuState {
  show: boolean;
  x: number;
  y: number;
  kind: 'node' | 'edge' | 'pane';
  id?: string;
  nodeType?: string;
}

const ctxMenu = ref<CtxMenuState>({ show: false, x: 0, y: 0, kind: 'pane' });

function closeCtxMenu() {
  ctxMenu.value.show = false;
}

// 自定义组件事件上不能用 .prevent/.stop 修饰符 (payload 不是原生事件),
// 在这里手动阻止默认右键菜单并阻断向 pane 的冒泡
function openNodeMenu(e: NodeMouseEvent) {
  const native = e.event as MouseEvent;
  native.preventDefault();
  native.stopPropagation();
  ctxMenu.value = { show: true, x: native.clientX, y: native.clientY, kind: 'node', id: e.node.id, nodeType: e.node.data.nodeType };
  emit('select-node', e.node.id);
}

function openEdgeMenu(e: EdgeMouseEvent) {
  const native = e.event as MouseEvent;
  native.preventDefault();
  native.stopPropagation();
  ctxMenu.value = { show: true, x: native.clientX, y: native.clientY, kind: 'edge', id: e.edge.id };
}

function openPaneMenu(e: MouseEvent) {
  e.preventDefault();
  ctxMenu.value = { show: true, x: e.clientX, y: e.clientY, kind: 'pane' };
}

function menuDeleteNode() {
  if (ctxMenu.value.id) emit('delete-node', ctxMenu.value.id);
  closeCtxMenu();
}

function menuDuplicateNode() {
  if (ctxMenu.value.id) emit('duplicate-node', ctxMenu.value.id);
  closeCtxMenu();
}

function menuDeleteEdge() {
  if (ctxMenu.value.id) emit('delete-edge', ctxMenu.value.id);
  closeCtxMenu();
}

function menuAddNode(type: string) {
  const point = screenToFlowCoordinate({ x: ctxMenu.value.x, y: ctxMenu.value.y });
  emit('add-node', type, { x: Math.round(point.x), y: Math.round(point.y) });
  closeCtxMenu();
}

</script>

<template>
  <div class="h-full w-full relative" @click="closeCtxMenu">
    <VueFlow
      v-model:nodes="nodes"
      v-model:edges="edges"
      :min-zoom="0.3"
      :max-zoom="1.8"
      :delete-key-code="['Backspace', 'Delete']"
      fit-view-on-init
      @connect="onConnect"
      @node-click="onNodeClick"
      @pane-click="onPaneClick"
      @edge-click="onEdgeClick"
      @node-context-menu="openNodeMenu"
      @edge-context-menu="openEdgeMenu"
      @pane-context-menu="openPaneMenu"
    >
      <Background :gap="16" />
      <MiniMap pannable zoomable />
      <template #node-orch="nodeProps">
        <component
          :is="nodeComponent(nodeProps.data.nodeType)"
          :data="nodeProps.data"
          :selected="nodeProps.id === selectedId"
          :status="traceOf(nodeProps.id)?.status ?? null"
          :delegated="traceOf(nodeProps.id)?.delegated ?? false"
          :title="refAgentTitle(nodeProps.data)"
        />
      </template>
    </VueFlow>

    <!-- 右键菜单 -->
    <div
      v-if="ctxMenu.show"
      class="ctx-menu"
      :style="{ left: `${ctxMenu.x}px`, top: `${ctxMenu.y}px` }"
      @click.stop
    >
      <template v-if="ctxMenu.kind === 'node'">
        <div class="ctx-menu-item" @click="menuDuplicateNode">
          <SvgIcon icon="mdi:content-copy" class="text-14px" /> 复制节点
        </div>
        <div class="ctx-menu-item ctx-menu-danger" @click="menuDeleteNode">
          <SvgIcon icon="mdi:trash-can-outline" class="text-14px" /> 删除节点
        </div>
      </template>
      <template v-else-if="ctxMenu.kind === 'edge'">
        <div class="ctx-menu-item ctx-menu-danger" @click="menuDeleteEdge">
          <SvgIcon icon="mdi:minus" class="text-14px" /> 删除连线
        </div>
      </template>
      <template v-else>
        <div
          v-for="(meta, type) in NODE_META"
          :key="type"
          class="ctx-menu-item"
          @click="menuAddNode(type)"
        >
          <SvgIcon :icon="meta.icon" class="text-14px" /> 添加{{ meta.label }}节点
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.ctx-menu {
  position: fixed;
  z-index: 1000;
  min-width: 150px;
  background: #fff;
  border-radius: 6px;
  box-shadow: 0 3px 14px rgba(0, 0, 0, 0.15);
  padding: 4px 0;
  font-size: 13px;
}
.dark .ctx-menu {
  background: #26262a;
}
.ctx-menu-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 14px;
  cursor: pointer;
  color: #333;
}
.dark .ctx-menu-item {
  color: #ddd;
}
.ctx-menu-item:hover {
  background: #f3f3f5;
}
.dark .ctx-menu-item:hover {
  background: #333338;
}
.ctx-menu-danger {
  color: #d03050;
}
</style>
