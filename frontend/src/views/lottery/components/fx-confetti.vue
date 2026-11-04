<script setup lang="ts">
import { onBeforeUnmount, ref } from "vue";

/**
 * 全屏彩带喷发特效 (转盘中奖时按稀有度配色触发):
 * burst(colors) 从屏幕底部两角喷出纸屑, rAF 驱动, 粒子落尽后自停并清屏。
 */
const canvasRef = ref<HTMLCanvasElement | null>(null);

interface Piece {
  x: number;
  y: number;
  vx: number;
  vy: number;
  w: number;
  h: number;
  rot: number;
  vrot: number;
  color: string;
  circle: boolean;
  life: number;
  maxLife: number;
}

let rafId = 0;
let pieces: Piece[] = [];
let lastTs = 0;
let running = false;

function burst(colors: string[], count = 130) {
  const canvas = canvasRef.value;
  if (!canvas || colors.length === 0) return;
  const W = (canvas.width = window.innerWidth);
  const H = (canvas.height = window.innerHeight);

  for (let i = 0; i < count; i += 1) {
    // 左右两底角向中上方喷发
    const fromLeft = i % 2 === 0;
    const maxLife = 2.2 + Math.random() * 0.9;
    pieces.push({
      x: fromLeft ? W * 0.08 : W * 0.92,
      y: H * 0.92,
      vx: (fromLeft ? 1 : -1) * (140 + Math.random() * 420),
      vy: -(620 + Math.random() * 520),
      w: 6 + Math.random() * 7,
      h: 9 + Math.random() * 8,
      rot: Math.random() * Math.PI * 2,
      vrot: (Math.random() - 0.5) * 12,
      color: colors[Math.floor(Math.random() * colors.length)],
      circle: Math.random() < 0.3,
      life: maxLife,
      maxLife
    });
  }

  if (!running) {
    running = true;
    lastTs = performance.now();
    rafId = requestAnimationFrame(tick);
  }
}

function tick(ts: number) {
  const canvas = canvasRef.value;
  if (!canvas) return;
  const ctx = canvas.getContext("2d");
  if (!ctx) return;
  const dt = Math.min((ts - lastTs) / 1000, 0.04);
  lastTs = ts;

  ctx.clearRect(0, 0, canvas.width, canvas.height);
  const gravity = 1500;
  const drag = Math.pow(0.4, dt);

  pieces = pieces.filter(p => {
    p.life -= dt;
    if (p.life <= 0 || p.y > canvas.height + 40) return false;
    p.vy += gravity * dt;
    p.vx *= drag;
    p.x += p.vx * dt;
    p.y += p.vy * dt;
    p.rot += p.vrot * dt;

    // 尾段淡出
    const fade = Math.min(p.life / 0.5, 1);
    ctx.save();
    ctx.globalAlpha = fade;
    ctx.translate(p.x, p.y);
    ctx.rotate(p.rot);
    ctx.fillStyle = p.color;
    if (p.circle) {
      ctx.beginPath();
      ctx.arc(0, 0, p.w / 2, 0, Math.PI * 2);
      ctx.fill();
    } else {
      // 翻转效果: 高度按旋转相位压缩
      ctx.fillRect(-p.w / 2, (-p.h / 2) * Math.abs(Math.sin(p.rot * 1.7)), p.w, p.h);
    }
    ctx.restore();
    return true;
  });

  if (pieces.length > 0) {
    rafId = requestAnimationFrame(tick);
  } else {
    running = false;
    ctx.clearRect(0, 0, canvas.width, canvas.height);
  }
}

onBeforeUnmount(() => {
  cancelAnimationFrame(rafId);
  pieces = [];
  running = false;
});

defineExpose({ burst });
</script>

<template>
  <canvas ref="canvasRef" class="fx-confetti" />
</template>

<style scoped>
.fx-confetti {
  position: fixed;
  inset: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  z-index: 3000;
}
</style>
