import { computed, ref } from "vue";

// RSS 阅读视图与分级阅读详情页共用的排版/朗读/对照偏好, localStorage 持久化。
// refs 提到模块级单例: 两个使用方 (KeepAlive 下可同时存活) 修改时实时联动。
const FONT_KEY = "reader_font_size";
const WIDTH_KEY = "reader_width";
const LINE_KEY = "reader_line_height";
const SERIF_KEY = "reader_serif";
const RATE_KEY = "reader_speech_rate";
const BI_KEY = "reader_bilingual_mode";
// 旧键名迁移 (rss_reader_* 时代是单页使用, 现已跨页共享)
const LEGACY_FONT_KEY = "rss_reader_font_size";
const LEGACY_WIDTH_KEY = "rss_reader_width";

const readStoredFont = () => {
  const v = Number(localStorage.getItem(FONT_KEY) ?? localStorage.getItem(LEGACY_FONT_KEY));
  return Number.isFinite(v) && v >= MIN_FONT && v <= MAX_FONT ? v : 17;
};
const readStoredWidth = () =>
  (localStorage.getItem(WIDTH_KEY) ?? localStorage.getItem(LEGACY_WIDTH_KEY)) === "wide" ? "wide" : "cozy";

const LINE_VALUES = [1.6, 1.9, 2.2] as const;
export type ReaderLineHeight = "compact" | "normal" | "loose";
const LINE_KEY_OF: Record<ReaderLineHeight, number> = { compact: 1.6, normal: 1.9, loose: 2.2 };

const readStoredLine = (): ReaderLineHeight => {
  const v = Number(localStorage.getItem(LINE_KEY));
  return v === 1.6 ? "compact" : v === 2.2 ? "loose" : "normal";
};

const readStoredSerif = () => localStorage.getItem(SERIF_KEY) === "1";

const readStoredRate = () => {
  const v = Number(localStorage.getItem(RATE_KEY));
  return Number.isFinite(v) && v >= 0.6 && v <= 1.5 ? v : 0.95;
};

const readStoredBi = (): BilingualMode => {
  const v = localStorage.getItem(BI_KEY);
  return v === "zh" || v === "en" || v === "bi" ? v : "bi";
};

export type ReaderWidth = "cozy" | "wide";
export type BilingualMode = "bi" | "zh" | "en";

export const MIN_FONT = 12;
export const MAX_FONT = 24;

const fontSize = ref(readStoredFont());
const width = ref<ReaderWidth>(readStoredWidth());
const lineHeight = ref<ReaderLineHeight>(readStoredLine());
const serif = ref(readStoredSerif());
const speechRate = ref(readStoredRate());
const bilingualMode = ref<BilingualMode>(readStoredBi());

export function useReaderPreferences() {
  /** 正文列最大宽度 (不含内边距) */
  const maxWidth = computed(() => (width.value === "wide" ? 860 : 660));
  const lineHeightValue = computed(() => LINE_KEY_OF[lineHeight.value]);

  const setFontSize = (size: number) => {
    fontSize.value = Math.min(MAX_FONT, Math.max(MIN_FONT, size));
    localStorage.setItem(FONT_KEY, String(fontSize.value));
    localStorage.removeItem(LEGACY_FONT_KEY);
  };

  const toggleWidth = () => {
    width.value = width.value === "cozy" ? "wide" : "cozy";
    localStorage.setItem(WIDTH_KEY, width.value);
    localStorage.removeItem(LEGACY_WIDTH_KEY);
  };

  const setLineHeight = (v: ReaderLineHeight) => {
    lineHeight.value = v;
    localStorage.setItem(LINE_KEY, String(LINE_KEY_OF[v]));
  };

  const setSerif = (v: boolean) => {
    serif.value = v;
    localStorage.setItem(SERIF_KEY, v ? "1" : "0");
  };

  const setSpeechRate = (v: number) => {
    speechRate.value = Math.min(1.5, Math.max(0.6, v));
    localStorage.setItem(RATE_KEY, String(speechRate.value));
  };

  /** 对照/译文/原文三态记忆 (RSS 打开已有译文的条目时套用) */
  const setBilingualMode = (v: BilingualMode) => {
    bilingualMode.value = v;
    localStorage.setItem(BI_KEY, v);
  };

  return {
    fontSize,
    width,
    maxWidth,
    lineHeight,
    lineHeightValue,
    serif,
    speechRate,
    bilingualMode,
    setFontSize,
    toggleWidth,
    setLineHeight,
    setSerif,
    setSpeechRate,
    setBilingualMode
  };
}
