<script setup lang="ts">
import { computed } from 'vue';
import BaseOrchNode from './base-orch-node.vue';

const props = defineProps<{
  data: { name?: string; config: Record<string, any> };
  selected?: boolean;
  status?: 'running' | 'success' | 'error' | null;
  /** title 被引用编排的名称 (靠它区分不同的子编排节点) */
  title?: string;
}>();

const cfg = computed(() => props.data.config || {});
const label = computed(() => {
  if (!cfg.value.orchestration_id) return '未选择编排';
  return props.title ? `#${cfg.value.orchestration_id} ${props.title}` : `引用 #${cfg.value.orchestration_id}`;
});
</script>

<template>
  <BaseOrchNode type="suborch" :name="data.name" :selected="selected" :status="status">
    <div class="flex items-center gap-1 flex-wrap">
      <span class="orch-chip">{{ label }}</span>
    </div>
  </BaseOrchNode>
</template>
