<script setup lang="ts">
import { NButton, NModal } from "naive-ui";
import type { GachaParticle, GachaPhase } from "../composables/use-gacha-animation";

/**
 * 原神抽卡动画全屏弹窗: 流星 → 闪光 → 揭示 → 结果 四阶段状态机展示。
 * 阶段/粒子/稀有度状态由父组件的 useGachaAnimation 驱动, 这里只负责渲染。
 */
defineProps<{
  gachaPhase: GachaPhase;
  gachaParticles: GachaParticle[];
  gachaRarity: number;
  drawResult: Api.Lottery.DrawResult | null;
}>();

const show = defineModel<boolean>("show", { required: true });

const emit = defineEmits<{
  close: [];
}>();
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
    <div
      class="gacha-overlay"
      :class="[`phase-${gachaPhase}`, `rarity-bg-${gachaRarity}`]"
    >
      <!-- 背景粒子 -->
      <div class="particles-container">
        <div
          v-for="p in gachaParticles"
          :key="p.id"
          class="particle"
          :style="{ left: `${p.x}%`, top: `${p.y}%`, animationDelay: `${p.delay}s` }"
        />
      </div>

      <!-- 流星阶段 -->
      <div v-if="gachaPhase === 'meteor'" class="meteor-stage">
        <div class="meteor-main" :class="`meteor-rarity-${gachaRarity}`">
          <div class="meteor-core" />
          <div class="meteor-glow" />
        </div>
        <div class="meteor-trail-container">
          <div
            v-for="i in 5"
            :key="i"
            class="meteor-trail-particle"
            :class="`trail-rarity-${gachaRarity}`"
            :style="{ animationDelay: `${i * 0.1}s` }"
          />
        </div>
      </div>

      <!-- 闪光阶段 -->
      <div v-if="gachaPhase === 'flash'" class="flash-stage">
        <div class="flash-circle" :class="`flash-rarity-${gachaRarity}`" />
        <div class="flash-rays">
          <div
            v-for="i in 12"
            :key="i"
            class="ray"
            :style="{ transform: `rotate(${i * 30}deg)` }"
            :class="`ray-rarity-${gachaRarity}`"
          />
        </div>
      </div>

      <!-- 揭示阶段 -->
      <div v-if="gachaPhase === 'reveal'" class="reveal-stage">
        <div class="reveal-burst" :class="`burst-rarity-${gachaRarity}`" />
        <div class="reveal-ring-container">
          <div
            v-for="i in 3"
            :key="i"
            class="reveal-ring"
            :class="[`ring-${i}`, `ring-rarity-${gachaRarity}`]"
          />
        </div>
      </div>

      <!-- 结果阶段 -->
      <div v-if="gachaPhase === 'result' && drawResult" class="result-stage">
        <div class="result-card-container">
          <!-- 背景光效 -->
          <div class="card-aura" :class="`aura-rarity-${gachaRarity}`" />

          <!-- 卡片 -->
          <div class="gacha-card" :class="`card-rarity-${gachaRarity}`">
            <!-- 卡片顶部光效 -->
            <div class="card-top-glow" :class="`glow-rarity-${gachaRarity}`" />

            <!-- 星星装饰 -->
            <div class="card-stars-decor">
              <span
                v-for="i in 6"
                :key="i"
                class="decor-star"
                :class="`star-rarity-${gachaRarity}`"
                :style="{ animationDelay: `${i * 0.2}s` }"
              >✦</span>
            </div>

            <!-- 主要内容 -->
            <div class="card-body">
              <!-- 图标/图片 -->
              <div class="card-icon-wrapper" :class="`icon-rarity-${gachaRarity}`">
                <img
                  v-if="drawResult.isWinner && drawResult.prize?.imageUrl"
                  :src="drawResult.prize.imageUrl"
                  class="card-prize-img"
                />
                <span
                  v-else
                  class="card-star-icon"
                  :class="`star-icon-rarity-${gachaRarity}`"
                >✦</span>
              </div>

              <!-- 名称 -->
              <h2 class="card-prize-name" :class="`name-rarity-${gachaRarity}`">
                {{ drawResult.isWinner ? drawResult.prize?.name : "谢谢惠顾" }}
              </h2>

              <!-- 描述 -->
              <p v-if="drawResult.isWinner && drawResult.prize" class="card-prize-desc">
                价值: {{ drawResult.prize.prizeValue }}
              </p>
              <p v-else class="card-prize-desc">✦ 下次一定 ✦</p>

              <!-- 星级 -->
              <div class="card-rarity-stars">
                <span
                  v-for="i in gachaRarity"
                  :key="i"
                  class="rarity-star"
                  :class="`rarity-star-${gachaRarity}`"
                >★</span>
              </div>
            </div>

            <!-- 底部按钮 -->
            <div class="card-footer">
              <NButton
                class="confirm-btn"
                :class="`btn-rarity-${gachaRarity}`"
                @click="emit('close')"
              >
                确认
              </NButton>
            </div>
          </div>
        </div>
      </div>
    </div>
  </NModal>
</template>

<style scoped lang="scss">
.gacha-overlay {
  position: fixed;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #1a1a3e 0%, #2a2a5e 50%, #1a1a3e 100%);
  z-index: 1000;
  overflow: hidden;
  transition: background 0.5s ease;

  &.rarity-bg-5 {
    background: linear-gradient(135deg, #2a2000 0%, #3a2800 50%, #2a2000 100%);
  }
  &.rarity-bg-4 {
    background: linear-gradient(135deg, #2a1a30 0%, #3a2040 50%, #2a1a30 100%);
  }
  &.rarity-bg-3 {
    background: linear-gradient(135deg, #1a2030 0%, #2a3040 50%, #1a2030 100%);
  }
  &.rarity-bg-2 {
    background: linear-gradient(135deg, #1a2a1a 0%, #2a3a2a 50%, #1a2a1a 100%);
  }
  &.rarity-bg-1 {
    background: linear-gradient(135deg, #2a2010 0%, #3a3020 50%, #2a2010 100%);
  }
}

// 粒子效果
.particles-container {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.particle {
  position: absolute;
  width: 4px;
  height: 4px;
  background: rgba(255, 255, 255, 0.6);
  border-radius: 50%;
  animation: particleFloat 4s ease-in-out infinite;
}

@keyframes particleFloat {
  0%,
  100% {
    transform: translateY(0) scale(1);
    opacity: 0.6;
  }
  50% {
    transform: translateY(-30px) scale(1.5);
    opacity: 1;
  }
}

// ==================== 流星阶段 ====================
.meteor-stage {
  position: absolute;
  top: 0;
  left: 50%;
  transform: translateX(-50%);
  width: 200px;
  height: 100%;
}

.meteor-main {
  position: absolute;
  top: -100px;
  left: 50%;
  transform: translateX(-50%);
  animation: meteorDrop 1.2s cubic-bezier(0.25, 0.46, 0.45, 0.94) forwards;
}

.meteor-core {
  width: 60px;
  height: 60px;
  border-radius: 50%;
  position: relative;
  z-index: 2;
}

.meteor-glow {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  width: 200px;
  height: 200px;
  border-radius: 50%;
  filter: blur(40px);
  animation: glowPulse 0.5s ease-in-out infinite alternate;
}

@keyframes glowPulse {
  0% {
    transform: translate(-50%, -50%) scale(1);
    opacity: 0.8;
  }
  100% {
    transform: translate(-50%, -50%) scale(1.2);
    opacity: 1;
  }
}

.meteor-rarity-5 {
  .meteor-core {
    background: radial-gradient(circle, #fff, #ffd700, #ff8c00);
  }
  .meteor-glow {
    background: radial-gradient(
      circle,
      rgba(255, 215, 0, 0.8),
      rgba(255, 140, 0, 0.4),
      transparent
    );
  }
}

.meteor-rarity-4 {
  .meteor-core {
    background: radial-gradient(circle, #fff, #ce93d8, #9b59b6);
  }
  .meteor-glow {
    background: radial-gradient(
      circle,
      rgba(206, 147, 216, 0.8),
      rgba(155, 89, 182, 0.4),
      transparent
    );
  }
}

.meteor-rarity-3 {
  .meteor-core {
    background: radial-gradient(circle, #fff, #64b5f6, #3498db);
  }
  .meteor-glow {
    background: radial-gradient(
      circle,
      rgba(100, 181, 246, 0.8),
      rgba(52, 152, 219, 0.4),
      transparent
    );
  }
}

.meteor-rarity-2 {
  .meteor-core {
    background: radial-gradient(circle, #fff, #81c784, #66bb6a);
  }
  .meteor-glow {
    background: radial-gradient(
      circle,
      rgba(129, 199, 132, 0.8),
      rgba(102, 187, 106, 0.4),
      transparent
    );
  }
}

.meteor-rarity-1 {
  .meteor-core {
    background: radial-gradient(circle, #fff, #ffb74d, #ffa726);
  }
  .meteor-glow {
    background: radial-gradient(
      circle,
      rgba(255, 183, 77, 0.8),
      rgba(255, 167, 38, 0.4),
      transparent
    );
  }
}

@keyframes meteorDrop {
  0% {
    top: -100px;
    opacity: 0;
    transform: translateX(-50%) scale(0.5);
  }
  10% {
    opacity: 1;
  }
  80% {
    transform: translateX(-50%) scale(1);
  }
  100% {
    top: 50%;
    opacity: 1;
    transform: translateX(-50%) scale(1.1);
  }
}

.meteor-trail-container {
  position: absolute;
  top: 0;
  left: 50%;
  transform: translateX(-50%);
  width: 100px;
  height: 100%;
}

.meteor-trail-particle {
  position: absolute;
  top: -200px;
  left: 50%;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  animation: trailDrop 1.2s cubic-bezier(0.25, 0.46, 0.45, 0.94) forwards;
}

.trail-rarity-5 {
  background: #ffd700;
  box-shadow: 0 0 20px #ffd700;
}
.trail-rarity-4 {
  background: #ce93d8;
  box-shadow: 0 0 20px #ce93d8;
}
.trail-rarity-3 {
  background: #64b5f6;
  box-shadow: 0 0 20px #64b5f6;
}
.trail-rarity-2 {
  background: #81c784;
  box-shadow: 0 0 20px #81c784;
}
.trail-rarity-1 {
  background: #ffb74d;
  box-shadow: 0 0 20px #ffb74d;
}

@keyframes trailDrop {
  0% {
    top: -200px;
    opacity: 0;
  }
  10% {
    opacity: 1;
  }
  100% {
    top: 45%;
    opacity: 0;
    transform: scale(0);
  }
}

// ==================== 闪光阶段 ====================
.flash-stage {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
}

.flash-circle {
  width: 300px;
  height: 300px;
  border-radius: 50%;
  animation: flashExpand 0.4s ease-out forwards;
}

.flash-rarity-5 {
  background: radial-gradient(
    circle,
    rgba(255, 215, 0, 1),
    rgba(255, 140, 0, 0.8),
    transparent
  );
}
.flash-rarity-4 {
  background: radial-gradient(
    circle,
    rgba(206, 147, 216, 1),
    rgba(155, 89, 182, 0.8),
    transparent
  );
}
.flash-rarity-3 {
  background: radial-gradient(
    circle,
    rgba(100, 181, 246, 1),
    rgba(52, 152, 219, 0.8),
    transparent
  );
}
.flash-rarity-2 {
  background: radial-gradient(
    circle,
    rgba(129, 199, 132, 1),
    rgba(102, 187, 106, 0.8),
    transparent
  );
}
.flash-rarity-1 {
  background: radial-gradient(
    circle,
    rgba(255, 183, 77, 1),
    rgba(255, 167, 38, 0.8),
    transparent
  );
}

@keyframes flashExpand {
  0% {
    transform: scale(0);
    opacity: 1;
  }
  100% {
    transform: scale(3);
    opacity: 0;
  }
}

.flash-rays {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  width: 400px;
  height: 400px;
}

.ray {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 4px;
  height: 200px;
  transform-origin: 0 0;
  animation: rayExpand 0.4s ease-out forwards;
}

.ray-rarity-5 {
  background: linear-gradient(to top, rgba(255, 215, 0, 0.8), transparent);
}
.ray-rarity-4 {
  background: linear-gradient(to top, rgba(206, 147, 216, 0.8), transparent);
}
.ray-rarity-3 {
  background: linear-gradient(to top, rgba(100, 181, 246, 0.8), transparent);
}
.ray-rarity-2 {
  background: linear-gradient(to top, rgba(129, 199, 132, 0.8), transparent);
}
.ray-rarity-1 {
  background: linear-gradient(to top, rgba(255, 183, 77, 0.8), transparent);
}

@keyframes rayExpand {
  0% {
    height: 0;
    opacity: 1;
  }
  100% {
    height: 200px;
    opacity: 0;
  }
}

// ==================== 揭示阶段 ====================
.reveal-stage {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
}

.reveal-burst {
  width: 200px;
  height: 200px;
  border-radius: 50%;
  animation: burstExpand 0.6s ease-out forwards;
}

.burst-rarity-5 {
  background: radial-gradient(circle, rgba(255, 215, 0, 0.6), transparent);
  box-shadow: 0 0 100px rgba(255, 215, 0, 0.8);
}
.burst-rarity-4 {
  background: radial-gradient(circle, rgba(206, 147, 216, 0.6), transparent);
  box-shadow: 0 0 100px rgba(155, 89, 182, 0.8);
}
.burst-rarity-3 {
  background: radial-gradient(circle, rgba(100, 181, 246, 0.6), transparent);
  box-shadow: 0 0 100px rgba(52, 152, 219, 0.8);
}
.burst-rarity-2 {
  background: radial-gradient(circle, rgba(129, 199, 132, 0.6), transparent);
  box-shadow: 0 0 100px rgba(102, 187, 106, 0.8);
}
.burst-rarity-1 {
  background: radial-gradient(circle, rgba(255, 183, 77, 0.6), transparent);
  box-shadow: 0 0 100px rgba(255, 167, 38, 0.8);
}

@keyframes burstExpand {
  0% {
    transform: scale(0);
    opacity: 1;
  }
  100% {
    transform: scale(4);
    opacity: 0;
  }
}

.reveal-ring-container {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
}

.reveal-ring {
  position: absolute;
  top: 50%;
  left: 50%;
  border: 3px solid;
  border-radius: 50%;
  transform: translate(-50%, -50%);
}

.ring-1 {
  width: 100px;
  height: 100px;
  animation: revealRingPulse 0.8s ease-out forwards;
}
.ring-2 {
  width: 150px;
  height: 150px;
  animation: revealRingPulse 0.8s ease-out 0.1s forwards;
}
.ring-3 {
  width: 200px;
  height: 200px;
  animation: revealRingPulse 0.8s ease-out 0.2s forwards;
}

.ring-rarity-5 {
  border-color: #ffd700;
  box-shadow: 0 0 30px rgba(255, 215, 0, 0.6);
}
.ring-rarity-4 {
  border-color: #ce93d8;
  box-shadow: 0 0 30px rgba(155, 89, 182, 0.6);
}
.ring-rarity-3 {
  border-color: #64b5f6;
  box-shadow: 0 0 30px rgba(52, 152, 219, 0.6);
}
.ring-rarity-2 {
  border-color: #81c784;
  box-shadow: 0 0 30px rgba(102, 187, 106, 0.6);
}
.ring-rarity-1 {
  border-color: #ffb74d;
  box-shadow: 0 0 30px rgba(255, 167, 38, 0.6);
}

/* 原文件里与转盘外圈的 ringPulse 同名互相覆盖, 拆分后改名各归其位 */
@keyframes revealRingPulse {
  0% {
    transform: translate(-50%, -50%) scale(0);
    opacity: 1;
  }
  50% {
    opacity: 1;
  }
  100% {
    transform: translate(-50%, -50%) scale(1.5);
    opacity: 0;
  }
}

// ==================== 结果阶段 ====================
.result-stage {
  position: relative;
  z-index: 10;
  animation: resultAppear 0.5s ease-out forwards;
}

@keyframes resultAppear {
  0% {
    transform: scale(0.8);
    opacity: 0;
  }
  100% {
    transform: scale(1);
    opacity: 1;
  }
}

.result-card-container {
  position: relative;
}

.card-aura {
  position: absolute;
  top: -50px;
  left: -50px;
  right: -50px;
  bottom: -50px;
  border-radius: 30px;
  filter: blur(30px);
  animation: auraPulse 2s ease-in-out infinite;
}

.aura-rarity-5 {
  background: radial-gradient(ellipse, rgba(255, 215, 0, 0.4), transparent);
}
.aura-rarity-4 {
  background: radial-gradient(ellipse, rgba(206, 147, 216, 0.4), transparent);
}
.aura-rarity-3 {
  background: radial-gradient(ellipse, rgba(100, 181, 246, 0.3), transparent);
}
.aura-rarity-2 {
  background: radial-gradient(ellipse, rgba(129, 199, 132, 0.3), transparent);
}
.aura-rarity-1 {
  background: radial-gradient(ellipse, rgba(255, 183, 77, 0.3), transparent);
}

@keyframes auraPulse {
  0%,
  100% {
    transform: scale(1);
    opacity: 0.8;
  }
  50% {
    transform: scale(1.05);
    opacity: 1;
  }
}

.gacha-card {
  width: 320px;
  background: linear-gradient(135deg, #2a2a4e, #1e1e3a);
  border-radius: 20px;
  overflow: hidden;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.4);
  border: 2px solid transparent;

  &.card-rarity-5 {
    border-color: rgba(255, 215, 0, 0.6);
  }
  &.card-rarity-4 {
    border-color: rgba(206, 147, 216, 0.6);
  }
  &.card-rarity-3 {
    border-color: rgba(100, 181, 246, 0.4);
  }
  &.card-rarity-2 {
    border-color: rgba(129, 199, 132, 0.4);
  }
  &.card-rarity-1 {
    border-color: rgba(255, 183, 77, 0.4);
  }
}

.card-top-glow {
  height: 4px;
}

.glow-rarity-5 {
  background: linear-gradient(90deg, transparent, #ffd700, transparent);
}
.glow-rarity-4 {
  background: linear-gradient(90deg, transparent, #ce93d8, transparent);
}
.glow-rarity-3 {
  background: linear-gradient(90deg, transparent, #64b5f6, transparent);
}
.glow-rarity-2 {
  background: linear-gradient(90deg, transparent, #81c784, transparent);
}
.glow-rarity-1 {
  background: linear-gradient(90deg, transparent, #ffb74d, transparent);
}

.card-stars-decor {
  display: flex;
  justify-content: center;
  gap: 8px;
  padding: 16px 0 8px;
}

.decor-star {
  font-size: 16px;
  animation: starTwinkle 1.5s ease-in-out infinite;
}

.star-rarity-5 {
  color: #ffd700;
}
.star-rarity-4 {
  color: #ce93d8;
}
.star-rarity-3 {
  color: #64b5f6;
}
.star-rarity-2 {
  color: #81c784;
}
.star-rarity-1 {
  color: #ffb74d;
}

@keyframes starTwinkle {
  0%,
  100% {
    opacity: 0.5;
    transform: scale(1);
  }
  50% {
    opacity: 1;
    transform: scale(1.3);
  }
}

.card-body {
  padding: 20px 24px 30px;
  text-align: center;
}

.card-icon-wrapper {
  width: 120px;
  height: 120px;
  margin: 0 auto 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  position: relative;
  background: rgba(255, 255, 255, 0.1);

  &::before {
    content: "";
    position: absolute;
    inset: -4px;
    border-radius: 50%;
    border: 3px solid;
    animation: iconBorderPulse 2s ease-in-out infinite;
  }
}

.icon-rarity-5::before {
  border-color: #ffd700;
}
.icon-rarity-4::before {
  border-color: #ce93d8;
}
.icon-rarity-3::before {
  border-color: #64b5f6;
}
.icon-rarity-2::before {
  border-color: #81c784;
}
.icon-rarity-1::before {
  border-color: #ffb74d;
}

@keyframes iconBorderPulse {
  0%,
  100% {
    transform: scale(1);
    opacity: 0.6;
  }
  50% {
    transform: scale(1.05);
    opacity: 1;
  }
}

.card-prize-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  border-radius: 50%;
}

.card-star-icon {
  font-size: 48px;
}

.star-icon-rarity-5 {
  color: #ffd700;
  text-shadow: 0 0 30px rgba(255, 215, 0, 0.8);
}
.star-icon-rarity-4 {
  color: #ce93d8;
  text-shadow: 0 0 30px rgba(206, 147, 216, 0.8);
}
.star-icon-rarity-3 {
  color: #64b5f6;
  text-shadow: 0 0 30px rgba(100, 181, 246, 0.6);
}
.star-icon-rarity-2 {
  color: #81c784;
  text-shadow: 0 0 30px rgba(129, 199, 132, 0.6);
}
.star-icon-rarity-1 {
  color: #ffb74d;
  text-shadow: 0 0 30px rgba(255, 183, 77, 0.6);
}

.card-prize-name {
  font-size: 24px;
  font-weight: 700;
  margin: 0 0 12px;
}

.name-rarity-5 {
  color: #ffd700;
  text-shadow: 0 0 20px rgba(255, 215, 0, 0.5);
}
.name-rarity-4 {
  color: #ce93d8;
  text-shadow: 0 0 20px rgba(206, 147, 216, 0.5);
}
.name-rarity-3 {
  color: #64b5f6;
}
.name-rarity-2 {
  color: #81c784;
}
.name-rarity-1 {
  color: #ffb74d;
}

.card-prize-desc {
  font-size: 14px;
  color: #aaa;
  margin: 0 0 16px;
}

.card-rarity-stars {
  display: flex;
  justify-content: center;
  gap: 4px;
}

.rarity-star {
  font-size: 24px;
}

.rarity-star-5 {
  color: #ffd700;
  text-shadow: 0 0 10px rgba(255, 215, 0, 0.6);
}
.rarity-star-4 {
  color: #ce93d8;
  text-shadow: 0 0 10px rgba(206, 147, 216, 0.6);
}
.rarity-star-3 {
  color: #64b5f6;
}
.rarity-star-2 {
  color: #81c784;
}
.rarity-star-1 {
  color: #ffb74d;
}

.card-footer {
  padding: 0 24px 24px;
}

.confirm-btn {
  width: 100%;
  height: 44px;
  border-radius: 22px;
  font-size: 16px;
  font-weight: 600;
  border: none;
  cursor: pointer;
  transition: all 0.2s;

  &.btn-rarity-5 {
    background: linear-gradient(135deg, #ffd700, #ffa500);
    color: #5d4e00;
    &:hover {
      box-shadow: 0 4px 20px rgba(255, 215, 0, 0.6);
    }
  }

  &.btn-rarity-4 {
    background: linear-gradient(135deg, #ce93d8, #ba68c8);
    color: white;
    &:hover {
      box-shadow: 0 4px 20px rgba(186, 104, 200, 0.6);
    }
  }

  &.btn-rarity-3 {
    background: linear-gradient(135deg, #64b5f6, #42a5f5);
    color: white;
    &:hover {
      box-shadow: 0 4px 20px rgba(66, 165, 245, 0.6);
    }
  }

  &.btn-rarity-2 {
    background: linear-gradient(135deg, #81c784, #66bb6a);
    color: white;
    &:hover {
      box-shadow: 0 4px 20px rgba(102, 187, 106, 0.6);
    }
  }

  &.btn-rarity-1 {
    background: linear-gradient(135deg, #ffb74d, #ffa726);
    color: #5d3e00;
    &:hover {
      box-shadow: 0 4px 20px rgba(255, 167, 38, 0.6);
    }
  }
}
</style>
