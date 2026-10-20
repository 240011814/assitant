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
 * 按 lang 挑选 voice，格式兼容：en-US / en_US / en
 * 只选同主语言码的 voice，避免某些安卓浏览器加载语音失败而静音
 */
const pickVoice = (voices: SpeechSynthesisVoice[], lang: string) => {
  if (!voices.length) return undefined;
  const target = normalizeLang(lang);
  const primary = target.split('-')[0];
  return (
    voices.find(v => normalizeLang(v.lang) === target) ||
    voices.find(v => normalizeLang(v.lang).replace(/-/g, '') === target.replace(/-/g, '')) ||
    voices.find(v => normalizeLang(v.lang).split(/[-_]/)[0] === primary)
  );
};

/** 每次调用 speak 递增，用于让「已取消/已过时」的延迟 speak 任务作废 */
let speakToken = 0;

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
