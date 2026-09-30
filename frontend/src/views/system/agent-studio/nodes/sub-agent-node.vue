<script setup lang="ts">
import { computed } from 'vue';
import BaseOrchNode from './base-orch-node.vue';

const props = defineProps<{
  data: { name?: string; config: Record<string, any> };
  selected?: boolean;
  status?: 'running' | 'success' | 'error' | null;
}>();

const cfg = computed(() => props.data.config || {});
const desc = computed(() => (cfg.value.description as string) || '');
const chips = computed(() => {
  const list: string[] = [];
  if (cfg.value.agent_id) list.push(`引用 #${cfg.value.agent_id}`);
  if (cfg.value.model) list.push(cfg.value.model);
  list.push(`工具×${(cfg.value.tools || []).length}`);
  return list;
});
</script>

<template>
  <BaseOrchNode type="subagent" :name="data.name" :selected="selected" :status="status">
    <div class="flex items-center gap-1 flex-wrap">
      <span class="orch-chip" :class="{ 'orch-chip-muted': chips.length === 0 }" :title="desc">
        {{ chips.length ? chips.join(' · ') : '未配置 (需接主 Agent)' }}
      </span>
    </div>
  </BaseOrchNode>
</template>
