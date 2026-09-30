<script setup lang="ts">
import BaseOrchNode from './base-orch-node.vue';

defineProps<{
  data: { name?: string; config: Record<string, any> };
  selected?: boolean;
  status?: 'running' | 'success' | 'error' | null;
}>();
</script>

<template>
  <BaseOrchNode type="merge" :name="data.name" :selected="selected" :status="status">
    <span class="orch-chip orch-chip-muted">按连线顺序合并</span>
  </BaseOrchNode>
</template>
