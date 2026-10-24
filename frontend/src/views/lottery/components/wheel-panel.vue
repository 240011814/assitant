<script setup lang="ts">
import { NButton, NInput } from "naive-ui";

/**
 * 转盘模式抽奖面板: SVG 扇区转盘 + 指针 + 灯泡装饰 + 姓名输入/抽奖按钮。
 * 扇区几何计算 (getSegmentPath/getTextPos) 与渲染内聚, 抽奖逻辑在父组件。
 */
defineProps<{
  wheelItems: { id: number; name: string; isPrize: boolean; prize?: Api.Lottery.Prize }[];
  segmentColors: string[];
  pointerRotation: number;
  drawing: boolean;
  canDraw: boolean;
  drawButtonText: string;
}>();

const userName = defineModel<string>("userName", { required: true });

const emit = defineEmits<{
  draw: [];
}>();

function getSegmentPath(index: number, total: number) {
  const angle = 360 / total;
  const startRad = ((index * angle - 90) * Math.PI) / 180;
  const endRad = (((index + 1) * angle - 90) * Math.PI) / 180;
  const r = 140;
  const cx = 150;
  const cy = 150;
  const x1 = cx + r * Math.cos(startRad);
  const y1 = cy + r * Math.sin(startRad);
  const x2 = cx + r * Math.cos(endRad);
  const y2 = cy + r * Math.sin(endRad);
  const large = angle > 180 ? 1 : 0;
  return `M ${cx} ${cy} L ${x1} ${y1} A ${r} ${r} 0 ${large} 1 ${x2} ${y2} Z`;
}

function getTextPos(index: number, total: number) {
  const angle = 360 / total;
  const midRad = ((index * angle + angle / 2 - 90) * Math.PI) / 180;
  const r = 90;
  const cx = 150;
  const cy = 150;
  return {
    x: cx + r * Math.cos(midRad),
    y: cy + r * Math.sin(midRad),
    rotate: index * angle + angle / 2,
  };
}
</script>

<template>
  <div class="wheel-section">
    <!-- 装饰星星 -->
    <div class="wheel-decor-stars">
      <span
        v-for="i in 8"
        :key="i"
        class="decor-sparkle"
        :style="{ animationDelay: `${i * 0.3}s` }"
      >✦</span>
    </div>

    <div class="wheel-container">
      <!-- 外圈装饰 -->
      <div class="wheel-outer-ring" />
      <div class="wheel-inner-ring" />

      <!-- 转盘 -->
      <svg viewBox="0 0 300 300" class="wheel-svg">
        <defs>
          <filter id="wheelShadow">
            <feDropShadow dx="0" dy="0" stdDeviation="3" flood-opacity="0.3" />
          </filter>
        </defs>
        <!-- 扇区 -->
        <g
          v-for="(item, index) in wheelItems"
          :key="item.id"
          filter="url(#wheelShadow)"
        >
          <path
            :d="getSegmentPath(index, wheelItems.length)"
            :fill="segmentColors[index % segmentColors.length]"
            stroke="rgba(255,255,255,0.8)"
            stroke-width="2"
          />
          <text
            :x="getTextPos(index, wheelItems.length).x"
            :y="getTextPos(index, wheelItems.length).y"
            text-anchor="middle"
            dominant-baseline="middle"
            fill="white"
            font-size="11"
            font-weight="bold"
            style="text-shadow: 0 1px 3px rgba(0, 0, 0, 0.5)"
            :transform="`rotate(${getTextPos(index, wheelItems.length).rotate}, ${
              getTextPos(index, wheelItems.length).x
            }, ${getTextPos(index, wheelItems.length).y})`"
          >
            {{ item.name.length > 5 ? item.name.slice(0, 5) + ".." : item.name }}
          </text>
        </g>
        <!-- 外边框 -->
        <circle
          cx="150"
          cy="150"
          r="145"
          fill="none"
          stroke="url(#goldGradient)"
          stroke-width="6"
        />
        <!-- 内边框 -->
        <circle
          cx="150"
          cy="150"
          r="138"
          fill="none"
          stroke="rgba(255,255,255,0.3)"
          stroke-width="1"
        />
        <!-- 中心装饰 -->
        <circle
          cx="150"
          cy="150"
          r="28"
          fill="url(#centerGradient)"
          stroke="white"
          stroke-width="3"
        />
        <circle cx="150" cy="150" r="18" fill="white" opacity="0.9" />
        <text
          x="150"
          y="155"
          text-anchor="middle"
          dominant-baseline="middle"
          font-size="14"
          font-weight="bold"
          fill="#667eea"
        >
          GO
        </text>
        <!-- 渐变定义 -->
        <defs>
          <linearGradient id="goldGradient" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" style="stop-color: #ffd700" />
            <stop offset="50%" style="stop-color: #ffa500" />
            <stop offset="100%" style="stop-color: #ffd700" />
          </linearGradient>
          <radialGradient id="centerGradient">
            <stop offset="0%" style="stop-color: #667eea" />
            <stop offset="100%" style="stop-color: #764ba2" />
          </radialGradient>
        </defs>
      </svg>

      <!-- 指针 -->
      <div
        class="pointer-wrapper"
        :style="{
          transform: `rotate(${pointerRotation}deg)`,
          transition: drawing
            ? 'transform 3.5s cubic-bezier(0.17, 0.67, 0.12, 0.99)'
            : 'none',
        }"
      >
        <div class="pointer" />
      </div>

      <!-- 灯泡装饰 -->
      <div class="wheel-lights">
        <span
          v-for="i in 12"
          :key="i"
          class="light-bulb"
          :class="{ 'light-on': !drawing }"
          :style="{ transform: `rotate(${i * 30}deg) translateY(-155px)` }"
        />
      </div>
    </div>

    <!-- 输入和按钮 -->
    <div class="draw-form">
      <NInput
        v-model:value="userName"
        placeholder="请输入您的姓名"
        size="large"
        round
        :disabled="!canDraw"
        @keyup.enter="emit('draw')"
      >
        <template #prefix>👤</template>
      </NInput>

      <NButton
        type="primary"
        size="large"
        round
        block
        :loading="drawing"
        :disabled="!canDraw"
        class="draw-btn"
        @click="emit('draw')"
      >
        🎯 {{ drawButtonText }}
      </NButton>
    </div>
  </div>
</template>

<style scoped lang="scss">
.wheel-section {
  background: linear-gradient(135deg, #f5f7fa 0%, #e4e8ec 100%);
  border-radius: 20px;
  padding: 30px 20px;
  margin-bottom: 20px;
  box-shadow: 0 10px 40px rgba(0, 0, 0, 0.15);
  position: relative;
  overflow: hidden;

  &::before {
    content: "";
    position: absolute;
    top: -50%;
    left: -50%;
    width: 200%;
    height: 200%;
    background: radial-gradient(circle, rgba(102, 126, 234, 0.05) 0%, transparent 50%);
    animation: sectionGlow 8s ease-in-out infinite;
  }
}

@keyframes sectionGlow {
  0%,
  100% {
    transform: translate(0, 0);
  }
  50% {
    transform: translate(5%, 5%);
  }
}

.wheel-decor-stars {
  display: flex;
  justify-content: center;
  gap: 20px;
  margin-bottom: 20px;
  position: relative;
  z-index: 1;
}

.decor-sparkle {
  font-size: 18px;
  color: #ffd700;
  animation: sparkleFloat 2s ease-in-out infinite;
}

@keyframes sparkleFloat {
  0%,
  100% {
    transform: translateY(0) scale(1);
    opacity: 0.6;
  }
  50% {
    transform: translateY(-8px) scale(1.2);
    opacity: 1;
  }
}

.wheel-container {
  position: relative;
  width: 320px;
  height: 320px;
  margin: 0 auto 24px;
  z-index: 1;
}

.wheel-outer-ring {
  position: absolute;
  top: -10px;
  left: -10px;
  right: -10px;
  bottom: -10px;
  border-radius: 50%;
  border: 3px solid rgba(255, 215, 0, 0.3);
  animation: ringPulse 3s ease-in-out infinite;
}

.wheel-inner-ring {
  position: absolute;
  top: -5px;
  left: -5px;
  right: -5px;
  bottom: -5px;
  border-radius: 50%;
  border: 2px solid rgba(102, 126, 234, 0.2);
}

@keyframes ringPulse {
  0%,
  100% {
    transform: scale(1);
    opacity: 0.5;
  }
  50% {
    transform: scale(1.02);
    opacity: 1;
  }
}

.wheel-svg {
  width: 100%;
  height: 100%;
  filter: drop-shadow(0 8px 20px rgba(0, 0, 0, 0.2));
  position: relative;
  z-index: 2;
}

/* 指针 - 固定在顶部中心 */
.pointer-wrapper {
  position: absolute;
  top: 0;
  left: 50%;
  width: 0;
  height: 0;
  transform-origin: 0 160px;
  z-index: 10;
}

.pointer {
  position: absolute;
  left: -15px;
  top: -8px;
  width: 30px;
  height: 168px;
  transform-origin: center bottom;
}

.pointer::before {
  content: "";
  position: absolute;
  top: 0;
  left: 50%;
  transform: translateX(-50%);
  width: 0;
  height: 0;
  border-left: 18px solid transparent;
  border-right: 18px solid transparent;
  border-top: 30px solid #ff4757;
  filter: drop-shadow(0 4px 8px rgba(255, 71, 87, 0.6));
}

.pointer::after {
  content: "";
  position: absolute;
  top: 28px;
  left: 50%;
  transform: translateX(-50%);
  width: 10px;
  height: 135px;
  background: linear-gradient(180deg, #ff4757 0%, #ff6b81 50%, #ff4757 100%);
  border-radius: 0 0 5px 5px;
  box-shadow: 0 2px 8px rgba(255, 71, 87, 0.4);
}

.wheel-lights {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 0;
  height: 0;
  z-index: 1;
}

.light-bulb {
  position: absolute;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: #ddd;
  transition: all 0.3s;

  &.light-on {
    background: #ffd700;
    box-shadow: 0 0 10px rgba(255, 215, 0, 0.8);
    animation: bulbTwinkle 1.5s ease-in-out infinite;
  }
}

@keyframes bulbTwinkle {
  0%,
  100% {
    opacity: 0.6;
  }
  50% {
    opacity: 1;
  }
}

.draw-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 280px;
  margin: 0 auto;
}

.draw-btn {
  height: 48px;
  font-size: 16px;
  font-weight: 600;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  border: none;
  box-shadow: 0 4px 15px rgba(102, 126, 234, 0.4);

  &:hover:not(:disabled) {
    transform: translateY(-1px);
    box-shadow: 0 6px 20px rgba(102, 126, 234, 0.5);
  }
}
</style>
