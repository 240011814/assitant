<script setup lang="ts">
import { NButton, NModal } from "naive-ui";

/** 转盘模式的抽奖结果弹窗 */
defineProps<{
  drawResult: Api.Lottery.DrawResult | null;
}>();

const show = defineModel<boolean>("show", { required: true });
</script>

<template>
  <NModal
    v-model:show="show"
    style="width: 90%; max-width: 400px"
    :bordered="false"
    :mask-closable="false"
  >
    <div
      v-if="drawResult"
      class="result-modal"
      :class="{ 'is-winner': drawResult.isWinner }"
    >
      <div class="result-icon">{{ drawResult.isWinner ? "🎉" : "😢" }}</div>
      <h3 class="result-title">{{ drawResult.message }}</h3>

      <div v-if="drawResult.isWinner && drawResult.prize" class="result-prize">
        <div class="result-prize-name">{{ drawResult.prize.name }}</div>
        <div class="result-prize-value">价值: {{ drawResult.prize.prizeValue }}</div>
      </div>

      <div v-else class="result-no-prize">
        <p>💪 下次一定中奖！</p>
      </div>

      <NButton type="primary" size="large" round block @click="show = false">
        知道了
      </NButton>
    </div>
  </NModal>
</template>

<style scoped lang="scss">
.result-modal {
  background: white;
  border-radius: 20px;
  padding: 40px 24px;
  text-align: center;

  &.is-winner {
    background: linear-gradient(135deg, #fff9e6 0%, #fff3cc 100%);
  }
}

.result-icon {
  font-size: 64px;
  margin-bottom: 16px;
}

.result-title {
  font-size: 22px;
  font-weight: 700;
  color: #333;
  margin: 0 0 20px;
}

.result-prize {
  padding: 16px;
  background: rgba(255, 215, 0, 0.2);
  border-radius: 12px;
  margin-bottom: 24px;
}

.result-prize-name {
  font-size: 20px;
  font-weight: 700;
  color: #b45309;
  margin-bottom: 4px;
}
.result-prize-value {
  font-size: 14px;
  color: #92400e;
}

.result-no-prize {
  margin-bottom: 24px;
  color: #666;
}
</style>
