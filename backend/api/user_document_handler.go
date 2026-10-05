package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"backend/service"
)

// UserDocumentHandler 用户文档管理: 上传/列表/删除/预签名下载/文本预览。
// 存储未配置时服务层返回明确错误
type UserDocumentHandler struct {
	docSvc *service.UserDocumentService
}

func NewUserDocumentHandler(docSvc *service.UserDocumentService) *UserDocumentHandler {
	return &UserDocumentHandler{docSvc: docSvc}
}

// HandleStatus 文档功能状态: 存储是否启用 / 检索是否就绪 / 上传上限。
// 仅要求登录 (与 mem0 status 同款), 前端据此展示"未开启"引导
func (h *UserDocumentHandler) HandleStatus(c *gin.Context) {
	SendSuccess(c, h.docSvc.Status())
}

// HandleUpload multipart 上传 (form 字段名 file)
func (h *UserDocumentHandler) HandleUpload(c *gin.Context) {
	userID := GetUserID(c)
	// 大小上限在读取前拦: 超限时 multipart 解析直接报错, 不落盘不落对象存储
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.docSvc.MaxUploadBytes()+4*1024)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		SendError(c, "400", "获取上传文件失败 (超过大小上限或缺少 file 字段): "+err.Error())
		return
	}
	src, err := fileHeader.Open()
	if err != nil {
		SendError(c, "400", "打开上传文件失败: "+err.Error())
		return
	}
	defer src.Close()

	doc, err := h.docSvc.Upload(userID, fileHeader.Filename, src, fileHeader.Size)
	if err != nil {
		SendError(c, "500", "上传失败: "+err.Error())
		return
	}
	SendSuccess(c, doc)
}

// HandleList 分页列表 (page/page_size)
func (h *UserDocumentHandler) HandleList(c *gin.Context) {
	userID := GetUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	docs, total, err := h.docSvc.List(userID, page, pageSize)
	if err != nil {
		SendError(c, "500", "获取文档列表失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{"items": docs, "total": total})
}

// HandleDelete 删除 (S3 对象 + 解析文本 + 记录)
func (h *UserDocumentHandler) HandleDelete(c *gin.Context) {
	userID := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		SendError(c, "400", "无效的文档 ID")
		return
	}
	if err := h.docSvc.Delete(userID, uint(id)); err != nil {
		SendError(c, "500", "删除失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

// HandleReindex 重建文档向量索引 (RAG 语义检索; 排队后台执行, 结果看列表的索引状态)
func (h *UserDocumentHandler) HandleReindex(c *gin.Context) {
	userID := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		SendError(c, "400", "无效的文档 ID")
		return
	}
	if err := h.docSvc.ReindexDocument(userID, uint(id)); err != nil {
		SendError(c, "500", "重建索引失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

// HandleDownload 返回预签名下载 URL (前端拿 URL 新窗口打开)
func (h *UserDocumentHandler) HandleDownload(c *gin.Context) {
	userID := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		SendError(c, "400", "无效的文档 ID")
		return
	}
	url, err := h.docSvc.DownloadURL(userID, uint(id))
	if err != nil {
		SendError(c, "500", "生成下载链接失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{"url": url, "expires_in": 1800})
}

// HandleText 文本预览 (字符区间, 与 AI 读取工具同一套分页语义)
func (h *UserDocumentHandler) HandleText(c *gin.Context) {
	userID := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		SendError(c, "400", "无效的文档 ID")
		return
	}
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	length, _ := strconv.Atoi(c.DefaultQuery("length", "6000"))
	chunk, total, filename, err := h.docSvc.ReadDocumentChunk(userID, uint(id), offset, length)
	if err != nil {
		SendError(c, "500", "读取文本失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{"chunk": chunk, "total_chars": total, "filename": filename, "offset": offset})
}
