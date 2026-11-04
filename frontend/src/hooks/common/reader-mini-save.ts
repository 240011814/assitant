import { onBeforeUnmount, onDeactivated } from "vue";
import { useEventListener } from "@vueuse/core";
import { useReaderSessionStore } from "@/store/modules/reader-session";
import type { ReaderSessionSnapshot } from "@/store/modules/reader-session";

/**
 * 阅读载体共用的"离开时自动收窗":
 * 组件失活/卸载 (切 tab、路由跳转) 与浏览器 beforeunload (关标签/刷新) 时,
 * 若 capture() 返回快照则收入全局小窗 (右下角卡片继续可见), 返回 null 则跳过
 * (用户主动退出、无阅读会话等由使用方在 capture 里判断)。
 * 使用方: RSS 列表页、分级阅读列表弹窗、分级阅读详情路由页。
 */
export function useReaderMiniLeaveSave(capture: () => ReaderSessionSnapshot | null) {
  const sessionStore = useReaderSessionStore();

  const saveIfAny = () => {
    const snapshot = capture();
    if (snapshot) sessionStore.minimize(snapshot);
  };

  onDeactivated(saveIfAny);
  onBeforeUnmount(saveIfAny);
  useEventListener(window, "beforeunload", saveIfAny);
}
