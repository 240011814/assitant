package api

import (
	"net/http"
	"strconv"
	"strings"

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

// HandleGetConfig 当前 TTS 模式与浏览器本地首选音源 (登录用户即可访问; 前端缓存后决定朗读路径)
func (h *TtsHandler) HandleGetConfig(c *gin.Context) {
	SendSuccess(c, gin.H{"mode": h.svc.Mode(), "frontend_voice": h.svc.FrontendVoice()})
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

// HandleTestTts 管理端播放测试: 用配置页当前表单值 (未保存也可测) 合成一段样例,
// 成功直接返回 audio/mpeg 供前端播放, 失败返回统一 JSON 错误体
func (h *TtsHandler) HandleTestTts(c *gin.Context) {
	var req struct {
		Mode    string `json:"mode"`
		Voice   string `json:"voice"`
		VoiceEN string `json:"voice_en"`
		VoiceZH string `json:"voice_zh"`
		Lang    string `json:"lang"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	lang := req.Lang
	if lang == "" {
		lang = "en-US"
	}
	cfg := h.svc.TestConfig(
		strings.ToLower(strings.TrimSpace(req.Mode)),
		strings.TrimSpace(req.Voice),
		strings.TrimSpace(req.VoiceEN),
		strings.TrimSpace(req.VoiceZH),
	)

	// 样例文本跟随测试语言, 听感贴近真实使用
	text := "Hello, this is the text to speech test."
	if strings.HasPrefix(strings.ToLower(strings.ReplaceAll(lang, "_", "-")), "zh") {
		text = "你好，这是一段语音朗读测试。"
	}

	audio, err := h.svc.SynthesizeWithConfig(c.Request.Context(), cfg, text, lang, 1.0)
	if err != nil {
		SendError(c, "500", "TTS 测试失败: "+err.Error())
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "audio/mpeg", audio)
}
