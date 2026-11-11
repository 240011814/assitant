<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from "vue";
import { NModal } from "naive-ui";
import type { GachaPhase } from "../composables/use-gacha-animation";
import { createWishScene } from "../composables/use-wish-scene";
import type { WishSceneHandle } from "../composables/use-wish-scene";
import PrizeResultCard from "./prize-result-card.vue";

/**
 * 原神祈愿风抽卡动画全屏弹窗: three.js canvas 场景 (星空/流星/光线/光柱)
 * 打底, 结果阶段前景叠加共享 PrizeResultCard。
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
    <div class="wish-overlay" :class="`phase-${gachaPhase}`" @click="onOverlayClick">
      <!-- three.js 场景画布 -->
      <canvas ref="canvasRef" class="wish-canvas" />
      <!-- 闪光白幕 (flash 阶段盖一层, 中性蓝白不泄露稀有度) -->
      <div class="flash-veil" :class="{ active: gachaPhase === 'flash' }" />

      <!-- 结果阶段: 前景共享结果卡 -->
      <div v-if="gachaPhase === 'result' && drawResult" class="result-stage" @click.stop>
        <PrizeResultCard
          :draw-result="drawResult"
          :rarity="gachaRarity"
          variant="gacha"
          confirm-text="确认"
          @confirm="emit('close')"
        />
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

.result-stage {
  position: relative;
  z-index: 10;
  perspective: 900px;
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
