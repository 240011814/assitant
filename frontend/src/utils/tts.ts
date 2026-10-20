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

/** 每次调用 speak 递增，用于让「已取消/已过时」的延迟 speak 任务作废 */
let speakToken = 0;

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

/**
 * 播放语音（稳定版，兼容安卓）
 */
export const speak = async (
  text: string,
  options?: {
    lang?: string;
    rate?: number;
  }
) => {
  if (!window.speechSynthesis) {
    throw new Error('SpeechSynthesis not supported');
  }

  const synth = window.speechSynthesis;
  const token = ++speakToken;

  // 1. 确保 voices ready（Chrome 必须；安卓取不到也不阻塞）
  if (!voicesReady) {
    await ensureVoices();
  }

  // 2. 停止之前播放
  synth.cancel();

  const utterance = new SpeechSynthesisUtterance(text);

  utterance.lang = options?.lang || 'en-US';
  utterance.rate = options?.rate ?? 0.9;

  // 3. 选择 voice：按宽松规则匹配，找不到就不设置（安卓上仅靠 lang 即可发音）
  const voice = pickVoice(getVoicesSafe(), utterance.lang);
  if (voice) {
    utterance.voice = voice;
  }

  // 4. 返回 Promise，播放结束时 resolve
  return new Promise<void>((resolve, reject) => {
    let done = false;

    const finish = () => {
      if (done) return;
      done = true;
      resolve();
    };

    const fail = (reason: string) => {
      if (done) return;
      done = true;
      // 主动 cancel 引起的 interrupted / canceled 不算失败
      if (reason === 'interrupted' || reason === 'canceled') {
        resolve();
        return;
      }
      console.error('TTS error:', reason);
      reject(new Error(reason));
    };

    utterance.addEventListener('end', finish);
    utterance.addEventListener('error', (e) => fail(e.error || 'speech error'));

    // 安卓 Chrome 已知 bug：引擎可能卡在 paused 状态，speak() 前先 resume() 才有声音
    synth.resume();

    // 已知安卓 bug：cancel() 后同帧调用 speak() 会被静默吞掉，
    // rAF 延迟一帧（桌面端习惯做法）在安卓上不够，再加一小段缓冲
    requestAnimationFrame(() => {
      setTimeout(() => {
        if (done || token !== speakToken) return;
        try {
          synth.speak(utterance);
        } catch (err) {
          console.error('TTS error:', err);
          reject(err as Error);
        }
      }, 100);
    });
  });
};

/**
 * 停止播放
 */
export const stopSpeak = () => {
  speakToken += 1;
  if (window.speechSynthesis) {
    window.speechSynthesis.cancel();
  }
};
