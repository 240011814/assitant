import { ref } from 'vue';
import { defineStore } from 'pinia';
import { localStg } from '@/utils/storage';
import { SetupStoreId } from '@/enum';

/** 全局阅读小窗会话快照 (RSS/分级阅读共用, 类型与 storage.d.ts 的 readerSession 保持一致) */
export type ReaderSessionSnapshot = NonNullable<StorageType.Local['readerSession']>;

/**
 * 全局阅读小窗会话 store:
 * 最小化时写入快照 (右下角卡片显示, 任意页面可见), 恢复或丢弃时清除;
 * 同步持久化到 localStorage, 浏览器标签关闭/刷新后仍可恢复续读。
 * 同一时刻只有一个小窗会话, 后收的覆盖先前的。
 */
export const useReaderSessionStore = defineStore(SetupStoreId.ReaderSession, () => {
  const session = ref<ReaderSessionSnapshot | null>(initSession());
  /** 卡片已点击等待目标页面消费 (消费前先隐藏卡片, 避免导航落空时卡片停留) */
  const restorePending = ref(false);

  function initSession(): ReaderSessionSnapshot | null {
    return localStg.get('readerSession') || null;
  }

  /** 收入小窗: 覆盖当前会话并持久化; 恢复请求进行中 (卡片刚被点击)时不覆盖——
   *  正在恢复的会话优先, 避免导航途中被旧页面 (弹窗还开着) 的自动收窗顶掉导致恢复落空 */
  function minimize(snapshot: ReaderSessionSnapshot) {
    if (restorePending.value) return;
    session.value = snapshot;
    localStg.set('readerSession', snapshot);
  }

  /** 丢弃小窗 */
  function dismiss() {
    session.value = null;
    restorePending.value = false;
    localStg.remove('readerSession');
  }

  /** 卡片点击: 通知目标页面准备恢复 (RSS 列表页 / 分级阅读详情页各自消费) */
  function requestRestore() {
    restorePending.value = true;
  }

  /**
   * 目标页面消费恢复请求: 返回快照并清空会话 (恢复即弹窗接管, 卡片退场);
   * 仅在 restorePending 时返回快照, 否则返回 null (幂等, 重复调用安全)。
   * 传 match 时只消费匹配的会话——kind/itemId 不符返回 null 且不清除,
   * 留给目标页面消费: 避免 RSS 页的 restorePending 监听误吞分级阅读的恢复请求 (反之亦然)。
   */
  function consumeRestore(match?: { kind?: 'rss' | 'reading'; itemId?: number }): ReaderSessionSnapshot | null {
    if (!restorePending.value || !session.value) return null;
    if (match?.kind && session.value.kind !== match.kind) return null;
    if (match?.itemId !== undefined && session.value.itemId !== match.itemId) return null;
    const snapshot = session.value;
    session.value = null;
    restorePending.value = false;
    localStg.remove('readerSession');
    return snapshot;
  }

  return {
    session,
    restorePending,
    minimize,
    dismiss,
    requestRestore,
    consumeRestore
  };
});
