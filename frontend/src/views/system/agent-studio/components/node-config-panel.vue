<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { NButton, NDynamicInput, NFormItem, NInput, NInputNumber, NSelect, useMessage } from 'naive-ui';
import type { Node as FlowNode } from '@vue-flow/core';
import type { OrchestrationResource } from '@/service/api';
import { NODE_META, type OrchNodeType } from '../nodes/registry';
import {
  fetchGetUserPrompt,
  fetchSaveUserPrompt,
  fetchSwitchUserPrompt,
  fetchDeleteUserPromptVersion
} from '@/service/api/ai';
import { useAuth } from '@/hooks/business/auth';

const props = defineProps<{
  nodeId: string;
  nodeType: OrchNodeType;
  name: string;
  config: Record<string, any>;
  allNodes: FlowNode[];
  edges: any[];
  resources: OrchestrationResource | null;
  /** currentOrchId 当前编排 id: 子编排引用下拉里排除自己 (避免自引用) */
  currentOrchId?: number | null;
}>();

const emit = defineEmits<{
  'update:data': [data: { name: string; config: Record<string, any> }];
  delete: [id: string];
}>();

// 面板是该节点配置的唯一编辑入口: 仅在切换节点时从 props 初始化本地副本,
// 之后不再反向同步 (否则属性回写 -> props 变化 -> 重置本地, 下拉选择会被冲掉)
function initLocal() {
  localName.value = props.name;
  const cfg = JSON.parse(JSON.stringify(props.config || {}));
  // 兼容历史数据缺省的数组/字符串字段, 避免受控组件值 undefined
  if ((props.nodeType === 'agent' || props.nodeType === 'subagent') && !Array.isArray(cfg.tools)) cfg.tools = [];
  if ((props.nodeType === 'agent' || props.nodeType === 'subagent') && cfg.max_retries == null) cfg.max_retries = 0;
  if ((props.nodeType === 'branch' || props.nodeType === 'router') && !Array.isArray(cfg.cases)) cfg.cases = [];
  if ((props.nodeType === 'branch' || props.nodeType === 'router') && cfg.max_loops == null) cfg.max_loops = 0;
  if (props.nodeType === 'branch' && !cfg.mode) cfg.mode = 'route';
  if (props.nodeType === 'subagent') {
    if (!cfg.agent_id) cfg.agent_id = 0;
    if (cfg.max_iterations == null) cfg.max_iterations = 0;
    if (cfg.timeout_seconds == null) cfg.timeout_seconds = 0;
  }
  if (props.nodeType === 'suborch' && !cfg.orchestration_id) cfg.orchestration_id = 0;
  localConfig.value = cfg;
}

const localName = ref(props.name);
const localConfig = ref<Record<string, any>>(JSON.parse(JSON.stringify(props.config || {})));

watch(
  () => props.nodeId,
  () => initLocal(),
  { immediate: true }
);

// 配置变更立即同步回画布节点
watch([localName, localConfig], () => {
  emit('update:data', { name: localName.value, config: JSON.parse(JSON.stringify(localConfig.value)) });
}, { deep: true });

const toolOptions = computed(() =>
  (props.resources?.tools || []).map(t => ({ label: t.display_name ? `${t.display_name} (${t.name})` : t.name, value: t.name }))
);

const modelOptions = computed(() => [
  { label: '默认模型', value: '' },
  ...(props.resources?.models || []).map(m => ({ label: m.display_name ? `${m.display_name} (${m.model_code})` : m.model_code, value: m.model_code }))
]);

const otherNodeOptions = computed(() =>
  props.allNodes.filter(n => n.id !== props.nodeId).map(n => ({ label: String(n.data?.name || n.id), value: String(n.id) }))
);

// 引用 Agent 下拉: 只列出后端放行的子Agent (agent_type=subagent 或填了委派说明),
// 标题后带委派说明便于挑选; 说明也会自动带进节点/委派工具描述
const agentOptions = computed(() => [
  { label: '内联定义 (下方系统提示词)', value: 0 },
  ...(props.resources?.agents || []).map(a => {
    const hint = String(a.delegation_description || a.description || '').trim();
    return { label: hint ? `#${a.id} ${a.title} — ${hint}` : `#${a.id} ${a.title}`, value: a.id };
  })
]);

// 委派说明决定主 Agent 何时调用该子Agent: 节点与被引用 Agent 都没写时给出强提示
const delegateDescMissing = computed(() => {
  if (props.nodeType !== 'subagent') return false;
  if (String(localConfig.value.description || '').trim()) return false;
  const agentId = Number(localConfig.value.agent_id || 0);
  if (!agentId) return true;
  const referenced = (props.resources?.agents || []).find(a => a.id === agentId);
  return !String(referenced?.delegation_description || '').trim() && !String(referenced?.description || '').trim();
});

// 选中被引用 Agent 时把它的委派说明带进节点 (节点留空时才填, 不覆盖手填内容)
function onAgentIdChange(id: number) {
  if (!id) return;
  const cfg = localConfig.value;
  if (String(cfg.description || '').trim()) return;
  const referenced = (props.resources?.agents || []).find(a => a.id === id);
  const hint = String(referenced?.delegation_description || referenced?.description || '').trim();
  if (hint) cfg.description = hint;
}

// 当前 Agent 节点是否挂载了子Agent (经委派边进来), 用于提示"委派指引会自动注入提示词"
const hasSubAgentIncoming = computed(() => {
  if (props.nodeType !== 'agent') return false;
  return (props.edges || []).some(
    e => e.source === props.nodeId && props.allNodes.find(n => n.id === e.target)?.data?.nodeType === 'subagent'
  );
});

// 分支/路由节点是否带循环回边 (orange 虚线边): 带回边时必须设置循环上限
const hasLoopOutEdge = computed(() =>
  (props.edges || []).some(e => e.source === props.nodeId && e.data?.kind === 'loop')
);

const branchTypeOptions = [
  { label: '包含 (contains)', value: 'contains' },
  { label: '等于 (equals)', value: 'equals' },
  { label: '正则 (regex)', value: 'regex' }
];

// NDynamicInput 需要可写绑定
const casesValue = computed({
  get: () => (Array.isArray(localConfig.value.cases) ? localConfig.value.cases : []),
  set: v => {
    localConfig.value.cases = v;
  }
});

// 分支条件新建行
function createBranchCase() {
  return { type: 'contains', value: '', target: '' };
}

// LLM 路由分类新建行
function createRouterCase() {
  return { label: '', description: '', target: '' };
}

// 子编排引用下拉: 排除当前正在编辑的编排 (自引用会被后端判为循环)
const orchestrationOptions = computed(() =>
  (props.resources?.orchestrations || [])
    .filter(o => !props.currentOrchId || o.id !== props.currentOrchId)
    .map(o => {
      const desc = String(o.description || '').trim();
      const suffix = o.enabled ? '' : ' (未启用)';
      return { label: desc ? `#${o.id} ${o.name} — ${desc}${suffix}` : `#${o.id} ${o.name}${suffix}`, value: o.id };
    })
);

// ---------- 提示词版本 (编排 Agent 节点, 与用户提示词共用 user_prompts 表) ----------
const message = useMessage();
const { hasAuth } = useAuth();
const canManagePrompt = computed(() => hasAuth('ai:prompt:manage'));

interface PromptVersion {
  id: number;
  version: number;
  remark: string;
  is_active: boolean;
  custom_prompt: string;
}
const promptVersions = ref<PromptVersion[]>([]);
const selectedVersionId = ref<number | null>(null);
const promptVersionLoading = ref(false);
const activeVersion = computed(() => promptVersions.value.find(v => v.is_active) || null);
const promptVersionOptions = computed(() =>
  promptVersions.value.map(v => ({
    label: `v${v.version}${v.is_active ? ' (启用)' : ''}${v.remark ? ` — ${v.remark}` : ''}`,
    value: v.id
  }))
);

async function loadPromptVersions() {
  const versioned = props.nodeType === 'agent' || props.nodeType === 'subagent';
  if (!versioned || !props.currentOrchId || !canManagePrompt.value) {
    promptVersions.value = [];
    return;
  }
  promptVersionLoading.value = true;
  try {
    const { data, error } = await fetchGetUserPrompt(props.currentOrchId, props.nodeId);
    if (!error && data) {
      promptVersions.value = (data.versions || []) as PromptVersion[];
      selectedVersionId.value = promptVersions.value.find(v => v.is_active)?.id ?? null;
    }
  } finally {
    promptVersionLoading.value = false;
  }
}

watch(
  () => [props.nodeType, props.nodeId, props.currentOrchId],
  () => loadPromptVersions(),
  { immediate: true }
);

async function handleSavePromptVersion() {
  if (!props.currentOrchId) return;
  const content = String(localConfig.value.system_prompt || '').trim();
  if (!content) {
    message.warning('画布提示词为空, 无法存为版本');
    return;
  }
  const { error } = await fetchSaveUserPrompt(props.currentOrchId, content, '', props.nodeId);
  if (error) {
    message.error(error.message || '保存版本失败');
    return;
  }
  message.success('已存为新版本并启用 (运行时以版本为准)');
  loadPromptVersions();
}

async function handleSwitchPromptVersion() {
  if (!props.currentOrchId || !selectedVersionId.value) {
    message.warning('请先选择要启用的版本');
    return;
  }
  const { error } = await fetchSwitchUserPrompt(props.currentOrchId, selectedVersionId.value, props.nodeId);
  if (error) {
    message.error(error.message || '切换版本失败');
    return;
  }
  message.success('已切换启用版本');
  loadPromptVersions();
}

async function handleDeletePromptVersion() {
  if (!props.currentOrchId || !selectedVersionId.value) {
    message.warning('请先选择要删除的版本');
    return;
  }
  const { error } = await fetchDeleteUserPromptVersion(props.currentOrchId, selectedVersionId.value, props.nodeId);
  if (error) {
    message.error(error.message || '删除版本失败');
    return;
  }
  message.success('已删除版本');
  loadPromptVersions();
}

const typeLabel = computed(() => `${NODE_META[props.nodeType as OrchNodeType]?.label ?? props.nodeType} 节点`);
</script>

<template>
  <div class="flex flex-col gap-3">
    <div class="flex items-center justify-between">
      <span class="text-sm font-bold">{{ typeLabel }}</span>
      <NButton size="tiny" quaternary type="error" @click="emit('delete', nodeId)">删除节点</NButton>
    </div>

    <NFormItem label="名称" label-placement="left" label-width="72" size="small">
      <NInput v-model:value="localName" size="small" placeholder="节点显示名" />
    </NFormItem>

    <!-- Agent 节点 -->
    <template v-if="nodeType === 'agent'">
      <div v-if="hasSubAgentIncoming" class="text-11px text-gray-400 mb-2 leading-5">
        已挂载子 Agent: 它们的名称/职责会自动并入本节点的系统提示词(主管委派指引), 无需在此重复描述; 想让主 Agent 更主动委派, 可在下方提示词里写明分工。
      </div>
      <NFormItem label="模型" label-placement="left" label-width="72" size="small">
        <NSelect v-model:value="localConfig.model" :options="modelOptions" size="small" />
      </NFormItem>
      <NFormItem label="系统提示词" label-placement="left" label-width="72" size="small">
        <NInput
          v-model:value="localConfig.system_prompt"
          type="textarea"
          :rows="5"
          size="small"
          placeholder="支持模板变量: {{.Input}} {{.current_time}} {{.user_id}} {{.user_profile}}"
        />
      </NFormItem>
      <NFormItem label="工具" label-placement="left" label-width="72" size="small">
        <NSelect
          v-model:value="localConfig.tools"
          multiple
          clearable
          size="small"
          :options="toolOptions"
          placeholder="选择 ReAct 可调用的工具"
        />
      </NFormItem>
      <NFormItem label="最大轮次" label-placement="left" label-width="72" size="small">
        <NInputNumber v-model:value="localConfig.max_iterations" :min="0" :max="100" size="small" class="w-full" placeholder="0 = 默认 25" />
      </NFormItem>
      <NFormItem label="失败重试" label-placement="left" label-width="72" size="small">
        <NInputNumber v-model:value="localConfig.max_retries" :min="0" :max="10" size="small" class="w-full" placeholder="0 = 默认 2 次" />
      </NFormItem>
    </template>

    <!-- 工具节点 -->
    <template v-else-if="nodeType === 'tool'">
      <NFormItem label="工具" label-placement="left" label-width="72" size="small">
        <NSelect v-model:value="localConfig.tool" :options="toolOptions" size="small" placeholder="选择要执行的工具" />
      </NFormItem>
      <p class="text-11px text-gray-400 leading-5">
        上游内容作为工具参数: JSON 直接使用, 非 JSON 包装为 {"input": ...}。输出为 tool 角色消息。
      </p>
    </template>

    <!-- 模板节点 -->
    <template v-else-if="nodeType === 'template'">
      <NFormItem label="模板" label-placement="left" label-width="72" size="small">
        <NInput
          v-model:value="localConfig.template"
          type="textarea"
          :rows="6"
          size="small"
          placeholder="Go text/template, 变量: {{.Input}} {{.current_time}} {{.user_id}} {{.user_profile}}"
        />
      </NFormItem>
    </template>

    <!-- 分支节点 -->
    <template v-else-if="nodeType === 'branch'">
      <NFormItem label="分发模式" label-placement="left" label-width="72" size="small">
        <NSelect
          v-model:value="localConfig.mode"
          size="small"
          :options="[
            { label: '互斥路由 (按条件选一路)', value: 'route' },
            { label: '并行分发 (全部目标同时执行)', value: 'parallel' }
          ]"
          placeholder="默认互斥路由"
        />
      </NFormItem>
      <template v-if="localConfig.mode === 'parallel'">
        <div class="text-11px text-orange-500 leading-5 mb-1">
          并行分发: 所有画布出边指向的目标同时执行, 条件不生效; 每条路径需汇入合并节点输出。
        </div>
      </template>
      <template v-else>
        <div class="text-11px text-gray-400">按上游内容路由, 命中顺序自上而下; 分支目标需与画布连线一致。</div>
        <NDynamicInput v-model:value="casesValue" :on-create="createBranchCase">
          <template #default="{ value }">
            <div class="flex flex-col gap-1 w-full">
              <div class="flex gap-1">
                <NSelect v-model:value="value.type" size="small" :options="branchTypeOptions" class="w-32" />
                <NInput v-model:value="value.value" size="small" placeholder="匹配值" />
              </div>
              <NSelect v-model:value="value.target" size="small" :options="otherNodeOptions" placeholder="目标节点" />
            </div>
          </template>
        </NDynamicInput>
        <NFormItem label="默认分支" label-placement="left" label-width="72" size="small">
          <NSelect v-model:value="localConfig.default_target" :options="otherNodeOptions" size="small" placeholder="无条件命中时的目标" />
        </NFormItem>
      </template>
      <template v-if="hasLoopOutEdge">
        <NFormItem label="循环上限" label-placement="left" label-width="72" size="small">
          <NInputNumber v-model:value="localConfig.max_loops" :min="1" :max="20" size="small" class="w-full" placeholder="回边最多执行次数" />
        </NFormItem>
        <p class="text-11px text-orange-500 leading-5 mb-1">
          该分支带循环回边: 命中回边最多 {{ localConfig.max_loops || '?' }} 次, 超过后强制走退出分支 (防止评审一直不通过导致死循环)。
        </p>
      </template>
      <template v-else>
        <NFormItem label="循环上限" label-placement="left" label-width="72" size="small">
          <NInputNumber v-model:value="localConfig.max_loops" :min="0" :max="20" size="small" class="w-full" placeholder="连循环回边后必填 (≥1)" />
        </NFormItem>
        <p class="text-11px text-gray-400 leading-5 mb-1">
          未使用循环回边时保持 0; 从分支连一条线回上游后右键该连线「转为循环回边」, 并把上限设为 ≥1。
        </p>
      </template>
    </template>

    <!-- LLM 路由节点 -->
    <template v-else-if="nodeType === 'router'">
      <div class="text-11px text-gray-400">由模型把上游内容分类成唯一标签并按标签路由; 分类目标需与画布连线一致。</div>
      <NFormItem label="模型" label-placement="left" label-width="72" size="small">
        <NSelect v-model:value="localConfig.model" :options="modelOptions" size="small" />
      </NFormItem>
      <NFormItem label="判定规则" label-placement="left" label-width="72" size="small">
        <NInput
          v-model:value="localConfig.instructions"
          type="textarea"
          :rows="3"
          size="small"
          placeholder="分类的判定规则与边界说明 (如: 咨询购买流程归售前; 已下单问题归售后)"
        />
      </NFormItem>
      <NDynamicInput v-model:value="casesValue" :on-create="createRouterCase">
        <template #default="{ value }">
          <div class="flex flex-col gap-1 w-full">
            <NInput v-model:value="value.label" size="small" placeholder="标签 (模型输出的分类名, 如: 售前)" />
            <NInput v-model:value="value.description" size="small" placeholder="判定说明 (帮助模型区分相近意图)" />
            <NSelect v-model:value="value.target" size="small" :options="otherNodeOptions" placeholder="目标节点" />
          </div>
        </template>
      </NDynamicInput>
      <NFormItem label="默认目标" label-placement="left" label-width="72" size="small">
        <NSelect v-model:value="localConfig.default_target" :options="otherNodeOptions" size="small" placeholder="模型输出无法归类时的目标" />
      </NFormItem>
      <template v-if="hasLoopOutEdge">
        <NFormItem label="循环上限" label-placement="left" label-width="72" size="small">
          <NInputNumber v-model:value="localConfig.max_loops" :min="1" :max="20" size="small" class="w-full" placeholder="回边最多执行次数" />
        </NFormItem>
        <p class="text-11px text-orange-500 leading-5 mb-1">
          该路由带循环回边: 命中回边最多 {{ localConfig.max_loops || '?' }} 次, 超过后强制走退出目标。
        </p>
      </template>
      <template v-else>
        <NFormItem label="循环上限" label-placement="left" label-width="72" size="small">
          <NInputNumber v-model:value="localConfig.max_loops" :min="0" :max="20" size="small" class="w-full" placeholder="连循环回边后必填 (≥1)" />
        </NFormItem>
        <p class="text-11px text-gray-400 leading-5 mb-1">
          未使用循环回边时保持 0; 从路由连一条线回上游后右键该连线「转为循环回边」, 并把上限设为 ≥1。
        </p>
      </template>
    </template>

    <!-- 字段提取节点 -->
    <template v-else-if="nodeType === 'extract'">
      <NFormItem label="字段路径" label-placement="left" label-width="72" size="small">
        <NInput v-model:value="localConfig.field" size="small" placeholder="如: result.content (数组段用下标: items.0.name)" />
      </NFormItem>
      <NFormItem label="兜底内容" label-placement="left" label-width="72" size="small">
        <NInput v-model:value="localConfig.fallback" type="textarea" :rows="2" size="small" placeholder="提取失败时输出; 留空则原样透传上游内容" />
      </NFormItem>
      <p class="text-11px text-gray-400 leading-5">
        上游内容需为 JSON (自动剥 ```json 代码围栏); 抽到字符串原样输出, 对象/数组转为 JSON 文本。适合放在工具/LLM 节点之后精简传给下游的内容。
      </p>
    </template>

    <!-- 子Agent 节点 -->
    <template v-else-if="nodeType === 'subagent'">
      <div class="text-11px text-gray-400">从主 Agent 拉线到本节点即建立委派; 主 Agent 运行时可按需调用子 Agent。</div>
      <NFormItem label="引用 Agent" label-placement="left" label-width="72" size="small">
        <NSelect v-model:value="localConfig.agent_id" :options="agentOptions" size="small" @update:value="onAgentIdChange" />
      </NFormItem>
      <NFormItem label="模型" label-placement="left" label-width="72" size="small">
        <NSelect v-model:value="localConfig.model" :options="modelOptions" size="small" />
      </NFormItem>
      <NFormItem label="系统提示词" label-placement="left" label-width="72" size="small">
        <NInput
          v-model:value="localConfig.system_prompt"
          type="textarea"
          :rows="4"
          size="small"
          :placeholder="localConfig.agent_id ? '已引用 Agent, 将使用其系统提示词 (此处填写会被忽略)' : '子 Agent 的角色与任务指令'"
        />
      </NFormItem>
      <NFormItem label="委派说明" label-placement="left" label-width="72" size="small">
        <NInput
          v-model:value="localConfig.description"
          type="textarea"
          :rows="2"
          size="small"
          placeholder="子 Agent 的职责说明, 供主 Agent 判断何时委派 (如: 负责联网调研)"
        />
      </NFormItem>
      <p v-if="delegateDescMissing" class="text-11px text-orange-500 leading-5 mb-2">
        委派说明为空: 主 Agent 只能凭节点名称判断是否委派, 很可能一直不调用该子 Agent。建议填写(或给被引用 Agent 补上简介/描述)。
      </p>
      <NFormItem label="工具" label-placement="left" label-width="72" size="small">
        <NSelect
          v-model:value="localConfig.tools"
          multiple
          clearable
          size="small"
          :options="toolOptions"
          placeholder="子 Agent 可调用的工具"
        />
      </NFormItem>
      <NFormItem label="最大轮次" label-placement="left" label-width="72" size="small">
        <NInputNumber v-model:value="localConfig.max_iterations" :min="0" :max="100" size="small" class="w-full" placeholder="0 = 默认 15" />
      </NFormItem>
      <NFormItem label="超时(秒)" label-placement="left" label-width="72" size="small">
        <NInputNumber v-model:value="localConfig.timeout_seconds" :min="0" :max="3600" size="small" class="w-full" placeholder="0 = 跟随编排整体超时" />
      </NFormItem>
      <NFormItem label="失败重试" label-placement="left" label-width="72" size="small">
        <NInputNumber v-model:value="localConfig.max_retries" :min="0" :max="10" size="small" class="w-full" placeholder="0 = 默认 2 次" />
      </NFormItem>
    </template>

    <!-- 提示词版本 (Agent / 子Agent 节点共用, 与用户提示词共用 user_prompts 表) -->
    <template v-if="(nodeType === 'agent' || nodeType === 'subagent') && canManagePrompt">
      <template v-if="currentOrchId">
        <NFormItem label="提示词版本" label-placement="left" label-width="72" size="small">
          <NSelect
            v-model:value="selectedVersionId"
            size="small"
            clearable
            :options="promptVersionOptions"
            :loading="promptVersionLoading"
            placeholder="历史版本 (仅管理, 不直接改写画布)"
          />
        </NFormItem>
        <div class="flex gap-2 mb-1">
          <NButton size="tiny" secondary @click="handleSavePromptVersion">存为新版本</NButton>
          <NButton
            size="tiny"
            secondary
            :disabled="!selectedVersionId || selectedVersionId === activeVersion?.id"
            @click="handleSwitchPromptVersion"
          >
            启用所选
          </NButton>
          <NButton size="tiny" quaternary type="error" :disabled="!selectedVersionId" @click="handleDeletePromptVersion">
            删除所选
          </NButton>
        </div>
        <p class="text-11px leading-5 mb-1" :class="activeVersion ? 'text-orange-500' : 'text-gray-400'">
          {{
            activeVersion
              ? `运行时以启用的版本 v${activeVersion.version} 为准${nodeType === 'subagent' ? ', 优先于被引用 Agent 的提示词' : ''}, 覆盖画布提示词 (版本为编排全局共享, 不区分用户)。`
              : '运行时使用画布提示词; 点「存为新版本」后, 所有用户运行时都以启用版本为准。'
          }}
        </p>
      </template>
      <p v-else class="text-11px text-gray-400 leading-5">编排保存后可在此维护提示词版本, 运行时以启用版本覆盖画布提示词。</p>
    </template>

    <!-- 合并节点 -->
    <template v-else-if="nodeType === 'merge'">
      <NFormItem label="分隔符" label-placement="left" label-width="72" size="small">
        <NInput v-model:value="localConfig.separator" size="small" placeholder="默认两个换行" />
      </NFormItem>
      <p class="text-11px text-gray-400 leading-5">汇聚多条入边的内容, 按 DSL 连线顺序拼接为一条用户消息。</p>
    </template>

    <!-- 子编排节点 -->
    <template v-else-if="nodeType === 'suborch'">
      <div class="text-11px text-gray-400">引用另一个已保存编排, 运行时整体执行该编排; 嵌套层级最多 5 层, 循环引用会在编译时报错。</div>
      <NFormItem label="引用编排" label-placement="left" label-width="72" size="small">
        <NSelect v-model:value="localConfig.orchestration_id" :options="orchestrationOptions" size="small" placeholder="选择要嵌套执行的编排" />
      </NFormItem>
      <p class="text-11px text-gray-400 leading-5">
        被引用编排需处于启用状态; 其对话历史与身份前言不注入, 仅接收本节点的上游内容并把最终输出传给下游。
      </p>
    </template>

    <!-- 结束节点 -->
    <template v-else>
      <p class="text-11px text-gray-400 leading-5">最终输出节点: 该节点的输出即调试面板展示的最终结果。</p>
    </template>
  </div>
</template>
