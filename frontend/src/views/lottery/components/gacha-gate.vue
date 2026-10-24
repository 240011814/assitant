<script setup lang="ts">
import { NButton, NInput } from "naive-ui";

/** 原神抽卡模式的祈愿入口: 星星装饰 + 祈愿按钮 + 姓名输入 */
defineProps<{
  drawing: boolean;
  canDraw: boolean;
  drawButtonText: string;
}>();

const userName = defineModel<string>("userName", { required: true });

const emit = defineEmits<{
  draw: [];
}>();
</script>

<template>
  <div class="gacha-section">
    <div class="gacha-container">
      <!-- 祈愿按钮 -->
      <div class="gacha-gate">
        <div class="gacha-stars">
          <span
            v-for="i in 5"
            :key="i"
            class="star"
            :style="{ animationDelay: `${i * 0.2}s` }"
          >✦</span>
        </div>
        <NButton
          type="primary"
          size="large"
          round
          :loading="drawing"
          :disabled="!canDraw"
          class="gacha-btn"
          @click="emit('draw')"
        >
          <span class="gacha-btn-icon">✦</span>
          {{ drawButtonText }}
        </NButton>
        <p class="gacha-hint">消耗一次祈愿机会</p>
      </div>
    </div>

    <!-- 输入 -->
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
    </div>
  </div>
</template>

<style scoped lang="scss">
.gacha-section {
  background: white;
  border-radius: 16px;
  padding: 24px 20px;
  margin-bottom: 20px;
  box-shadow: 0 8px 30px rgba(0, 0, 0, 0.15);
}

.gacha-container {
  text-align: center;
  margin-bottom: 24px;
}

.gacha-gate {
  position: relative;
  padding: 40px 0;
}

.gacha-stars {
  display: flex;
  justify-content: center;
  gap: 16px;
  margin-bottom: 24px;
}

.star {
  font-size: 24px;
  color: #ffd700;
  animation: twinkle 2s ease-in-out infinite;
}

@keyframes twinkle {
  0%,
  100% {
    opacity: 0.5;
    transform: scale(1);
  }
  50% {
    opacity: 1;
    transform: scale(1.2);
  }
}

.gacha-btn {
  height: 56px;
  padding: 0 48px;
  font-size: 18px;
  font-weight: 600;
  background: linear-gradient(135deg, #ffd700 0%, #ffa500 100%);
  border: none;
  color: #5d4e00;
  box-shadow: 0 4px 20px rgba(255, 215, 0, 0.4);

  &:hover:not(:disabled) {
    transform: translateY(-2px);
    box-shadow: 0 8px 30px rgba(255, 215, 0, 0.6);
  }

  &:active:not(:disabled) {
    transform: translateY(0);
  }
}

.gacha-btn-icon {
  margin-right: 8px;
  font-size: 20px;
}

.gacha-hint {
  margin-top: 12px;
  font-size: 12px;
  color: #999;
}

.draw-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 280px;
  margin: 0 auto;
}
</style>
