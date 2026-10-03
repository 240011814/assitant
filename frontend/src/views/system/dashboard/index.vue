<script setup lang="ts">
import { ref } from "vue";
import OverviewPanel from "./modules/overview.vue";
import TokenUsagePanel from "./modules/token-usage.vue";

defineOptions({ name: "SystemDashboard" });

const activeTab = ref("overview");
</script>

<template>
  <div class="h-full overflow-auto p-6">
    <NTabs v-model:value="activeTab" type="line">
      <NTabPane name="overview" tab="系统概览">
        <OverviewPanel />
      </NTabPane>
      <NTabPane name="token" tab="Token 用量">
        <TokenUsagePanel />
      </NTabPane>
    </NTabs>
  </div>
</template>
