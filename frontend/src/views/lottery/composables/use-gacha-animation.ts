import { computed, ref } from 'vue';
import type { Ref } from 'vue';
import type { MessageApi } from 'naive-ui';
import { fetchDrawLottery } from '@/service/api';

type GachaPhase = 'idle' | 'meteor' | 'flash' | 'reveal' | 'result';

interface GachaParticle {
  id: number;
  x: number;
  y: number;
  delay: number;
}

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
  /** 抽卡音效（来自 useAudioCue） */
  playGachaSound: (rarity: number) => void;
  /** 抽奖完成后刷新页面数据（组件内的 loadData） */
  loadData: () => void;
}

/**
 * 抽奖动画状态机：转盘指针旋转角度 + 原神抽卡动画（粒子/流星/闪光/揭示/结果）
 * 从 views/lottery/[id].vue 原样抽取，setTimeout 分阶段时序与原实现一致
 */
export function useGachaAnimation(options: GachaAnimationOptions) {
  const { userName, drawing, drawResult, getActivityId, message, playClickSound, playGachaSound, loadData } = options;

  const pointerRotation = ref(0);

  // 原神抽卡模式状态
  const showGachaAnimation = ref(false);
  const gachaPhase = ref<GachaPhase>('idle');
  const gachaParticles = ref<GachaParticle[]>([]);

  // 原神抽卡模式 - 根据奖品等级确定稀有度等级
  const gachaRarity = computed(() => {
    if (!drawResult.value?.isWinner || !drawResult.value.prize) return 1; // 未中奖用1星（明亮风格）
    const level = drawResult.value.prize.prizeLevel;
    // 奖品等级: 1-特等奖, 2-一等奖, 3-二等奖, 4-三等奖, 0-未设置
    const levelMap: Record<number, number> = {
      1: 5, // 特等奖 → 金色传说
      2: 4, // 一等奖 → 紫色史诗
      3: 3, // 二等奖 → 蓝色稀有
      4: 2, // 三等奖 → 绿色普通
      0: 2 // 未设置 → 绿色普通
    };
    return levelMap[level] || 2;
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

  // 原神抽卡模式
  async function handleGachaDraw() {
    if (!userName.value.trim()) {
      message.warning('请输入您的姓名');
      return;
    }

    playClickSound();
    drawing.value = true;
    showGachaAnimation.value = true;
    gachaPhase.value = 'idle';

    // 生成粒子效果
    gachaParticles.value = Array.from({ length: 30 }, (_, i) => ({
      id: i,
      x: Math.random() * 100,
      y: Math.random() * 100,
      delay: Math.random() * 2
    }));

    const activityId = getActivityId();
    const { data, error } = await fetchDrawLottery(activityId, userName.value.trim());

    if (error) {
      drawing.value = false;
      showGachaAnimation.value = false;
      message.error('祈愿失败');
      return;
    }

    drawResult.value = data;

    // 计算稀有度
    const rarity = data.isWinner && data.prize
      ? data.prize.prizeValue >= 100
        ? 5
        : data.prize.prizeValue >= 50
        ? 4
        : data.prize.prizeValue >= 20
        ? 3
        : 2
      : 2;

    // 阶段1：流星下落
    gachaPhase.value = 'meteor';
    playGachaSound(rarity);

    await new Promise(resolve => setTimeout(resolve, 1200));

    // 阶段2：闪光
    gachaPhase.value = 'flash';

    await new Promise(resolve => setTimeout(resolve, 400));

    // 阶段3：揭示
    gachaPhase.value = 'reveal';

    await new Promise(resolve => setTimeout(resolve, 600));

    // 阶段4：显示结果
    gachaPhase.value = 'result';
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
    gachaParticles,
    gachaRarity,
    spinToSegment,
    handleGachaDraw,
    closeGachaAnimation
  };
}

export type GachaAnimationReturn = ReturnType<typeof useGachaAnimation>;
