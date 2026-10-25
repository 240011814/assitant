<script setup lang="ts">
import { computed } from "vue";
import { useRoute } from "vue-router";
import { useAppStore } from "@/store/modules/app";
import ReadingDetailView from "../reading/components/reading-detail-view.vue";

// 阅读详情路由页: 业务在 ReadingDetailView 组件 (与列表页弹窗共用), 这里只提供页面容器
const appStore = useAppStore();
const route = useRoute();
const docId = computed(() => Number(route.params.id));
</script>

<template>
  <div class="h-full flex" :class="appStore.isMobile ? 'p-2' : 'p-4'">
    <div class="flex-1 min-w-0 rounded-lg overflow-hidden border border-gray-100 dark:border-gray-800 shadow-sm">
      <ReadingDetailView :doc-id="docId" mode="page" />
    </div>
  </div>
</template>
