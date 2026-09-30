<script setup lang="ts">
import { computed } from 'vue';
import { Handle, Position } from '@vue-flow/core';
import { NODE_META, nodeDisplayColor, type OrchNodeType } from './registry';

/**
 * 所有编排节点的共用外观: 连接桩 / 头部(图标+名称+状态配色) / 内容插槽
 * 类型组件只负责提供插槽内容, 不要在这里之外重复节点 chrome 代码
 */
const props = defineProps<{
  type: OrchNodeType;
  name?: string;
  selected?: boolean;
  status?: 'running' | 'success' | 'error' | null;
  /** hideSource 隐藏右侧出线桩 (子Agent 只能被委派, 不能向外连线) */
  hideSource?: boolean;
  /** delegated 本次调试运行中该节点已被委派 (子Agent 节点用) */
  delegated?: boolean;
}>();

const meta = computed(() => NODE_META[props.type]);
const color = computed(() => nodeDisplayColor(props.type, props.status));
const headerLabel = computed(() => props.name || meta.value.label);
</script>

<template>
  <div class="orch-node" :class="{ selected, running: status === 'running' }" :style="{ borderColor: color }">
    <Handle type="target" id="in" :position="Position.Left" class="orch-handle" />
    <div class="orch-node-header" :style="{ background: color }">
      <SvgIcon :icon="meta.icon" class="text-14px" />
      <span class="truncate">{{ headerLabel }}</span>
      <span v-if="delegated" class="orch-delegated" title="本次运行已被主 Agent 委派">委派</span>
    </div>
    <div class="orch-node-body">
      <slot />
    </div>
    <Handle v-if="!hideSource" type="source" id="out" :position="Position.Right" class="orch-handle" />
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
.orch-handle {
  width: 9px;
  height: 9px;
  background: #2080f0;
  border: 2px solid #fff;
}
.orch-delegated {
  margin-left: auto;
  padding: 0 4px;
  border-radius: 3px;
  background: rgba(255, 255, 255, 0.28);
  font-size: 10px;
  font-weight: 500;
  line-height: 14px;
}
</style>

<style>
/* 节点内容芯片: 各类型组件共用 (scoped 无法作用于插槽内容) */
.orch-chip {
  display: inline-flex;
  align-items: center;
  max-width: 100%;
  padding: 0 5px;
  border-radius: 4px;
  background: #f3f3f5;
  color: #555;
  font-size: 11px;
  line-height: 18px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.dark .orch-chip {
  background: #333338;
  color: #bbb;
}
.orch-chip-muted {
  color: #999;
}
</style>
