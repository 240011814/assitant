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
  /** errorNodeIds 校验/调试失败的出错节点: 红圈高亮, 由父级在画布改动后清除 */
  errorNodeIds?: string[];
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
// 出错节点集合 (校验/调试失败): 命中的节点加红圈
const errorIdSet = computed(() => new Set(props.errorNodeIds || []));
// 每种节点类型的组件由注册表提供, 画布只按类型动态渲染
const nodeComponent = (type: string) => NODE_META[type as keyof typeof NODE_META]?.component;

/** 子Agent 节点被引用 Agent 的标题: 节点名默认都是「子Agent」, 画布上靠它区分 */
function refAgentTitle(data: any): string {
  const agentId = Number(data?.config?.agent_id || 0);
  if (!agentId) return '';
  return props.resources?.agents?.find(a => a.id === agentId)?.title || '';
}

/** 子编排节点被引用编排的名称 */
function refOrchTitle(data: any): string {
  const orchId = Number(data?.config?.orchestration_id || 0);
  if (!orchId) return '';
  return props.resources?.orchestrations?.find(o => o.id === orchId)?.name || '';
}

/** 被引用资源的显示名: 按节点类型取对应的资源列表 */
function refTitle(data: any): string {
  const type = data?.nodeType;
  if (type === 'subagent') return refAgentTitle(data);
  if (type === 'suborch') return refOrchTitle(data);
  return '';
}

// ---------- 连线 ----------
// 与后端校验规则一致: 非 merge 节点单入边、非 branch/router 节点单出边 (委派边除外);
// Agent -> 子Agent 为委派边: 一个主 Agent 可委派多个子Agent, 子Agent 不可向外连线;
// branch/router -> 上游节点为循环回边 (构成受控循环, 由分支的 max_loops 限次)
function nodeTypeOf(id?: string): string {
  return nodes.value.find(n => n.id === id)?.data.nodeType || '';
}

/** 多出线路由点: branch 与 router (LLM路由) */
const MULTI_OUT_TYPES = new Set(['branch', 'router']);

const edgeKindOf = (e: FlowEdge) => (e.data?.kind === 'loop' ? 'loop' : 'flow');

function flowOutCount(source: string): number {
  return edges.value.filter(e => e.source === source && nodeTypeOf(e.target) !== 'subagent' && edgeKindOf(e) !== 'loop').length;
}

function hasLoopOutEdge(source: string): boolean {
  return edges.value.some(e => e.source === source && edgeKindOf(e) === 'loop');
}

/** target 沿正向边能否到达 source: 能则 source->target 是一条回边 */
function reachesForward(from: string, to: string): boolean {
  const stack = [from];
  const seen = new Set<string>();
  while (stack.length) {
    const cur = stack.pop()!;
    if (cur === to) return true;
    if (seen.has(cur)) continue;
    seen.add(cur);
    for (const e of edges.value) {
      if (e.source === cur && edgeKindOf(e) !== 'loop') stack.push(e.target);
    }
  }
  return false;
}

/** 能否从 source 到 target 建一条循环回边 (后端校验为准, 这里做同规则的前置判断) */
function isValidLoopConnection(source: string, target: string): boolean {
  if (source === target) return false;
  if (!MULTI_OUT_TYPES.has(nodeTypeOf(source))) return false;
  if (nodeTypeOf(target) === 'subagent') return false;
  if (hasLoopOutEdge(source)) return false;
  // 回边必须真的成环: target 沿正向边可达 source
  return reachesForward(target, source);
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
  if (isValidLoopConnection(source, target)) return true;
  if (targetType !== 'merge' && edges.value.some(e => e.target === target)) return false;
  if (!MULTI_OUT_TYPES.has(sourceType) && flowOutCount(source) >= 1) return false;
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
    } else if (MULTI_OUT_TYPES.has(sourceType) && hasLoopOutEdge(source!) && !edges.value.some(e => e.source === source && e.target === target)) {
      message.warning('该节点已有一条循环回边 (每个分支/路由节点只能一条)');
    } else if (targetType !== 'merge' && edges.value.some(e => e.target === target)) {
      if (MULTI_OUT_TYPES.has(sourceType)) {
        // 分支/路由连向已有入边的上游: 基本都是想画回边, 但正向路径未画完时无法自动识别为回边
        message.warning(
          '普通连线不允许连向已有入边的节点; 若要建立循环, 请先画完正向路径 (目标 → … → 本节点) 再连回边, 或先把这条线画成普通连线后右键它选「转为循环回边」'
        );
      } else {
        message.warning('该节点已有一条入边 (多路合并请使用合并节点; 从分支/路由节点连回上游可建立循环回边)');
      }
    } else if (!MULTI_OUT_TYPES.has(sourceType) && flowOutCount(source) >= 1) {
      message.warning('该节点已有一条出边 (多路分发请使用分支/LLM路由节点)');
    } else {
      message.warning('不允许的连线');
    }
    return;
  }
  const { source, target } = connection;
  const isLoop = isValidLoopConnection(source!, target!);
  edges.value = [
    ...edges.value,
    {
      id: `e_${Date.now().toString(36)}`,
      source: source!,
      target: target!,
      sourceHandle: connection.sourceHandle || 'out',
      targetHandle: connection.targetHandle || 'in',
      data: isLoop ? { kind: 'loop' } : undefined,
      class: isLoop ? 'orch-loop-edge' : undefined
    }
  ];
  // 画成的是普通边但正向图已成环: 后端校验必报"存在循环连线", 当场提示用右键转回边,
  // 避免用户画完一圈才发现回边没被识别 (正向路径没画完时回边会被静默当成普通边)
  if (!isLoop && reachesForward(target!, source!)) {
    message.warning('该连线与现有正向连线构成环: 校验会报"存在循环连线"; 若要循环, 请右键这条连线选择「转为循环回边」');
  }
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

// ---------- 边转循环回边 ----------
// 回边不再只依赖画线时刻的正向可达性自动识别 (画早了会被当成普通边/被拒),
// 支持把已画的边显式转为回边 (或转回), 规则与后端校验一致

function edgeById(id?: string): FlowEdge | undefined {
  return edges.value.find(e => e.id === id);
}

/** 能否转为循环回边: 仅 branch/router 可发出, 目标非子Agent, 源未已有回边 */
function canConvertToLoop(edge: FlowEdge): boolean {
  if (edgeKindOf(edge) === 'loop') return false;
  if (!MULTI_OUT_TYPES.has(nodeTypeOf(edge.source))) return false;
  if (nodeTypeOf(edge.target) === 'subagent') return false;
  return !hasLoopOutEdge(edge.source);
}

function canConvertToLoopById(id?: string): boolean {
  const e = edgeById(id);
  return !!e && canConvertToLoop(e);
}

function isLoopEdgeById(id?: string): boolean {
  const e = edgeById(id);
  return !!e && edgeKindOf(e) === 'loop';
}

function setEdgeLoop(edgeId: string, isLoop: boolean) {
  edges.value = edges.value.map(e =>
    e.id === edgeId
      ? { ...e, data: isLoop ? { ...(e.data || {}), kind: 'loop' } : undefined, class: isLoop ? 'orch-loop-edge' : undefined }
      : e
  );
}

function menuToggleLoopEdge() {
  const edge = edgeById(ctxMenu.value.id);
  closeCtxMenu();
  if (!edge) return;
  const toLoop = edgeKindOf(edge) !== 'loop';
  if (toLoop && !canConvertToLoop(edge)) return;
  setEdgeLoop(edge.id, toLoop);
  if (!toLoop) {
    if (reachesForward(edge.target, edge.source)) {
      message.warning('已取消循环回边: 该连线与正向连线仍构成环, 校验会报"存在循环连线"');
    }
    return;
  }
  // 回边要真正生效还差两件事: 分类条件包含回边目标 + 循环上限 max_loops >= 1 (后端校验)
  const srcNode = nodes.value.find(n => n.id === edge.source);
  const cfg = srcNode?.data?.config || {};
  const cases = Array.isArray(cfg.cases) ? cfg.cases : [];
  const inCases = cases.some((cs: any) => cs?.target === edge.target);
  const maxLoops = Number(cfg.max_loops || 0);
  const missing: string[] = [];
  if (!inCases) missing.push('分类条件需包含回边目标');
  if (maxLoops < 1) missing.push('循环上限需设置 ≥1');
  if (missing.length) {
    message.warning(`已转为循环回边, 还需配置才能通过校验: ${missing.join('、')} (在该节点配置面板中设置)`);
  } else if (!reachesForward(edge.target, edge.source)) {
    message.warning('已转为循环回边, 但目标暂不能沿正向边连回本节点 (正向路径未画完), 循环不会闭合; 请补全正向连线');
  }
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
        <div class="orch-node-shell" :class="{ 'orch-node-error': errorIdSet.has(nodeProps.id) }">
          <component
            :is="nodeComponent(nodeProps.data.nodeType)"
            :data="nodeProps.data"
            :selected="nodeProps.id === selectedId"
            :status="traceOf(nodeProps.id)?.status ?? null"
            :delegated="traceOf(nodeProps.id)?.delegated ?? false"
            :title="refTitle(nodeProps.data)"
          />
        </div>
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
        <div
          v-if="canConvertToLoopById(ctxMenu.id)"
          class="ctx-menu-item"
          @click="menuToggleLoopEdge"
        >
          <SvgIcon icon="mdi:sync-alert" class="text-14px" /> 转为循环回边
        </div>
        <div
          v-else-if="isLoopEdgeById(ctxMenu.id)"
          class="ctx-menu-item"
          @click="menuToggleLoopEdge"
        >
          <SvgIcon icon="mdi:sync-off" class="text-14px" /> 取消循环回边
        </div>
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

<style>
/* 循环回边: 橙色虚线 (边渲染在 VueFlow 内部 SVG, 需用全局样式) */
.orch-loop-edge .vue-flow__edge-path {
  stroke: #d97706;
  stroke-dasharray: 7 4;
}

/* 校验/调试失败的出错节点: 红圈标记 (套在节点组件外层, 不侵入各节点组件) */
.orch-node-shell {
  border-radius: 8px;
}
.orch-node-error {
  outline: 2px solid #d03050;
  outline-offset: 2px;
}
</style>
