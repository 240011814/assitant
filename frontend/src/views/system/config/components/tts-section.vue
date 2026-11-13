<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from "vue";
import { useMessage } from "naive-ui";
import type { SelectOption } from "naive-ui";
import { getAuthorization } from "@/service/request/shared";
import { getServiceBaseURL } from "@/utils/service";
import { speakLocal, stopSpeak, ensureBrowserVoices } from "@/utils/tts";
import { $t } from "@/locales";
import { saveConfigs } from "../shared";
import type { ConfigMap } from "../shared";

defineOptions({ name: "TtsSection" });

const props = defineProps<{ config: ConfigMap }>();

const message = useMessage();
const savingTts = ref(false);
const testingTts = ref(false);
const playingTts = ref(false);
const ttsMode = ref("frontend");
const ttsVoice = ref<string | null>("");
const ttsEdgeVoiceEn = ref<string | null>("");
const ttsEdgeVoiceZh = ref<string | null>("");
const ttsFrontendVoice = ref<string | null>("");
const browserVoiceOptions = ref<{ label: string; value: string }[]>([]);
const browserVoicesSupported = ref(true);
let ttsTestAudio: HTMLAudioElement | null = null;
let ttsTestUrl: string | null = null;

const ttsModeOptions = computed(() => [
  { label: $t("page.system.config.ttsModeFrontend"), value: "frontend" },
  { label: $t("page.system.config.ttsModeOpenai"), value: "openai" },
  { label: $t("page.system.config.ttsModeEdge"), value: "edge" }
]);

watch(
  () => props.config,
  cfg => {
    ttsMode.value = cfg.tts_mode || "frontend";
    ttsVoice.value = cfg.tts_openai_voice ?? "";
    ttsEdgeVoiceEn.value = cfg.tts_edge_voice_en ?? "";
    ttsEdgeVoiceZh.value = cfg.tts_edge_voice_zh ?? "";
    ttsFrontendVoice.value = cfg.tts_frontend_voice ?? "";
    // 配置默认就是浏览器本地模式时, 模式 watch 不触发, 主动枚举一次音源
    if (ttsMode.value === "frontend") {
      void loadBrowserVoiceOptions();
    }
  },
  { immediate: true }
);

/** 切到浏览器本地模式时枚举当前浏览器的真实音源 (用户无需猜支持哪些) */
async function loadBrowserVoiceOptions() {
  if (browserVoiceOptions.value.length > 0) return;
  const voices = await ensureBrowserVoices();
  browserVoicesSupported.value = voices.length > 0;
  browserVoiceOptions.value = voices
    .map(v => ({ label: `${v.name} (${v.lang})`, value: v.name }))
    .sort((a, b) => a.label.localeCompare(b.label));
}

watch(ttsMode, mode => {
  if (mode === "frontend") {
    void loadBrowserVoiceOptions();
  }
});

/** 音色下拉过滤: 同时匹配显示名与值 (用户可直接输入 en-US-AriaNeural 这类音色名), 大小写不敏感 */
function voiceFilter(pattern: string, option: SelectOption): boolean {
  const p = pattern.trim().toLowerCase();
  if (!p) return true;
  return (
    String(option.label ?? "").toLowerCase().includes(p) ||
    String(option.value ?? "").toLowerCase().includes(p)
  );
}

// 音色候选 (NSelect filterable+tag: 下拉可选, 也允许直接输入任意音色名)
const openaiVoiceOptions = ["alloy", "ash", "ballad", "coral", "echo", "fable", "nova", "onyx", "sage", "shimmer", "verse"].map(v => ({
  label: v,
  value: v
}));
const edgeVoiceEnOptions = [
  { label: "Aria · 美式女声", value: "en-US-AriaNeural" },
  { label: "Jenny · 美式女声", value: "en-US-JennyNeural" },
  { label: "Emma · 多语女声", value: "en-US-EmmaMultilingualNeural" },
  { label: "Andrew · 美式男声", value: "en-US-AndrewNeural" },
  { label: "Guy · 美式男声", value: "en-US-GuyNeural" },
  { label: "Sonia · 英式女声", value: "en-GB-SoniaNeural" },
  { label: "Ryan · 英式男声", value: "en-GB-RyanNeural" }
];
const edgeVoiceZhOptions = [
  { label: "晓晓 · 女声", value: "zh-CN-XiaoxiaoNeural" },
  { label: "晓伊 · 女声", value: "zh-CN-XiaoyiNeural" },
  { label: "云希 · 男声", value: "zh-CN-YunxiNeural" },
  { label: "云健 · 男声", value: "zh-CN-YunjianNeural" },
  { label: "晓曼 · 粤语女声", value: "zh-HK-HiuMaanNeural" },
  { label: "曉臻 · 台湾女声", value: "zh-TW-HsiaoChenNeural" }
];

/** NSelect 清空后值为 null, 统一安全取字符串再 trim */
const ttsTrim = (v: string | null | undefined) => (v ?? "").trim();

async function handleSaveTts() {
  savingTts.value = true;
  try {
    const ok = await saveConfigs([
      ["tts_mode", ttsMode.value, $t("page.system.config.remarkTtsMode")],
      ["tts_openai_voice", ttsTrim(ttsVoice.value), $t("page.system.config.remarkTtsVoice")],
      ["tts_edge_voice_en", ttsTrim(ttsEdgeVoiceEn.value), $t("page.system.config.remarkTtsEdgeVoiceEn")],
      ["tts_edge_voice_zh", ttsTrim(ttsEdgeVoiceZh.value), $t("page.system.config.remarkTtsEdgeVoiceZh")],
      ["tts_frontend_voice", ttsTrim(ttsFrontendVoice.value), $t("page.system.config.remarkTtsFrontendVoice")],
    ]);
    if (ok) message.success($t("page.system.config.ttsSaved"));
  } finally {
    savingTts.value = false;
  }
}

// 原生 fetch 的 baseURL 与 axios 封装保持一致 (开发代理/生产直连)
const isHttpProxy = import.meta.env.DEV && import.meta.env.VITE_HTTP_PROXY === "Y";
const { baseURL: ttsFetchBaseURL } = getServiceBaseURL(import.meta.env, isHttpProxy);

function stopTtsTest() {
  if (ttsTestAudio) {
    ttsTestAudio.pause();
    ttsTestAudio = null;
  }
  if (ttsTestUrl) {
    URL.revokeObjectURL(ttsTestUrl);
    ttsTestUrl = null;
  }
  // 浏览器本地朗读一并停止 (speakLocal 的 Promise 会随之正常收尾)
  stopSpeak();
  playingTts.value = false;
}

/** 播放测试: 用表单当前值 (未保存也可测) 合成样例并播放; 播放中再点即停止 */
async function handlePlayTestTts() {
  if (playingTts.value) {
    stopTtsTest();
    return;
  }
  stopTtsTest();

  // 浏览器本地模式: 直接用本机 speechSynthesis 发声, 音色取表单当前值
  if (ttsMode.value === "frontend") {
    if (!("speechSynthesis" in window)) {
      message.error($t("page.system.config.ttsBrowserNoVoices"));
      return;
    }
    const voiceName = ttsTrim(ttsFrontendVoice.value);
    const voice = browserVoiceOptions.value.find(o => o.value === voiceName);
    // 样例文本跟随所选音源的语言 (无匹配则英文)
    const isZhVoice = voice ? /\(zh/i.test(voice.label) : false;
    const sample = isZhVoice ? "你好，这是一段语音朗读测试。" : "Hello, this is the text to speech test.";
    playingTts.value = true;
    testingTts.value = true;
    try {
      await speakLocal(sample, {
        lang: isZhVoice ? "zh-CN" : "en-US",
        rate: 1,
        voice: voiceName || undefined
      });
    } catch (err: any) {
      message.error($t("page.system.config.ttsTestFailed", { error: err?.message || $t("page.system.config.unknownError") }));
    } finally {
      stopTtsTest();
      testingTts.value = false;
    }
    return;
  }

  // 服务端模式: 后端合成 mp3 播放
  // 样例语言跟随用户配置: 配了英文音色用英文样例, 只配了中文音色则用中文样例 (否则听不到所选音色)
  const testLang =
    ttsMode.value === "edge" && !ttsTrim(ttsEdgeVoiceEn.value) && ttsTrim(ttsEdgeVoiceZh.value) ? "zh-CN" : "en-US";
  testingTts.value = true;
  try {
    const Authorization = getAuthorization();
    const res = await fetch(`${ttsFetchBaseURL}/api/admin/system-config/test-tts`, {
      method: "post",
      headers: {
        "Content-Type": "application/json",
        ...(Authorization ? { Authorization } : {})
      },
      body: JSON.stringify({
        mode: ttsMode.value,
        voice: ttsTrim(ttsVoice.value),
        voice_en: ttsTrim(ttsEdgeVoiceEn.value),
        voice_zh: ttsTrim(ttsEdgeVoiceZh.value),
        lang: testLang
      })
    });
    const contentType = res.headers.get("content-type") || "";
    if (!res.ok || contentType.includes("json")) {
      let msg = `HTTP ${res.status}`;
      try {
        const json = await res.json();
        msg = json?.msg || msg;
      } catch {
        /* 保留 HTTP 状态码提示 */
      }
      message.error($t("page.system.config.ttsTestFailed", { error: msg }));
      return;
    }
    const blob = await res.blob();
    ttsTestUrl = URL.createObjectURL(blob);
    const el = new Audio(ttsTestUrl);
    ttsTestAudio = el;
    el.addEventListener("ended", stopTtsTest);
    el.addEventListener("error", stopTtsTest);
    playingTts.value = true;
    await el.play();
  } catch (err: any) {
    stopTtsTest();
    message.error($t("page.system.config.ttsTestFailed", { error: err?.message || $t("page.system.config.unknownError") }));
  } finally {
    testingTts.value = false;
  }
}

onUnmounted(() => {
  stopTtsTest();
});
</script>

<template>
  <!-- 语音朗读 (TTS) -->
  <div class="p-4 rounded-lg border border-gray-200 dark:border-gray-700">
    <div class="mb-4">
      <div class="font-bold text-gray-800 dark:text-gray-200">
        {{ $t("page.system.config.tts") }}
      </div>
      <div class="text-sm text-gray-500 dark:text-gray-400 mt-1">
        {{ $t("page.system.config.ttsDesc") }}
      </div>
    </div>
    <NForm label-placement="left" label-width="120">
      <NFormItem :label="$t('page.system.config.ttsMode')">
        <NSelect v-model:value="ttsMode" :options="ttsModeOptions" class="max-w-90" />
      </NFormItem>
      <NAlert v-if="ttsMode === 'openai'" type="info" :bordered="false" class="mb-3">
        {{ $t("page.system.config.ttsOpenaiHint") }}
      </NAlert>
      <template v-if="ttsMode === 'frontend'">
        <NAlert v-if="!browserVoicesSupported" type="warning" :bordered="false" class="mb-3">
          {{ $t("page.system.config.ttsBrowserNoVoices") }}
        </NAlert>
        <NFormItem :label="$t('page.system.config.ttsFrontendVoice')">
          <NSelect
            v-model:value="ttsFrontendVoice"
            :filter="voiceFilter"
            :options="browserVoiceOptions"
            :placeholder="$t('page.system.config.ttsFrontendVoicePlaceholder')"
            filterable
            tag
            clearable
          />
        </NFormItem>
      </template>
      <NFormItem v-if="ttsMode === 'openai'" :label="$t('page.system.config.ttsVoice')" path="ttsVoice">
        <NSelect
          v-model:value="ttsVoice"
          :filter="voiceFilter"
          :options="openaiVoiceOptions"
          placeholder="alloy"
          filterable
          tag
          clearable
        />
      </NFormItem>
      <NGrid v-else-if="ttsMode === 'edge'" :cols="2" :x-gap="12" :y-gap="8">
        <NFormItemGi :label="$t('page.system.config.ttsEdgeVoiceEn')" path="ttsEdgeVoiceEn">
          <NSelect
            v-model:value="ttsEdgeVoiceEn"
            :filter="voiceFilter"
            :options="edgeVoiceEnOptions"
            placeholder="en-US-AriaNeural"
            filterable
            tag
            clearable
          />
        </NFormItemGi>
        <NFormItemGi :label="$t('page.system.config.ttsEdgeVoiceZh')" path="ttsEdgeVoiceZh">
          <NSelect
            v-model:value="ttsEdgeVoiceZh"
            :filter="voiceFilter"
            :options="edgeVoiceZhOptions"
            placeholder="zh-CN-XiaoxiaoNeural"
            filterable
            tag
            clearable
          />
        </NFormItemGi>
      </NGrid>
      <NFormItem class="mt-4">
        <div class="flex items-center gap-3">
          <NButton type="primary" :loading="savingTts" @click="handleSaveTts">
            {{ $t("page.system.config.saveTts") }}
          </NButton>
          <NButton secondary :loading="testingTts" @click="handlePlayTestTts">
            <template #icon>
              <div :class="playingTts ? 'i-mdi:stop-circle-outline' : 'i-mdi:volume-high'" />
            </template>
            {{ playingTts ? $t("page.system.config.stopTtsTest") : $t("page.system.config.playTest") }}
          </NButton>
        </div>
      </NFormItem>
    </NForm>
  </div>
</template>
