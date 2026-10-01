import { markRaw, type Component } from 'vue';
import AgentNode from './agent-node.vue';
import ToolNode from './tool-node.vue';
import TemplateNode from './template-node.vue';
import BranchNode from './branch-node.vue';
import RouterNode from './router-node.vue';
import ExtractNode from './extract-node.vue';
import MergeNode from './merge-node.vue';
import EndNode from './end-node.vue';
import SubAgentNode from './sub-agent-node.vue';
import SubOrchNode from './sub-orch-node.vue';

/** 编排节点类型 (与后端 DSL 的 node.type 一致) */
export type OrchNodeType = 'agent' | 'tool' | 'template' | 'branch' | 'router' | 'extract' | 'merge' | 'end' | 'subagent' | 'suborch';

export interface OrchNodeMeta {
  /** 画布与右键菜单展示名 */
  label: string;
  icon: string;
  color: string;
  /** 新建节点时的默认配置 (makeNode 会深拷贝) */
  defaultConfig: Record<string, any>;
  /** 画布节点组件 */
  component: Component;
}

/** 节点类型注册表: 唯一事实源, 新增类型只需在此登记 */
export const NODE_META: Record<OrchNodeType, OrchNodeMeta> = {
  agent: {
    label: 'Agent',
    icon: 'mdi:robot-outline',
    color: '#2080f0',
    defaultConfig: { model: '', system_prompt: '', tools: [], max_iterations: 0 },
    component: markRaw(AgentNode)
  },
  tool: {
    label: '工具',
    icon: 'mdi:wrench-outline',
    color: '#18a058',
    defaultConfig: { tool: '' },
    component: markRaw(ToolNode)
  },
  template: {
    label: '模板',
    icon: 'mdi:text-box-edit-outline',
    color: '#f0a020',
    defaultConfig: { template: '{{.Input}}' },
    component: markRaw(TemplateNode)
  },
  branch: {
    label: '分支',
    icon: 'mdi:source-branch',
    color: '#d03050',
    defaultConfig: { cases: [], default_target: '' },
    component: markRaw(BranchNode)
  },
  router: {
    label: 'LLM路由',
    icon: 'mdi:routes',
    color: '#c2410c',
    defaultConfig: { model: '', instructions: '', cases: [], default_target: '' },
    component: markRaw(RouterNode)
  },
  extract: {
    label: '字段提取',
    icon: 'mdi:code-json',
    color: '#0e7490',
    defaultConfig: { field: '', fallback: '' },
    component: markRaw(ExtractNode)
  },
  merge: {
    label: '合并',
    icon: 'mdi:call-merge',
    color: '#8a2be2',
    defaultConfig: { separator: '\n\n' },
    component: markRaw(MergeNode)
  },
  end: {
    label: '结束',
    icon: 'mdi:check-circle-outline',
    color: '#666666',
    defaultConfig: {},
    component: markRaw(EndNode)
  },
  subagent: {
    label: '子Agent',
    icon: 'mdi:account-group-outline',
    color: '#0fa9a0',
    defaultConfig: { agent_id: 0, model: '', system_prompt: '', description: '', tools: [] },
    component: markRaw(SubAgentNode)
  },
  suborch: {
    label: '子编排',
    icon: 'mdi:graph-outline',
    color: '#7c3aed',
    defaultConfig: { orchestration_id: 0 },
    component: markRaw(SubOrchNode)
  }
};

/** 调试运行状态色 (覆盖类型色) */
export function nodeDisplayColor(type: string, status?: 'running' | 'success' | 'error' | null): string {
  if (status === 'error') return '#d03050';
  if (status === 'success') return '#18a058';
  return NODE_META[type as OrchNodeType]?.color || '#2080f0';
}

/** 右键菜单/工具栏的节点类型选项 */
export function nodeTypeOptions() {
  return (Object.keys(NODE_META) as OrchNodeType[]).map(type => ({ label: NODE_META[type].label, value: type }));
}
