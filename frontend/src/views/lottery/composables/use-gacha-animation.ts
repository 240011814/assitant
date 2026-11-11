import { computed, ref } from 'vue';
import type { Ref } from 'vue';
import type { MessageApi } from 'naive-ui';
import { fetchDrawLottery } from '@/service/api';
import { resolveRarity } from './rarity-theme';

export type GachaPhase = 'idle' | 'meteor' | 'flash' | 'reveal' | 'result';

/** 转盘旋转时长 (ms): CSS transition、指针落定回弹计时、旋转咔哒声间隔三处共用 */
export const WHEEL_SPIN_MS = 3500;

export interface GachaAnimationOptions {
  /** 用户名（组件内共享，此处只读） */
  userName: Ref<string>;
  /** 抽奖进行中标记（组件内共享，此处读写） */
  drawing: Ref<boolean>;
  /** 抽奖结果（组件内共享，此处写入；结果弹窗仍在组件中展示） */
  drawResult: Ref<Api.Lottery.DrawResult | null>;
  /** 当前活动 id，等价于原组件的 Number(route.params.id) */
  getActivityId: () => number;
  /** naive-ui message 实例 */
  message: MessageApi;
  /** 点击音效（来自 useAudioCue，保证全局共用同一个 AudioContext） */
  playClickSound: () => void;
  /** 原神祈愿音效（WAV 采样整段播放, 来自 useAudioCue） */
  playWishExecute: () => void;
  /** 抽奖完成后刷新页面数据（组件内的 loadData） */
  loadData: () => void;
}

/**
 * 抽奖动画状态机：转盘指针旋转角度 + 原神抽卡动画阶段时序。
 * three.js 视觉在 use-wish-scene / gacha-animation-modal 中渲染, 这里只管阶段推进。
 * 各阶段 sleep 可被 skip() 打断, 直接进入结果阶段 (原神同款跳过)。
 */
export function useGachaAnimation(options: GachaAnimationOptions) {
  const { userName, drawing, drawResult, getActivityId, message, playClickSound, playWishExecute, loadData } = options;

  const pointerRotation = ref(0);

  // 原神抽卡模式状态
  const showGachaAnimation = ref(false);
  const gachaPhase = ref<GachaPhase>('idle');

  // 原神抽卡模式 - 根据奖品等级确定稀有度等级 (与结果卡/彩带/场景配色共用一套映射)
  const gachaRarity = computed(() => {
    if (!drawResult.value?.isWinner || !drawResult.value.prize) return 1; // 未中奖用1星（明亮风格）
    return resolveRarity(drawResult.value.prize.prizeLevel);
  });

  /** 转盘旋转：至少转5圈后停在目标扇区，写入 pointerRotation */
  function spinToSegment(targetIndex: number, totalItems: number) {
    const segmentAngle = 360 / totalItems;
    // 目标扇区中心的角度（从顶部顺时针）
    const targetAngle = targetIndex * segmentAngle + segmentAngle / 2;

    // 计算至少转5圈后到达目标位置的旋转角度
    const currentAngle = pointerRotation.value % 360;
    const minSpins = 5;
    // 从当前角度出发，至少转minSpins圈，再加上到达目标的偏移
    const extraSpins = minSpins + Math.floor(Math.random() * 3);
    const finalRotation = pointerRotation.value + extraSpins * 360 + ((targetAngle - currentAngle + 360) % 360);
    pointerRotation.value = finalRotation;
  }

  // ---- 可跳过的阶段 sleep: skip() 置位后, 未完成的 sleep 立刻放行 ----
  let skipRequested = false;
  let wakeSkip: (() => void) | null = null;

  function phaseSleep(ms: number) {
    return new Promise<void>(resolve => {
      const timer = window.setTimeout(() => {
        wakeSkip = null;
        resolve();
      }, ms);
      wakeSkip = () => {
        window.clearTimeout(timer);
        wakeSkip = null;
        resolve();
      };
    });
  }

  /** 跳过剩余动画阶段直达结果 (弹窗"跳过"按钮/点击任意处) */
  function skip() {
    if (gachaPhase.value === 'result' || gachaPhase.value === 'idle') return;
    skipRequested = true;
    wakeSkip?.();
  }

  // 原神抽卡模式
  async function handleGachaDraw() {
    if (!userName.value.trim()) {
      message.warning('请输入您的姓名');
      return;
    }

    playClickSound();
    drawing.value = true;
    skipRequested = false;
    showGachaAnimation.value = true;
    gachaPhase.value = 'idle';

    const activityId = getActivityId();
    const { data, error } = await fetchDrawLottery(activityId, userName.value.trim());

    if (error) {
      // 具体原因由 request 层统一弹出
      drawing.value = false;
      showGachaAnimation.value = false;
      return;
    }

    drawResult.value = data;

    // 阶段1：流星划过 (原神祈愿音效整段起播 — 与游戏内一致, 采样自带流星/爆发/揭示节奏)
    gachaPhase.value = 'meteor';
    playWishExecute();

    await phaseSleep(2600);

    // 阶段2：爆发闪光 (跳过则直达结果)
    if (!skipRequested) {
      gachaPhase.value = 'flash';
      await phaseSleep(650);
    }

    // 阶段3：光柱 + 旋转光线揭示 (此刻才"炸色")
    if (!skipRequested) {
      gachaPhase.value = 'reveal';
      await phaseSleep(1500);
    }

    // 阶段4：显示结果
    gachaPhase.value = 'result';
    skipRequested = false;
    drawing.value = false;

    loadData();
  }

  function closeGachaAnimation() {
    showGachaAnimation.value = false;
    gachaPhase.value = 'idle';
  }

  return {
    pointerRotation,
    showGachaAnimation,
    gachaPhase,
    gachaRarity,
    spinToSegment,
    handleGachaDraw,
    skip,
    closeGachaAnimation
  };
}

export type GachaAnimationReturn = ReturnType<typeof useGachaAnimation>;
