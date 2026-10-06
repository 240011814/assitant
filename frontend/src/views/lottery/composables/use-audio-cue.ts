import { ref } from 'vue';

/**
 * 抽奖音效：基于 Web AudioContext 合成音效
 * 从 views/lottery/[id].vue 原样抽取，AudioContext 为惰性创建，调用时序与原实现一致
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

  function playGachaSound(rarity: number) {
    // 流星下落音效
    playSound(800, 0.3, 'sine', 0.2);
    setTimeout(() => playSound(600, 0.2, 'sine', 0.15), 200);

    // 揭示音效（根据稀有度不同）
    setTimeout(() => {
      if (rarity >= 5) {
        // 传说 - 华丽的和弦
        playSound(523, 0.8, 'sine', 0.3);
        setTimeout(() => playSound(659, 0.6, 'sine', 0.25), 100);
        setTimeout(() => playSound(784, 0.6, 'sine', 0.25), 200);
        setTimeout(() => playSound(1047, 0.8, 'sine', 0.3), 300);
      } else if (rarity >= 4) {
        // 史诗 - 双音
        playSound(440, 0.6, 'sine', 0.25);
        setTimeout(() => playSound(554, 0.5, 'sine', 0.2), 150);
      } else {
        // 普通 - 单音
        playSound(349, 0.4, 'sine', 0.2);
      }
    }, 800);
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
    playGachaSound,
    playClickSound,
    playWheelSpinSound,
    playWinSound,
    playLoseSound
  };
}

export type AudioCueReturn = ReturnType<typeof useAudioCue>;
