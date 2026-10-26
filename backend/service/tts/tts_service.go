package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"backend/model"

	"github.com/gorilla/websocket"
)

// TTS 语音合成: 前端朗读的统一后端音源。
// 模式存 system_config tts_mode: frontend(浏览器本地 Web Speech) / openai(OpenAI 兼容 /audio/speech) /
// edge(微软 Edge 在线朗读, 免费, 走 wss 朗读协议)。openai/edge 模式下前端经 /api/tts 取 mp3 播放,
// 按内容 hash 落盘缓存 (单词/例句重复播放零成本)。
// 注意: OpenAI 兼容 Provider 若 base_url 指向阿里云百炼或小米 MiMo, 其兼容层不含 /audio/speech,
// 后端会透明改用各自的原生 TTS 协议 (synthDashScope / synthXiaomi), 对使用方式无任何变化。
const (
	TtsModeFrontend = "frontend"
	TtsModeOpenAI   = "openai"
	TtsModeEdge     = "edge"

	ttsDefaultModel       = "tts-1"
	ttsDefaultOpenAIVoice = "alloy"
	ttsDefaultEdgeVoiceEN = "en-US-AriaNeural"
	ttsDefaultEdgeVoiceZH = "zh-CN-XiaoxiaoNeural"
	ttsDefaultCacheDir    = "tts-cache"

	// 阿里云百炼原生 TTS 兜底: 未在模型管理标记默认 TTS 模型时使用的模型与音色
	dashScopeDefaultModel = "qwen-audio-3.1-tts-flash"
	dashScopeFemaleVoice  = "longanhuan_v3.1" // 多语种女声
	dashScopeMaleVoice    = "xunanchuan_v3.1" // 多语种男声

	// 小米 MiMo TTS 兜底: TTS 走 chat/completions + audio 字段 (兼容层无 /audio/speech)
	mimoDefaultModel = "mimo-v2.5-tts"
	mimoDefaultVoice = "mimo_default"

	ttsSynthTimeout  = 60 * time.Second
	ttsMaxTextRunes  = 4000 // 单次合成文本上限 (rune), 与 OpenAI 4096 限制留余量
	ttsMaxAudioBytes = 32 << 20
)

// ttsConfig TTS 运行配置快照 (mu 保护, 读时取快照)。
// openai 模式的 base_url/api_key 不在此处: 复用 AI 对话默认激活的 Provider;
// 模型 code 也优先取该 Provider 下标记为默认 TTS 的模型 (ActiveTTSModelCode),
// Provider/默认 TTS 模型变更即时生效
type ttsConfig struct {
	Mode          string // frontend / openai / edge
	OpenAIModel   string
	OpenAIVoice   string
	EdgeVoiceEN   string
	EdgeVoiceZH   string
	FrontendVoice string // 浏览器本地模式首选音源名 (按设备可用性回落)
	CacheDir      string
}

// TtsService 语音合成门面: 配置热刷新 (tts_* 键) + OpenAI/Edge 合成 + 磁盘缓存
type TtsService struct {
	configSvc  *SystemConfigService
	aiAgent    *AIAgentService
	httpClient *http.Client
	mu         sync.RWMutex
	cfg        ttsConfig
	flightMu   sync.Mutex
	flights    map[string]chan struct{} // 进行中的合成请求去重 (并发同文本只合成一次)
}

// NewTtsService 构造并加载一次配置 (system_config 无 tts_* 键时为默认值);
// aiAgent 提供 openai 模式所需的默认激活 Provider 端点, 允许 nil (openai 模式不可用)
func NewTtsService(configSvc *SystemConfigService, aiAgent *AIAgentService) *TtsService {
	s := &TtsService{
		configSvc:  configSvc,
		aiAgent:    aiAgent,
		httpClient: &http.Client{Timeout: ttsSynthTimeout},
		flights:    make(map[string]chan struct{}),
	}
	s.RefreshConfig()
	return s
}

// RefreshConfig 从 system_config 读取 tts_* 键 (RAG/S3 同款热刷新, 配置页保存后触发)
func (s *TtsService) RefreshConfig() {
	getVal := func(key string) string {
		v, _ := s.configSvc.GetValue(key)
		return strings.TrimSpace(v)
	}

	s.mu.Lock()
	s.cfg = buildTtsConfig(getVal)
	s.mu.Unlock()
}

// buildTtsConfig 纯函数: getVal 提供 system_config 取值, 空字段回落默认值
func buildTtsConfig(getVal func(string) string) ttsConfig {
	mode := strings.ToLower(getVal("tts_mode"))
	switch mode {
	case TtsModeOpenAI, TtsModeEdge, TtsModeFrontend:
	default:
		mode = TtsModeFrontend
	}

	cfg := ttsConfig{
		Mode:          mode,
		OpenAIModel:   ttsDefaultModel,
		OpenAIVoice:   getVal("tts_openai_voice"),
		EdgeVoiceEN:   getVal("tts_edge_voice_en"),
		EdgeVoiceZH:   getVal("tts_edge_voice_zh"),
		FrontendVoice: getVal("tts_frontend_voice"),
		CacheDir:      getVal("tts_cache_dir"),
	}
	if cfg.OpenAIVoice == "" {
		cfg.OpenAIVoice = ttsDefaultOpenAIVoice
	}
	if cfg.EdgeVoiceEN == "" {
		cfg.EdgeVoiceEN = ttsDefaultEdgeVoiceEN
	}
	if cfg.EdgeVoiceZH == "" {
		cfg.EdgeVoiceZH = ttsDefaultEdgeVoiceZH
	}
	if cfg.CacheDir == "" {
		cfg.CacheDir = ttsDefaultCacheDir
	}
	return cfg
}

func (s *TtsService) snapshot() ttsConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Mode 当前生效模式 (frontend 时前端走浏览器本地朗读, /api/tts 返回 404)
func (s *TtsService) Mode() string { return s.snapshot().Mode }

// FrontendVoice 浏览器本地模式的首选音源名 (空串表示由前端按平台自动挑选)
func (s *TtsService) FrontendVoice() string { return s.snapshot().FrontendVoice }

// SynthesizeWithConfig 用指定配置快照合成 (管理端测试接口: 表单值未保存也可验证)
func (s *TtsService) SynthesizeWithConfig(ctx context.Context, cfg ttsConfig, text, lang string, rate float64) ([]byte, error) {
	return s.synthesize(ctx, cfg, text, lang, rate)
}

// TestConfig 用配置页表单值构建测试配置 (空字段回落默认值), 不影响运行配置;
// openai 模式端点和模型始终取默认激活 Provider / 默认 TTS 模型
func (s *TtsService) TestConfig(mode, voice, voiceEN, voiceZH string) ttsConfig {
	if mode != TtsModeOpenAI && mode != TtsModeEdge {
		mode = TtsModeOpenAI
	}
	cfg := s.snapshot()
	cfg.Mode = mode
	if voice != "" {
		cfg.OpenAIVoice = voice
	}
	if voiceEN != "" {
		cfg.EdgeVoiceEN = voiceEN
	}
	if voiceZH != "" {
		cfg.EdgeVoiceZH = voiceZH
	}
	return cfg
}

// edgeVoiceFor 按语言主码挑 Edge 音色 (zh 用中文音色, 其余用英文音色)
func (c ttsConfig) edgeVoiceFor(lang string) string {
	if strings.HasPrefix(strings.ToLower(strings.ReplaceAll(lang, "_", "-")), "zh") {
		return c.EdgeVoiceZH
	}
	return c.EdgeVoiceEN
}

// Synthesize 合成 mp3。mode 为 openai/edge 时按配置出声, frontend 返回明确错误;
// 相同 (模式/音色/语速/文本) 落盘缓存, 并发同 key 只合成一次。
func (s *TtsService) Synthesize(ctx context.Context, text, lang string, rate float64) ([]byte, error) {
	return s.synthesize(ctx, s.snapshot(), text, lang, rate)
}

// synthesize 带配置快照的合成 (测试接口用表单值覆盖配置, 不落盘前先合成)
func (s *TtsService) synthesize(ctx context.Context, cfg ttsConfig, text, lang string, rate float64) ([]byte, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("合成文本为空")
	}
	if len([]rune(text)) > ttsMaxTextRunes {
		return nil, fmt.Errorf("文本过长 (上限 %d 字符)", ttsMaxTextRunes)
	}
	if lang == "" {
		lang = "en-US"
	}
	rate = clampRate(rate)

	var openAIBaseURL, openAIKey string
	switch cfg.Mode {
	case TtsModeOpenAI:
		// 复用 AI 对话默认激活的 Provider, 未配置时给出明确指引
		if s.aiAgent == nil {
			return nil, errors.New("OpenAI TTS 需要 AI Provider, 但 AI 服务未初始化")
		}
		var ok bool
		openAIBaseURL, openAIKey, ok = s.aiAgent.ActiveOpenAIEndpoint()
		if !ok || openAIBaseURL == "" || openAIKey == "" {
			return nil, errors.New("未找到默认激活的 AI Provider, 请先在 AI 配置中启用")
		}
		// 模型管理里标记的默认 TTS 模型优先; 误把 ASR 模型标成默认 TTS 时给出明确指引,
		// 而不是让服务商返回难懂的 400/404
		modelCode, modelType, hasMarkedTTS := s.aiAgent.ActiveTTSModel()
		if hasMarkedTTS {
			if modelType == model.AIModelTypeASR {
				return nil, fmt.Errorf("默认 TTS 模型 %q 是语音识别 (ASR) 模型, 请在模型管理中改标语音合成模型", modelCode)
			}
			cfg.OpenAIModel = modelCode
		}
		// 真 OpenAI 未标记时回落 tts-1; 阿里云百炼 / 小米 MiMo 的模型名不做猜测,
		// 未标记默认 TTS 模型直接报错, 避免"看似能用其实用错模型"
		switch {
		case isDashScopeBaseURL(openAIBaseURL):
			if !hasMarkedTTS {
				return nil, fmt.Errorf("未在模型管理中标记默认 TTS 模型, 阿里云百炼不支持自动选择, 请标记 %s 等语音合成模型", dashScopeDefaultModel)
			}
			cfg.OpenAIVoice = dashScopeVoice(cfg.OpenAIVoice)
		case isXiaomiBaseURL(openAIBaseURL):
			if !hasMarkedTTS {
				return nil, fmt.Errorf("未在模型管理中标记默认 TTS 模型, 小米 MiMo 不支持自动选择, 请标记 %s 等语音合成模型", mimoDefaultModel)
			}
			cfg.OpenAIVoice = mimoVoice(cfg.OpenAIVoice, lang)
		default:
			if cfg.OpenAIModel == "" {
				cfg.OpenAIModel = ttsDefaultModel
			}
		}
	case TtsModeEdge:
		// Edge 无需密钥
	default:
		return nil, errors.New("TTS 模式为 frontend, 请使用浏览器本地朗读")
	}

	key := cacheKey(cfg, text, lang, rate)
	if audio, err := s.readCache(cfg, key); err == nil {
		return audio, nil
	}

	// 并发同 key 去重: 先到的合成, 后到的等结果读缓存
	s.flightMu.Lock()
	if done, ok := s.flights[key]; ok {
		s.flightMu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if audio, err := s.readCache(cfg, key); err == nil {
			return audio, nil
		}
		return nil, errors.New("TTS 合成失败")
	}
	done := make(chan struct{})
	s.flights[key] = done
	s.flightMu.Unlock()
	defer func() {
		s.flightMu.Lock()
		delete(s.flights, key)
		s.flightMu.Unlock()
		close(done)
	}()

	ctx, cancel := context.WithTimeout(ctx, ttsSynthTimeout)
	defer cancel()

	var (
		audio []byte
		err   error
	)
	if cfg.Mode == TtsModeOpenAI {
		audio, err = s.synthOpenAI(ctx, openAIBaseURL, openAIKey, cfg, text, lang, rate)
	} else {
		audio, err = s.synthEdge(ctx, cfg.edgeVoiceFor(lang), rate, text)
	}
	if err != nil {
		return nil, err
	}
	if len(audio) == 0 {
		return nil, errors.New("TTS 返回空音频")
	}
	_ = s.writeCache(cfg, key, audio) // 缓存写失败不影响本次返回
	return audio, nil
}

func clampRate(rate float64) float64 {
	if rate <= 0 || rate != rate { // 0 或 NaN 取默认
		return 1.0
	}
	if rate < 0.5 {
		return 0.5
	}
	if rate > 2.0 {
		return 2.0
	}
	return rate
}

// cacheKey 内容寻址: 模式+模型+音色+语速+语言+文本 的 sha256 (配置变化自动失效)
func cacheKey(cfg ttsConfig, text, lang string, rate float64) string {
	voice := cfg.OpenAIVoice
	if cfg.Mode == TtsModeEdge {
		voice = cfg.edgeVoiceFor(lang)
	}
	modelCode := ""
	if cfg.Mode == TtsModeOpenAI {
		modelCode = cfg.OpenAIModel
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s|%.2f|%s", cfg.Mode, modelCode, voice, lang, rate, text)))
	return hex.EncodeToString(sum[:])
}

func (s *TtsService) cachePath(cfg ttsConfig, key string) string {
	return filepath.Join(cfg.CacheDir, key+".mp3")
}

func (s *TtsService) readCache(cfg ttsConfig, key string) ([]byte, error) {
	data, err := os.ReadFile(s.cachePath(cfg, key))
	if err != nil || len(data) == 0 {
		return nil, errors.New("cache miss")
	}
	return data, nil
}

// writeCache 原子写 (临时文件 + rename), 避免并发读到半截文件
func (s *TtsService) writeCache(cfg ttsConfig, key string, audio []byte) error {
	if err := os.MkdirAll(cfg.CacheDir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(cfg.CacheDir, "tts-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(audio); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Rename(name, s.cachePath(cfg, key))
}

// synthOpenAI 调 OpenAI 兼容 /audio/speech (tts-1 等标准接口; 本地兼容服务亦可)。
// baseURL/key 来自默认激活的 AI Provider (与 AI 对话共用配置);
// base_url 指向阿里云百炼 / 小米 MiMo 时其兼容层无 /audio/speech,
// 透明转发到 synthDashScope / synthXiaomi。
func (s *TtsService) synthOpenAI(ctx context.Context, baseURL, apiKey string, cfg ttsConfig, text string, lang string, rate float64) ([]byte, error) {
	if isDashScopeBaseURL(baseURL) {
		return s.synthDashScope(ctx, baseURL, apiKey, cfg, text, lang, rate)
	}
	if isXiaomiBaseURL(baseURL) {
		return s.synthXiaomi(ctx, baseURL, apiKey, cfg, text, lang, rate)
	}

	payload, err := json.Marshal(map[string]any{
		"model":           cfg.OpenAIModel,
		"input":           text,
		"voice":           cfg.OpenAIVoice,
		"response_format": "mp3",
		"speed":           rate,
	})
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(baseURL, "/") + "/audio/speech"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI TTS 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("OpenAI TTS 返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	audio, err := io.ReadAll(io.LimitReader(resp.Body, ttsMaxAudioBytes))
	if err != nil {
		return nil, fmt.Errorf("读取音频失败: %w", err)
	}
	return audio, nil
}

// isDashScopeBaseURL 判断 OpenAI 兼容 Provider 是否指向阿里云百炼 (含业务空间专属域名)。
func isDashScopeBaseURL(baseURL string) bool {
	lower := strings.ToLower(baseURL)
	return strings.Contains(lower, "aliyuncs.com") || strings.Contains(lower, "dashscope")
}

// dashScopeVoice 把 OpenAI 音色名映射为百炼多语种音色; 已是百炼音色名时原样透传。
func dashScopeVoice(voice string) string {
	voice = strings.TrimSpace(voice)
	switch strings.ToLower(voice) {
	case "", "alloy", "nova", "shimmer", "coral", "sage", "fable", "ballad":
		return dashScopeFemaleVoice
	case "echo", "onyx", "ash", "verse":
		return dashScopeMaleVoice
	default:
		return voice // 用户直接填了百炼音色 (如 longanhuan_v3.1 / Emily_v3.1)
	}
}

// dashScopeLanguageType 语言码 -> 百炼 multimodal-generation 的 language_type 取值
func dashScopeLanguageType(lang string) string {
	switch {
	case strings.HasPrefix(strings.ToLower(strings.ReplaceAll(lang, "_", "-")), "zh"):
		return "Chinese"
	case strings.HasPrefix(strings.ToLower(strings.ReplaceAll(lang, "_", "-")), "en"):
		return "English"
	default:
		return "Auto"
	}
}

// dashScopeTTSEndpoint 由 Provider 的 compatible-mode 端点推导百炼原生 TTS 端点。
// qwen-audio/cosyvoice 走业务空间域名的 SpeechSynthesizer (返回 mp3 URL);
// qwen3-tts 仅支持标准地域域名的 multimodal-generation (业务空间域名不支持, 实测 400)。
func dashScopeTTSEndpoint(baseURL, model string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "qwen3-tts") {
		host := "https://dashscope.aliyuncs.com"
		if strings.Contains(strings.ToLower(baseURL), "ap-southeast-1") || strings.Contains(strings.ToLower(baseURL), "dashscope-intl") {
			host = "https://dashscope-intl.aliyuncs.com"
		}
		return host + "/api/v1/services/aigc/multimodal-generation/generation"
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	base = strings.TrimSuffix(base, "/compatible-mode/v1")
	base = strings.TrimSuffix(base, "/compatible-mode")
	base = strings.TrimSuffix(base, "/api/v1")
	return base + "/api/v1/services/audio/tts/SpeechSynthesizer"
}

// synthDashScope 阿里云百炼原生 TTS: 非流式返回音频 URL, 下载后即为 mp3。
// 在 OpenAI 模式内透明调用, 对外接口/缓存/前端播放链路与标准 /audio/speech 完全一致。
func (s *TtsService) synthDashScope(ctx context.Context, baseURL, apiKey string, cfg ttsConfig, text string, lang string, rate float64) ([]byte, error) {
	modelCode := strings.TrimSpace(cfg.OpenAIModel)
	if modelCode == "" {
		return nil, errors.New("阿里云百炼 TTS 未配置模型, 请在模型管理中标记默认 TTS 模型 (如 qwen-audio-3.1-tts-flash)")
	}
	voice := dashScopeVoice(cfg.OpenAIVoice)

	input := map[string]any{"text": text, "voice": voice}
	if strings.HasPrefix(strings.ToLower(modelCode), "qwen3-tts") {
		input["language_type"] = dashScopeLanguageType(lang)
	} else {
		input["format"] = "mp3"
		input["sample_rate"] = 24000
		input["rate"] = rate
	}
	payload, err := json.Marshal(map[string]any{"model": modelCode, "input": input})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dashScopeTTSEndpoint(baseURL, modelCode), strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("百炼 TTS 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, ttsMaxAudioBytes))
	if err != nil {
		return nil, fmt.Errorf("读取百炼 TTS 响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("百炼 TTS 返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Output  struct {
			Audio struct {
				URL  string `json:"url"`
				Data string `json:"data"`
			} `json:"audio"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析百炼 TTS 响应失败: %w", err)
	}
	if result.Code != "" {
		if result.Message == "" {
			result.Message = result.Code
		}
		return nil, fmt.Errorf("百炼 TTS 错误: %s", result.Message)
	}
	if result.Output.Audio.Data != "" {
		audio, err := base64.StdEncoding.DecodeString(result.Output.Audio.Data)
		if err != nil {
			return nil, fmt.Errorf("解码百炼 TTS 音频失败: %w", err)
		}
		return audio, nil
	}
	if result.Output.Audio.URL == "" {
		return nil, errors.New("百炼 TTS 未返回音频地址")
	}

	audioReq, err := http.NewRequestWithContext(ctx, http.MethodGet, result.Output.Audio.URL, nil)
	if err != nil {
		return nil, err
	}
	audioResp, err := s.httpClient.Do(audioReq)
	if err != nil {
		return nil, fmt.Errorf("下载百炼 TTS 音频失败: %w", err)
	}
	defer audioResp.Body.Close()
	if audioResp.StatusCode != http.StatusOK {
		audioBody, _ := io.ReadAll(io.LimitReader(audioResp.Body, 1024))
		return nil, fmt.Errorf("下载百炼 TTS 音频返回 %d: %s", audioResp.StatusCode, strings.TrimSpace(string(audioBody)))
	}
	audio, err := io.ReadAll(io.LimitReader(audioResp.Body, ttsMaxAudioBytes))
	if err != nil {
		return nil, fmt.Errorf("读取百炼 TTS 音频失败: %w", err)
	}
	return audio, nil
}

// isXiaomiBaseURL 判断 OpenAI 兼容 Provider 是否指向小米 MiMo 开放平台。
func isXiaomiBaseURL(baseURL string) bool {
	lower := strings.ToLower(baseURL)
	return strings.Contains(lower, "xiaomimimo.com") || strings.Contains(lower, "mimo.xiaomi.com")
}

// mimoVoice 音色解析 (按语言区分, 见预置音色列表: 冰糖/茉莉/苏打/白桦为中文, Mia/Chloe/Milo/Dean 为英文):
// OpenAI 标准音色名按性别映射, 其余 (MiMo 音色名 / data URI) 原样透传
func mimoVoice(voice, lang string) string {
	voice = strings.TrimSpace(voice)
	if voice == "" {
		return mimoDefaultVoice
	}
	switch strings.ToLower(voice) {
	case "alloy", "nova", "shimmer", "coral", "sage", "fable", "ballad":
		if isZhLang(lang) {
			return "冰糖"
		}
		return "Mia"
	case "echo", "onyx", "ash", "verse":
		if isZhLang(lang) {
			return "苏打"
		}
		return "Milo"
	default:
		return voice // 用户直接填了 MiMo 音色 (如 茉莉 / Chloe / mimo_default) 或克隆 data URI
	}
}

// isZhLang 判断是否中文语种
func isZhLang(lang string) bool {
	return strings.HasPrefix(strings.ToLower(strings.ReplaceAll(lang, "_", "-")), "zh")
}

// mimoChatEndpoint 归一化为小米官方 TTS 端点: 必须带 /v1/chat/completions
// (实测不带 /v1 是 404), base_url 填或不填 /v1、甚至填全路径都能兼容。
func mimoChatEndpoint(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	base = strings.TrimSuffix(base, "/chat/completions")
	base = strings.TrimSuffix(base, "/v1")
	return base + "/v1/chat/completions"
}

// synthXiaomi 小米 MiMo 原生 TTS: 其"OpenAI 兼容"指走 chat/completions + audio 字段,
// 而非标准 /audio/speech。按官方文档:
//   - 待合成文本必须放在 role=assistant 的消息里, user 消息可选 (传风格/语速指令, 内容不会被朗读)
//   - audio.format 支持 wav/mp3/pcm/pcm16, 非流式取 mp3
//   - 响应在 choices[0].message.audio.data (base64)
//
// 对外接口/缓存/前端播放链路与标准 /audio/speech 完全一致。
func (s *TtsService) synthXiaomi(ctx context.Context, baseURL, apiKey string, cfg ttsConfig, text string, lang string, rate float64) ([]byte, error) {
	modelCode := strings.TrimSpace(cfg.OpenAIModel)
	if modelCode == "" {
		return nil, errors.New("小米 TTS 未配置模型, 请在模型管理中标记默认 TTS 模型 (如 mimo-v2.5-tts)")
	}
	voice := mimoVoice(cfg.OpenAIVoice, lang)

	// user 消息仅在需要调语速时携带指令 (官方: 自然语言指令放在 user 消息, 不会被朗读)
	messages := []map[string]string{
		{"role": "assistant", "content": text},
	}
	if rate != 1.0 {
		messages = append([]map[string]string{
			{"role": "user", "content": fmt.Sprintf("语速调整为正常语速的 %.2f 倍。", rate)},
		}, messages...)
	}

	payload, err := json.Marshal(map[string]any{
		"model":    modelCode,
		"messages": messages,
		"stream":   false,
		"audio": map[string]any{
			"format": "mp3",
			"voice":  voice,
		},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mimoChatEndpoint(baseURL), strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("api-key", apiKey) // 小米文档首选的鉴权头, 与 Bearer 同时带上

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("小米 TTS 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, ttsMaxAudioBytes))
	if err != nil {
		return nil, fmt.Errorf("读取小米 TTS 响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("小米 TTS 返回 %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		Error *struct {
			Message string `json:"message"`
			Code    any    `json:"code"`
		} `json:"error"`
		Choices []struct {
			Message struct {
				Audio struct {
					Data string `json:"data"`
				} `json:"audio"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析小米 TTS 响应失败: %w", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("小米 TTS 错误: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 || result.Choices[0].Message.Audio.Data == "" {
		return nil, errors.New("小米 TTS 未返回音频数据")
	}
	audio, err := base64.StdEncoding.DecodeString(result.Choices[0].Message.Audio.Data)
	if err != nil {
		return nil, fmt.Errorf("解码小米 TTS 音频失败: %w", err)
	}
	return audio, nil
}

// synthEdge 走 Edge 浏览器朗读同款 wss 协议 (免费在线神经音源)。
// 鉴权: TrustedClientToken + Sec-MS-GEC v2 token (见 tts_edge_drm.go) + MUID Cookie;
// 403 时按响应 Date 头校正时钟偏移后重试一次。
func (s *TtsService) synthEdge(ctx context.Context, voice string, rate float64, text string) ([]byte, error) {
	audio, err := s.synthEdgeOnce(ctx, voice, rate, text)
	if err == nil {
		return audio, nil
	}
	if !strings.Contains(err.Error(), "HTTP 403") {
		return nil, err
	}
	// 时钟偏移窗口边界: 校正 skew 后重试一次
	audio, err = s.synthEdgeOnce(ctx, voice, rate, text)
	if err != nil {
		return nil, err
	}
	return audio, nil
}

// synthEdgeOnce 单次 Edge 合成: 握手 → speech.config → SSML → 收帧到 turn.end
func (s *TtsService) synthEdgeOnce(ctx context.Context, voice string, rate float64, text string) ([]byte, error) {
	u := fmt.Sprintf(
		"wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1?TrustedClientToken=%s&ConnectionId=%s&Sec-MS-GEC=%s&Sec-MS-GEC-Version=%s",
		edgeTrustedClientToken, randomHex(16), edgeGenerateSecMSGEC(), edgeSecMSGECVersion,
	)

	header := http.Header{}
	header.Set("Pragma", "no-cache")
	header.Set("Cache-Control", "no-cache")
	header.Set("Origin", "chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold")
	header.Set("User-Agent", edgeUserAgent)
	header.Set("Accept-Language", "en-US,en;q=0.9")
	header.Set("Cookie", "muid="+edgeGenerateMUID()+";")

	dialer := &websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, resp, err := dialer.DialContext(ctx, u, header)
	if err != nil {
		if resp != nil {
			// 403 多为本机时钟与服务器有偏差, 记下偏移供重试
			if resp.StatusCode == http.StatusForbidden {
				edgeAdjustClockSkew(resp)
			}
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			return nil, fmt.Errorf("Edge TTS 连接失败: %w (HTTP %d: %s)", err, resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return nil, fmt.Errorf("Edge TTS 连接失败: %w", err)
	}
	defer func() { _ = conn.Close() }()

	ts := time.Now().UTC().Format("Mon Jan 02 2006 15:04:05 GMT+0000 (Coordinated Universal Time)")

	// 1. speech.config: 输出 24khz 48kbps mp3, 不要边界元数据
	cfgMsg := "X-Timestamp:" + ts + "\r\n" +
		"Content-Type:application/json; charset=utf-8\r\n" +
		"Path:speech.config\r\n\r\n" +
		`{"context":{"synthesis":{"audio":{"metadataoptions":{"sentenceBoundaryEnabled":"false","wordBoundaryEnabled":"false"},"outputFormat":"audio-24khz-48kbitrate-mono-mp3"}}}}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(cfgMsg)); err != nil {
		return nil, fmt.Errorf("Edge TTS 发送配置失败: %w", err)
	}

	// 2. SSML 合成请求 (rate 1.0 -> +0%)
	ssml := fmt.Sprintf(
		"<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='en-US'><voice name='%s'><prosody rate='%s' pitch='+0Hz' volume='+0%%'>%s</prosody></voice></speak>",
		escapeXML(voice), edgeRatePercent(rate), escapeXML(text),
	)
	ssmlMsg := "X-RequestId:" + randomHex(16) + "\r\n" +
		"Content-Type:application/ssml+xml\r\n" +
		"X-Timestamp:" + ts + "Z\r\n" +
		"Path:ssml\r\n\r\n" + ssml
	if err := conn.WriteMessage(websocket.TextMessage, []byte(ssmlMsg)); err != nil {
		return nil, fmt.Errorf("Edge TTS 发送 SSML 失败: %w", err)
	}

	// 3. 收帧直到 turn.end; 二进制帧前 2 字节为头长度, 头之后即 mp3 分片
	var buf []byte
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		mt, data, err := conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("Edge TTS 读取失败: %w", err)
		}
		switch mt {
		case websocket.TextMessage:
			if strings.Contains(string(data), "Path:turn.end") {
				if len(buf) == 0 {
					return nil, errors.New("Edge TTS 未返回音频")
				}
				return buf, nil
			}
		case websocket.BinaryMessage:
			if len(data) <= 2 {
				continue
			}
			headerLen := int(binary.BigEndian.Uint16(data[:2]))
			if len(data) < 2+headerLen {
				continue
			}
			buf = append(buf, data[2+headerLen:]...)
			if len(buf) > ttsMaxAudioBytes {
				return nil, errors.New("Edge TTS 音频超出大小上限")
			}
		}
	}
}

// edgeRatePercent 语速转 SSML 百分比: 1.0 -> "+0%", 1.2 -> "+20%", 0.9 -> "-10%"
func edgeRatePercent(rate float64) string {
	pct := int(math.Round((rate - 1.0) * 100))
	if pct > 100 {
		pct = 100
	}
	if pct < -50 {
		pct = -50
	}
	return fmt.Sprintf("%+d%%", pct)
}

func escapeXML(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '\'':
			b.WriteString("&apos;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := cryptoRandRead(b); err != nil {
		now := time.Now().UnixNano()
		for i := range b {
			b[i] = byte(now >> (uint(i%8) * 8))
		}
	}
	return hex.EncodeToString(b)
}

// cryptoRandRead 独立变量便于 randomHex 在无 crypto 场景兜底
var cryptoRandRead = rand.Read
