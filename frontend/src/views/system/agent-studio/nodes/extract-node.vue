<script setup lang="ts">
import { computed } from 'vue';
import BaseOrchNode from './base-orch-node.vue';

const props = defineProps<{
  data: { name?: string; config: Record<string, any> };
  selected?: boolean;
  status?: 'running' | 'success' | 'error' | null;
}>();

const field = computed(() => (props.data.config?.field as string) || '');
const hasFallback = computed(() => !!props.data.config?.fallback);
</script>

<template>
  <BaseOrchNode type="extract" :name="data.name" :selected="selected" :status="status">
    <div class="flex items-center gap-1 flex-wrap">
      <span class="orch-chip" :class="{ 'orch-chip-muted': !field }" :title="field">{{ field || '未配置字段' }}</span>
      <span v-if="hasFallback" class="orch-chip orch-chip-muted">有兜底</span>
    </div>
  </BaseOrchNode>
</template>
