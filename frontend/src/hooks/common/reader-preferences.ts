import { computed, ref } from "vue";

// RSS 阅读视图与分级阅读详情页共用的排版偏好: 字号 14~20 + 版宽 舒适/宽敞, localStorage 持久化。
// refs 提到模块级单例: 两个使用方 (KeepAlive 下可同时存活) 修改字号/版宽时实时联动。
const FONT_KEY = "reader_font_size";
const WIDTH_KEY = "reader_width";
// 旧键名迁移 (rss_reader_* 时代是单页使用, 现已跨页共享)
const LEGACY_FONT_KEY = "rss_reader_font_size";
const LEGACY_WIDTH_KEY = "rss_reader_width";

const readStoredFont = () => {
  const v = Number(localStorage.getItem(FONT_KEY) ?? localStorage.getItem(LEGACY_FONT_KEY));
  return Number.isFinite(v) && v >= 14 && v <= 20 ? v : 17;
};
const readStoredWidth = () =>
  (localStorage.getItem(WIDTH_KEY) ?? localStorage.getItem(LEGACY_WIDTH_KEY)) === "wide" ? "wide" : "cozy";

export type ReaderWidth = "cozy" | "wide";

const fontSize = ref(readStoredFont());
const width = ref<ReaderWidth>(readStoredWidth());

export function useReaderPreferences() {
  /** 正文列最大宽度 (不含内边距) */
  const maxWidth = computed(() => (width.value === "wide" ? 860 : 660));

  const setFontSize = (size: number) => {
    fontSize.value = Math.min(20, Math.max(14, size));
    localStorage.setItem(FONT_KEY, String(fontSize.value));
    localStorage.removeItem(LEGACY_FONT_KEY);
  };

  const toggleWidth = () => {
    width.value = width.value === "cozy" ? "wide" : "cozy";
    localStorage.setItem(WIDTH_KEY, width.value);
    localStorage.removeItem(LEGACY_WIDTH_KEY);
  };

  return { fontSize, width, maxWidth, setFontSize, toggleWidth };
}
