import { $t } from "@/locales";

/**
 * 分级阅读共享工具: 列表页 / 详情组件 / RSS 列表 (难度分档同源) 共用。
 */

/** 文档标注状态标签 (NTag type + 文案) */
export const readingStatusTag = (status: string) => {
  switch (status) {
    case 'annotated':
      return { type: 'success' as const, label: $t('page.ai.reader.statusAnnotated') };
    case 'annotating':
      return { type: 'info' as const, label: $t('page.ai.reader.statusAnnotating') };
    case 'failed':
      return { type: 'error' as const, label: $t('page.ai.reader.statusFailed') };
    default:
      return { type: 'default' as const, label: $t('page.ai.reader.statusPending') };
  }
};

/** 难度标签: 按预估生词率分档 (0=未算返回 null 不显示) */
export const difficultyTag = (rate?: number) => {
  if (!rate || rate <= 0) return null;
  if (rate < 0.03) return { label: $t('page.ai.reader.easy'), type: 'success' as const };
  if (rate <= 0.08) return { label: $t('page.ai.reader.medium'), type: 'info' as const };
  return { label: $t('page.ai.reader.hard'), type: 'warning' as const };
};
