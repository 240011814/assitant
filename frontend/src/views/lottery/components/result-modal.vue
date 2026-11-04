<script setup lang="ts">
import { computed } from "vue";
import { NButton, NModal } from "naive-ui";
import { rarityTheme, resolveRarity } from "../composables/rarity-theme";

/** 转盘模式的抽奖结果弹窗: 按奖品稀有度主题化 (与抽卡卡/彩带同一套配色) */
const props = defineProps<{
  drawResult: Api.Lottery.DrawResult | null;
}>();

const show = defineModel<boolean>("show", { required: true });

const rarity = computed(() => {
  if (!props.drawResult?.isWinner || !props.drawResult.prize) return 1;
  return resolveRarity(props.drawResult.prize.prizeLevel);
});
const theme = computed(() => rarityTheme(rarity.value));
const themeVars = computed(() => ({
  "--r-primary": theme.value.primary,
  "--r-secondary": theme.value.secondary,
  "--r-glow": theme.value.glow
}));
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
      :style="themeVars"
    >
      <div class="result-aura" />
      <div class="result-shine" />

      <div class="result-icon">{{ drawResult.isWinner ? "🎉" : "😢" }}</div>

      <div v-if="drawResult.isWinner" class="result-stars">
        <span v-for="i in rarity" :key="i" class="result-star">★</span>
      </div>

      <h3 class="result-title">{{ drawResult.message }}</h3>

      <div v-if="drawResult.isWinner && drawResult.prize" class="result-prize">
        <div class="result-prize-name">{{ drawResult.prize.name }}</div>
        <div class="result-prize-value">价值: {{ drawResult.prize.prizeValue }} · {{ theme.label }}</div>
      </div>

      <div v-else class="result-no-prize">
        <p>💪 下次一定中奖！</p>
      </div>

      <NButton class="result-confirm" size="large" round block @click="show = false">
        知道了
      </NButton>
    </div>
  </NModal>
</template>

<style scoped lang="scss">
.result-modal {
  position: relative;
  overflow: hidden;
  background: linear-gradient(160deg, #171d38 0%, #0d1226 100%);
  border-radius: 20px;
  padding: 36px 24px 28px;
  text-align: center;
  border: 1.5px solid color-mix(in srgb, var(--r-primary) 50%, transparent);
  box-shadow: 0 0 40px color-mix(in srgb, var(--r-glow) 24%, transparent);

  &.is-winner {
    animation: modalPop 0.4s cubic-bezier(0.2, 1.2, 0.4, 1) both;
  }
}

@keyframes modalPop {
  0% {
    transform: scale(0.86);
    opacity: 0;
  }
  100% {
    transform: scale(1);
    opacity: 1;
  }
}

.result-aura {
  position: absolute;
  inset: -40px;
  filter: blur(36px);
  background: radial-gradient(ellipse at 50% 30%, color-mix(in srgb, var(--r-primary) 34%, transparent), transparent 70%);
  pointer-events: none;
}

.result-shine {
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: linear-gradient(115deg, transparent 34%, rgba(255, 255, 255, 0.14) 47%, rgba(255, 255, 255, 0.28) 50%, rgba(255, 255, 255, 0.14) 53%, transparent 66%);
  background-size: 260% 100%;
  animation: shineSweep 2.4s ease-in-out infinite;
}

@keyframes shineSweep {
  0% {
    background-position: 130% 0;
  }
  55%,
  100% {
    background-position: -130% 0;
  }
}

.result-icon {
  position: relative;
  font-size: 58px;
  margin-bottom: 10px;
}

.result-stars {
  position: relative;
  display: flex;
  justify-content: center;
  gap: 4px;
  margin-bottom: 8px;

  .result-star {
    font-size: 18px;
    color: var(--r-primary);
    text-shadow: 0 0 10px color-mix(in srgb, var(--r-primary) 70%, transparent);
    animation: starPop 0.4s cubic-bezier(0.2, 1.4, 0.4, 1) both;

    @for $i from 1 through 5 {
      &:nth-child(#{$i}) {
        animation-delay: #{0.12 * $i}s;
      }
    }
  }
}

@keyframes starPop {
  0% {
    transform: scale(0) rotate(-40deg);
    opacity: 0;
  }
  100% {
    transform: scale(1) rotate(0);
    opacity: 1;
  }
}

.result-title {
  position: relative;
  font-size: 21px;
  font-weight: 700;
  color: #fff;
  margin: 0 0 20px;
}

.result-prize {
  position: relative;
  padding: 16px;
  background: color-mix(in srgb, var(--r-primary) 14%, transparent);
  border: 1px solid color-mix(in srgb, var(--r-primary) 30%, transparent);
  border-radius: 12px;
  margin-bottom: 24px;
}

.result-prize-name {
  font-size: 19px;
  font-weight: 700;
  color: var(--r-primary);
  text-shadow: 0 0 14px color-mix(in srgb, var(--r-primary) 55%, transparent);
  margin-bottom: 4px;
  word-break: break-all;
}
.result-prize-value {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.6);
}

.result-no-prize {
  position: relative;
  margin-bottom: 24px;
  color: rgba(255, 255, 255, 0.65);
}

.result-confirm {
  position: relative;
  border: none;
  font-weight: 600;
  color: #1c1600;
  background: linear-gradient(135deg, var(--r-primary), var(--r-secondary));

  &:hover {
    box-shadow: 0 4px 20px color-mix(in srgb, var(--r-primary) 50%, transparent);
  }
}
</style>
