<script setup lang="ts">
import { useAppStore } from "@/store/modules/app";
import StatsPanel from "./modules/stats-panel.vue";

const appStore = useAppStore();
</script>

<template>
  <div class="h-full flex-col flex overflow-auto" :class="appStore.isMobile ? 'p-2' : 'p-4'">
    <StatsPanel />
  </div>
</template>

<style scoped></style>
