<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from "vue";
import { NButton, NModal } from "naive-ui";
import type { GachaPhase } from "../composables/use-gacha-animation";
import { createWishScene } from "../composables/use-wish-scene";
import type { WishSceneHandle } from "../composables/use-wish-scene";
import { rarityTheme } from "../composables/rarity-theme";

/**
 * 原神祈愿风抽卡动画全屏弹窗: three.js canvas 场景 (星空/流星/光线/光柱)
 * 打底, 结果阶段前景叠加 DOM 卡片 (镭射扫光 + 星级弹入)。
 * 场景生命周期跟随弹窗开关; 流星/闪光/揭示阶段可点击任意处或"跳过"直达结果。
 */
const props = defineProps<{
  gachaPhase: GachaPhase;
  gachaRarity: number;
  drawResult: Api.Lottery.DrawResult | null;
}>();

const show = defineModel<boolean>("show", { required: true });

const emit = defineEmits<{
  close: [];
  skip: [];
}>();

const canvasRef = ref<HTMLCanvasElement | null>(null);
let scene: WishSceneHandle | null = null;

const theme = computed(() => rarityTheme(props.gachaRarity));
const themeVars = computed(() => ({
  "--r-primary": theme.value.primary,
  "--r-secondary": theme.value.secondary,
  "--r-glow": theme.value.glow
}));

watch(show, async open => {
  if (open) {
    await nextTick();
    if (canvasRef.value && !scene) {
      scene = createWishScene(canvasRef.value, props.gachaRarity);
      scene.setPhase(props.gachaPhase);
    }
  } else {
    scene?.dispose();
    scene = null;
  }
});

watch(
  () => props.gachaPhase,
  phase => {
    if (show.value) scene?.setPhase(phase);
  }
);

watch(
  () => props.gachaRarity,
  rarity => {
    if (show.value) scene?.setRarity(rarity);
  }
);

onBeforeUnmount(() => {
  scene?.dispose();
  scene = null;
});

function onOverlayClick() {
  // 流星/闪光/揭示阶段点击任意处跳过 (原神同款), 结果阶段由卡片按钮关闭
  if (["meteor", "flash", "reveal"].includes(props.gachaPhase)) {
    emit("skip");
  }
}
</script>

<template>
  <NModal
    v-model:show="show"
    style="width: 100%; height: 100%; background: transparent"
    :bordered="false"
    :mask-closable="false"
    :close-on-esc="false"
    :show-icon="false"
  >
    <div class="wish-overlay" :class="`phase-${gachaPhase}`" :style="themeVars" @click="onOverlayClick">
      <!-- three.js 场景画布 -->
      <canvas ref="canvasRef" class="wish-canvas" />
      <!-- 闪光白幕 (flash 阶段盖一层) -->
      <div class="flash-veil" :class="{ active: gachaPhase === 'flash' }" />

      <!-- 结果阶段: 前景 DOM 卡片 -->
      <div v-if="gachaPhase === 'result' && drawResult" class="result-stage" @click.stop>
        <div class="result-card" :class="{ 'no-prize': !drawResult.isWinner }">
          <div class="card-aura" />
          <div class="card-shine" />

          <div class="card-stars-row">
            <span v-for="i in gachaRarity" :key="i" class="card-star" :style="{ animationDelay: `${0.15 * i}s` }">★</span>
          </div>

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

          <NButton class="card-confirm" @click="emit('close')">确认</NButton>
        </div>
      </div>

      <!-- 跳过按钮 (动画阶段可见) -->
      <button v-if="gachaPhase !== 'result'" class="skip-btn" @click.stop="emit('skip')">
        跳过 ›
      </button>
    </div>
  </NModal>
</template>

<style scoped lang="scss">
.wish-overlay {
  position: fixed;
  inset: 0;
  z-index: 1000;
  overflow: hidden;
  background: #070b1e;
  display: flex;
  align-items: center;
  justify-content: center;
}

.wish-canvas {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  display: block;
}

.flash-veil {
  position: absolute;
  inset: 0;
  // 中性蓝白闪光: 揭晓前不泄露稀有度配色
  background: radial-gradient(circle at 50% 50%, #fff 0%, #cfe0ff 38%, transparent 75%);
  opacity: 0;
  pointer-events: none;

  &.active {
    animation: veilFlash 0.6s ease-out forwards;
  }
}

@keyframes veilFlash {
  0% {
    opacity: 0;
  }
  25% {
    opacity: 0.95;
  }
  100% {
    opacity: 0;
  }
}

// ==================== 结果卡片 ====================
.result-stage {
  position: relative;
  z-index: 10;
  perspective: 900px;
}

.result-card {
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

.card-stars-row {
  display: flex;
  justify-content: center;
  gap: 5px;
  margin-bottom: 14px;
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

// ==================== 跳过按钮 ====================
.skip-btn {
  position: absolute;
  right: 18px;
  bottom: 22px;
  z-index: 20;
  padding: 7px 16px;
  font-size: 13px;
  letter-spacing: 0.12em;
  color: rgba(255, 255, 255, 0.78);
  background: rgba(255, 255, 255, 0.08);
  border: 1px solid rgba(255, 255, 255, 0.24);
  border-radius: 999px;
  cursor: pointer;
  backdrop-filter: blur(6px);
  transition: all 0.2s;

  &:hover {
    color: #fff;
    background: rgba(255, 255, 255, 0.16);
  }
}
</style>
