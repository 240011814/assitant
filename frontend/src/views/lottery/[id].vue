<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { NButton, NSpin, NTag, useMessage } from "naive-ui";
import {
  fetchDrawLottery,
  fetchGetDrawLimits,
  fetchGetLotteryActivity,
  fetchGetLotteryPrizes,
  fetchGetLotteryRecords,
  fetchGetLotteryWinners,
} from "@/service/api";
import { localStg } from "@/utils/storage";
import skyCloudsUrl from "@/assets/imgs/lottery/sky-clouds.jpg";
import { useAudioCue } from "./composables/use-audio-cue";
import { useGachaAnimation } from "./composables/use-gacha-animation";
import { rarityTheme, resolveRarity } from "./composables/rarity-theme";
import WheelPanel from "./components/wheel-panel.vue";
import GachaGate from "./components/gacha-gate.vue";
import RecordsPanel from "./components/records-panel.vue";
import PrizeModal from "./components/prize-modal.vue";
import WinnerModal from "./components/winner-modal.vue";
import ResultModal from "./components/result-modal.vue";
import GachaAnimationModal from "./components/gacha-animation-modal.vue";
import FxConfetti from "./components/fx-confetti.vue";

const route = useRoute();
const router = useRouter();
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
const confettiRef = ref<InstanceType<typeof FxConfetti> | null>(null);

// 登录态: 页面加载时判定 (与 request 实例附加 Authorization 的条件一致, token 存在即随请求带上)
const token = localStg.get("token");
const isLoggedIn = Boolean(token);

// 从 JWT payload 取账号用户名 (登录用户实名预填; base64url 解析失败返回空串)
function decodeJwtUserName(jwtToken: string): string {
  try {
    const payload = JSON.parse(atob(jwtToken.split(".")[1].replace(/-/g, "+").replace(/_/g, "/")));
    return typeof payload.userName === "string" ? payload.userName : "";
  } catch {
    return "";
  }
}

// 音效管理
const { playClickSound, playWishExecute, playWheelSpinSound, playWinSound, playLoseSound } = useAudioCue();

// 转盘/原神抽卡动画状态机
const {
  pointerRotation,
  showGachaAnimation,
  gachaPhase,
  gachaRarity,
  spinToSegment,
  handleGachaDraw,
  skip,
  closeGachaAnimation
} = useGachaAnimation({
  userName,
  drawing,
  drawResult,
  getActivityId: () => Number(route.params.id),
  message,
  playClickSound,
  playWishExecute,
  loadData
});

// 奢华珠宝色转盘: 深酒红/香槟金/孔雀蓝/皇室紫交替, 与金色外圈呼应
const segmentColors = [
  "#a03030",
  "#e3c06a",
  "#1f4e6b",
  "#5e3a87",
  "#275e4d",
  "#b3742a",
  "#822f4f",
  "#2f6f9e",
  "#c99244",
  "#3d5a2f",
  "#6d3060",
  "#245a56",
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
  const map: Record<number, { label: string; type: 'default' | 'primary' | 'info' | 'success' | 'warning' | 'error'; icon: string }> = {
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

// 活动关闭匿名抽奖且未登录 → 用登录引导替换抽奖面板
const needLogin = computed(() => {
  return activity.value?.allowAnonymous === false && !isLoggedIn;
});

function goLogin() {
  router.push({ path: "/login", query: { redirect: route.fullPath } });
}

const drawButtonText = computed(() => {
  if (drawing.value) return "抽奖中...";
  if (activity.value?.status === 0) return "活动未开始";
  if (activity.value?.status === 2) return "活动已结束";
  return isGachaMode.value ? "祈愿一次" : "开始抽奖";
});

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
    // 关闭匿名且已登录: 预填账号用户名 (实名, 后端会以登录身份强制覆盖)
    if (activity.value?.allowAnonymous === false && isLoggedIn && token && !userName.value.trim()) {
      const accountName = decodeJwtUserName(token);
      if (accountName) {
        userName.value = accountName;
      }
    }
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
  if (needLogin.value) {
    message.warning("该活动需要登录后参与");
    return;
  }
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

  // 播放结果音效
  if (data.isWinner) {
    playWinSound();
    // 按奖品稀有度喷彩带 (结果弹窗弹出前)
    confettiRef.value?.burst(rarityTheme(resolveRarity(data.prize?.prizeLevel ?? 0)).confetti);
  } else {
    playLoseSound();
  }

  drawing.value = false;
  drawResult.value = data;
  showResultModal.value = true;
  loadData();
}

function openWinnerModal() {
  loadWinners();
  showWinnerModal.value = true;
}

onMounted(() => {
  loadData();
});
</script>

<template>
  <div class="lottery-page" :style="{ backgroundImage: `url(${skyCloudsUrl})` }">
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
              <NTag :type="statusConfig.type" :bordered="false" round size="small">
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
              <NButton quaternary size="small" @click="showPrizeModal = true">
                🎁 奖品 ({{ prizes.length }})
              </NButton>
              <NButton quaternary size="small" @click="openWinnerModal">
                🏆 中奖名单
              </NButton>
            </div>
          </div>

          <!-- 关闭匿名抽奖且未登录: 登录引导 -->
          <div v-if="needLogin" class="activity-card login-gate">
            <div class="login-gate-icon">🔒</div>
            <p class="login-gate-text">该活动不允许匿名参与, 请先登录后抽奖</p>
            <NButton type="primary" block round @click="goLogin">去登录</NButton>
          </div>

          <!-- 转盘模式 -->
          <WheelPanel
            v-else-if="!isGachaMode"
            v-model:user-name="userName"
            :wheel-items="wheelItems"
            :pointer-rotation="pointerRotation"
            :drawing="drawing"
            :can-draw="canDraw"
            :draw-button-text="drawButtonText"
            @draw="handleDraw"
          />

          <!-- 原神抽卡模式 -->
          <GachaGate
            v-else
            v-model:user-name="userName"
            :drawing="drawing"
            :can-draw="canDraw"
            :draw-button-text="drawButtonText"
            @draw="handleDraw"
          />

          <!-- 抽奖记录 -->
          <RecordsPanel :records="records" @refresh="loadRecords" />
        </template>

        <div v-else-if="!loading" class="empty-tip" style="padding: 60px">活动不存在</div>
      </NSpin>
    </div>

    <!-- 奖品弹窗 -->
    <PrizeModal v-model:show="showPrizeModal" :prizes="prizes" :segment-colors="segmentColors" />

    <!-- 中奖名单弹窗 -->
    <WinnerModal v-model:show="showWinnerModal" :winners="winners" />

    <!-- 结果弹窗 -->
    <ResultModal v-model:show="showResultModal" :draw-result="drawResult" />

    <!-- 中奖彩带 (转盘模式, 按稀有度配色) -->
    <FxConfetti ref="confettiRef" />

    <!-- 原神抽卡动画弹窗 -->
    <GachaAnimationModal
      v-model:show="showGachaAnimation"
      :gacha-phase="gachaPhase"
      :gacha-rarity="gachaRarity"
      :draw-result="drawResult"
      @skip="skip"
      @close="closeGachaAnimation"
    />
  </div>
</template>

<style scoped lang="scss">
.lottery-page {
  min-height: 100vh;
  // 云海星空照片打底 (天空资产), 尺寸随视口
  background-size: cover;
  background-position: center top;
  background-repeat: no-repeat;
  background-attachment: fixed;
  position: relative;
}

// 紫夜幕罩: 压暗天空照片, 营造参考图的紫金色夜晚氛围
.bg-overlay {
  position: fixed;
  inset: 0;
  background: linear-gradient(
    180deg,
    rgba(28, 16, 66, 0.9) 0%,
    rgba(56, 30, 108, 0.8) 42%,
    rgba(40, 22, 84, 0.82) 70%,
    rgba(20, 12, 48, 0.92) 100%
  );
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
  background: linear-gradient(160deg, rgba(26, 20, 58, 0.82), rgba(14, 10, 34, 0.88));
  border: 1px solid rgba(255, 214, 110, 0.24);
  backdrop-filter: blur(6px);
  border-radius: 16px;
  padding: 20px;
  margin-bottom: 20px;
  box-shadow: 0 8px 30px rgba(0, 0, 0, 0.4);
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
  color: #fff;
}

.card-desc {
  font-size: 13px;
  color: rgba(255, 255, 255, 0.62);
  margin: 0 0 12px;
  line-height: 1.5;
}

.limit-info {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 12px;
  padding: 10px 14px;
  background: rgba(255, 214, 110, 0.08);
  border-radius: 10px;
  border: 1px solid rgba(255, 214, 110, 0.18);
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
  color: rgba(255, 255, 255, 0.78);
}

.limit-count {
  font-weight: 700;
  color: #ffd76e;
  font-size: 15px;
}

.card-actions {
  display: flex;
  gap: 8px;
}

.login-gate {
  text-align: center;
  padding: 32px 20px;
}

.login-gate-icon {
  font-size: 40px;
  margin-bottom: 8px;
}

.login-gate-text {
  font-size: 14px;
  color: rgba(255, 255, 255, 0.78);
  margin: 0 0 16px;
}

.empty-tip {
  text-align: center;
  padding: 40px 20px;
  color: #999;
  font-size: 14px;
}
</style>
