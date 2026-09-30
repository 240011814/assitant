<script setup lang="ts">
import { computed } from 'vue';
import BaseOrchNode from './base-orch-node.vue';

const props = defineProps<{
  data: { name?: string; config: Record<string, any> };
  selected?: boolean;
  status?: 'running' | 'success' | 'error' | null;
}>();

const cfg = computed(() => props.data.config || {});
const tools = computed(() => (Array.isArray(cfg.value.tools) ? (cfg.value.tools as string[]) : []));
</script>

<template>
  <BaseOrchNode type="agent" :name="data.name" :selected="selected" :status="status">
    <div class="flex items-center gap-1 flex-wrap">
      <span class="orch-chip">{{ cfg.model || '默认模型' }}</span>
      <span v-for="t in tools.slice(0, 2)" :key="t" class="orch-chip">{{ t }}</span>
      <span v-if="tools.length > 2" class="orch-chip">+{{ tools.length - 2 }}</span>
      <span v-if="!tools.length" class="orch-chip orch-chip-muted">无工具</span>
    </div>
  </BaseOrchNode>
</template>
