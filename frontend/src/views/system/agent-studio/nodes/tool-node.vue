<script setup lang="ts">
import { computed } from 'vue';
import BaseOrchNode from './base-orch-node.vue';

const props = defineProps<{
  data: { name?: string; config: Record<string, any> };
  selected?: boolean;
  status?: 'running' | 'success' | 'error' | null;
}>();

const tool = computed(() => (props.data.config?.tool as string) || '');
</script>

<template>
  <BaseOrchNode type="tool" :name="data.name" :selected="selected" :status="status">
    <span class="orch-chip" :class="{ 'orch-chip-muted': !tool }">{{ tool || '未选择工具' }}</span>
  </BaseOrchNode>
</template>
