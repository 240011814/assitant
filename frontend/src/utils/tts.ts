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
let modeCache: { mode: TtsMode; at: number } | null = null;
let modePromise: Promise<TtsMode> | null = null;

const resolveMode = (): Promise<TtsMode> => {
  if (modeCache && Date.now() - modeCache.at < MODE_TTL) return Promise.resolve(modeCache.mode);
  if (modePromise) return modePromise;

  const p = (async (): Promise<TtsMode> => {
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
        modeCache = { mode, at: Date.now() };
        return mode;
      }
    } catch {
      /* 离线 / 旧后端: 回落前端本地朗读 */
    }
    modeCache = { mode: 'frontend', at: Date.now() };
    return 'frontend';
  })();

  modePromise = p;
  void p.finally(() => {
    modePromise = null;
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
 * 选择顺序：用户记忆的音源 > 音源优先级表 > 首个匹配项
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

  // 1. 用户手动选过的音源优先
  const preferred = getPreferredVoiceName();
  if (preferred) {
    for (const pool of pools) {
      const hit = pool.find(v => v.name === preferred || v.voiceURI === preferred);
      if (hit) return hit;
    }
  }

  // 2. 按平台音源优先级表匹配，保证 macOS / Windows / 安卓听感尽量一致
  const patterns = VOICE_PRIORITY[primary];
  if (patterns) {
    for (const pool of pools) {
      for (const pattern of patterns) {
        const hit = pool.find(v => pattern.test(v.name));
        if (hit) return hit;
      }
    }
  }

  // 3. 兜底：第一个语言匹配项（保持旧行为）
  return pools[0][0];
};

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
  immediate: boolean
): Promise<void> =>
  new Promise<void>((resolve, reject) => {
    const synth = window.speechSynthesis!;
    const utterance = new SpeechSynthesisUtterance(text);
    utterance.lang = lang;
    utterance.rate = rate;

    const voice = pickVoice(getVoicesSafe(), lang);
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
 * 按句子边界切块 (中英文标点都认), 单句超长再硬切。
 * 桌面 Chrome 用网络音源时单条 utterance 约 15 秒会被掐断, 必须分块排队;
 * 服务端模式分块还能提高缓存命中率 (单词/短句重复播放免合成)。
 */
const SENTENCE_SPLIT = /([.!?？！。；;…]+[\s"'"”）)\]】]*)/;

const splitText = (text: string, maxLen: number): string[] => {
  const normalized = text.replace(/\s+/g, ' ').trim();
  if (!normalized) return [];
  if (normalized.length <= maxLen) return [normalized];

  // split 的捕获组使「正文与分隔符」交替出现, 两两合并成一个句子单元
  const pieces = normalized.split(SENTENCE_SPLIT);
  const units: string[] = [];
  for (let i = 0; i < pieces.length; i += 2) {
    const unit = ((pieces[i] || '') + (pieces[i + 1] || '')).trim();
    if (unit) units.push(unit);
  }

  const chunks: string[] = [];
  let buf = '';
  const flush = () => {
    if (buf.trim()) chunks.push(buf.trim());
    buf = '';
  };
  for (const unit of units) {
    if (unit.length > maxLen) {
      flush();
      for (let i = 0; i < unit.length; i += maxLen) {
        chunks.push(unit.slice(i, i + maxLen).trim());
      }
      continue;
    }
    if ((buf + unit).length > maxLen) flush();
    buf += unit;
  }
  flush();
  return chunks.filter(Boolean);
};

/** 每次调用 speak 递增，用于让「已取消/已过时」的播放任务作废 */
let speakToken = 0;

const speakViaBrowser = async (text: string, lang: string, rate: number, token: number) => {
  const chunks = splitText(text, 180);
  for (let i = 0; i < chunks.length; i += 1) {
    if (token !== speakToken) return;
    // eslint-disable-next-line no-await-in-loop
    await speakUtterance(chunks[i], lang, rate, token, i === 0);
  }
};

// ===================== 服务端合成播放 (openai / edge 模式) =====================

let audioEl: HTMLAudioElement | null = null;
let audioUrl: string | null = null;
let audioCancel: (() => void) | null = null;

const stopAudio = () => {
  if (audioCancel) {
    const cancel = audioCancel;
    audioCancel = null;
    cancel();
  }
  if (audioEl) {
    audioEl.pause();
    audioEl.removeAttribute('src');
    audioEl = null;
  }
  if (audioUrl) {
    URL.revokeObjectURL(audioUrl);
    audioUrl = null;
  }
};

const fetchAudio = async (text: string, lang: string, rate: number): Promise<Blob> => {
  const query = new URLSearchParams({ text, lang, rate: String(rate) });
  const res = await fetch(`${baseURL}/api/tts?${query.toString()}`, { headers: authHeaders() });
  if (!res.ok) {
    throw new Error(`TTS HTTP ${res.status}`);
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
  return res.blob();
};

const playBlob = (blob: Blob, token: number) =>
  new Promise<void>((resolve, reject) => {
    const url = URL.createObjectURL(blob);
    audioUrl = url;
    const el = new Audio(url);
    audioEl = el;
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

    el.addEventListener('ended', finish);
    el.addEventListener('error', fail);
    el.play().catch(fail);
  });

const speakViaServer = async (text: string, lang: string, rate: number, token: number) => {
  // 分块粒度对齐缓存: 短句整块, 长段落按 1500 字符切 (OpenAI 单次上限 4096)
  const chunks = splitText(text, 1500);
  for (const chunk of chunks) {
    if (token !== speakToken) return;
    // eslint-disable-next-line no-await-in-loop
    const blob = await fetchAudio(chunk, lang, rate);
    if (token !== speakToken) {
      stopAudio();
      return;
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
  const mode = await resolveMode();

  // 3. 服务端合成优先 (openai / edge)
  if (mode !== 'frontend') {
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

  await speakViaBrowser(clean, lang, rate, token);
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
