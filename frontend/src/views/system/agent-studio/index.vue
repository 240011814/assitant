<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { NButton, NCard, NCheckbox, NDropdown, NEmpty, NInput, NModal, NPopconfirm, NSpace, NSwitch, NTag, useMessage } from 'naive-ui';
import { useAppStore } from '@/store/modules/app';
import { useAuth } from '@/hooks/business/auth';
import CanvasPanel from './components/canvas-panel.vue';
import NodeConfigPanel from './components/node-config-panel.vue';
import DebugPanel from './components/debug-panel.vue';
import type { NodeTrace } from './components/debug-panel.vue';
import { NODE_META, nodeTypeOptions } from './nodes/registry';
import { ORCHESTRATION_TEMPLATES } from './templates';
import type { OrchestrationTemplate } from './templates';
import { fetchOrchestrations, fetchCreateOrchestration, fetchUpdateOrchestration, fetchDeleteOrchestration, fetchOrchestrationResources, fetchValidateOrchestration, fetchOrchestrationDebugNode, fetchMyOrchTemplates, fetchSaveMyOrchTemplate, fetchDeleteMyOrchTemplate } from '@/service/api';
import type { AIOrchestrationItem, OrchestrationResource, OrchTemplateItem } from '@/service/api';

const message = useMessage();
const appStore = useAppStore();
const { hasAuth } = useAuth();

// ---------- 编排列表 ----------
const loading = ref(false);
const items = ref<AIOrchestrationItem[]>([]);
const currentId = ref<number | null>(null);
const currentName = ref('');
const currentDesc = ref('');
const currentEnabled = ref(true);

// ---------- 画布状态 ----------
const flowNodes = ref<any[]>([]);
const flowEdges = ref<any[]>([]);
const selectedNodeId = ref<string | null>(null);
const nodeTraces = ref<Record<string, NodeTrace>>({});
const debugRunning = ref(false);

// ---------- 资源 ----------
const resources = ref<OrchestrationResource | null>(null);




// ---------- 交互状态: undo/入参/单节点试跑/自定义模板 ----------
const undoStack = ref<string[]>([]);
const redoStack = ref<string[]>([]);
const dslParams = ref<Array<{ name: string; description: string; required: boolean }>>([]);
const showParamsModal = ref(false);
const showNodeRunModal = ref(false);
const nodeRunInput = ref('');
const nodeRunLoading = ref(false);
const nodeRunResult = ref<{ output: string; total_ms: number } | null>(null);
const nodeRunError = ref('');
const nodeRunTargetId = ref('');
const debugParamValues = ref<Record<string, string>>({});
const showSaveTemplateModal = ref(false);
const templateForm = ref({ name: '', description: '' });
const savingTemplate = ref(false);
const myTemplates = ref<OrchTemplateItem[]>([]);

// ---------- 序列化: 画布 <-> DSL ----------
function serializeDefinition(): string {
  return JSON.stringify({
    version: 1,
    params: dslParams.value.filter(p => p.name.trim()).map(p => ({ name: p.name.trim(), description: p.description, required: p.required })),
    nodes: flowNodes.value.map(n => ({
      id: n.id,
      type: n.data.nodeType as string,
      name: (n.data.name || n.id) as string,
      config: n.data.config ?? {},
      // chat_first_turn: 编排对话会话激活后跳过该节点 (后端按此字段编译直通/路由回放)
      chat_first_turn: n.data.chatFirstTurn === true ? true : undefined,
      position: { x: Math.round(n.position?.x ?? 0), y: Math.round(n.position?.y ?? 0) }
    })),
    edges: flowEdges.value.map(e => ({
      source: e.source,
      target: e.target,
      label: e.label || '',
      kind: e.data?.kind === 'loop' ? 'loop' : 'flow'
    }))
  });
}

function loadDefinition(definition: string) {
  try {
    const dsl = JSON.parse(definition || '{}');
    dslParams.value = (dsl.params || []).map((p: any) => ({ name: p.name || '', description: p.description || '', required: p.required === true }));
    flowNodes.value = (dsl.nodes || []).map((n: any) => ({
      id: n.id,
      type: 'orch',
      position: { x: n.position?.x ?? 0, y: n.position?.y ?? 0 },
      sourcePosition: 'right',
      targetPosition: 'left',
      data: { nodeType: n.type, name: n.name || n.id, config: n.config || {}, chatFirstTurn: n.chat_first_turn === true || n.chatFirstTurn === true }
    }));
    flowEdges.value = (dsl.edges || []).map((e: any, i: number) => ({
      id: `e_${i}_${e.source}_${e.target}`,
      source: e.source,
      target: e.target,
      sourceHandle: 'out',
      targetHandle: 'in',
      label: e.label || undefined,
      // 循环回边: data.kind 承载类型, class 用于虚线样式
      data: e.kind === 'loop' ? { kind: 'loop' } : undefined,
      class: e.kind === 'loop' ? 'orch-loop-edge' : undefined
    }));
  } catch {
    message.error('编排定义解析失败');
    flowNodes.value = [];
    flowEdges.value = [];
  }
  clearErrorNodes();
  // 画布整体重载是 undo 的还原点: 清空栈, 以当前状态为新基线
  undoStack.value = [];
  redoStack.value = [];
  refreshBaseline();
}

function handleDuplicateNode(nodeId: string) {
  const source = flowNodes.value.find(n => n.id === nodeId);
  if (!source) return;
  const id = `n_${Date.now().toString(36)}_${Math.floor(Math.random() * 1000)}`;
  flowNodes.value.push({
    id,
    type: 'orch',
    position: { x: (source.position?.x ?? 0) + 40, y: (source.position?.y ?? 0) + 40 },
    sourcePosition: 'right',
    targetPosition: 'left',
    data: JSON.parse(JSON.stringify(source.data))
  });
  selectedNodeId.value = id;
  pushUndo();
  clearErrorNodes();
  message.success('已复制节点');
}

function handleCanvasAddNode(nodeType: string, position: { x: number; y: number }) {
  makeNode(nodeType, position);
}

function makeNode(nodeType: string, position: { x: number; y: number }) {
  const meta = NODE_META[nodeType as keyof typeof NODE_META];
  if (!meta) return;
  const id = `n_${Date.now().toString(36)}_${Math.floor(Math.random() * 1000)}`;
  flowNodes.value.push({
    id,
    type: 'orch',
    position,
    sourcePosition: 'right',
    targetPosition: 'left',
    data: {
      nodeType,
      name: meta.label,
      config: JSON.parse(JSON.stringify(meta.defaultConfig))
    }
  });
  selectedNodeId.value = id;
  pushUndo();
  clearErrorNodes();
}

// ---------- 新建 (从模板) ----------
const showCreateModal = ref(false);

function openCreate() {
  showCreateModal.value = true;
}

function applyTemplate(t: OrchestrationTemplate) {
  currentId.value = null;
  currentName.value = '';
  currentDesc.value = '';
  currentEnabled.value = true;
  nodeTraces.value = {};
  selectedNodeId.value = null;
  // 模板定义即标准 DSL, 复用 loadDefinition 铺画布 (loadDefinition 内会清除错误高亮)
  loadDefinition(JSON.stringify(t.definition));
  showCreateModal.value = false;
  if (t.hint) {
    message.info(t.hint, { duration: 6000 });
  }
}

// ---------- 列表加载 ----------
async function loadList() {
  loading.value = true;
  try {
    const { data } = await fetchOrchestrations();
    if (data) items.value = data;
  } catch {
    /* ignore */
  }
  loading.value = false;
}

async function loadResources() {
  try {
    const { data } = await fetchOrchestrationResources();
    if (data) resources.value = data;
  } catch {
    /* ignore */
  }
}

function openItem(item: AIOrchestrationItem) {
  currentId.value = item.id;
  currentName.value = item.name;
  currentDesc.value = item.description;
  currentEnabled.value = item.enabled;
  loadDefinition(item.definition);
  nodeTraces.value = {};
  selectedNodeId.value = null;
}

// ---------- 保存 ----------
const showSaveModal = ref(false);
const saveForm = reactive({ name: '', description: '' });

function openSaveAs() {
  saveForm.name = currentName.value ? `${currentName.value}-副本` : '';
  saveForm.description = currentDesc.value;
  showSaveModal.value = true;
}

function requireName(): boolean {
  if (!saveForm.name.trim()) {
    message.warning('请输入编排名称');
    return false;
  }
  return true;
}

async function handleCreate() {
  if (!requireName()) return;
  const { data, error } = await fetchCreateOrchestration({
    name: saveForm.name.trim(),
    description: saveForm.description,
    definition: serializeDefinition(),
    enabled: currentEnabled.value
  });
  if (error || !data) {
    message.error(error?.message || '创建失败');
    return;
  }
  message.success('创建成功');
  showSaveModal.value = false;
  currentId.value = data.id;
  currentName.value = data.name;
  currentDesc.value = data.description;
  currentEnabled.value = data.enabled;
  loadList();
}

async function handleSave() {
  if (currentId.value === null) {
    openSaveAs();
    return;
  }
  const { error } = await fetchUpdateOrchestration(currentId.value, {
    name: currentName.value,
    description: currentDesc.value,
    definition: serializeDefinition(),
    enabled: currentEnabled.value
  });
  if (error) {
    message.error(error.message || '保存失败');
    return;
  }
  message.success('保存成功');
  loadList();
}

async function handleToggle(item: AIOrchestrationItem, enabled: boolean) {
  const { error } = await fetchUpdateOrchestration(item.id, { enabled });
  if (error) {
    message.error('更新失败');
    return;
  }
  item.enabled = enabled;
}

async function handleDelete(item: AIOrchestrationItem) {
  const { error } = await fetchDeleteOrchestration(item.id);
  if (error) {
    message.error('删除失败');
    return;
  }
  message.success('删除成功');
  if (currentId.value === item.id) {
    currentId.value = null;
    currentName.value = '';
    flowNodes.value = [];
    flowEdges.value = [];
  }
  loadList();
}

// ---------- 校验 ----------
const validating = ref(false);
// 出错节点集合: 校验/调试失败时画布红圈高亮并选中第一个出错节点, 画布一旦改动即清除
const errorNodeIds = ref<string[]>([]);

function setErrorNodes(ids: string[]) {
  errorNodeIds.value = [...new Set(ids.filter(Boolean))];
  if (errorNodeIds.value.length > 0) {
    selectedNodeId.value = errorNodeIds.value[0];
  }
}

function clearErrorNodes() {
  if (errorNodeIds.value.length > 0) errorNodeIds.value = [];
}

// 画布改动后旧的高亮即失效: 画布内交互 (拖动/连线/删除) 走 canvas-change 事件,
// 程序化修改 (配置面板写回/加减节点/加载定义) 在修改处显式调用 clearErrorNodes。
// 此前用 deep watch 监听整个节点图, 拖动节点时每帧全量深比较, 已移除。

async function handleValidate() {
  validating.value = true;
  try {
    const { data, error } = await fetchValidateOrchestration(serializeDefinition());
    if (error || !data) {
      message.error(error?.message || '校验失败');
      return;
    }
    if (data.valid) {
      clearErrorNodes();
      message.success(`校验通过 (${data.mode} 编排${data.warnings?.length ? `, ${data.warnings.length} 条警告` : ''})`);
      data.warnings?.forEach(w => message.warning(w, { duration: 6000 }));
    } else {
      data.errors?.forEach(e => message.error(e, { duration: 8000 }));
      setErrorNodes((data.error_items || []).map(i => i.node_id || ''));
      if (errorNodeIds.value.length > 0) {
        message.info('已定位到第一个出错节点 (红圈标记), 可在右侧修正后重新校验');
      }
    }
  } finally {
    validating.value = false;
  }
}

// ---------- 调试 ----------
const showDebug = ref(false);

function toggleDebug() {
  showDebug.value = !showDebug.value;
  if (showDebug.value && flowNodes.value.length === 0) {
    message.info('画布为空, 请先添加节点');
  }
}

function onDebugStateChange(running: boolean) {
  debugRunning.value = running;
  if (running) clearErrorNodes();
}

function onDebugTraces(traces: Record<string, NodeTrace>) {
  nodeTraces.value = traces;
}

const selectedNode = computed(() => flowNodes.value.find(n => n.id === selectedNodeId.value) || null);

function handleUpdateNodeData(data: { name: string; config: Record<string, any>; chatFirstTurn?: boolean }) {
  const node = flowNodes.value.find(n => n.id === selectedNodeId.value);
  if (node) {
    node.data = { ...node.data, ...data };
    pushUndo(800);
    clearErrorNodes();
  }
}

function handleDeleteNode(nodeId: string) {
  flowNodes.value = flowNodes.value.filter(n => n.id !== nodeId);
  flowEdges.value = flowEdges.value.filter(e => e.source !== nodeId && e.target !== nodeId);
  selectedNodeId.value = null;
  pushUndo();
  clearErrorNodes();
}

function handleDeleteEdge(edgeId: string) {
  flowEdges.value = flowEdges.value.filter(e => e.id !== edgeId);
  pushUndo();
  clearErrorNodes();
}

// ---------- undo/redo ----------
// 快照式: baseline 保存最近稳定状态, 结构性变更 (加/删/连线/配置改动) 时把 baseline 压栈。
// 位置拖动不入栈 (噪声大且价值低); 快照为完整 DSL 定义, 恢复走 loadDefinition

let undoBaseline = '';
let lastDataPushAt = 0;


function refreshBaseline() {
  undoBaseline = serializeDefinition();
}

function pushUndo(coalesceMs = 0) {
  const now = Date.now();
  if (coalesceMs > 0 && now - lastDataPushAt < coalesceMs) {
    // 连续配置改动合并为一个 undo 步
    lastDataPushAt = now;
    undoBaseline = serializeDefinition();
    return;
  }
  lastDataPushAt = now;
  if (undoBaseline) undoStack.value.push(undoBaseline);
  if (undoStack.value.length > 50) undoStack.value.shift();
  redoStack.value = [];
  undoBaseline = serializeDefinition();
}

function applyUndoSnapshot(def: string) {
  loadDefinition(def);
  undoBaseline = def;
}

function handleUndo() {
  const prev = undoStack.value.pop();
  if (!prev) return;
  redoStack.value.push(undoBaseline);
  applyUndoSnapshot(prev);
}

function handleRedo() {
  const next = redoStack.value.pop();
  if (!next) return;
  undoStack.value.push(undoBaseline);
  applyUndoSnapshot(next);
}

// 画布变更: 结构性 (add/remove) 入 undo 栈; 其余 (拖动/选中) 仅清除错误高亮
function handleCanvasChange(changes: Array<{ type: string }>) {
  clearErrorNodes();
  if (changes.some(ch => ch.type === 'add' || ch.type === 'remove')) {
    pushUndo();
  }
}

function handleUndoKeydown(e: KeyboardEvent) {
  if (!(e.ctrlKey || e.metaKey) || e.key.toLowerCase() !== 'z') return;
  const tag = (e.target as HTMLElement | null)?.tagName;
  if (tag === 'INPUT' || tag === 'TEXTAREA') return; // 输入框内保留原生撤销
  e.preventDefault();
  if (e.shiftKey) handleRedo();
  else handleUndo();
}

function openNodeRun(nodeId: string) {
  nodeRunInput.value = '';
  nodeRunResult.value = null;
  nodeRunError.value = '';
  showNodeRunModal.value = true;
  nodeRunTargetId.value = nodeId;
}


async function handleNodeRun() {
  if (!nodeRunInput.value.trim() || !nodeRunTargetId.value) return;
  nodeRunLoading.value = true;
  nodeRunError.value = '';
  try {
    const { data, error } = await fetchOrchestrationDebugNode({
      definition: serializeDefinition(),
      node_id: nodeRunTargetId.value,
      input: nodeRunInput.value,
      params: paramValues()
    });
    if (error || !data) {
      nodeRunError.value = error?.message || '试跑失败';
      return;
    }
    nodeRunResult.value = { output: data.output, total_ms: data.total_ms };
  } finally {
    nodeRunLoading.value = false;
  }
}

function paramValues(): Record<string, string> {
  const values: Record<string, string> = {};
  for (const p of dslParams.value) {
    values[p.name] = debugParamValues.value[p.name] || '';
  }
  return values;
}

// 调试面板的入参填值 (传给 DebugPanel 渲染字段)


async function handleSaveTemplate() {
  if (!templateForm.value.name.trim()) {
    message.warning('请输入模板名称');
    return;
  }
  savingTemplate.value = true;
  try {
    const { error } = await fetchSaveMyOrchTemplate({
      name: templateForm.value.name.trim(),
      description: templateForm.value.description,
      definition: serializeDefinition()
    });
    if (error) {
      message.error(error.message || '保存模板失败');
      return;
    }
    message.success('已存为我的模板');
    showSaveTemplateModal.value = false;
    loadMyTemplates();
  } finally {
    savingTemplate.value = false;
  }
}



async function loadMyTemplates() {
  const { data, error } = await fetchMyOrchTemplates();
  if (!error && data) myTemplates.value = data;
}

function applyMyTemplate(t: OrchTemplateItem) {
  currentId.value = null;
  currentName.value = t.name;
  currentDesc.value = t.description;
  currentEnabled.value = true;
  nodeTraces.value = {};
  selectedNodeId.value = null;
  loadDefinition(t.definition);
  showCreateModal.value = false;
  message.success(`已从「${t.name}」加载`);
}

async function deleteMyTemplate(t: OrchTemplateItem) {
  const { error } = await fetchDeleteMyOrchTemplate(t.id);
  if (error) {
    message.error(error.message || '删除失败');
    return;
  }
  loadMyTemplates();
}

const addNodeOptions = nodeTypeOptions().map(o => ({ label: o.label, key: o.value }));

function handleAddNode(key: string | number) {
  const x = 120 + Math.random() * 220;
  const y = 100 + Math.random() * 160;
  makeNode(String(key), { x: Math.round(x), y: Math.round(y) });
}

onMounted(() => {
  loadList();
  loadResources();
  loadMyTemplates();
  window.addEventListener('keydown', handleUndoKeydown);
});

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleUndoKeydown);
});
</script>

<template>
  <div class="h-full flex-col flex gap-3" :class="appStore.isMobile ? 'p-2' : 'p-4'">
    <!-- ====== 工具栏 ====== -->
    <NCard :bordered="false" size="small" class="shrink-0">
      <div class="flex items-center gap-3 flex-wrap">
        <NTag :bordered="false" type="info" size="small" class="shrink-0">
          {{ currentId === null ? '未保存草稿' : `#${currentId} v${items.find(i => i.id === currentId)?.version ?? ''}` }}
        </NTag>
        <span class="font-medium">{{ currentName || '未命名编排' }}</span>
        <span v-if="currentDesc" class="text-xs text-gray-500 truncate max-w-60">{{ currentDesc }}</span>
        <NSpace size="small" class="ml-auto">
          <NButton size="small" secondary quaternary :disabled="undoStack.length === 0" title="撤销 (Ctrl+Z)" @click="handleUndo">
            <template #icon><SvgIcon icon="mdi:undo" /></template>
          </NButton>
          <NButton size="small" secondary quaternary :disabled="redoStack.length === 0" title="重做 (Ctrl+Shift+Z)" @click="handleRedo">
            <template #icon><SvgIcon icon="mdi:redo" /></template>
          </NButton>
          <NButton size="small" secondary @click="showParamsModal = true">
            <template #icon><SvgIcon icon="mdi:tune" /></template>
            入参{{ dslParams.length ? `(${dslParams.length})` : '' }}
          </NButton>
          <NButton
            size="small"
            secondary
            title="把当前画布存为我的模板"
            @click="templateForm = { name: currentName || '', description: currentDesc || '' }; showSaveTemplateModal = true"
          >
            <template #icon><SvgIcon icon="mdi:collapse-all-outline" /></template>
            存为模板
          </NButton>
          <NDropdown :options="addNodeOptions" trigger="click" @select="handleAddNode">
            <NButton size="small" secondary>
              <template #icon><SvgIcon icon="mdi:plus" /></template>
              添加节点
            </NButton>
          </NDropdown>
          <NButton size="small" secondary :loading="validating" @click="handleValidate">校验</NButton>
          <NButton size="small" secondary @click="openSaveAs">
            <template #icon><SvgIcon icon="mdi:content-save-plus-outline" /></template>
            另存为
          </NButton>
          <NButton
            v-if="currentId === null ? hasAuth('system:orchestration:create') : hasAuth('system:orchestration:update')"
            size="small"
            type="primary"
            @click="handleSave"
          >
            <template #icon><SvgIcon icon="mdi:content-save-outline" /></template>
            保存
          </NButton>
          <NButton size="small" :type="showDebug ? 'warning' : 'default'" secondary @click="toggleDebug">
            <template #icon><SvgIcon icon="mdi:bug-outline" /></template>
            {{ showDebug ? '收起调试' : '调试' }}
          </NButton>
        </NSpace>
      </div>
    </NCard>

    <!-- ====== 主体 ====== -->
    <div class="flex-1 flex gap-3 min-h-0" :class="appStore.isMobile ? 'flex-col' : ''">
      <!-- 编排列表 -->
      <NCard
        :bordered="false"
        size="small"
        class="shrink-0 overflow-hidden"
        :class="appStore.isMobile ? 'max-h-48' : 'w-64'"
        title="编排列表"
      >
        <div class="mb-2">
          <NButton v-if="hasAuth('system:orchestration:create')" size="small" block secondary @click="openCreate">
            <template #icon><SvgIcon icon="mdi:plus" /></template>
            新建编排
          </NButton>
        </div>
        <div class="overflow-auto" :class="appStore.isMobile ? 'max-h-24' : 'max-h-[calc(100vh-22rem)]'">
          <div
            v-for="item in items"
            :key="item.id"
            class="p-2 rounded mb-1 cursor-pointer border border-transparent hover:bg-gray-100 dark:hover:bg-gray-800"
            :class="currentId === item.id ? 'bg-blue-50 dark:bg-blue-900/30 border-blue-200 dark:border-blue-800' : ''"
            @click="openItem(item)"
          >
            <div class="flex items-center justify-between gap-2">
              <span class="text-sm font-medium truncate">{{ item.name }}</span>
              <NSwitch
                :value="item.enabled"
                size="small"
                :disabled="!hasAuth('system:orchestration:update')"
                @update:value="(v: boolean) => handleToggle(item, v)"
              />
            </div>
            <div class="text-xs text-gray-400 mt-1 flex items-center gap-2">
              <span>v{{ item.version }}</span>
              <NPopconfirm v-if="hasAuth('system:orchestration:delete')" @positive-click="handleDelete(item)">
                <template #trigger>
                  <span class="text-red-400 hover:text-red-500 cursor-pointer">删除</span>
                </template>
                确认删除编排「{{ item.name }}」？
              </NPopconfirm>
            </div>
          </div>
          <NEmpty v-if="items.length === 0 && !loading" description="暂无编排" size="small" class="py-6" />
        </div>
      </NCard>

      <!-- 画布 -->
      <NCard :bordered="false" size="small" class="flex-1 min-h-0 flex flex-col" body-style="flex:1;display:flex;flex-direction:column;min-height:0;padding:0;">
        <CanvasPanel
          v-model:nodes="flowNodes"
          v-model:edges="flowEdges"
          :node-traces="nodeTraces"
          :selected-id="selectedNodeId"
          :refit-key="showDebug"
          :resources="resources"
          :error-node-ids="errorNodeIds"
          @select-node="(id: string | null) => (selectedNodeId = id)"
          @delete-node="handleDeleteNode"
          @duplicate-node="handleDuplicateNode"
          @add-node="handleCanvasAddNode"
          @delete-edge="handleDeleteEdge"
          @canvas-change="handleCanvasChange"
        />
      </NCard>

      <!-- 节点配置 -->
      <NCard v-if="selectedNode" :bordered="false" size="small" class="shrink-0 overflow-auto" :class="appStore.isMobile ? 'max-h-96' : 'w-80 max-h-full'">
        <NodeConfigPanel
          :node-id="selectedNode.id"
          :node-type="selectedNode.data.nodeType"
          :name="selectedNode.data.name"
          :config="selectedNode.data.config"
          :chat-first-turn="selectedNode.data.chatFirstTurn === true"
          :all-nodes="flowNodes"
          :edges="flowEdges"
          :resources="resources"
          :current-orch-id="currentId"
          @update:data="handleUpdateNodeData"
          @delete="handleDeleteNode"
          @debug-node="openNodeRun"
        />
      </NCard>
    </div>

    <!-- ====== 调试面板 ====== -->
    <NCard v-if="showDebug" :bordered="false" size="small" class="shrink-0" :style="{ height: appStore.isMobile ? '20rem' : '24rem' }">
      <DebugPanel
        :orchestration-id="currentId"
        :get-definition="serializeDefinition"
        :has-nodes="flowNodes.length > 0"
        @running-change="onDebugStateChange"
        @traces-change="onDebugTraces"
        @error-nodes="setErrorNodes"
      />
    </NCard>

    <!-- ====== 新建编排: 模板选择 ====== -->
    <NModal
      v-model:show="showCreateModal"
      preset="card"
      title="新建编排 · 选择模板"
      :style="{ width: appStore.isMobile ? '95vw' : '760px' }"
    >
      <div class="grid gap-2 max-h-[60vh] overflow-auto" :class="appStore.isMobile ? 'grid-cols-1' : 'grid-cols-3'">
        <div
          v-for="t in ORCHESTRATION_TEMPLATES"
          :key="t.key"
          class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 cursor-pointer hover:border-blue-400 dark:hover:border-blue-500 hover:bg-blue-50/50 dark:hover:bg-blue-900/20 transition-colors"
          @click="applyTemplate(t)"
        >
          <div class="flex items-center gap-2 mb-1">
            <SvgIcon :icon="t.icon" class="text-16px text-blue-500 shrink-0" />
            <span class="text-sm font-medium">{{ t.label }}</span>
          </div>
          <div class="text-xs text-gray-500 dark:text-gray-400 leading-5">{{ t.description }}</div>
        </div>
      </div>
      <div class="text-xs text-gray-400 mt-2">模板预填了节点、连线与校验配置; 依赖实例资源的节点 (工具/子编排) 加载后需补选。</div>

      <!-- 我的模板 -->
      <div v-if="myTemplates.length" class="mt-4">
        <div class="text-sm font-medium mb-2">我的模板</div>
        <div class="grid gap-2 max-h-52 overflow-auto" :class="appStore.isMobile ? 'grid-cols-1' : 'grid-cols-3'">
          <div
            v-for="t in myTemplates"
            :key="t.id"
            class="p-3 rounded-lg border border-gray-200 dark:border-gray-700 cursor-pointer hover:border-blue-400 dark:hover:border-blue-500 transition-colors relative group"
            @click="applyMyTemplate(t)"
          >
            <div class="text-sm font-medium truncate pr-5">{{ t.name }}</div>
            <div class="text-xs text-gray-500 dark:text-gray-400 leading-5 line-clamp-2">{{ t.description || '无描述' }}</div>
            <span
              class="absolute top-2 right-2 text-red-400 hover:text-red-500 text-xs hidden group-hover:block"
              @click.stop="deleteMyTemplate(t)"
            >删除</span>
          </div>
        </div>
      </div>
    </NModal>

    <!-- ====== 另存为弹窗 ====== -->
    <NModal v-model:show="showSaveModal" preset="card" title="保存编排" :style="{ width: appStore.isMobile ? '95vw' : '480px' }">
      <div class="flex flex-col gap-3">
        <NInput v-model:value="saveForm.name" placeholder="编排名称" />
        <NInput v-model:value="saveForm.description" type="textarea" :rows="2" placeholder="描述 (可选)" />
      </div>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showSaveModal = false">取消</NButton>
          <NButton type="primary" @click="handleCreate">保存</NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- ====== 编排入参编辑 ====== -->
    <NModal v-model:show="showParamsModal" preset="card" title="编排入参" :style="{ width: appStore.isMobile ? '95vw' : '560px' }">
      <p class="text-xs text-gray-500 mb-2">
        入参按名传值: 调试面板/编排对话/定时任务运行时填值, 节点模板用 <code>params.参数名</code> 引用 (与 current_time 等会话变量同命名空间)。
      </p>
      <div v-for="(p, idx) in dslParams" :key="idx" class="flex gap-2 mb-2 items-center">
        <NInput v-model:value="p.name" placeholder="参数名 (如 stock_code)" size="small" class="w-40" />
        <NInput v-model:value="p.description" placeholder="说明 (可选)" size="small" />
        <NCheckbox v-model:checked="p.required" size="small">必填</NCheckbox>
        <NButton size="tiny" quaternary type="error" @click="dslParams.splice(idx, 1)">删除</NButton>
      </div>
      <NButton size="small" dashed block @click="dslParams.push({ name: '', description: '', required: false })">
        添加参数
      </NButton>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showParamsModal = false">完成</NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- ====== 单节点试跑 ====== -->
    <NModal v-model:show="showNodeRunModal" preset="card" title="单节点试跑" :style="{ width: appStore.isMobile ? '95vw' : '640px' }">
      <div class="flex flex-col gap-3">
        <NInput v-model:value="nodeRunInput" type="textarea" :rows="3" placeholder="输入内容 (作为该节点的上游输入)" />
        <NButton type="primary" size="small" :loading="nodeRunLoading" :disabled="!nodeRunInput.trim()" @click="handleNodeRun">
          运行
        </NButton>
        <div v-if="nodeRunError" class="text-red-500 text-sm">{{ nodeRunError }}</div>
        <div v-if="nodeRunResult" class="rounded bg-gray-50 dark:bg-gray-800 p-3">
          <div class="text-xs text-gray-400 mb-1">输出 · {{ nodeRunResult.total_ms }}ms</div>
          <pre class="text-xs whitespace-pre-wrap break-all max-h-72 overflow-auto">{{ nodeRunResult.output }}</pre>
        </div>
      </div>
    </NModal>

    <!-- ====== 存为我的模板 ====== -->
    <NModal v-model:show="showSaveTemplateModal" preset="card" title="存为我的模板" :style="{ width: appStore.isMobile ? '95vw' : '480px' }">
      <div class="flex flex-col gap-3">
        <NInput v-model:value="templateForm.name" placeholder="模板名称" />
        <NInput v-model:value="templateForm.description" type="textarea" :rows="2" placeholder="描述 (可选)" />
        <p class="text-xs text-gray-400">仅自己可见; 保存前会做画布结构校验。</p>
      </div>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showSaveTemplateModal = false">取消</NButton>
          <NButton type="primary" :loading="savingTemplate" @click="handleSaveTemplate">保存</NButton>
        </NSpace>
      </template>
    </NModal>
  </div>
</template>
