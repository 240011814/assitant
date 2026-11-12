import type { RSSSource } from '@/service/api';

/**
 * RSS 共享工具 (Folo 风格辅助): 订阅源头像颜色 / 名称 / 相对时间,
 * 列表页与阅读弹窗 (rss-viewer) 共用, 传 sources 避免各自的响应式闭包。
 */

export const avatarColor = (id: number) => `hsl(${(id * 67) % 360}, 62%, 52%)`;

export const sourceInitial = (sources: RSSSource[], sourceId: number) => {
  const title = sources.find((s) => s.id === sourceId)?.title?.trim();
  return (title || '?').charAt(0).toUpperCase();
};

export const sourceTitle = (sources: RSSSource[], sourceId: number) =>
  sources.find((s) => s.id === sourceId)?.title || `源 #${sourceId}`;

/** 相对时间: 刚刚 / n 分钟前 / n 小时前 / 昨天 / n 天前 / 日期 */
export const relTime = (iso?: string | null) => {
  if (!iso) return '';
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return '';
  const diff = Date.now() - t;
  const minute = 60_000;
  const hour = 3_600_000;
  const day = 86_400_000;
  if (diff < minute) return '刚刚';
  if (diff < hour) return `${Math.floor(diff / minute)} 分钟前`;
  if (diff < day) return `${Math.floor(diff / hour)} 小时前`;
  if (diff < 2 * day) return '昨天';
  if (diff < 7 * day) return `${Math.floor(diff / day)} 天前`;
  return new Date(iso).toLocaleDateString();
};
