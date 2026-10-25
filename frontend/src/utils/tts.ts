import { getServiceBaseURL } from '@/utils/service';
import { localStg } from '@/utils/storage';

/**
 * 统一 TTS 入口: 按系统配置 tts_mode 分流
 * - frontend: 浏览器本地 Web Speech (兼容处理见 speakViaBrowser)
 * - openai / edge: 后端 /api/tts 合成 mp3 → <audio> 播放 (跨浏览器一致, 音色统一),
 *   失败自动回退浏览器本地朗读
 */
type TtsMode = 'frontend' | 'openai' | 'edge';

const isHttpProxy = import.meta.env.DEV && import.meta.env.VITE_HTTP_PROXY === 'Y';
const { baseURL } = getServiceBaseURL(import.meta.env, isHttpProxy);

const authHeaders = (): Record<string, string> => {
  const token = localStg.get('token');
  return token ? { Authorization: `Bearer ${token}` } : {};
};

// ===================== 模式解析 (后端 /api/tts/config, 短缓存) =====================

const MODE_TTL = 30_000;
interface TtsServerConfig {
  mode: TtsMode;
  /** 浏览器本地模式的首选音源名 (管理员配置, 设备无此音源时自动回落) */
  frontendVoice: string;
}
let configCache: (TtsServerConfig & { at: number }) | null = null;
let configPromise: Promise<TtsServerConfig> | null = null;

const resolveTtsConfig = (): Promise<TtsServerConfig> => {
  if (configCache && Date.now() - configCache.at < MODE_TTL) {
    return Promise.resolve({ mode: configCache.mode, frontendVoice: configCache.frontendVoice });
  }
  if (configPromise) return configPromise;

  const p = (async (): Promise<TtsServerConfig> => {
    try {
      const ctrl = new AbortController();
      const timer = window.setTimeout(() => ctrl.abort(), 3000);
      const res = await fetch(`${baseURL}/api/tts/config`, {
        headers: authHeaders(),
        signal: ctrl.signal
      });
      window.clearTimeout(timer);
      const json = await res.json();
      const mode = json?.data?.mode;
      if (mode === 'openai' || mode === 'edge' || mode === 'frontend') {
        const cfg: TtsServerConfig = {
          mode,
          frontendVoice: typeof json?.data?.frontend_voice === 'string' ? json.data.frontend_voice : ''
        };
        configCache = { ...cfg, at: Date.now() };
        return cfg;
      }
    } catch {
      /* 离线 / 旧后端: 回落前端本地朗读 */
    }
    const fallback: TtsServerConfig = { mode: 'frontend', frontendVoice: '' };
    configCache = { ...fallback, at: Date.now() };
    return fallback;
  })();

  configPromise = p;
  void p.finally(() => {
    configPromise = null;
  });
  return p;
};

// ===================== 浏览器本地朗读 (Web Speech) =====================

let voicesReady = false;

const LANG_TIMEOUT = 1000;

const normalizeLang = (lang: string) => lang.toLowerCase().replace(/_/g, '-');

const getVoicesSafe = (): SpeechSynthesisVoice[] => {
  if (!window.speechSynthesis) return [];
  try {
    return window.speechSynthesis.getVoices() || [];
  } catch {
    return [];
  }
};

/**
 * 等待 voices 加载完成
 *
 * 桌面 Chrome 首次调用 getVoices() 常返回空数组，需等待 voiceschanged；
 * 但安卓上（尤其部分国产浏览器 / WebView）voiceschanged 可能永远不触发，
 * 所以必须加超时兜底，绝不让 Promise 永远挂起（否则 speak 永远不会执行）。
 */
const ensureVoices = (): Promise<SpeechSynthesisVoice[]> => {
  const cached = getVoicesSafe();
  if (cached.length > 0) {
    voicesReady = true;
    return Promise.resolve(cached);
  }

  return new Promise((resolve) => {
    const synth = window.speechSynthesis!;
    let settled = false;

    const finish = () => {
      if (settled) return;
      settled = true;
      synth.removeEventListener('voiceschanged', handler);
      const voices = getVoicesSafe();
      if (voices.length > 0) {
        voicesReady = true;
      }
      resolve(voices);
    };

    const handler = () => finish();
    synth.addEventListener('voiceschanged', handler);
    // 超时兜底：安卓上取不到 voices 也要继续播放（仅靠 lang 也能正常发音）
    setTimeout(finish, LANG_TIMEOUT);
  });
};

/**
 * 各平台内置音源参差不齐（macOS 的 en 列表前面是 Daniel/Albert 等男声，
 * Windows 是 David/Zira，Chrome 各平台还内置 Google 音源），
 * getVoices() 的顺序又不可靠，所以按下面的「女性/自然音源」优先级匹配，
 * 让不同平台听感尽量一致。按主语言码区分。
 */
const VOICE_PRIORITY: Record<string, RegExp[]> = {
  en: [
    /google us english/i, // Chrome 各平台内置，跨平台最一致
    /aria/i,              // Windows Edge 在线自然音
    /ava/i,               // macOS/iOS
    /allison|samantha|serena|susan|karen|moira|tessa|jenny|zira/i,
    /female.*(us|united states)/i,
  ],
  zh: [
    /google.*(普通话|mandarin)/i,
    /yu.?shu|shu.?yu/i,   // 客户端常见女声
    /ting.?ting|xiaoyan|xiao.?yan|huihui|yaoyao|han.?han|sin.?ji/i,
  ],
};

/** 用户手动选定并记忆的音源（localStorage） */
const VOICE_STORAGE_KEY = 'tts_preferred_voice';

export const getPreferredVoiceName = (): string | null => {
  try {
    return localStorage.getItem(VOICE_STORAGE_KEY);
  } catch {
    return null;
  }
};

export const setPreferredVoiceName = (name: string | null) => {
  try {
    if (name) {
      localStorage.setItem(VOICE_STORAGE_KEY, name);
    } else {
      localStorage.removeItem(VOICE_STORAGE_KEY);
    }
  } catch {
    /* localStorage 不可用时忽略，仅本次会话不生效 */
  }
};

/** 列出指定主语言的可选音源（供设置界面用）；lang 不传则返回全部 */
export const listVoiceOptions = (lang?: string) => {
  const voices = getVoicesSafe();
  if (!lang) return voices;
  const primary = normalizeLang(lang).split('-')[0];
  return voices.filter(v => normalizeLang(v.lang).split(/[-_]/)[0] === primary);
};

/**
 * 按 lang 挑选 voice，格式兼容：en-US / en_US / en
 * 选择顺序：用户记忆的音源 > 管理员配置的本地首选音源 > 音源优先级表 > 首个匹配项
 * 找不到就不设置 voice（安卓上仅靠 lang 即可发音）
 */
const pickVoice = (voices: SpeechSynthesisVoice[], lang: string) => {
  if (!voices.length) return undefined;
  const target = normalizeLang(lang);
  const primary = target.split('-')[0];

  const sameLang = (v: SpeechSynthesisVoice) =>
    normalizeLang(v.lang).replace(/[-_]/g, '') === target.replace(/[-_]/g, '');
  const samePrimary = (v: SpeechSynthesisVoice) =>
    normalizeLang(v.lang).split(/[-_]/)[0] === primary;

  const pools = [voices.filter(sameLang), voices.filter(samePrimary)].filter(pool => pool.length > 0);
  if (!pools.length) return undefined;

  const findNamed = (name: string) => {
    if (!name) return undefined;
    for (const pool of pools) {
      const hit = pool.find(v => v.name === name || v.voiceURI === name);
      if (hit) return hit;
    }
    return undefined;
  };

  // 1. 用户手动选过的音源优先 (localStorage, 设备本地)
  const preferred = getPreferredVoiceName();
  if (preferred) {
    const hit = findNamed(preferred);
    if (hit) return hit;
  }

  // 2. 管理员配置的本地首选音源 (system_config tts_frontend_voice)
  const hit = findNamed(serverFrontendVoice);
  if (hit) return hit;

  // 3. 按平台音源优先级表匹配，保证 macOS / Windows / 安卓听感尽量一致
  const patterns = VOICE_PRIORITY[primary];
  if (patterns) {
    for (const pool of pools) {
      for (const pattern of patterns) {
        const byPattern = pool.find(v => pattern.test(v.name));
        if (byPattern) return byPattern;
      }
    }
  }

  // 4. 兜底：第一个语言匹配项（保持旧行为）
  return pools[0][0];
};

/** 管理员配置的本地首选音源 (resolveTtsConfig 成功后写入) */
let serverFrontendVoice = '';

const isIOS = () => {
  if (typeof navigator === 'undefined') return false;
  const ua = navigator.userAgent;
  // iPadOS 13+ 伪装成 Mac, 靠触点数识别
  return /iphone|ipad|ipod/i.test(ua) || (/macintosh/i.test(ua) && navigator.maxTouchPoints > 1);
};

/**
 * iOS Safari 要求 speechSynthesis 在用户手势调用栈里「用过一次」才解锁,
 * 之后任意时机调用才有声音。首次任意交互时静音念一个空格完成解锁。
 */
let speechUnlocked = false;
const unlockSpeechOnce = () => {
  if (speechUnlocked) return;
  speechUnlocked = true;
  document.removeEventListener('pointerdown', unlockSpeechOnce, true);
  const synth = window.speechSynthesis;
  if (!synth) return;
  try {
    const u = new SpeechSynthesisUtterance(' ');
    u.volume = 0;
    synth.speak(u);
  } catch {
    /* 解锁失败不影响后续正常调用 */
  }
};
if (typeof document !== 'undefined' && typeof window !== 'undefined' && 'speechSynthesis' in window) {
  document.addEventListener('pointerdown', unlockSpeechOnce, true);
}

/** 防部分浏览器 GC 掉队列中的 utterance 导致中途静音: 持有引用直到播完 */
const liveUtterances: SpeechSynthesisUtterance[] = [];

/**
 * 播放单条 utterance, 结束时 resolve。
 * iOS 上首条必须同步发 (保持手势调用栈); 其余平台 cancel 后延迟发 (安卓会静默吞掉同帧 speak)。
 */
const speakUtterance = (
  text: string,
  lang: string,
  rate: number,
  token: number,
  immediate: boolean,
  voiceName?: string
): Promise<void> =>
  new Promise<void>((resolve, reject) => {
    const synth = window.speechSynthesis!;
    const utterance = new SpeechSynthesisUtterance(text);
    utterance.lang = lang;
    utterance.rate = rate;

    const voices = getVoicesSafe();
    let voice = voiceName
      ? voices.find(v => v.name === voiceName || v.voiceURI === voiceName)
      : undefined;
    if (!voice) {
      voice = pickVoice(voices, lang);
    }
    if (voice) {
      utterance.voice = voice;
    }

    liveUtterances.push(utterance);
    let done = false;

    const cleanup = () => {
      const idx = liveUtterances.indexOf(utterance);
      if (idx >= 0) liveUtterances.splice(idx, 1);
    };
    const finish = () => {
      if (done) return;
      done = true;
      cleanup();
      resolve();
    };
    const fail = (reason: unknown) => {
      if (done) return;
      done = true;
      cleanup();
      // 主动 cancel 引起的 interrupted / canceled 不算失败
      if (reason === 'interrupted' || reason === 'canceled') {
        resolve();
        return;
      }
      console.error('TTS error:', reason);
      reject(new Error(String(reason)));
    };

    utterance.addEventListener('end', finish);
    utterance.addEventListener('error', (e) => fail((e as SpeechSynthesisErrorEvent).error || 'speech error'));

    // 安卓 Chrome 已知 bug：引擎可能卡在 paused 状态，speak() 前先 resume() 才有声音
    synth.resume();

    const fire = () => {
      if (done) return;
      // 已被取消/取代: 直接收尾, 不让 Promise 悬挂
      if (token !== speakToken) {
        finish();
        return;
      }
      try {
        synth.speak(utterance);
      } catch (err) {
        fail(err);
      }
    };

    if (immediate && isIOS()) {
      fire();
    } else {
      requestAnimationFrame(() => {
        setTimeout(fire, 100);
      });
    }
  });

// ===================== 长文本分块 =====================

/**
 * 按句子边界切分为句元组 (中英文标点都认), 单句超长再硬切。
 * 桌面 Chrome 用网络音源时单条 utterance 约 15 秒会被掐断, 必须分块排队;
 * 服务端模式分块还能提高缓存命中率 (单词/短句重复播放免合成)。
 * 注意: 不在这里做长度重组, 交给 groupByLang 按语言分组后再合并。
 */
const SENTENCE_SPLIT = /([.!?？！。；;…]+[\s"'"”）)\]】]*)/;

const splitSentences = (text: string, maxLen: number): string[] => {
  const normalized = text.replace(/\s+/g, ' ').trim();
  if (!normalized) return [];

  // split 的捕获组使「正文与分隔符」交替出现, 两两合并成一个句子单元
  const pieces = normalized.split(SENTENCE_SPLIT);
  const units: string[] = [];
  for (let i = 0; i < pieces.length; i += 2) {
    const unit = ((pieces[i] || '') + (pieces[i + 1] || '')).trim();
    if (!unit) continue;
    if (unit.length > maxLen) {
      for (let j = 0; j < unit.length; j += maxLen) {
        units.push(unit.slice(j, j + maxLen).trim());
      }
    } else {
      units.push(unit);
    }
  }
  return units.filter(Boolean);
};

/** 每次调用 speak 递增，用于让「已取消/已过时」的播放任务作废 */
let speakToken = 0;

/** 块级语言检测: CJK 字符占比高视为中文, 否则用调用方默认语言 (支持中英混排逐句切音色) */
const detectChunkLang = (text: string, fallback: string): string => {
  let cjk = 0;
  for (const ch of text) {
    if (ch >= '\u4e00' && ch <= '\u9fff') cjk += 1;
  }
  return cjk * 2 > text.length ? 'zh-CN' : fallback;
};

interface SpeechSegment {
  text: string;
  lang: string;
}

/** 相邻同语言句块合并成段 (合并后不超 maxLen), 每段用各自语言请求合成/选音色 */
const groupByLang = (chunks: string[], fallback: string, maxLen: number): SpeechSegment[] => {
  const segs: SpeechSegment[] = [];
  for (const chunk of chunks) {
    const lang = detectChunkLang(chunk, fallback);
    const last = segs[segs.length - 1];
    if (last && last.lang === lang && last.text.length + chunk.length + 1 <= maxLen) {
      last.text += ` ${chunk}`;
    } else {
      segs.push({ text: chunk, lang });
    }
  }
  return segs;
};

const speakViaBrowser = async (
  text: string,
  lang: string,
  rate: number,
  token: number,
  voiceName?: string
) => {
  const units = splitSentences(text, 180);
  const segs = groupByLang(units, lang, 180);
  for (let i = 0; i < segs.length; i += 1) {
    if (token !== speakToken) return;
    // eslint-disable-next-line no-await-in-loop
    await speakUtterance(segs[i].text, segs[i].lang, rate, token, i === 0, voiceName);
  }
};

// ===================== 服务端合成播放 (openai / edge 模式) =====================

// 单一持久 Audio 元素: 部分国产移动浏览器 (vivo/UC/QQ 等) 对频繁新建媒体元素
// 或连续多次 blob 播放有限制, 复用同一元素换 src 是兼容性最好的做法
let audioEl: HTMLAudioElement | null = null;
let audioUrl: string | null = null;
let audioCancel: (() => void) | null = null;

const getAudioEl = (): HTMLAudioElement => {
  if (!audioEl) {
    audioEl = new Audio();
    audioEl.preload = 'auto';
  }
  return audioEl;
};

const stopAudio = () => {
  if (audioCancel) {
    const cancel = audioCancel;
    audioCancel = null;
    cancel();
  }
  if (audioEl) {
    try {
      audioEl.pause();
    } catch {
      /* 忽略 */
    }
    audioEl.removeAttribute('src');
  }
  if (audioUrl) {
    URL.revokeObjectURL(audioUrl);
    audioUrl = null;
  }
};

/** 网络层失败判定 (TypeError: failed to fetch 等), 仅这类错误做自动重试 */
const isNetworkError = (err: unknown): boolean => {
  const msg = String((err as Error)?.message || err).toLowerCase();
  return msg.includes('failed to fetch') || msg.includes('networkerror') || msg.includes('load failed') || msg.includes('network request failed');
};

const fetchAudio = async (text: string, lang: string, rate: number): Promise<Blob> => {
  const query = new URLSearchParams({ text, lang, rate: String(rate) });
  const url = `${baseURL}/api/tts?${query.toString()}`;
  let lastErr: unknown = null;
  for (let attempt = 0; attempt < 2; attempt += 1) {
    try {
      const res = await fetch(url, { headers: authHeaders() });
      if (!res.ok) {
        throw new Error(`语音服务错误 (${res.status})`);
      }
      const contentType = res.headers.get('content-type') || '';
      if (!contentType.includes('audio')) {
        // 后端统一 JSON 错误体 (如 frontend 模式 404 / 合成失败 500)
        let msg = 'TTS 服务错误';
        try {
          const json = await res.json();
          msg = json?.msg || msg;
        } catch {
          /* 保留默认消息 */
        }
        throw new Error(msg);
      }
      return await res.blob();
    } catch (err) {
      lastErr = err;
      // 业务错误 (状态码/参数) 不重试; 移动网络抖动重试一次
      if (attempt === 0 && isNetworkError(err)) {
        // eslint-disable-next-line no-await-in-loop
        await new Promise(r => window.setTimeout(r, 600));
        continue;
      }
      if (isNetworkError(err)) {
        throw new Error('网络异常,无法获取语音');
      }
      throw err;
    }
  }
  throw lastErr instanceof Error ? lastErr : new Error(String(lastErr));
};

const playBlob = (blob: Blob, token: number) =>
  new Promise<void>((resolve, reject) => {
    const el = getAudioEl();
    // 清理上一段的 URL 与取消器 (元素本身复用)
    if (audioUrl) {
      URL.revokeObjectURL(audioUrl);
      audioUrl = null;
    }
    audioCancel = null;
    const url = URL.createObjectURL(blob);
    audioUrl = url;
    let done = false;

    const finish = () => {
      if (done) return;
      done = true;
      audioCancel = null;
      resolve();
    };
    const fail = () => {
      if (done) return;
      done = true;
      audioCancel = null;
      reject(new Error('音频播放失败'));
    };

    // 被 stopSpeak / 新 speak 取代时按正常结束收尾
    audioCancel = () => {
      if (done) return;
      done = true;
      try {
        el.pause();
      } catch {
        /* 忽略 */
      }
      resolve();
    };
    if (token !== speakToken) {
      audioCancel();
      return;
    }

    // 持久元素必须用 on* 赋值 (addEventListener 会跨句叠加监听器)
    el.onended = finish;
    el.onerror = fail;
    el.src = url;
    el.play().catch(() => {
      // 部分移动浏览器偶发 play() 首次失败, 稍候重试一次
      window.setTimeout(() => {
        if (done) return;
        el.play().then(finish).catch(fail);
      }, 300);
    });
  });

const speakViaServer = async (text: string, lang: string, rate: number, token: number) => {
  // 分块粒度对齐缓存: 短句整块, 长段落按 1500 字符切 (OpenAI 单次上限 4096);
  // 相邻同语言句块合并后按各自语言合成, 中英混排自动切换音色
  const units = splitSentences(text, 1500);
  const segs = groupByLang(units, lang, 1200);
  // 预取下一段: 当前段播放期间并行完成请求, 移动网络下消除句间等待
  let pending: Promise<Blob | null> | null = null;
  let pendingErr: unknown = null;
  for (let i = 0; i < segs.length; i += 1) {
    if (token !== speakToken) return;
    let blob: Blob;
    if (pending) {
      // eslint-disable-next-line no-await-in-loop
      const prefetched = await pending;
      if (!prefetched) throw pendingErr instanceof Error ? pendingErr : new Error(String(pendingErr));
      blob = prefetched;
      pending = null;
      pendingErr = null;
    } else {
      // eslint-disable-next-line no-await-in-loop
      blob = await fetchAudio(segs[i].text, segs[i].lang, rate);
    }
    if (token !== speakToken) {
      stopAudio();
      return;
    }
    if (i + 1 < segs.length) {
      const next = segs[i + 1];
      pendingErr = null;
      pending = fetchAudio(next.text, next.lang, rate).catch(err => {
        pendingErr = err;
        return null;
      });
    }
    // eslint-disable-next-line no-await-in-loop
    await playBlob(blob, token);
  }
};

// ===================== 对外入口 =====================

/**
 * 播放语音 (统一入口):
 * - 按 tts_mode 分流: openai/edge 走后端合成 mp3, 失败自动回退浏览器本地
 * - 长文本自动分块, 规避 Chrome 网络音源 15 秒掐断
 * - 播放结束 resolve, 被取消/取代也 resolve (不算错误)
 */
export const speak = async (
  text: string,
  options?: {
    lang?: string;
    rate?: number;
  }
) => {
  const lang = options?.lang || 'en-US';
  const rate = options?.rate ?? 0.9;
  const token = ++speakToken;

  // 1. 停止之前的播放 (音频 + 本地引擎)
  stopAudio();
  if (window.speechSynthesis) {
    try {
      window.speechSynthesis.cancel();
    } catch {
      /* 忽略 */
    }
  }

  const clean = (text || '').trim();
  if (!clean) return;

  // 2. 解析模式 (短缓存; 后端不可达时回落本地)
  const cfg = await resolveTtsConfig();
  serverFrontendVoice = cfg.frontendVoice;

  // 3. 服务端合成优先 (openai / edge)
  if (cfg.mode !== 'frontend') {
    try {
      await speakViaServer(clean, lang, rate, token);
      return;
    } catch (err) {
      // 已被新调用/停止取代时不兜底, 直接结束
      if (token !== speakToken) return;
      if (!window.speechSynthesis) throw err;
      console.warn('[tts] 服务端合成失败, 回退浏览器本地朗读:', err);
    }
  }

  // 4. 浏览器本地朗读
  if (!window.speechSynthesis) {
    throw new Error('SpeechSynthesis not supported');
  }

  if (!voicesReady) {
    await ensureVoices();
    if (token !== speakToken) return;
  }

  await speakViaBrowser(clean, lang, rate, token, undefined);
};

/**
 * 强制浏览器本地朗读 (管理端配置页测试用): 不走模式分流,
 * 可显式指定音源名 (未指定时按 用户记忆 > 平台优先级 挑选)
 */
export const speakLocal = async (
  text: string,
  options?: {
    lang?: string;
    rate?: number;
    voice?: string;
  }
) => {
  const lang = options?.lang || 'en-US';
  const rate = options?.rate ?? 1;
  const token = ++speakToken;

  stopAudio();
  if (window.speechSynthesis) {
    try {
      window.speechSynthesis.cancel();
    } catch {
      /* 忽略 */
    }
  }

  const clean = (text || '').trim();
  if (!clean) return;
  if (!window.speechSynthesis) {
    throw new Error('SpeechSynthesis not supported');
  }

  if (!voicesReady) {
    await ensureVoices();
    if (token !== speakToken) return;
  }

  await speakViaBrowser(clean, lang, rate, token, options?.voice);
};

/**
 * 等待浏览器音源列表就绪并返回 (管理端配置页枚举可选项用)。
 * 浏览器不支持时返回空数组。
 */
export const ensureBrowserVoices = async (): Promise<SpeechSynthesisVoice[]> => {
  if (typeof window === 'undefined' || !('speechSynthesis' in window)) return [];
  await ensureVoices();
  return getVoicesSafe();
};

/**
 * 停止播放 (本地引擎 + 服务端音频一并停止)
 */
export const stopSpeak = () => {
  speakToken += 1;
  stopAudio();
  if (window.speechSynthesis) {
    try {
      window.speechSynthesis.cancel();
    } catch {
      /* 忽略 */
    }
  }
};
