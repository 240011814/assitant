<script setup lang="ts">
import { computed } from 'vue';
import BaseOrchNode from './base-orch-node.vue';

const props = defineProps<{
  data: { name?: string; config: Record<string, any> };
  selected?: boolean;
  status?: 'running' | 'success' | 'error' | null;
}>();

const preview = computed(() => {
  const tpl = (props.data.config?.template as string) || '';
  const firstLine = tpl.split('\n').find(l => l.trim());
  return firstLine?.trim().slice(0, 26) || '';
});
</script>

<template>
  <BaseOrchNode type="template" :name="data.name" :selected="selected" :status="status">
    <span class="orch-chip" :class="{ 'orch-chip-muted': !preview }" :title="data.config?.template">
      {{ preview || '未配置模板' }}
    </span>
  </BaseOrchNode>
</template>
