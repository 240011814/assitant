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
  background: #f9f9f9;
  border-radius: 12px;
  transition: all 0.2s;

  &:hover {
    background: #f0f0f0;
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
  color: #333;
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
  background: rgba(0, 0, 0, 0.05);
}

.prize-meta {
  font-size: 12px;
  color: #999;
}

.prize-remain {
  text-align: center;
}

.remain-num {
  display: block;
  font-size: 20px;
  font-weight: 700;
  color: #667eea;
}
.remain-label {
  font-size: 11px;
  color: #999;
}

.empty-tip {
  text-align: center;
  padding: 40px 20px;
  color: #999;
  font-size: 14px;
}
</style>
