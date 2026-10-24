<script setup lang="ts">
import { NModal, NScrollbar, NTag } from "naive-ui";

/** 中奖名单弹窗 (前三名奖牌图标) */
defineProps<{
  winners: Api.Lottery.Record[];
}>();

const show = defineModel<boolean>("show", { required: true });
</script>

<template>
  <NModal
    v-model:show="show"
    preset="card"
    title="🏆 中奖名单"
    style="width: 90%; max-width: 500px"
  >
    <NScrollbar style="max-height: 60vh">
      <div v-if="winners.length" class="winner-list">
        <div v-for="(winner, index) in winners" :key="winner.id" class="winner-item">
          <span class="winner-rank">{{
            index < 3 ? ["🥇", "🥈", "🥉"][index] : index + 1
          }}</span>
          <div class="winner-info">
            <span class="winner-name">{{
              winner.userName || `用户#${winner.userId}`
            }}</span>
            <span class="winner-time">{{
              new Date(winner.createdAt).toLocaleString()
            }}</span>
          </div>
          <NTag type="warning" size="small" :bordered="false">
            {{ winner.prizeName }}
          </NTag>
        </div>
      </div>
      <div v-else class="empty-tip">暂无中奖记录</div>
    </NScrollbar>
  </NModal>
</template>

<style scoped lang="scss">
.winner-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.winner-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px;
  background: #f9f9f9;
  border-radius: 12px;
}

.winner-rank {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  flex-shrink: 0;
}

.winner-info {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.winner-name {
  font-size: 14px;
  font-weight: 500;
  color: #333;
}
.winner-time {
  font-size: 12px;
  color: #999;
}

.empty-tip {
  text-align: center;
  padding: 40px 20px;
  color: #999;
  font-size: 14px;
}
</style>
