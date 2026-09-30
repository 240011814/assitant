<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { NButton, NDynamicInput, NFormItem, NInput, NInputNumber, NSelect, NSpace } from 'naive-ui';
import type { Node as FlowNode } from '@vue-flow/core';
import type { OrchestrationResource } from '@/service/api';
import { NODE_META, type OrchNodeType } from '../nodes/registry';

const props = defineProps<{
  nodeId: string;
  nodeType: 'agent' | 'tool' | 'template' | 'branch' | 'merge' | 'end';
  name: string;
  config: Record<string, any>;
  allNodes: FlowNode[];
  edges: any[];
  resources: OrchestrationResource | null;
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
  if (props.nodeType === 'agent' && !Array.isArray(cfg.tools)) cfg.tools = [];
  if (props.nodeType === 'branch' && !Array.isArray(cfg.cases)) cfg.cases = [];
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
      <div class="text-11px text-gray-400">按上游内容路由, 命中顺序自上而下; 分支目标需与画布连线一致。</div>
      <NDynamicInput v-model:value="casesValue" :on-create="() => ({ type: 'contains', value: '', target: '' })">
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

    <!-- 合并节点 -->
    <template v-else-if="nodeType === 'merge'">
      <NFormItem label="分隔符" label-placement="left" label-width="72" size="small">
        <NInput v-model:value="localConfig.separator" size="small" placeholder="默认两个换行" />
      </NFormItem>
      <p class="text-11px text-gray-400 leading-5">汇聚多条入边的内容, 按 DSL 连线顺序拼接为一条用户消息。</p>
    </template>

    <!-- 结束节点 -->
    <template v-else>
      <p class="text-11px text-gray-400 leading-5">最终输出节点: 该节点的输出即调试面板展示的最终结果。</p>
    </template>
  </div>
</template>
