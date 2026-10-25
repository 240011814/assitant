package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
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

	"github.com/gorilla/websocket"
)

// TTS 语音合成: 前端朗读的统一后端音源。
// 模式存 system_config tts_mode: frontend(浏览器本地 Web Speech) / openai(OpenAI 兼容 /audio/speech) /
// edge(微软 Edge 在线朗读, 免费, 走 wss 朗读协议)。openai/edge 模式下前端经 /api/tts 取 mp3 播放,
// 按内容 hash 落盘缓存 (单词/例句重复播放零成本)。
const (
	TtsModeFrontend = "frontend"
	TtsModeOpenAI   = "openai"
	TtsModeEdge     = "edge"

	ttsDefaultModel       = "tts-1"
	ttsDefaultOpenAIVoice = "alloy"
	ttsDefaultEdgeVoiceEN = "en-US-AriaNeural"
	ttsDefaultEdgeVoiceZH = "zh-CN-XiaoxiaoNeural"
	ttsDefaultCacheDir    = "tts-cache"

	ttsSynthTimeout  = 60 * time.Second
	ttsMaxTextRunes  = 4000 // 单次合成文本上限 (rune), 与 OpenAI 4096 限制留余量
	ttsMaxAudioBytes = 32 << 20
)

// ttsConfig TTS 运行配置快照 (mu 保护, 读时取快照)
type ttsConfig struct {
	Mode         string // frontend / openai / edge
	OpenAIBaseURL string
	OpenAIKey     string
	OpenAIModel   string
	OpenAIVoice   string
	EdgeVoiceEN   string
	EdgeVoiceZH   string
	CacheDir      string
}

// TtsService 语音合成门面: 配置热刷新 (tts_* 键) + OpenAI/Edge 合成 + 磁盘缓存
type TtsService struct {
	configSvc  *SystemConfigService
	httpClient *http.Client
	mu         sync.RWMutex
	cfg        ttsConfig
	flightMu   sync.Mutex
	flights    map[string]chan struct{} // 进行中的合成请求去重 (并发同文本只合成一次)
}

// NewTtsService 构造并加载一次配置 (system_config 无 tts_* 键时为默认值)
func NewTtsService(configSvc *SystemConfigService) *TtsService {
	s := &TtsService{
		configSvc:  configSvc,
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

	mode := strings.ToLower(getVal("tts_mode"))
	switch mode {
	case TtsModeOpenAI, TtsModeEdge, TtsModeFrontend:
	default:
		mode = TtsModeFrontend
	}

	cfg := ttsConfig{
		Mode:          mode,
		OpenAIBaseURL: getVal("tts_openai_base_url"),
		OpenAIKey:     getVal("tts_openai_api_key"),
		OpenAIModel:   getVal("tts_openai_model"),
		OpenAIVoice:   getVal("tts_openai_voice"),
		EdgeVoiceEN:   getVal("tts_edge_voice_en"),
		EdgeVoiceZH:   getVal("tts_edge_voice_zh"),
		CacheDir:      getVal("tts_cache_dir"),
	}
	if cfg.OpenAIModel == "" {
		cfg.OpenAIModel = ttsDefaultModel
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

	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
}

func (s *TtsService) snapshot() ttsConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Mode 当前生效模式 (frontend 时前端走浏览器本地朗读, /api/tts 返回 404)
func (s *TtsService) Mode() string { return s.snapshot().Mode }

// SynthesizeWithConfig 用指定配置快照合成 (管理端测试接口: 表单值未保存也可验证)
func (s *TtsService) SynthesizeWithConfig(ctx context.Context, cfg ttsConfig, text, lang string, rate float64) ([]byte, error) {
	return s.synthesize(ctx, cfg, text, lang, rate)
}

// TestConfig 用配置页表单值构建测试配置 (空字段回落默认值), 不影响运行配置
func (s *TtsService) TestConfig(mode, baseURL, apiKey, model, voice, voiceEN, voiceZH string) ttsConfig {
	if mode != TtsModeOpenAI && mode != TtsModeEdge {
		mode = TtsModeOpenAI
	}
	cfg := s.snapshot()
	cfg.Mode = mode
	cfg.OpenAIBaseURL = baseURL
	cfg.OpenAIKey = apiKey
	if model != "" {
		cfg.OpenAIModel = model
	}
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

// EdgeVoiceForLang edgeVoiceFor 的导出版 (api 层测试接口选默认样例文本用)
func (c ttsConfig) EdgeVoiceForLang(lang string) string { return c.edgeVoiceFor(lang) }

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

	switch cfg.Mode {
	case TtsModeOpenAI:
		if cfg.OpenAIBaseURL == "" || cfg.OpenAIKey == "" {
			return nil, errors.New("OpenAI TTS 未配置 base_url / api_key")
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
		audio, err = s.synthOpenAI(ctx, cfg, text, rate)
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

// cacheKey 内容寻址: 模式+音色+语速+语言+文本 的 sha256 (配置变化自动失效)
func cacheKey(cfg ttsConfig, text, lang string, rate float64) string {
	voice := cfg.OpenAIVoice
	if cfg.Mode == TtsModeEdge {
		voice = cfg.edgeVoiceFor(lang)
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%.2f|%s", cfg.Mode, voice, lang, rate, text)))
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
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, s.cachePath(cfg, key))
}

// synthOpenAI 调 OpenAI 兼容 /audio/speech (tts-1 等标准接口; 本地兼容服务亦可)
func (s *TtsService) synthOpenAI(ctx context.Context, cfg ttsConfig, text string, rate float64) ([]byte, error) {
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
	url := strings.TrimRight(cfg.OpenAIBaseURL, "/") + "/audio/speech"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.OpenAIKey)

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
	defer conn.Close()

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
