<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { useAppStore } from "@/store/modules/app";
import { useRouterPush } from "@/hooks/common/router";
import { useReaderMiniLeaveSave } from "@/hooks/common/reader-mini-save";
import { useReaderSessionStore } from "@/store/modules/reader-session";
import type { ReaderSessionSnapshot } from "@/store/modules/reader-session";
import ReadingDetailView from "../reading/components/reading-detail-view.vue";

// 阅读详情路由页: 业务在 ReadingDetailView 组件 (与列表页弹窗共用), 这里只提供页面容器 + 全局小窗会话
const appStore = useAppStore();
const route = useRoute();
const docId = computed(() => Number(route.params.id));
// 卸载时路由已切走 (computed 读到新路由得 NaN), 小窗快照用最近一次有效 docId
let lastDocId = Number(route.params.id) || 0;
watch(docId, (v) => {
  if (Number.isFinite(v) && v > 0) lastDocId = v;
});
const { routerPushByKey } = useRouterPush();
const sessionStore = useReaderSessionStore();
const detailRef = ref<InstanceType<typeof ReadingDetailView> | null>(null);

// 用户主动离开 (返回列表 / 最小化跳转) 时置位, 离开自动收窗不再触发
let leaveIntentionally = false;

// 阅读中的快照; 文档未加载完或主动离开时不收窗
const captureDocSession = (): ReaderSessionSnapshot | null => {
  if (leaveIntentionally) return null;
  const doc = detailRef.value?.doc;
  if (!doc) return null;
  return {
    kind: "reading",
    itemId: lastDocId,
    title: doc.title || `文档 #${lastDocId}`,
    subtitle: "分级阅读",
    scrollTop: detailRef.value?.getScrollTop() ?? 0,
    progress: detailRef.value?.getReadProgress() ?? 0
  };
};

// 切 tab / 路由跳转 / 关浏览器标签 → 自动收入全局小窗
useReaderMiniLeaveSave(captureDocSession);

// 返回列表: 主动退出, 不留小窗
const handleBack = () => {
  leaveIntentionally = true;
  routerPushByKey("ai_reading");
};

// 最小化: 存全局小窗会话后跳回列表 (右下角卡片随时恢复续读)
const handleMinimize = () => {
  const snapshot = captureDocSession();
  if (snapshot) sessionStore.minimize(snapshot);
  leaveIntentionally = true;
  routerPushByKey("ai_reading");
};
</script>

<template>
  <div class="h-full flex" :class="appStore.isMobile ? 'p-2' : 'p-4'">
    <div class="flex-1 min-w-0 rounded-lg overflow-hidden border border-gray-100 dark:border-gray-800 shadow-sm">
      <ReadingDetailView
        ref="detailRef"
        :doc-id="docId"
        mode="page"
        @back="handleBack"
        @minimize="handleMinimize"
      />
    </div>
  </div>
</template>
