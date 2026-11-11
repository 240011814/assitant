<script setup lang="ts">
import { NModal, NScrollbar } from "naive-ui";

/** 奖品列表弹窗: 等级标签/颜色计算与展示内聚 */
defineProps<{
  prizes: Api.Lottery.Prize[];
  segmentColors: string[];
}>();

const show = defineModel<boolean>("show", { required: true });

// 奖品等级标签
function getPrizeLevelLabel(level: number): string {
  const map: Record<number, string> = {
    0: "",
    1: "特等奖",
    2: "一等奖",
    3: "二等奖",
    4: "三等奖",
  };
  return map[level] || "";
}

function getPrizeLevelColor(level: number): string {
  const map: Record<number, string> = {
    0: "#999",
    1: "#ff4757",
    2: "#ffa502",
    3: "#2ed573",
    4: "#1e90ff",
  };
  return map[level] || "#999";
}
</script>

<template>
  <NModal
    v-model:show="show"
    preset="card"
    title="🎁 奖品列表"
    class="lottery-dark-modal"
    style="width: 90%; max-width: 500px"
  >
    <NScrollbar style="max-height: 60vh">
      <div v-if="prizes.length" class="prize-list">
        <div v-for="(prize, index) in prizes" :key="prize.id" class="prize-item">
          <div
            class="prize-icon-box"
            :style="{
              background: segmentColors[index % segmentColors.length],
            }"
          >
            <img v-if="prize.imageUrl" :src="prize.imageUrl" class="prize-image" />
            <span v-else>🎁</span>
          </div>
          <div class="prize-detail">
            <div class="prize-name">
              <span
                v-if="getPrizeLevelLabel(prize.prizeLevel)"
                class="prize-level-tag"
                :style="{ color: getPrizeLevelColor(prize.prizeLevel) }"
              >
                {{ getPrizeLevelLabel(prize.prizeLevel) }}
              </span>
              {{ prize.name }}
            </div>
            <div class="prize-meta">
              价值 {{ prize.prizeValue }} · 概率
              {{ (prize.displayProbability * 100).toFixed(1) }}%
            </div>
          </div>
          <div class="prize-remain">
            <span class="remain-num">{{ prize.remainingCount }}</span>
            <span class="remain-label">剩余</span>
          </div>
        </div>
      </div>
      <div v-else class="empty-tip">暂无奖品</div>
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

.prize-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.prize-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px;
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 12px;
  transition: all 0.2s;

  &:hover {
    background: rgba(255, 255, 255, 0.11);
  }
}

.prize-icon-box {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  flex-shrink: 0;
  overflow: hidden;
}

.prize-image {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.prize-detail {
  flex: 1;
}

.prize-name {
  font-size: 15px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.92);
  margin-bottom: 4px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.prize-level-tag {
  font-size: 12px;
  font-weight: 700;
  padding: 2px 6px;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.1);
}

.prize-meta {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.5);
}

.prize-remain {
  text-align: center;
}

.remain-num {
  display: block;
  font-size: 20px;
  font-weight: 700;
  color: #ffd76e;
}
.remain-label {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.45);
}

.empty-tip {
  text-align: center;
  padding: 40px 20px;
  color: rgba(255, 255, 255, 0.45);
  font-size: 14px;
}
</style>
