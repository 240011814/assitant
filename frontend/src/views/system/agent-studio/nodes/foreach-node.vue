<script setup lang="ts">
import { computed } from 'vue';
import BaseOrchNode from './base-orch-node.vue';

const props = defineProps<{
  data: { name?: string; config: Record<string, any> };
  selected?: boolean;
  status?: 'running' | 'success' | 'error' | null;
}>();

const preview = computed(() => {
  const tpl = (props.data.config?.item_template as string) || '';
  const firstLine = tpl.split('\n').find(l => l.trim());
  return firstLine?.trim().slice(0, 26) || '';
});
</script>

<template>
  <BaseOrchNode type="foreach" :name="data.name" :selected="selected" :status="status">
    <span class="orch-chip" :class="{ 'orch-chip-muted': !preview }" :title="data.config?.item_template">
      {{ preview || '逐项处理' }}
    </span>
    <span v-if="data.config?.items_path" class="orch-chip orch-chip-muted">via {{ data.config.items_path }}</span>
  </BaseOrchNode>
</template>
