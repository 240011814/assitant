<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { NButton, NCard, NDropdown, NEmpty, NInput, NModal, NPopconfirm, NSelect, NSpace, NSwitch, NTag, useMessage } from 'naive-ui';
import { useAppStore } from '@/store/modules/app';
import { useAuth } from '@/hooks/business/auth';
import CanvasPanel from './components/canvas-panel.vue';
import NodeConfigPanel from './components/node-config-panel.vue';
import DebugPanel from './components/debug-panel.vue';
import type { NodeTrace } from './components/debug-panel.vue';
import { NODE_META, nodeTypeOptions } from './nodes/registry';
import {
  fetchOrchestrations,
  fetchCreateOrchestration,
  fetchUpdateOrchestration,
  fetchDeleteOrchestration,
  fetchOrchestrationResources,
  fetchValidateOrchestration,
  type AIOrchestrationItem,
  type OrchestrationResource,
  type OrchNode,
  type OrchestrationValidateResult
} from '@/service/api';

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
interface FlowNodeData {
  nodeType: OrchNode['type'];
  name: string;
  config: Record<string, any>;
}
const flowNodes = ref<any[]>([]);
const flowEdges = ref<any[]>([]);
const selectedNodeId = ref<string | null>(null);
const nodeTraces = ref<Record<string, NodeTrace>>({});
const debugRunning = ref(false);

// ---------- 资源 ----------
const resources = ref<OrchestrationResource | null>(null);



// ---------- 序列化: 画布 <-> DSL ----------
function serializeDefinition(): string {
  return JSON.stringify({
    version: 1,
    nodes: flowNodes.value.map(n => ({
      id: n.id,
      type: n.data.nodeType as string,
      name: (n.data.name || n.id) as string,
      config: n.data.config ?? {},
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
    flowNodes.value = (dsl.nodes || []).map((n: any) => ({
      id: n.id,
      type: 'orch',
      position: { x: n.position?.x ?? 0, y: n.position?.y ?? 0 },
      sourcePosition: 'right',
      targetPosition: 'left',
      data: { nodeType: n.type, name: n.name || n.id, config: n.config || {} }
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
}

// 新建编排时的示例链: 模板 -> Agent -> 结束
function seedSampleDefinition() {
  flowNodes.value = [];
  flowEdges.value = [];
  makeNode('template', { x: 60, y: 140 });
  const tplId = selectedNodeId.value!;
  (flowNodes.value[0].data.config as Record<string, any>).template = '请处理以下内容:\n{{.Input}}';
  makeNode('agent', { x: 360, y: 120 });
  const agentId = selectedNodeId.value!;
  makeNode('end', { x: 660, y: 140 });
  selectedNodeId.value = null;
  flowEdges.value.push(
    { id: 'e_seed_1', source: tplId, target: agentId, sourceHandle: 'out', targetHandle: 'in' },
    { id: 'e_seed_2', source: agentId, target: flowNodes.value[2].id, sourceHandle: 'out', targetHandle: 'in' }
  );
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

function openCreateWithSample() {
  currentId.value = null;
  currentName.value = '';
  currentDesc.value = '';
  currentEnabled.value = true;
  nodeTraces.value = {};
  selectedNodeId.value = null;
  seedSampleDefinition();
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

// 画布任何改动后旧的高亮即失效
watch([flowNodes, flowEdges], clearErrorNodes, { deep: true });

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

function handleUpdateNodeData(data: { name: string; config: Record<string, any> }) {
  const node = flowNodes.value.find(n => n.id === selectedNodeId.value);
  if (node) {
    node.data = { ...node.data, ...data };
  }
}

function handleDeleteNode(nodeId: string) {
  flowNodes.value = flowNodes.value.filter(n => n.id !== nodeId);
  flowEdges.value = flowEdges.value.filter(e => e.source !== nodeId && e.target !== nodeId);
  selectedNodeId.value = null;
}

function handleDeleteEdge(edgeId: string) {
  flowEdges.value = flowEdges.value.filter(e => e.id !== edgeId);
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
          <NButton v-if="hasAuth('system:orchestration:create')" size="small" block secondary @click="openCreateWithSample">
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
      <NCard :bordered="false" size="small" class="flex-1 min-h-0 flex flex-col" :body-style="'flex:1;display:flex;flex-direction:column;min-height:0;padding:0;'">
        <CanvasPanel
          v-model:nodes="flowNodes"
          v-model:edges="flowEdges"
          :node-traces="nodeTraces"
          :selected-id="selectedNodeId"
          :debug-running="debugRunning"
          :refit-key="showDebug"
          :resources="resources"
          :error-node-ids="errorNodeIds"
          @select-node="(id: string | null) => (selectedNodeId = id)"
          @delete-node="handleDeleteNode"
          @duplicate-node="handleDuplicateNode"
          @add-node="handleCanvasAddNode"
          @delete-edge="handleDeleteEdge"
        />
      </NCard>

      <!-- 节点配置 -->
      <NCard v-if="selectedNode" :bordered="false" size="small" class="shrink-0 overflow-auto" :class="appStore.isMobile ? 'max-h-96' : 'w-80 max-h-full'">
        <NodeConfigPanel
          :node-id="selectedNode.id"
          :node-type="selectedNode.data.nodeType"
          :name="selectedNode.data.name"
          :config="selectedNode.data.config"
          :all-nodes="flowNodes"
          :edges="flowEdges"
          :resources="resources"
          :current-orch-id="currentId"
          @update:data="handleUpdateNodeData"
          @delete="handleDeleteNode"
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
  </div>
</template>
