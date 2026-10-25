import { computed, ref } from "vue";

// RSS 阅读视图与分级阅读详情页共用的排版偏好: 字号 14~20 + 版宽 舒适/宽敞, localStorage 持久化
const READER_FONT_KEY = "rss_reader_font_size";
const READER_WIDTH_KEY = "rss_reader_width";

export type ReaderWidth = "cozy" | "wide";

export function useReaderPreferences() {
  const fontSize = ref(Number(localStorage.getItem(READER_FONT_KEY)) || 17);
  const width = ref<ReaderWidth>(localStorage.getItem(READER_WIDTH_KEY) === "wide" ? "wide" : "cozy");
  /** 正文列最大宽度 (不含内边距) */
  const maxWidth = computed(() => (width.value === "wide" ? 860 : 660));

  const setFontSize = (size: number) => {
    fontSize.value = Math.min(20, Math.max(14, size));
    localStorage.setItem(READER_FONT_KEY, String(fontSize.value));
  };

  const toggleWidth = () => {
    width.value = width.value === "cozy" ? "wide" : "cozy";
    localStorage.setItem(READER_WIDTH_KEY, width.value);
  };

  return { fontSize, width, maxWidth, setFontSize, toggleWidth };
}
