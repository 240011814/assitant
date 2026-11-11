<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from "vue";
import { NButton, NInput } from "naive-ui";
import { WHEEL_SPIN_MS } from "../composables/use-gacha-animation";

/**
 * 转盘模式抽奖面板: SVG 扇区转盘 + 金属外圈 + 跑马灯 + 金色饰针指针 + 姓名输入/抽奖按钮。
 * 扇区几何计算 (getSegmentPath/getTextPos) 与渲染内聚, 抽奖逻辑在父组件;
 * 旋转机制不变 (pointerRotation + 3.5s CSS transition), 落定时指针回弹。
 * 容器响应式: min(320px, 100%), 灯泡/指针用百分比定位, 移动端不溢出。
 */
const props = defineProps<{
  wheelItems: { id: number; name: string; isPrize: boolean; prize?: Api.Lottery.Prize }[];
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

/** 带图扇区: 图标位置 (半径 74) 与图下名称位置 (半径 114, 水平排布) */
function getIconPos(index: number, total: number) {
  const angle = 360 / total;
  const midRad = ((index * angle + angle / 2 - 90) * Math.PI) / 180;
  const cx = 150;
  const cy = 150;
  return {
    icon: { x: cx + 74 * Math.cos(midRad), y: cy + 74 * Math.sin(midRad) },
    label: { x: cx + 112 * Math.cos(midRad), y: cy + 112 * Math.sin(midRad) },
  };
}

/** 跑马灯灯泡沿外圈百分比定位 (半径 51.5%, 随容器缩放) */
function bulbStyle(i: number) {
  const angle = ((i - 1) * 22.5 - 90) * (Math.PI / 180);
  const r = 51.5;
  return {
    left: `${50 + r * Math.cos(angle)}%`,
    top: `${50 + r * Math.sin(angle)}%`,
    animationDelay: props.drawing ? `${(i % 4) * 0.15}s` : `${i * 0.22}s`,
  };
}

// 指针落定回弹: pointerRotation 每次抽奖更新一次, 旋转过渡 WHEEL_SPIN_MS, 结束时触发 wobble
const pointerLanded = ref(false);
let landedTimer = 0;

watch(
  () => props.pointerRotation,
  () => {
    window.clearTimeout(landedTimer);
    landedTimer = window.setTimeout(() => {
      pointerLanded.value = true;
      landedTimer = window.setTimeout(() => {
        pointerLanded.value = false;
      }, 900);
    }, WHEEL_SPIN_MS - 50);
  }
);

onBeforeUnmount(() => {
  window.clearTimeout(landedTimer);
});
</script>

<template>
  <div class="wheel-section">
    <!-- 顶部氛围灯带 -->
    <div class="wheel-lights-bar">
      <span v-for="i in 14" :key="i" class="bar-light" :style="{ animationDelay: `${(i % 7) * 0.12}s` }" />
    </div>

    <div class="wheel-container">
      <!-- 金属外圈 -->
      <div class="wheel-rim" :class="{ spinning: drawing }" />
      <div class="wheel-rim-inner" />

      <!-- 转盘 -->
      <svg viewBox="0 0 300 300" class="wheel-svg">
        <defs>
          <filter id="wheelShadow">
            <feDropShadow dx="0" dy="0" stdDeviation="3" flood-opacity="0.35" />
          </filter>
          <!-- 半透明玻璃渐变扇区 (参考图: 紫/蓝/粉 交替) -->
          <linearGradient id="segGrad0" x1="0" y1="0" x2="0.7" y2="1">
            <stop offset="0%" stop-color="rgba(148, 96, 235, 0.92)" />
            <stop offset="100%" stop-color="rgba(78, 42, 158, 0.94)" />
          </linearGradient>
          <linearGradient id="segGrad1" x1="0" y1="0" x2="0.7" y2="1">
            <stop offset="0%" stop-color="rgba(88, 134, 240, 0.92)" />
            <stop offset="100%" stop-color="rgba(44, 74, 176, 0.94)" />
          </linearGradient>
          <linearGradient id="segGrad2" x1="0" y1="0" x2="0.7" y2="1">
            <stop offset="0%" stop-color="rgba(206, 104, 214, 0.92)" />
            <stop offset="100%" stop-color="rgba(136, 52, 156, 0.94)" />
          </linearGradient>
          <linearGradient id="segGrad3" x1="0" y1="0" x2="0.7" y2="1">
            <stop offset="0%" stop-color="rgba(110, 78, 214, 0.92)" />
            <stop offset="100%" stop-color="rgba(56, 34, 132, 0.94)" />
          </linearGradient>
          <!-- 玻璃光泽: 左上高光 → 右下微暗, 叠在扇区上增加立体感 -->
          <linearGradient id="wheelGloss" x1="0" y1="0" x2="0.55" y2="1">
            <stop offset="0%" stop-color="rgba(255,255,255,0.26)" />
            <stop offset="42%" stop-color="rgba(255,255,255,0.06)" />
            <stop offset="100%" stop-color="rgba(10,5,30,0.3)" />
          </linearGradient>
          <radialGradient id="hubGradient" cx="50%" cy="38%" r="70%">
            <stop offset="0%" stop-color="#fdf6e0" />
            <stop offset="45%" stop-color="#e9c25a" />
            <stop offset="100%" stop-color="#9a6f18" />
          </radialGradient>
        </defs>
        <!-- 扇区 (半透明玻璃渐变 + 金描边); 配了图片的奖品显示圆形图标 + 图下名称 -->
        <g
          v-for="(item, index) in wheelItems"
          :key="item.id"
          filter="url(#wheelShadow)"
        >
          <path
            :d="getSegmentPath(index, wheelItems.length)"
            :fill="`url(#segGrad${index % 4})`"
            stroke="rgba(232, 184, 75, 0.85)"
            stroke-width="1.8"
          />
          <template v-if="item.prize?.imageUrl">
            <clipPath :id="`prize-clip-${item.id}`">
              <circle
                :cx="getIconPos(index, wheelItems.length).icon.x"
                :cy="getIconPos(index, wheelItems.length).icon.y"
                r="20"
              />
            </clipPath>
            <image
              :href="item.prize.imageUrl"
              :x="getIconPos(index, wheelItems.length).icon.x - 20"
              :y="getIconPos(index, wheelItems.length).icon.y - 20"
              width="40"
              height="40"
              :clip-path="`url(#prize-clip-${item.id})`"
              preserveAspectRatio="xMidYMid slice"
            />
            <circle
              :cx="getIconPos(index, wheelItems.length).icon.x"
              :cy="getIconPos(index, wheelItems.length).icon.y"
              r="20"
              fill="none"
              stroke="rgba(255, 226, 130, 0.8)"
              stroke-width="2"
            />
            <text
              :x="getIconPos(index, wheelItems.length).label.x"
              :y="getIconPos(index, wheelItems.length).label.y"
              text-anchor="middle"
              dominant-baseline="middle"
              fill="#ffffff"
              font-size="11"
              font-weight="bold"
              paint-order="stroke"
              stroke="rgba(24, 14, 52, 0.65)"
              stroke-width="2.5"
            >
              {{ item.name.length > 8 ? item.name.slice(0, 8) + ".." : item.name }}
            </text>
          </template>
          <text
            v-else
            :x="getTextPos(index, wheelItems.length).x"
            :y="getTextPos(index, wheelItems.length).y"
            text-anchor="middle"
            dominant-baseline="middle"
            fill="#ffffff"
            font-size="12"
            font-weight="bold"
            paint-order="stroke"
            stroke="rgba(24, 14, 52, 0.65)"
            stroke-width="2.5"
            :transform="`rotate(${getTextPos(index, wheelItems.length).rotate}, ${
              getTextPos(index, wheelItems.length).x
            }, ${getTextPos(index, wheelItems.length).y})`"
          >
            {{ item.name.length > 6 ? item.name.slice(0, 6) + ".." : item.name }}
          </text>
        </g>
        <!-- 光泽叠层 -->
        <circle cx="150" cy="150" r="140" fill="url(#wheelGloss)" pointer-events="none" />
        <!-- 金色内圈描线 -->
        <circle
          cx="150"
          cy="150"
          r="138"
          fill="none"
          stroke="rgba(255, 226, 130, 0.5)"
          stroke-width="1.5"
        />
        <!-- 金色星芒枢纽 (参考图: 八角星芒 + 亮芯) -->
        <g transform="translate(150,150)">
          <rect x="-26" y="-26" width="52" height="52" rx="7" fill="url(#hubGradient)" stroke="#7a5518" stroke-width="1.6" />
          <rect x="-26" y="-26" width="52" height="52" rx="7" fill="url(#hubGradient)" stroke="#7a5518" stroke-width="1.6" transform="rotate(45)" />
          <circle r="18" fill="rgba(255, 246, 210, 0.96)" stroke="#caa53d" stroke-width="2" />
          <text
            y="1"
            text-anchor="middle"
            dominant-baseline="middle"
            font-size="14"
            font-weight="900"
            fill="#8a6510"
            letter-spacing="1"
          >
            GO
          </text>
        </g>
      </svg>

      <!-- 外圈金色饰钻 (上下左右四方位, 不随盘转动) -->
      <span
        v-for="(a, i) in [0, 90, 180, 270]"
        :key="`stud-${i}`"
        class="rim-stud"
        :style="{
          left: `${50 + 50 * Math.cos(((a - 90) * Math.PI) / 180)}%`,
          top: `${50 + 50 * Math.sin(((a - 90) * Math.PI) / 180)}%`,
        }"
      />

      <!-- 跑马灯灯泡 (百分比定位) -->
      <div class="wheel-lights" :class="{ chasing: drawing }">
        <span v-for="i in 16" :key="i" class="light-bulb" :style="bulbStyle(i)" />
      </div>

      <!-- 金色饰针指针 (整层旋转, 针挂在顶部) -->
      <div
        class="pointer-layer"
        :style="{
          transform: `rotate(${pointerRotation}deg)`,
          transition: drawing
            ? `transform ${WHEEL_SPIN_MS}ms cubic-bezier(0.17, 0.67, 0.12, 0.99)`
            : 'none',
        }"
      >
        <div class="pointer-pos">
          <svg
            class="pointer-svg"
            :class="{ landed: pointerLanded }"
            viewBox="0 0 60 100"
            aria-hidden="true"
          >
            <defs>
              <linearGradient id="pointerGold" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stop-color="#ffe28a" />
                <stop offset="45%" stop-color="#f0c040" />
                <stop offset="100%" stop-color="#9a6f18" />
              </linearGradient>
            </defs>
            <circle cx="30" cy="14" r="12" fill="url(#pointerGold)" stroke="#5d4a00" stroke-width="2" />
            <circle cx="30" cy="14" r="5.5" fill="#fff3c4" opacity="0.9" />
            <path
              d="M30 24 L14 58 L30 49 L46 58 Z"
              fill="url(#pointerGold)"
              stroke="#5d4a00"
              stroke-width="2"
              stroke-linejoin="round"
            />
          </svg>
        </div>
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
  background:
    radial-gradient(ellipse at 50% -10%, rgba(255, 214, 110, 0.14) 0%, transparent 55%),
    linear-gradient(165deg, rgba(34, 24, 74, 0.72) 0%, rgba(22, 14, 52, 0.78) 55%, rgba(14, 8, 36, 0.84) 100%);
  border: 1px solid rgba(255, 214, 110, 0.26);
  backdrop-filter: blur(5px);
  border-radius: 20px;
  padding: 26px 20px 30px;
  margin-bottom: 20px;
  box-shadow: 0 10px 40px rgba(0, 0, 0, 0.35);
  position: relative;
  overflow: hidden;
}

// 外圈金色饰钻 (四方位, 与转盘同心)
.rim-stud {
  position: absolute;
  width: 7.2%;
  aspect-ratio: 1;
  transform: translate(-50%, -50%) rotate(45deg);
  border-radius: 16%;
  background: linear-gradient(135deg, #ffe28a 0%, #e9b949 45%, #a8781c 100%);
  border: 2px solid #7a5518;
  box-shadow:
    0 0 12px rgba(255, 214, 110, 0.75),
    inset 0 0 4px rgba(255, 255, 255, 0.65);
  z-index: 5;
  pointer-events: none;
}

// 顶部氛围灯带
.wheel-lights-bar {
  display: flex;
  justify-content: center;
  gap: 14px;
  margin-bottom: 18px;
}

.bar-light {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: rgba(255, 214, 110, 0.35);
  animation: barTwinkle 1.8s ease-in-out infinite;
}

@keyframes barTwinkle {
  0%,
  100% {
    opacity: 0.35;
    transform: scale(0.85);
  }
  50% {
    opacity: 1;
    transform: scale(1.25);
    box-shadow: 0 0 8px rgba(255, 214, 110, 0.8);
  }
}

// 响应式: 小屏收窄到容器宽度, 内部全部百分比/相对定位
.wheel-container {
  position: relative;
  width: min(320px, 100%);
  aspect-ratio: 1;
  margin: 0 auto 24px;
}

// 金属拉丝外圈: 多段 conic-gradient 交替明暗 + 内外阴影
.wheel-rim {
  position: absolute;
  inset: -4.5%;
  border-radius: 50%;
  background: conic-gradient(
    from 6deg,
    #8a5a00,
    #ffd700 7%,
    #7a4f00 15%,
    #ffe28a 24%,
    #8a5a00 33%,
    #ffd700 44%,
    #6e4600 54%,
    #ffe28a 65%,
    #8a5a00 74%,
    #ffd700 86%,
    #8a5a00
  );
  box-shadow:
    0 12px 34px rgba(0, 0, 0, 0.5),
    inset 0 2px 5px rgba(255, 255, 255, 0.55),
    inset 0 -4px 9px rgba(0, 0, 0, 0.45);
  transition: box-shadow 0.3s;

  &.spinning {
    box-shadow:
      0 12px 44px rgba(0, 0, 0, 0.6),
      0 0 26px rgba(255, 214, 110, 0.4),
      inset 0 2px 5px rgba(255, 255, 255, 0.55),
      inset 0 -4px 9px rgba(0, 0, 0, 0.45);
  }
}

.wheel-rim-inner {
  position: absolute;
  inset: -2%;
  border-radius: 50%;
  border: 2px solid rgba(70, 48, 0, 0.55);
  box-shadow: inset 0 0 14px rgba(0, 0, 0, 0.45);
  pointer-events: none;
}

.wheel-svg {
  width: 100%;
  height: 100%;
  filter: drop-shadow(0 8px 20px rgba(0, 0, 0, 0.35));
  position: relative;
  z-index: 2;
  display: block;
}

// 跑马灯: 待机柔和交替闪烁, 旋转时按序追逐
.wheel-lights {
  position: absolute;
  inset: 0;
  z-index: 3;
  pointer-events: none;
}

.light-bulb {
  position: absolute;
  width: 3.4%;
  aspect-ratio: 1;
  border-radius: 50%;
  transform: translate(-50%, -50%);
  background: #8a6d1f;
  box-shadow: inset 0 0 3px rgba(0, 0, 0, 0.5);
  animation: bulbIdle 2.2s ease-in-out infinite;

  .chasing & {
    animation: bulbChase 0.6s linear infinite;
  }
}

@keyframes bulbIdle {
  0%,
  100% {
    background: #8a6d1f;
    box-shadow: inset 0 0 3px rgba(0, 0, 0, 0.5);
  }
  50% {
    background: #ffe28a;
    box-shadow: 0 0 10px rgba(255, 214, 110, 0.85);
  }
}

@keyframes bulbChase {
  0%,
  62% {
    background: #6b5416;
    box-shadow: none;
  }
  74% {
    background: #fff7cf;
    box-shadow: 0 0 14px rgba(255, 226, 138, 0.95);
  }
  100% {
    background: #6b5416;
    box-shadow: none;
  }
}

// 指针层: 整层绕盘心旋转, 金饰针挂在顶部
.pointer-layer {
  position: absolute;
  inset: 0;
  z-index: 10;
  pointer-events: none;
}

.pointer-pos {
  position: absolute;
  top: -5%;
  left: 50%;
  height: 44%;
  transform: translateX(-50%);
}

.pointer-svg {
  height: 100%;
  width: auto;
  display: block;
  filter: drop-shadow(0 4px 10px rgba(0, 0, 0, 0.45));

  &.landed {
    transform-origin: 50% 14%;
    animation: pointerWobble 0.9s cubic-bezier(0.36, 0.07, 0.19, 0.97);
  }
}

@keyframes pointerWobble {
  0% {
    transform: rotate(0);
  }
  22% {
    transform: rotate(10deg);
  }
  46% {
    transform: rotate(-6.5deg);
  }
  68% {
    transform: rotate(3.5deg);
  }
  86% {
    transform: rotate(-1.8deg);
  }
  100% {
    transform: rotate(0);
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
  background: linear-gradient(135deg, #f0c040 0%, #d99a1f 55%, #b97e10 100%);
  border: none;
  color: #3d2e00;
  box-shadow: 0 4px 18px rgba(240, 192, 64, 0.35);

  &:hover:not(:disabled) {
    transform: translateY(-1px);
    box-shadow: 0 6px 24px rgba(240, 192, 64, 0.5);
  }
}
</style>
