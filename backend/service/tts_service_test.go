package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestClampRate(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{0, 1.0}, {0.9, 0.9}, {0.3, 0.5}, {3.0, 2.0}, {1.0, 1.0},
	}
	for _, c := range cases {
		if got := clampRate(c.in); got != c.want {
			t.Errorf("clampRate(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestEdgeRatePercent(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{1.0, "+0%"}, {1.2, "+20%"}, {0.9, "-10%"}, {5.0, "+100%"}, {0.1, "-50%"},
	}
	for _, c := range cases {
		if got := edgeRatePercent(c.in); got != c.want {
			t.Errorf("edgeRatePercent(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestEscapeXML(t *testing.T) {
	got := escapeXML(`a&b<c>"d" 'e'`)
	want := "a&amp;b&lt;c&gt;&quot;d&quot; &apos;e&apos;"
	if got != want {
		t.Errorf("escapeXML = %q, want %q", got, want)
	}
}

// TestEdgeSecMSGECV2 v2 token: 格式 <16hex>-<64hex>, 同一 5 分钟窗口内稳定,
// 变体 SHA-256 应区别于标准 SHA-256
func TestEdgeSecMSGECV2(t *testing.T) {
	a := edgeGenerateSecMSGEC()
	b := edgeGenerateSecMSGEC()
	if a != b {
		t.Errorf("同一窗口内 token 应一致:\n%s\n%s", a, b)
	}
	parts := strings.SplitN(a, "-", 2)
	if len(parts) != 2 || len(parts[0]) != 16 || len(parts[1]) != 64 {
		t.Fatalf("token 应为 <16hex>-<64hex>, got %q", a)
	}
	if parts[0] != strings.ToUpper(parts[0]) || parts[1] != strings.ToUpper(parts[1]) {
		t.Error("token 应为大写十六进制")
	}

	input := []byte("1334066400000000006A5AA1D4EAFF4E9FB37E23D68491D6F4")
	variant := edgeSHA256Digest(input)
	standard := sha256.Sum256(input)
	if variant == standard {
		t.Fatal("变体 SHA-256 不应与标准 SHA-256 结果相同")
	}
	// 同输入结果稳定
	if edgeSHA256Digest(input) != variant {
		t.Fatal("同输入变体摘要应稳定")
	}
}

// TestEdgeSHA256DigestShape 变体摘要跨块长度 (触发多块填充路径) 不 panic 且长度恒 32
func TestEdgeSHA256DigestShape(t *testing.T) {
	salt := []byte("1234567")
	for _, n := range []int{0, 1, 50, 56, 57, 63, 64, 120, 200} {
		data := append(append([]byte{}, salt...), make([]byte, n)...)
		for i := range data[7:] {
			data[7+i] = byte(i)
		}
		if len(edgeSHA256Digest(data)) != 32 {
			t.Fatalf("长度 %d 输入摘要长度异常", len(data))
		}
	}
}

// TestEdgeDRMReferenceVector 对照 Python edge-tts 7.3.1 的固定向量
// (ticks=134144736000000000, 即 unix 1770000100 5 分钟窗口)
func TestEdgeDRMReferenceVector(t *testing.T) {
	strToHash := "1341447360000000006A5AA1D4EAFF4E9FB37E23D68491D6F4"
	digest := edgeSHA256Digest([]byte(strToHash))
	cs64A, cs64B := edgeCS64Digest([]byte(strToHash), digest[:])
	var cs64 [8]byte
	binary.LittleEndian.PutUint32(cs64[:4], cs64A)
	binary.LittleEndian.PutUint32(cs64[4:], cs64B)
	t.Logf("cs64   : %X", cs64[:])
	t.Logf("sha256 : %X", digest[:])
	wantSHA := "87C1FE6B212DC02BD1C09F05EE72850D2069A3CC92C5F462FDD6DDE2B15E8B08"
	if got := fmt.Sprintf("%X", digest[:]); got != wantSHA {
		t.Errorf("sha256 不匹配:\n got %s\nwant %s", got, wantSHA)
	}
	wantCS64 := "00DDB33FD01602D8"
	if got := fmt.Sprintf("%X", cs64[:]); got != wantCS64 {
		t.Errorf("cs64 不匹配:\n got %s\nwant %s", got, wantCS64)
	}
}

// TestBuildTtsConfig 配置解析: 模式回落 / 默认值 / frontend 音色
func TestBuildTtsConfig(t *testing.T) {
	cfg := buildTtsConfig(func(string) string { return "" })
	if cfg.Mode != TtsModeFrontend {
		t.Errorf("空配置模式应回落 frontend, got %q", cfg.Mode)
	}
	if cfg.OpenAIModel != ttsDefaultModel || cfg.OpenAIVoice != ttsDefaultOpenAIVoice {
		t.Errorf("默认模型/音色不对: %q %q", cfg.OpenAIModel, cfg.OpenAIVoice)
	}
	if cfg.EdgeVoiceEN != ttsDefaultEdgeVoiceEN || cfg.EdgeVoiceZH != ttsDefaultEdgeVoiceZH {
		t.Errorf("默认 Edge 音色不对: %q %q", cfg.EdgeVoiceEN, cfg.EdgeVoiceZH)
	}
	if cfg.FrontendVoice != "" {
		t.Errorf("frontend 音色默认应为空, got %q", cfg.FrontendVoice)
	}

	vals := map[string]string{
		"tts_mode":           "EDGE",
		"tts_frontend_voice": "Microsoft Aria Online (Natural)",
	}
	cfg = buildTtsConfig(func(key string) string { return vals[key] })
	if cfg.Mode != TtsModeEdge {
		t.Errorf("模式大小写应归一, got %q", cfg.Mode)
	}
	if cfg.FrontendVoice != vals["tts_frontend_voice"] {
		t.Errorf("frontend 音色应透传, got %q", cfg.FrontendVoice)
	}

	vals["tts_mode"] = "invalid"
	if cfg := buildTtsConfig(func(key string) string { return vals[key] }); cfg.Mode != TtsModeFrontend {
		t.Errorf("非法模式应回落 frontend, got %q", cfg.Mode)
	}
}

func TestTtsCacheRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s := &TtsService{flights: map[string]chan struct{}{}}
	cfg := ttsConfig{Mode: TtsModeEdge, EdgeVoiceEN: "en-US-AriaNeural", EdgeVoiceZH: "zh-CN-XiaoxiaoNeural", CacheDir: dir}
	key := cacheKey(cfg, "hello", "en-US", 1.0)

	if _, err := s.readCache(cfg, key); err == nil {
		t.Fatal("空缓存应 miss")
	}
	if err := s.writeCache(cfg, key, []byte("fake-mp3")); err != nil {
		t.Fatalf("写缓存失败: %v", err)
	}
	got, err := s.readCache(cfg, key)
	if err != nil || string(got) != "fake-mp3" {
		t.Fatalf("读缓存失败: %v %q", err, got)
	}
	// 内容变化 key 应变化
	if cacheKey(cfg, "hello!", "en-US", 1.0) == key {
		t.Error("文本变化缓存 key 应变化")
	}
	if cacheKey(cfg, "hello", "en-US", 1.1) == key {
		t.Error("语速变化缓存 key 应变化")
	}
}

func TestSynthesizeModeGuard(t *testing.T) {
	s := &TtsService{flights: map[string]chan struct{}{}}
	cfg := ttsConfig{Mode: TtsModeFrontend, CacheDir: t.TempDir()}
	if _, err := s.synthesize(context.Background(), cfg, "hi", "en-US", 1.0); err == nil {
		t.Fatal("frontend 模式应返回错误")
	}

	cfg.Mode = TtsModeOpenAI // aiAgent 未注入 (nil)
	if _, err := s.synthesize(context.Background(), cfg, "hi", "en-US", 1.0); err == nil || !strings.Contains(err.Error(), "Provider") {
		t.Fatalf("OpenAI 缺 Provider 应报未配置, got %v", err)
	}

	// 空文本 / 超长文本
	cfg.Mode = TtsModeEdge
	cfg.EdgeVoiceEN = "en-US-AriaNeural"
	if _, err := s.synthesize(context.Background(), cfg, "  ", "en-US", 1.0); err == nil {
		t.Fatal("空文本应报错")
	}
	long := strings.Repeat("a", ttsMaxTextRunes+1)
	if _, err := s.synthesize(context.Background(), cfg, long, "en-US", 1.0); err == nil {
		t.Fatal("超长文本应报错")
	}
}

func TestEdgeVoiceForLang(t *testing.T) {
	cfg := ttsConfig{EdgeVoiceEN: "en-US-AriaNeural", EdgeVoiceZH: "zh-CN-XiaoxiaoNeural"}
	for _, lang := range []string{"zh-CN", "zh_TW", "ZH"} {
		if got := cfg.edgeVoiceFor(lang); got != cfg.EdgeVoiceZH {
			t.Errorf("edgeVoiceFor(%s) = %s, want 中文音色", lang, got)
		}
	}
	for _, lang := range []string{"en-US", "ja-JP", ""} {
		if got := cfg.edgeVoiceFor(lang); got != cfg.EdgeVoiceEN {
			t.Errorf("edgeVoiceFor(%s) = %s, want 英文音色", lang, got)
		}
	}
}

// TestEdgeSynthLive 真连 Edge 朗读服务, 验证库连通性 (鉴权/协议由依赖库处理)。
// 默认跳过, 需要时 TTS_LIVE=1 go test -run TestEdgeSynthLive
func TestEdgeSynthLive(t *testing.T) {
	if os.Getenv("TTS_LIVE") == "" {
		t.Skip("set TTS_LIVE=1 to run live edge tts test")
	}
	s := &TtsService{}
	audio, err := s.synthEdge(context.Background(), "en-US-AriaNeural", 1.0, "Hello, this is a live test of the speech service.")
	if err != nil {
		t.Fatalf("Edge 合成失败: %v", err)
	}
	if len(audio) < 1000 {
		t.Fatalf("音频过小 (%d bytes), 疑似异常", len(audio))
	}
	// mp3 流应有 ID3 头或 MPEG 帧同步字
	hasID3 := audio[0] == 'I' && audio[1] == 'D' && audio[2] == '3'
	hasSync := audio[0] == 0xFF && audio[1]&0xE0 == 0xE0
	if !hasID3 && !hasSync {
		t.Fatalf("返回内容不像 mp3, 前 4 字节: % x", audio[:4])
	}
	t.Logf("Edge live 合成成功: %d bytes", len(audio))
}

func TestMain(m *testing.M) { os.Exit(m.Run()) }
