/**
 * 抽奖稀有度主题体系: 转盘彩带 / 抽卡 three.js 场景 / 结果弹窗共用同一套配色,
 * 保证"金=特等奖"的视觉语言在所有动画里一致。
 *
 * 稀有度映射沿用 prizeLevel (model: 1-特等奖, 2-一等奖, 3-二等奖, 4-三等奖, 0-未设置):
 * 1→5★ 金色传说, 2→4★ 紫色史诗, 3→3★ 蓝色稀有, 4/0→2★ 绿色普通; 未中奖→1★ 明亮风。
 */
export interface RarityTheme {
  /** 主色 (光线/边框/标题) */
  primary: string;
  /** 辅色 (渐变次级/粒子) */
  secondary: string;
  /** 辉光色 (bloom/阴影) */
  glow: string;
  /** 彩带粒子色板 (2D confetti) */
  confetti: string[];
  label: string;
}

export const RARITY_THEMES: Record<number, RarityTheme> = {
  5: {
    primary: '#ffd700',
    secondary: '#ffb300',
    glow: '#ffec9e',
    confetti: ['#ffd700', '#ffb300', '#fff3c4', '#ff8f00', '#ffffff'],
    label: '金色传说'
  },
  4: {
    primary: '#ce93d8',
    secondary: '#9b59b6',
    glow: '#e7c6f5',
    confetti: ['#ce93d8', '#9b59b6', '#f3e5f5', '#7b1fa2', '#ffffff'],
    label: '紫色史诗'
  },
  3: {
    primary: '#64b5f6',
    secondary: '#3498db',
    glow: '#bbdefb',
    confetti: ['#64b5f6', '#3498db', '#e3f2fd', '#1565c0', '#ffffff'],
    label: '蓝色稀有'
  },
  2: {
    primary: '#81c784',
    secondary: '#66bb6a',
    glow: '#c8e6c9',
    confetti: ['#81c784', '#66bb6a', '#e8f5e9', '#2e7d32', '#ffffff'],
    label: '绿色普通'
  },
  1: {
    // 未中奖: 冷灰蓝, 与金色传说明确区分 (谢谢惠顾要"素")
    primary: '#94a3b8',
    secondary: '#64748b',
    glow: '#cbd5e1',
    confetti: ['#94a3b8', '#cbd5e1', '#64748b'],
    label: '谢谢惠顾'
  }
};

export function rarityTheme(rarity: number): RarityTheme {
  return RARITY_THEMES[rarity] || RARITY_THEMES[2];
}

/** prizeLevel → 稀有度星级 (1-5); 未中奖由调用方传 1 */
export function resolveRarity(prizeLevel: number): number {
  const levelMap: Record<number, number> = {
    1: 5, // 特等奖 → 金色传说
    2: 4, // 一等奖 → 紫色史诗
    3: 3, // 二等奖 → 蓝色稀有
    4: 2, // 三等奖 → 绿色普通
    0: 2 // 未设置 → 绿色普通
  };
  return levelMap[prizeLevel] || 2;
}
