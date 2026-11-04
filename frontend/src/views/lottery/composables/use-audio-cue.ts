import { ref } from 'vue';
import wishExecuteUrl from '@/assets/sound/genshin-wish-execute.ogg';

/**
 * 抽奖音效：基于 Web AudioContext（合成音效 + 原神祈愿采样音）
 * AudioContext 为惰性创建；祈愿音效为 WAV 采样, 首次播放时解码并缓存 AudioBuffer
 */
export function useAudioCue() {
  // 音效管理
  const audioContext = ref<AudioContext | null>(null);

  function initAudio() {
    if (!audioContext.value) {
      audioContext.value = new AudioContext();
    }
  }

  function playSound(frequency: number, duration: number, type: OscillatorType = 'sine', volume: number = 0.3) {
    try {
      initAudio();
      const ctx = audioContext.value!;
      const oscillator = ctx.createOscillator();
      const gainNode = ctx.createGain();

      oscillator.connect(gainNode);
      gainNode.connect(ctx.destination);

      oscillator.frequency.setValueAtTime(frequency, ctx.currentTime);
      oscillator.type = type;

      gainNode.gain.setValueAtTime(volume, ctx.currentTime);
      gainNode.gain.exponentialRampToValueAtTime(0.01, ctx.currentTime + duration);

      oscillator.start(ctx.currentTime);
      oscillator.stop(ctx.currentTime + duration);
    } catch {
      // 静默处理音频错误
    }
  }

  // 原神祈愿完整音效采样 (WAV ~7s: 流星 → 爆发 → 揭示全流程), 解码结果跨抽奖缓存
  let wishBuffer: AudioBuffer | null = null;

  async function loadWishBuffer(): Promise<AudioBuffer | null> {
    if (wishBuffer) return wishBuffer;
    try {
      initAudio();
      const context = audioContext.value;
      if (!context) return null;
      const resp = await fetch(wishExecuteUrl);
      const arr = await resp.arrayBuffer();
      wishBuffer = await context.decodeAudioData(arr);
      return wishBuffer;
    } catch {
      return null;
    }
  }

  /** 原神祈愿音效: 祈愿动画开始时整段播放 (与游戏内一致, 音效自带全流程节奏) */
  async function playWishExecute() {
    try {
      initAudio();
      const context = audioContext.value;
      if (!context) return;
      // 自动播放策略兜底: 用户手势路径下通常已激活, 仍补一次 resume
      if (context.state === 'suspended') {
        await context.resume();
      }
      const buffer = await loadWishBuffer();
      if (!buffer) return;
      const source = context.createBufferSource();
      const gain = context.createGain();
      source.buffer = buffer;
      gain.gain.value = 0.85;
      source.connect(gain);
      gain.connect(context.destination);
      source.start();
    } catch {
      // 静默处理音频错误
    }
  }

  function playClickSound() {
    playSound(1200, 0.1, 'square', 0.1);
  }

  // 转盘旋转音效
  function playWheelSpinSound() {
    // 模拟转盘咔哒声
    let count = 0;
    const interval = setInterval(() => {
      if (count >= 20) {
        clearInterval(interval);
        return;
      }
      playSound(800 + count * 50, 0.05, 'square', 0.1);
      count++;
    }, 150);
  }

  // 中奖音效
  function playWinSound() {
    playSound(523, 0.3, 'sine', 0.3);
    setTimeout(() => playSound(659, 0.3, 'sine', 0.25), 100);
    setTimeout(() => playSound(784, 0.3, 'sine', 0.25), 200);
    setTimeout(() => playSound(1047, 0.5, 'sine', 0.3), 300);
  }

  // 未中奖音效
  function playLoseSound() {
    playSound(440, 0.3, 'sine', 0.2);
    setTimeout(() => playSound(349, 0.4, 'sine', 0.15), 200);
  }

  return {
    audioContext,
    initAudio,
    playSound,
    playWishExecute,
    playClickSound,
    playWheelSpinSound,
    playWinSound,
    playLoseSound
  };
}

export type AudioCueReturn = ReturnType<typeof useAudioCue>;
