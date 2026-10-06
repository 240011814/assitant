<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { NButton, NInput, NModal, NScrollbar, NSpin, NTag, useMessage } from "naive-ui";
import {
  fetchDrawLottery,
  fetchGetDrawLimits,
  fetchGetLotteryActivity,
  fetchGetLotteryPrizes,
  fetchGetLotteryRecords,
  fetchGetLotteryWinners,
} from "@/service/api";
import { useAudioCue } from "./composables/use-audio-cue";
import { useGachaAnimation } from "./composables/use-gacha-animation";

const route = useRoute();
const message = useMessage();

const loading = ref(true);
const drawing = ref(false);
const activity = ref<Api.Lottery.Activity | null>(null);
const prizes = ref<Api.Lottery.Prize[]>([]);
const winners = ref<Api.Lottery.Record[]>([]);
const records = ref<Api.Lottery.Record[]>([]);
const userName = ref("");
const showResultModal = ref(false);
const showPrizeModal = ref(false);
const showWinnerModal = ref(false);
const drawResult = ref<Api.Lottery.DrawResult | null>(null);
const drawLimits = ref<Api.Lottery.DrawLimits | null>(null);

// 音效管理
const { playClickSound, playGachaSound, playWheelSpinSound, playWinSound, playLoseSound } = useAudioCue();

// 转盘/原神抽卡动画状态机
const {
  pointerRotation,
  showGachaAnimation,
  gachaPhase,
  gachaParticles,
  gachaRarity,
  spinToSegment,
  handleGachaDraw,
  closeGachaAnimation
} = useGachaAnimation({
  userName,
  drawing,
  drawResult,
  getActivityId: () => Number(route.params.id),
  message,
  playClickSound,
  playGachaSound,
  loadData
});

const segmentColors = [
  "#FF6B6B",
  "#4ECDC4",
  "#45B7D1",
  "#96CEB4",
  "#FFEAA7",
  "#DDA0DD",
  "#98D8C8",
  "#F7DC6F",
  "#BB8FCE",
  "#85C1E9",
  "#F8C471",
  "#82E0AA",
];

const wheelItems = computed(() => {
  const items: {
    id: number;
    name: string;
    isPrize: boolean;
    prize: Api.Lottery.Prize | undefined;
  }[] = prizes.value.map((p) => ({
    id: p.id,
    name: p.name,
    isPrize: true,
    prize: p,
  }));

  const totalProb = prizes.value.reduce((sum, p) => sum + p.probability, 0);

  if (totalProb < 1) {
    items.push({
      id: 0,
      name: "谢谢惠顾",
      isPrize: false,
      prize: undefined,
    });
  }

  return items;
});

const statusConfig = computed(() => {
  const map: Record<number, { label: string; type: string; icon: string }> = {
    0: { label: "未开始", type: "info", icon: "⏳" },
    1: { label: "进行中", type: "success", icon: "🎯" },
    2: { label: "已结束", type: "warning", icon: "🏁" },
  };
  return map[activity.value?.status || 0] || map[0];
});

const canDraw = computed(() => {
  return activity.value?.status === 1 && !drawing.value;
});

const isGachaMode = computed(() => {
  return activity.value?.drawMode === 1;
});

const drawButtonText = computed(() => {
  if (drawing.value) return "抽奖中...";
  if (activity.value?.status === 0) return "活动未开始";
  if (activity.value?.status === 2) return "活动已结束";
  return isGachaMode.value ? "祈愿一次" : "开始抽奖";
});

// 奖品等级标签
function getPrizeLevelLabel(level: number): string {
  const map: Record<number, string> = {
    0: "",
    1: "特等奖",
    2: "一等奖",
    3: "二等奖",
    4: "三等奖",
  };
  return map[level] || "";
}

function getPrizeLevelColor(level: number): string {
  const map: Record<number, string> = {
    0: "#999",
    1: "#ff4757",
    2: "#ffa502",
    3: "#2ed573",
    4: "#1e90ff",
  };
  return map[level] || "#999";
}

async function loadData() {
  const activityId = Number(route.params.id);
  if (!activityId) {
    loading.value = false;
    return;
  }

  loading.value = true;
  const [activityRes, prizesRes] = await Promise.all([
    fetchGetLotteryActivity(activityId),
    fetchGetLotteryPrizes(activityId),
  ]);

  if (!activityRes.error) {
    activity.value = activityRes.data;
  }
  if (!prizesRes.error) {
    prizes.value = prizesRes.data;
  }
  loading.value = false;

  loadRecords();
  // 只有当用户名已输入时才加载限制信息
  if (userName.value.trim()) {
    loadDrawLimits();
  }
}

async function loadWinners() {
  const activityId = Number(route.params.id);
  if (!activityId) return;

  const { data, error } = await fetchGetLotteryWinners({ activityId, pageSize: 20 });
  if (!error) {
    winners.value = data.list;
  }
}

async function loadRecords() {
  const activityId = Number(route.params.id);
  if (!activityId) return;

  const { data, error } = await fetchGetLotteryRecords({ activityId, pageSize: 20 });
  if (!error) {
    records.value = data.list;
  }
}

async function loadDrawLimits() {
  const activityId = Number(route.params.id);
  if (!activityId) return;

  const { data, error } = await fetchGetDrawLimits(
    activityId,
    userName.value.trim() || undefined
  );
  if (!error) {
    drawLimits.value = data;
  }
}

// 监听用户名变化，实时更新限制信息
watch(userName, (newVal) => {
  if (newVal.trim()) {
    loadDrawLimits();
  } else {
    // 用户名为空时重置限制信息
    drawLimits.value = null;
  }
});

async function handleDraw() {
  if (!userName.value.trim()) {
    message.warning("请输入您的姓名");
    return;
  }

  // 根据模式选择不同的抽奖方式
  if (isGachaMode.value) {
    await handleGachaDraw();
    return;
  }

  playClickSound();
  drawing.value = true;
  const activityId = Number(route.params.id);

  const { data, error } = await fetchDrawLottery(activityId, userName.value.trim());

  if (error) {
    drawing.value = false;
    message.error("抽奖失败");
    return;
  }

  let targetIndex = 0;
  if (data.isWinner && data.prize) {
    targetIndex = wheelItems.value.findIndex(
      (item) => item.isPrize && item.prize?.id === data.prize?.id
    );
    if (targetIndex === -1) targetIndex = 0;
  } else {
    const thankYouIndex = wheelItems.value.findIndex((item) => !item.isPrize);
    targetIndex = thankYouIndex >= 0 ? thankYouIndex : 0;
  }

  spinToSegment(targetIndex, wheelItems.value.length);

  // 播放旋转音效
  playWheelSpinSound();

  await new Promise((resolve) => setTimeout(resolve, 3500));

  // 撮放结果音效
  if (data.isWinner) {
    playWinSound();
  } else {
    playLoseSound();
  }

  drawing.value = false;
  drawResult.value = data;
  showResultModal.value = true;
  loadData();
}

function openPrizeModal() {
  showPrizeModal.value = true;
}

function openWinnerModal() {
  loadWinners();
  showWinnerModal.value = true;
}

function formatTime(dateStr: string) {
  const date = new Date(dateStr);
  const now = new Date();
  const diff = now.getTime() - date.getTime();
  const minutes = Math.floor(diff / 60000);
  const hours = Math.floor(diff / 3600000);
  const days = Math.floor(diff / 86400000);

  if (minutes < 1) return "刚刚";
  if (minutes < 60) return `${minutes}分钟前`;
  if (hours < 24) return `${hours}小时前`;
  if (days < 7) return `${days}天前`;
  return date.toLocaleDateString();
}

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

onMounted(() => {
  loadData();
});
</script>

<template>
  <div class="lottery-page">
    <div class="bg-overlay" />

    <div class="lottery-container">
      <header class="lottery-header">
        <h1 class="header-title">{{ isGachaMode ? "✨ 祈愿之门" : "🎰 幸运抽奖" }}</h1>
        <p class="header-subtitle">
          {{ isGachaMode ? "命运的星辉，等待你的召唤" : "转动转盘，赢取大奖" }}
        </p>
      </header>

      <NSpin :show="loading" class="w-full">
        <template v-if="activity">
          <div class="activity-card">
            <div class="card-header">
              <h2 class="card-title">{{ activity.name }}</h2>
              <NTag :type="statusConfig.type as any" :bordered="false" round size="small">
                {{ statusConfig.icon }} {{ statusConfig.label }}
              </NTag>
            </div>

            <p v-if="activity.description" class="card-desc">
              {{ activity.description }}
            </p>

            <!-- 限制信息 -->
            <div
              v-if="activity.dailyLimit > 0 || activity.totalLimit > 0"
              class="limit-info"
            >
              <div v-if="activity.dailyLimit > 0" class="limit-item">
                <span class="limit-icon">📅</span>
                <span class="limit-text">
                  今日剩余:
                  <span class="limit-count">{{
                    drawLimits
                      ? Math.max(0, activity.dailyLimit - drawLimits.userDailyUsed)
                      : "-"
                  }}</span>
                  / {{ activity.dailyLimit }} 次
                </span>
              </div>
              <div v-if="activity.totalLimit > 0" class="limit-item">
                <span class="limit-icon">🎯</span>
                <span class="limit-text">
                  总剩余:
                  <span class="limit-count">{{
                    drawLimits
                      ? Math.max(0, activity.totalLimit - drawLimits.userTotalUsed)
                      : "-"
                  }}</span>
                  / {{ activity.totalLimit }} 次
                </span>
              </div>
            </div>

            <div class="card-actions">
              <NButton quaternary size="small" @click="openPrizeModal">
                🎁 奖品 ({{ prizes.length }})
              </NButton>
              <NButton quaternary size="small" @click="openWinnerModal">
                🏆 中奖名单
              </NButton>
            </div>
          </div>

          <!-- 转盘模式 -->
          <div v-if="!isGachaMode" class="wheel-section">
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
                @keyup.enter="handleDraw"
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
                @click="handleDraw"
              >
                🎯 {{ drawButtonText }}
              </NButton>
            </div>
          </div>

          <!-- 原神抽卡模式 -->
          <div v-else class="gacha-section">
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
                  @click="handleDraw"
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
                @keyup.enter="handleDraw"
              >
                <template #prefix>👤</template>
              </NInput>
            </div>
          </div>

          <!-- 抽奖记录 -->
          <div class="records-card">
            <div class="records-header">
              <span>📋 抽奖记录 ({{ records.length }})</span>
              <NButton text type="primary" size="small" @click="loadRecords">
                刷新
              </NButton>
            </div>

            <div class="records-list">
              <template v-if="records.length">
                <div
                  v-for="record in records"
                  :key="record.id"
                  class="record-item"
                  :class="{ 'is-winner': record.isWinner }"
                >
                  <span class="record-icon">{{ record.isWinner ? "🎉" : "🎰" }}</span>
                  <div class="record-info">
                    <span class="record-name">{{
                      record.userName || `用户#${record.userId}`
                    }}</span>
                    <span class="record-time">{{ formatTime(record.createdAt) }}</span>
                  </div>
                  <div class="record-result">
                    <NTag
                      :type="record.isWinner ? 'success' : 'default'"
                      size="small"
                      :bordered="false"
                    >
                      {{ record.isWinner ? "中奖" : "未中奖" }}
                    </NTag>
                    <span v-if="record.prizeName" class="record-prize">{{
                      record.prizeName
                    }}</span>
                  </div>
                </div>
              </template>
              <div v-else class="empty-tip">暂无抽奖记录</div>
            </div>
          </div>
        </template>

        <div v-else-if="!loading" class="empty-tip" style="padding: 60px">活动不存在</div>
      </NSpin>
    </div>

    <!-- 奖品弹窗 -->
    <NModal
      v-model:show="showPrizeModal"
      preset="card"
      title="🎁 奖品列表"
      style="width: 90%; max-width: 500px"
    >
      <NScrollbar style="max-height: 60vh">
        <div v-if="prizes.length" class="prize-list">
          <div v-for="prize in prizes" :key="prize.id" class="prize-item">
            <div
              class="prize-icon-box"
              :style="{
                background: segmentColors[prizes.indexOf(prize) % segmentColors.length],
              }"
            >
              <img v-if="prize.imageUrl" :src="prize.imageUrl" class="prize-image" />
              <span v-else>🎁</span>
            </div>
            <div class="prize-detail">
              <div class="prize-name">
                <span
                  v-if="getPrizeLevelLabel(prize.prizeLevel)"
                  class="prize-level-tag"
                  :style="{ color: getPrizeLevelColor(prize.prizeLevel) }"
                >
                  {{ getPrizeLevelLabel(prize.prizeLevel) }}
                </span>
                {{ prize.name }}
              </div>
              <div class="prize-meta">
                价值 {{ prize.prizeValue }} · 概率
                {{ (prize.displayProbability * 100).toFixed(1) }}%
              </div>
            </div>
            <div class="prize-remain">
              <span class="remain-num">{{ prize.remainingCount }}</span>
              <span class="remain-label">剩余</span>
            </div>
          </div>
        </div>
        <div v-else class="empty-tip">暂无奖品</div>
      </NScrollbar>
    </NModal>

    <!-- 中奖名单弹窗 -->
    <NModal
      v-model:show="showWinnerModal"
      preset="card"
      title="🏆 中奖名单"
      style="width: 90%; max-width: 500px"
    >
      <NScrollbar style="max-height: 60vh">
        <div v-if="winners.length" class="winner-list">
          <div v-for="(winner, index) in winners" :key="winner.id" class="winner-item">
            <span class="winner-rank">{{
              index < 3 ? ["🥇", "🥈", "🥉"][index] : index + 1
            }}</span>
            <div class="winner-info">
              <span class="winner-name">{{
                winner.userName || `用户#${winner.userId}`
              }}</span>
              <span class="winner-time">{{
                new Date(winner.createdAt).toLocaleString()
              }}</span>
            </div>
            <NTag type="warning" size="small" :bordered="false">
              {{ winner.prizeName }}
            </NTag>
          </div>
        </div>
        <div v-else class="empty-tip">暂无中奖记录</div>
      </NScrollbar>
    </NModal>

    <!-- 结果弹窗 -->
    <NModal
      v-model:show="showResultModal"
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

        <NButton type="primary" size="large" round block @click="showResultModal = false">
          知道了
        </NButton>
      </div>
    </NModal>

    <!-- 原神抽卡动画弹窗 -->
    <NModal
      v-model:show="showGachaAnimation"
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
                  @click="closeGachaAnimation"
                >
                  确认
                </NButton>
              </div>
            </div>
          </div>
        </div>
      </div>
    </NModal>
  </div>
</template>

<style scoped lang="scss">
.lottery-page {
  min-height: 100vh;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  position: relative;
}

.bg-overlay {
  position: fixed;
  inset: 0;
  background: url("data:image/svg+xml,%3Csvg width='60' height='60' viewBox='0 0 60 60' xmlns='http://www.w3.org/2000/svg'%3E%3Cg fill='none' fill-rule='evenodd'%3E%3Cg fill='%23ffffff' fill-opacity='0.05'%3E%3Ccircle cx='30' cy='30' r='2'/%3E%3C/g%3E%3C/g%3E%3C/svg%3E");
  pointer-events: none;
}

.lottery-container {
  max-width: 480px;
  margin: 0 auto;
  padding: 20px 16px 40px;
  position: relative;
  z-index: 1;
}

.lottery-header {
  text-align: center;
  padding: 24px 0 20px;
  color: white;
}

.header-title {
  font-size: 28px;
  font-weight: 700;
  margin: 0 0 8px;
  text-shadow: 0 2px 8px rgba(0, 0, 0, 0.2);
}

.header-subtitle {
  font-size: 14px;
  opacity: 0.8;
  margin: 0;
}

.activity-card {
  background: white;
  border-radius: 16px;
  padding: 20px;
  margin-bottom: 20px;
  box-shadow: 0 8px 30px rgba(0, 0, 0, 0.15);
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.card-title {
  font-size: 18px;
  font-weight: 600;
  margin: 0;
  color: #1a1a1a;
}

.card-desc {
  font-size: 13px;
  color: #666;
  margin: 0 0 12px;
  line-height: 1.5;
}

.limit-info {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 12px;
  padding: 10px 14px;
  background: linear-gradient(135deg, #f8f9fa, #e9ecef);
  border-radius: 10px;
  border: 1px solid #e2e8f0;
}

.limit-item {
  display: flex;
  align-items: center;
  gap: 6px;
}

.limit-icon {
  font-size: 16px;
}

.limit-text {
  font-size: 13px;
  font-weight: 500;
  color: #4a5568;
}

.limit-count {
  font-weight: 700;
  color: #667eea;
  font-size: 15px;
}

.card-actions {
  display: flex;
  gap: 8px;
}

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

.records-card {
  background: white;
  border-radius: 16px;
  overflow: hidden;
  box-shadow: 0 8px 30px rgba(0, 0, 0, 0.15);
}

.records-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px;
  font-weight: 600;
  font-size: 15px;
  border-bottom: 1px solid #f0f0f0;
}

.records-list {
  max-height: 300px;
  overflow-y: auto;
}

.record-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 20px;
  border-bottom: 1px solid #f5f5f5;
  transition: background 0.2s;

  &:last-child {
    border-bottom: none;
  }
  &:hover {
    background: #fafafa;
  }
  &.is-winner {
    background: #fffbeb;
  }
}

.record-icon {
  font-size: 20px;
}

.record-info {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.record-name {
  font-size: 14px;
  font-weight: 500;
  color: #333;
}
.record-time {
  font-size: 12px;
  color: #999;
}

.record-result {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
}

.record-prize {
  font-size: 12px;
  color: #666;
}

.empty-tip {
  text-align: center;
  padding: 40px 20px;
  color: #999;
  font-size: 14px;
}

.prize-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.prize-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px;
  background: #f9f9f9;
  border-radius: 12px;
  transition: all 0.2s;

  &:hover {
    background: #f0f0f0;
  }
}

.prize-icon-box {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  flex-shrink: 0;
  overflow: hidden;
}

.prize-image {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.prize-detail {
  flex: 1;
}

.prize-name {
  font-size: 15px;
  font-weight: 600;
  color: #333;
  margin-bottom: 4px;
  display: flex;
  align-items: center;
  gap: 8px;
}

.prize-level-tag {
  font-size: 12px;
  font-weight: 700;
  padding: 2px 6px;
  border-radius: 4px;
  background: rgba(0, 0, 0, 0.05);
}

.prize-meta {
  font-size: 12px;
  color: #999;
}

.prize-remain {
  text-align: center;
}

.remain-num {
  display: block;
  font-size: 20px;
  font-weight: 700;
  color: #667eea;
}
.remain-label {
  font-size: 11px;
  color: #999;
}

.winner-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.winner-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px;
  background: #f9f9f9;
  border-radius: 12px;
}

.winner-rank {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  flex-shrink: 0;
}

.winner-info {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.winner-name {
  font-size: 14px;
  font-weight: 500;
  color: #333;
}
.winner-time {
  font-size: 12px;
  color: #999;
}

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

// ==================== 原神抽卡模式样式 ====================
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

// ==================== 原神抽卡动画样式 ====================
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
  animation: ringPulse 0.8s ease-out forwards;
}
.ring-2 {
  width: 150px;
  height: 150px;
  animation: ringPulse 0.8s ease-out 0.1s forwards;
}
.ring-3 {
  width: 200px;
  height: 200px;
  animation: ringPulse 0.8s ease-out 0.2s forwards;
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

@keyframes ringPulse {
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
