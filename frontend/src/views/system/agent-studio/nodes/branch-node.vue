<script setup lang="ts">
import { computed } from 'vue';
import BaseOrchNode from './base-orch-node.vue';

const props = defineProps<{
  data: { name?: string; config: Record<string, any> };
  selected?: boolean;
  status?: 'running' | 'success' | 'error' | null;
}>();

const cases = computed(() => (Array.isArray(props.data.config?.cases) ? (props.data.config.cases as { value: string }[]) : []));
const hasDefault = computed(() => !!props.data.config?.default_target);
const maxLoops = computed(() => Number(props.data.config?.max_loops || 0));
</script>

<template>
  <BaseOrchNode type="branch" :name="data.name" :selected="selected" :status="status">
    <div class="flex items-center gap-1 flex-wrap">
      <span class="orch-chip" :class="{ 'orch-chip-muted': !cases.length }">{{ cases.length }} 个条件</span>
      <span v-if="maxLoops" class="orch-chip" title="带循环回边, 最多执行次数">循环≤{{ maxLoops }}</span>
      <span v-if="hasDefault" class="orch-chip orch-chip-muted">有默认分支</span>
      <span v-else class="orch-chip orch-chip-muted">未设默认</span>
    </div>
  </BaseOrchNode>
</template>
