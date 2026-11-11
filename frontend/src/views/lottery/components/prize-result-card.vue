<script setup lang="ts">
import { computed } from "vue";
import { NButton } from "naive-ui";
import { rarityTheme } from "../composables/rarity-theme";

/**
 * 抽奖结果展示卡 (转盘 result-modal 与抽卡动画 result 阶段共用):
 * 深色玻璃卡 + 稀有度主题配色 + 镭射扫光 + 星级弹入。
 * variant: 'gacha' 抽卡(默认, 无标题行) / 'wheel' 转盘(大 emoji + 消息标题行)。
 */
const props = withDefaults(
  defineProps<{
    drawResult: Api.Lottery.DrawResult;
    rarity: number;
    variant?: "gacha" | "wheel";
    confirmText?: string;
  }>(),
  { variant: "gacha", confirmText: "确认" }
);

const emit = defineEmits<{
  confirm: [];
}>();

const theme = computed(() => rarityTheme(props.rarity));
const themeVars = computed(() => ({
  "--r-primary": theme.value.primary,
  "--r-secondary": theme.value.secondary,
  "--r-glow": theme.value.glow
}));
</script>

<template>
  <div class="prize-card" :class="{ 'no-prize': !drawResult.isWinner }" :style="themeVars">
    <div class="card-aura" />
    <div class="card-shine" />

    <div v-if="variant === 'wheel'" class="card-emoji">{{ drawResult.isWinner ? "🎉" : "😢" }}</div>

    <div class="card-stars-row">
      <span v-for="i in rarity" :key="i" class="card-star" :style="{ animationDelay: `${0.15 * i}s` }">★</span>
    </div>

    <h2 v-if="variant === 'wheel'" class="card-heading">{{ drawResult.message }}</h2>

    <div class="card-icon">
      <img v-if="drawResult.isWinner && drawResult.prize?.imageUrl" :src="drawResult.prize.imageUrl" alt="" />
      <span v-else class="card-icon-glyph">✦</span>
    </div>

    <h2 class="card-name">
      {{ drawResult.isWinner ? drawResult.prize?.name : "谢谢惠顾" }}
    </h2>
    <p v-if="drawResult.isWinner && drawResult.prize" class="card-desc">
      价值: {{ drawResult.prize.prizeValue }} · {{ theme.label }}
    </p>
    <p v-else class="card-desc">✦ 下次一定 ✦</p>

    <NButton class="card-confirm" @click="emit('confirm')">{{ confirmText }}</NButton>
  </div>
</template>

<style scoped lang="scss">
.prize-card {
  width: min(320px, 84vw);
  padding: 26px 24px 24px;
  text-align: center;
  border-radius: 20px;
  position: relative;
  overflow: hidden;
  background: linear-gradient(160deg, rgba(20, 26, 58, 0.92), rgba(10, 14, 34, 0.96));
  border: 1.5px solid color-mix(in srgb, var(--r-primary) 55%, transparent);
  box-shadow:
    0 24px 70px rgba(0, 0, 0, 0.55),
    0 0 42px color-mix(in srgb, var(--r-glow) 26%, transparent);
  animation: cardFlipIn 0.55s cubic-bezier(0.2, 0.9, 0.3, 1.15) both;

  &.no-prize {
    border-color: rgba(255, 255, 255, 0.22);
    filter: saturate(0.55);
  }
}

@keyframes cardFlipIn {
  0% {
    transform: rotateY(70deg) scale(0.82);
    opacity: 0;
  }
  100% {
    transform: rotateY(0) scale(1);
    opacity: 1;
  }
}

.card-aura {
  position: absolute;
  inset: -46px;
  border-radius: 32px;
  filter: blur(34px);
  background: radial-gradient(ellipse at 50% 40%, color-mix(in srgb, var(--r-primary) 42%, transparent), transparent 70%);
  animation: auraBreath 2.2s ease-in-out infinite;
  pointer-events: none;
}

@keyframes auraBreath {
  0%,
  100% {
    opacity: 0.75;
    transform: scale(1);
  }
  50% {
    opacity: 1;
    transform: scale(1.05);
  }
}

// 镭射扫光: 斜向高光周期性掠过卡片
.card-shine {
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: linear-gradient(115deg, transparent 32%, rgba(255, 255, 255, 0.16) 46%, rgba(255, 255, 255, 0.34) 50%, rgba(255, 255, 255, 0.16) 54%, transparent 68%);
  background-size: 260% 100%;
  animation: shineSweep 2.6s ease-in-out infinite;
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

.card-emoji {
  position: relative;
  font-size: 44px;
  margin-bottom: 8px;
}

.card-stars-row {
  display: flex;
  justify-content: center;
  gap: 5px;
  margin-bottom: 12px;
  min-height: 22px;
}

.card-star {
  font-size: 20px;
  color: var(--r-primary);
  text-shadow: 0 0 12px color-mix(in srgb, var(--r-primary) 70%, transparent);
  animation: starPop 0.45s cubic-bezier(0.2, 1.4, 0.4, 1) both;
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

.card-heading {
  position: relative;
  font-size: 19px;
  font-weight: 700;
  margin: 0 0 14px;
  color: #fff;
}

.card-icon {
  width: 108px;
  height: 108px;
  margin: 0 auto 16px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: radial-gradient(circle at 50% 38%, color-mix(in srgb, var(--r-primary) 30%, transparent), rgba(255, 255, 255, 0.06) 70%);
  border: 2px solid color-mix(in srgb, var(--r-primary) 45%, transparent);
  box-shadow: 0 0 26px color-mix(in srgb, var(--r-glow) 34%, transparent);

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    border-radius: 50%;
  }
}

.card-icon-glyph {
  font-size: 46px;
  color: var(--r-primary);
  text-shadow: 0 0 24px var(--r-glow);
}

.card-name {
  font-size: 23px;
  font-weight: 700;
  margin: 0 0 8px;
  color: #fff;
  text-shadow: 0 0 18px color-mix(in srgb, var(--r-primary) 55%, transparent);
  word-break: break-all;
}

.card-desc {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.62);
  margin: 0 0 20px;
}

.card-confirm {
  width: 100%;
  height: 44px;
  border-radius: 22px;
  font-size: 16px;
  font-weight: 600;
  border: none;
  color: #1c1600;
  background: linear-gradient(135deg, var(--r-primary), var(--r-secondary));

  &:hover {
    box-shadow: 0 4px 22px color-mix(in srgb, var(--r-primary) 55%, transparent);
  }
}
</style>
