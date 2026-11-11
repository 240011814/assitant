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
    class="lottery-dark-modal"
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
// NModal teleport 到 body, 需 :global 覆盖卡片本体 (紫夜深色玻璃)
:global(.lottery-dark-modal.n-card) {
  background: linear-gradient(160deg, rgba(26, 20, 58, 0.97), rgba(14, 10, 34, 0.97)) !important;
  border: 1px solid rgba(255, 214, 110, 0.24);

  .n-card-header__main {
    color: #fff;
    font-weight: 600;
  }
}

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
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.08);
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
  color: rgba(255, 255, 255, 0.92);
}
.winner-time {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.45);
}

.empty-tip {
  text-align: center;
  padding: 40px 20px;
  color: rgba(255, 255, 255, 0.45);
  font-size: 14px;
}
</style>
