package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"backend/service"

	"github.com/gin-gonic/gin"
)

// TtsHandler 语音合成接口: 前端按 /api/tts/config 的 mode 分流,
// openai/edge 模式经 /api/tts 取 mp3 (带磁盘缓存), frontend 模式前端本地朗读
type TtsHandler struct {
	svc *service.TtsService
}

func NewTtsHandler(svc *service.TtsService) *TtsHandler {
	return &TtsHandler{svc: svc}
}

// HandleGetConfig 当前 TTS 模式 (登录用户即可访问; 前端缓存后决定朗读路径)
func (h *TtsHandler) HandleGetConfig(c *gin.Context) {
	SendSuccess(c, gin.H{"mode": h.svc.Mode()})
}

// HandleSynthesize 合成并返回 audio/mpeg。frontend 模式返回 404 code, 前端据此回退本地朗读
func (h *TtsHandler) HandleSynthesize(c *gin.Context) {
	if h.svc.Mode() == service.TtsModeFrontend {
		SendError(c, "404", "TTS mode is frontend")
		return
	}

	text := strings.TrimSpace(c.Query("text"))
	if text == "" {
		SendError(c, "400", "text 不能为空")
		return
	}
	lang := c.Query("lang")
	if lang == "" {
		lang = "en-US"
	}
	rate := 1.0
	if v := c.Query("rate"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			rate = f
		}
	}

	audio, err := h.svc.Synthesize(c.Request.Context(), text, lang, rate)
	if err != nil {
		SendError(c, "500", "语音合成失败: "+err.Error())
		return
	}

	c.Header("Cache-Control", "private, max-age=86400")
	c.Data(http.StatusOK, "audio/mpeg", audio)
}

// HandleTestTts 管理端测试: 用配置页当前表单值 (未保存也可测) 合成一段样例文本
func (h *TtsHandler) HandleTestTts(c *gin.Context) {
	var req struct {
		Mode    string `json:"mode"`
		BaseURL string `json:"base_url"`
		APIKey  string `json:"api_key"`
		Model   string `json:"model"`
		Voice   string `json:"voice"`
		VoiceEN string `json:"voice_en"`
		VoiceZH string `json:"voice_zh"`
		Lang    string `json:"lang"`
		Text    string `json:"text"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	cfg := h.svc.TestConfig(
		strings.ToLower(strings.TrimSpace(req.Mode)),
		strings.TrimSpace(req.BaseURL),
		strings.TrimSpace(req.APIKey),
		strings.TrimSpace(req.Model),
		strings.TrimSpace(req.Voice),
		strings.TrimSpace(req.VoiceEN),
		strings.TrimSpace(req.VoiceZH),
	)

	text := strings.TrimSpace(req.Text)
	if text == "" {
		if strings.HasPrefix(strings.ToLower(cfg.EdgeVoiceForLang(req.Lang)), "zh") {
			text = "你好，语音朗读已经配置成功。"
		} else {
			text = "Hello, text to speech is ready."
		}
	}
	lang := req.Lang
	if lang == "" {
		lang = "en-US"
	}

	start := time.Now()
	audio, err := h.svc.SynthesizeWithConfig(c.Request.Context(), cfg, text, lang, 1.0)
	if err != nil {
		SendError(c, "500", "TTS 测试失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{
		"message":    fmt.Sprintf("合成成功: %d KB, 耗时 %d ms", len(audio)/1024, time.Since(start).Milliseconds()),
		"bytes":      len(audio),
		"elapsed_ms": time.Since(start).Milliseconds(),
	})
}
