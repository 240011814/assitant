<script setup lang="ts">
import { NButton, NTag } from "naive-ui";

/** 抽奖记录列表 (深色玻璃卡, 首屏 20 条 + "加载更多" 追加) */
defineProps<{
  records: Api.Lottery.Record[];
  total: number;
  hasMore: boolean;
  loadingMore: boolean;
}>();

const emit = defineEmits<{
  refresh: [];
  loadMore: [];
}>();

function formatTime(dateStr: string) {
  const date = new Date(dateStr);
  const now = new Date();
  const diff = now.getTime() - date.getTime();
  const minutes = Math.floor(diff / 60000);
  const hours = Math.floor(diff / 3600000);
  const days = Math.floor(diff / 86400000);

  if (minutes < 1) return "刚刚";
  if (minutes < 60) return `${minutes}分钟前`;
  if (hours < 24) return `${hours}小时前`;
  if (days < 7) return `${days}天前`;
  return date.toLocaleDateString();
}
</script>

<template>
  <div class="records-card">
    <div class="records-header">
      <span>📋 抽奖记录 ({{ total }})</span>
      <NButton text type="primary" size="small" @click="emit('refresh')">
        刷新
      </NButton>
    </div>

    <div class="records-list">
      <template v-if="records.length">
        <div
          v-for="record in records"
          :key="record.id"
          class="record-item"
          :class="{ 'is-winner': record.isWinner }"
        >
          <span class="record-icon">{{ record.isWinner ? "🎉" : "🎰" }}</span>
          <div class="record-info">
            <span class="record-name">{{
              record.userName || `用户#${record.userId}`
            }}</span>
            <span class="record-time">{{ formatTime(record.createdAt) }}</span>
          </div>
          <div class="record-result">
            <NTag
              :type="record.isWinner ? 'success' : 'default'"
              size="small"
              :bordered="false"
            >
              {{ record.isWinner ? "中奖" : "未中奖" }}
            </NTag>
            <span v-if="record.prizeName" class="record-prize">{{
              record.prizeName
            }}</span>
          </div>
        </div>
      </template>
      <div v-else class="empty-tip">暂无抽奖记录</div>

      <!-- 加载更多 -->
      <div v-if="records.length" class="load-more-wrap">
        <NButton
          v-if="hasMore"
          text
          type="primary"
          size="small"
          :loading="loadingMore"
          @click="emit('loadMore')"
        >
          加载更多
        </NButton>
        <span v-else class="all-loaded">— 已加载全部 —</span>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.records-card {
  background: linear-gradient(160deg, rgba(26, 20, 58, 0.82), rgba(14, 10, 34, 0.88));
  border: 1px solid rgba(255, 214, 110, 0.18);
  backdrop-filter: blur(6px);
  border-radius: 16px;
  overflow: hidden;
  box-shadow: 0 8px 30px rgba(0, 0, 0, 0.35);
}

.records-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px;
  font-weight: 600;
  font-size: 15px;
  color: #fff;
  border-bottom: 1px solid rgba(255, 214, 110, 0.14);
}

.records-list {
  max-height: 320px;
  overflow-y: auto;
}

.record-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 20px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  transition: background 0.2s;

  &:last-of-type {
    border-bottom: none;
  }
  &:hover {
    background: rgba(255, 255, 255, 0.05);
  }
  &.is-winner {
    background: rgba(255, 214, 110, 0.08);
  }
}

.record-icon {
  font-size: 20px;
}

.record-info {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.record-name {
  font-size: 14px;
  font-weight: 500;
  color: rgba(255, 255, 255, 0.92);
}
.record-time {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.45);
}

.record-result {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
}

.record-prize {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.55);
}

.empty-tip {
  text-align: center;
  padding: 40px 20px;
  color: rgba(255, 255, 255, 0.45);
  font-size: 14px;
}

.load-more-wrap {
  display: flex;
  justify-content: center;
  padding: 10px 0 12px;
}

.all-loaded {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.3);
}
</style>
