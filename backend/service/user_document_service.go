package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	einoDocx "github.com/cloudwego/eino-ext/components/document/parser/docx"
	einoHTML "github.com/cloudwego/eino-ext/components/document/parser/html"
	einoPDF "github.com/cloudwego/eino-ext/components/document/parser/pdf"
	einoXlsx "github.com/cloudwego/eino-ext/components/document/parser/xlsx"
	"github.com/cloudwego/eino/components/document/parser"
	"github.com/google/uuid"

	interfaces "backend/interface"
	"backend/model"
)

// 用户文档配置默认值
const (
	// docDefaultChunk 文档读取工具默认单次返回字符数
	docDefaultChunk = 6000
	// docMaxChunk 单次读取上限 (防止模型一次拉爆上下文)
	docMaxChunk = 20000
	// docParseTimeout 单文件解析超时 (大 xlsx 转换可能偏慢)
	docParseTimeout = 60 * time.Second
	// docPresignExpiry 预签名下载链接有效期
	docPresignExpiry = 30 * time.Minute
)

// UserDocumentService 用户文档管理: 文件存 S3 兼容对象存储, 上传后同步解析为纯文本
// (同样存对象存储), AI 经 list_user_documents / read_document 工具按需分页读取;
// 解析成功且 RAG 就绪时异步建立向量索引 (ClickHouse), 支持语义检索。
// 存储配置存 system_config (s3_*/rag_* 键, SMTP 同款), 改后热刷新无需重启
type UserDocumentService struct {
	configSvc      *SystemConfigService
	rag            *RagService
	mu             sync.RWMutex // 保护 maxUploadBytes
	maxUploadBytes int64
	parseMaxChars  int
	parsers        map[string]parser.Parser
}

// ragReady 文档检索就绪 = RAG 配置开启 + 向量存储可用 (嵌入配置完整性由 RagService.ready 校验)
func (s *UserDocumentService) ragReady() bool {
	return s.rag != nil && s.rag.Enabled() && s.rag.Available()
}

// DocumentStatus 文档功能状态 (供前端展示"未开启"引导)
type DocumentStatus struct {
	// Enabled S3 存储已配置且可用, 文档上传/管理可用
	Enabled bool `json:"enabled"`
	// RAGReady 语义检索就绪 (RAG 配置开启 + ClickHouse 可用)
	RAGReady bool `json:"rag_ready"`
	// MaxUploadMB 单文件上传上限 MB
	MaxUploadMB int64 `json:"max_upload_mb"`
	// LastError 存储未就绪时的失败原因 (未启用/连接失败), 便于前端与管理员定位
	LastError string `json:"last_error"`
}

// Status 返回文档功能当前状态 (前端文档页判断展示"未开启"引导)
func (s *UserDocumentService) Status() DocumentStatus {
	lastErr := ""
	if GetS3() == nil {
		lastErr = GetS3LastError()
		if lastErr == "" {
			lastErr = "未启用或未配置 S3 存储 (系统配置 → 用户文档存储)"
		}
	}
	return DocumentStatus{
		Enabled:     GetS3() != nil,
		RAGReady:    s.ragReady(),
		MaxUploadMB: s.MaxUploadBytes() / 1024 / 1024,
		LastError:   lastErr,
	}
}

// NewUserDocumentService configSvc 为空时无法读取存储配置, 文档功能保持关闭态;
// rag 允许为 nil (仅关闭向量索引/检索)
func NewUserDocumentService(configSvc *SystemConfigService, rag *RagService) *UserDocumentService {
	s := &UserDocumentService{
		configSvc:      configSvc,
		rag:            rag,
		maxUploadBytes: 20 * 1024 * 1024,
		parseMaxChars:  200000,
		parsers:        map[string]parser.Parser{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// 各格式解析器构建失败只跳过该格式 (上传仍可用, 解析报失败), 不阻断启动
	if p, err := einoPDF.NewPDFParser(ctx, &einoPDF.Config{}); err != nil {
		log.Printf("[UserDocument] PDF 解析器构建失败: %v", err)
	} else {
		s.parsers[".pdf"] = p
	}
	if p, err := einoXlsx.NewXlsxParser(ctx, &einoXlsx.Config{}); err != nil {
		log.Printf("[UserDocument] XLSX 解析器构建失败: %v", err)
	} else {
		s.parsers[".xlsx"] = p
	}
	if p, err := einoDocx.NewDocxParser(ctx, &einoDocx.Config{}); err != nil {
		log.Printf("[UserDocument] DOCX 解析器构建失败: %v", err)
	} else {
		s.parsers[".docx"] = p
	}
	if p, err := einoHTML.NewParser(ctx, &einoHTML.Config{}); err != nil {
		log.Printf("[UserDocument] HTML 解析器构建失败: %v", err)
	} else {
		s.parsers[".html"] = p
		s.parsers[".htm"] = p
	}
	// txt/md/csv/json 等纯文本格式用内置 TextParser
	s.parsers[".txt"] = parser.TextParser{}
	s.parsers[".md"] = parser.TextParser{}
	s.parsers[".csv"] = parser.TextParser{}
	s.parsers[".json"] = parser.TextParser{}
	s.RefreshStorageConfig()
	return s
}

// RefreshStorageConfig 从 system_config 读取 s3_* 键并重建对象存储客户端
// (与 SMTP RefreshConfig 同款: 系统配置页保存后热刷新, 无需重启)。
// 构建失败时保留旧连接继续服务, 仅记录日志
func (s *UserDocumentService) RefreshStorageConfig() {
	getVal := func(key string) string {
		v, _ := s.configSvc.GetValue(key)
		return v
	}
	isTrue := func(v string) bool { return v == "1" || strings.EqualFold(v, "true") }

	// endpoint 容错: 去空白/尾部斜杠; 若直接粘贴了带 scheme 的地址, 以 scheme 为准
	endpoint, secure := NormalizeS3Endpoint(getVal("s3_endpoint"), isTrue(getVal("s3_secure")))

	cfg := S3StorageConfig{
		Enabled:      isTrue(getVal("s3_enabled")),
		Endpoint:     endpoint,
		Region:       strings.TrimSpace(getVal("s3_region")),
		Bucket:       strings.TrimSpace(getVal("s3_bucket")),
		AccessKey:    strings.TrimSpace(getVal("s3_access_key")),
		SecretKey:    strings.TrimSpace(getVal("s3_secret_key")),
		Secure:       secure,
		UsePathStyle: isTrue(getVal("s3_use_path_style")),
	}
	if err := rebuildS3Storage(cfg); err != nil {
		log.Printf("[UserDocument] 刷新 S3 配置失败(保留原有连接): %v", err)
	}

	maxMB := 20
	if v := getVal("s3_max_upload_mb"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxMB = n
		}
	}
	s.mu.Lock()
	s.maxUploadBytes = int64(maxMB) * 1024 * 1024
	s.mu.Unlock()
}

// MaxUploadBytes 当前单文件上传上限 (字节)
func (s *UserDocumentService) MaxUploadBytes() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.maxUploadBytes
}

var docExtMime = map[string]string{
	".pdf":  "application/pdf",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".csv":  "text/csv",
	".html": "text/html",
	".htm":  "text/html",
	".md":   "text/markdown",
	".txt":  "text/plain",
	".json": "application/json",
}

// Upload 上传文档: 读入内存(上限校验) -> 存 S3 -> 同步解析 -> 元数据落库。
// 解析失败不影响上传 (ParseStatus=failed), 文件本身仍可下载
func (s *UserDocumentService) Upload(userID uint, filename string, reader io.Reader, declaredSize int64) (*model.UserDocument, error) {
	s3s := GetS3()
	if s3s == nil {
		return nil, errors.New("文档存储未启用 (请在系统配置中开启并填写 S3 存储配置)")
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if _, ok := docExtMime[ext]; !ok {
		return nil, fmt.Errorf("不支持的文件类型 %q (支持: pdf/xlsx/docx/csv/html/htm/md/txt/json)", ext)
	}

	// 读入内存以复用 (上传 + 解析两份消费); 超限拒绝
	data, err := io.ReadAll(io.LimitReader(reader, s.maxUploadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取上传内容失败: %w", err)
	}
	if int64(len(data)) > s.maxUploadBytes {
		return nil, fmt.Errorf("文件超过大小上限 %dMB", s.maxUploadBytes/1024/1024)
	}
	if declaredSize > 0 && declaredSize > s.maxUploadBytes {
		return nil, fmt.Errorf("文件超过大小上限 %dMB", s.maxUploadBytes/1024/1024)
	}
	if len(data) == 0 {
		return nil, errors.New("文件内容为空")
	}

	objectKey := fmt.Sprintf("user-docs/%d/%s%s", userID, uuid.NewString(), ext)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := s3s.PutObject(ctx, objectKey, bytes.NewReader(data), int64(len(data)), docExtMime[ext]); err != nil {
		return nil, fmt.Errorf("上传到对象存储失败: %w", err)
	}

	doc := &model.UserDocument{
		UserID:      userID,
		Filename:    filepath.Base(filename),
		Ext:         ext,
		MimeType:    docExtMime[ext],
		SizeBytes:   int64(len(data)),
		ObjectKey:   objectKey,
		ParseStatus: "none",
	}
	if err := DB.Create(doc).Error; err != nil {
		return nil, err
	}

	// 同步解析 (解析文本同存 S3); 失败只记状态, 不影响已上传文件
	s.parseAndStore(ctx, doc, data)
	if err := DB.Model(&model.UserDocument{}).Where("id = ?", doc.ID).Updates(map[string]any{
		"parse_status": doc.ParseStatus, "parse_error": doc.ParseError,
		"text_key": doc.TextKey, "text_chars": doc.TextChars,
	}).Error; err != nil {
		log.Printf("[UserDocument] 更新解析状态失败 doc=%d err=%v", doc.ID, err)
	}

	// RAG: 解析成功且检索就绪时异步建立向量索引 (嵌入大量分块耗时, 不卡上传请求)
	if doc.ParseStatus == "ok" && s.ragReady() {
		if err := DB.Model(&model.UserDocument{}).Where("id = ?", doc.ID).Update("index_status", "pending").Error; err != nil {
			log.Printf("[RAG] 标记待索引失败 doc=%d: %v", doc.ID, err)
		} else {
			doc.IndexStatus = "pending"
			go s.indexDocument(doc.ID)
		}
	}
	return doc, nil
}

// parseAndStore 解析文件内容并存解析文本 (就地更新 doc 的解析字段)
func (s *UserDocumentService) parseAndStore(ctx context.Context, doc *model.UserDocument, data []byte) {
	p, ok := s.parsers[doc.Ext]
	if !ok {
		doc.ParseStatus = "none"
		doc.ParseError = "不支持的格式"
		return
	}
	parseCtx, cancel := context.WithTimeout(ctx, docParseTimeout)
	defer cancel()
	docs, err := p.Parse(parseCtx, bytes.NewReader(data))
	if err != nil {
		doc.ParseStatus = "failed"
		doc.ParseError = truncateRunes(err.Error(), 500)
		return
	}
	var sb strings.Builder
	for _, d := range docs {
		if d == nil || d.Content == "" {
			continue
		}
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(d.Content)
	}
	text := sb.String()
	if strings.TrimSpace(text) == "" {
		doc.ParseStatus = "failed"
		doc.ParseError = "未解析出文本内容 (可能为扫描件/空文档)"
		return
	}
	runes := []rune(text)
	if len(runes) > s.parseMaxChars {
		text = string(runes[:s.parseMaxChars]) + "\n\n…(内容过长已截断)"
	}
	textKey := doc.ObjectKey + ".txt"
	if err := GetS3().PutObject(ctx, textKey, strings.NewReader(text), int64(len(text)), "text/plain; charset=utf-8"); err != nil {
		doc.ParseStatus = "failed"
		doc.ParseError = "解析文本存储失败: " + truncateRunes(err.Error(), 200)
		return
	}
	doc.ParseStatus = "ok"
	doc.ParseError = ""
	doc.TextKey = textKey
	doc.TextChars = len([]rune(text))
}

// List 分页列出用户文档 (时间倒序)
func (s *UserDocumentService) List(userID uint, page, pageSize int) ([]model.UserDocument, int64, error) {
	page = NormalizePage(page)
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}
	var total int64
	if err := DB.Model(&model.UserDocument{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var docs []model.UserDocument
	err := DB.Where("user_id = ?", userID).Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&docs).Error
	return docs, total, err
}

// Get 获取用户自己的文档元数据 (user_id 双重校验防越权)
func (s *UserDocumentService) Get(userID uint, id uint) (*model.UserDocument, error) {
	var doc model.UserDocument
	if err := DB.Where("id = ? AND user_id = ?", id, userID).First(&doc).Error; err != nil {
		return nil, err
	}
	return &doc, nil
}

// Delete 删除文档: S3 原文件 + 解析文本 + DB 记录
func (s *UserDocumentService) Delete(userID uint, id uint) error {
	doc, err := s.Get(userID, id)
	if err != nil {
		return err
	}
	if s3s := GetS3(); s3s != nil {
		keys := []string{doc.ObjectKey}
		if doc.TextKey != "" {
			keys = append(keys, doc.TextKey)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s3s.RemoveObjects(ctx, keys...); err != nil {
			return err
		}
	}
	// 向量块级联清理 (best effort: 存储不可用只记日志; MySQL 主键不复用, 残留块不会串到新文档,
	// 但在清理前仍可能被语义检索命中, 文档行已删时文件名显示为空)
	if s.rag != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.rag.DeleteDocumentChunks(ctx, userID, id); err != nil {
			log.Printf("[RAG] 清理文档向量块失败 (文档仍会删除) doc=%d: %v", id, err)
		}
	}
	return DB.Delete(&model.UserDocument{}, doc.ID).Error
}

// DownloadURL 生成短期预签名下载链接
func (s *UserDocumentService) DownloadURL(userID uint, id uint) (string, error) {
	doc, err := s.Get(userID, id)
	if err != nil {
		return "", err
	}
	s3s := GetS3()
	if s3s == nil {
		return "", errors.New("文档存储未启用")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return s3s.PresignGet(ctx, doc.ObjectKey, docPresignExpiry)
}

// docChunk 按 rune 区间切片 (rune 级分页, 避免切断多字节字符):
// offset<0 归 0, length<=0 取默认 6000, 超 docMaxChunk 封顶, offset 越界返回空串
func docChunk(text string, offset, length int) (string, int) {
	runes := []rune(text)
	if offset < 0 {
		offset = 0
	}
	if length <= 0 {
		length = docDefaultChunk
	}
	if length > docMaxChunk {
		length = docMaxChunk
	}
	if offset >= len(runes) {
		return "", len(runes)
	}
	end := offset + length
	if end > len(runes) {
		end = len(runes)
	}
	return string(runes[offset:end]), len(runes)
}

// ReadDocumentChunk 按字符区间读取解析文本
func (s *UserDocumentService) ReadDocumentChunk(userID uint, id uint, offset, length int) (string, int, string, error) {
	doc, err := s.Get(userID, id)
	if err != nil {
		return "", 0, "", err
	}
	if doc.ParseStatus != "ok" {
		return "", 0, doc.Filename, fmt.Errorf("文档未成功解析 (状态 %s): %s", doc.ParseStatus, doc.ParseError)
	}
	s3s := GetS3()
	if s3s == nil {
		return "", 0, "", errors.New("文档存储未启用")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	data, err := s3s.GetObject(ctx, doc.TextKey)
	if err != nil {
		return "", 0, "", fmt.Errorf("读取解析文本失败: %w", err)
	}
	chunk, total := docChunk(string(data), offset, length)
	return chunk, total, doc.Filename, nil
}

// ListDocuments / ReadDocumentChunk 实现 interfaces.UserDocumentStore (注入 AI 工具)

// ListDocuments 列出用户文档元数据 (interfaces.UserDocumentStore)
func (s *UserDocumentService) ListDocuments(userID uint) ([]interfaces.DocumentInfo, error) {
	var docs []model.UserDocument
	if err := DB.Where("user_id = ?", userID).Order("id DESC").Limit(100).Find(&docs).Error; err != nil {
		return nil, err
	}
	out := make([]interfaces.DocumentInfo, 0, len(docs))
	for _, d := range docs {
		out = append(out, interfaces.DocumentInfo{
			ID:          d.ID,
			Filename:    d.Filename,
			Ext:         d.Ext,
			SizeBytes:   d.SizeBytes,
			ParseStatus: d.ParseStatus,
			TextChars:   d.TextChars,
			CreatedAt:   d.CreatedAt.Format("2006-01-02 15:04"),
		})
	}
	return out, nil
}

// ============ RAG 向量索引与语义检索 ============

// ReindexDocument 重建文档向量索引 (上传时自动索引失败 / 换嵌入模型后手动触发)。
// 排队后由后台协程执行, 结果写回 index_status
func (s *UserDocumentService) ReindexDocument(userID, id uint) error {
	doc, err := s.Get(userID, id)
	if err != nil {
		return err
	}
	if doc.ParseStatus != "ok" {
		return fmt.Errorf("文档未成功解析 (状态 %s), 无法建立索引", doc.ParseStatus)
	}
	if !s.ragReady() {
		return errors.New("文档检索未启用 (需开启 RAG 配置、配置嵌入模型, 且 ClickHouse 可用)")
	}
	if err := DB.Model(&model.UserDocument{}).Where("id = ?", doc.ID).Updates(map[string]any{
		"index_status": "pending", "index_error": "",
	}).Error; err != nil {
		return err
	}
	go s.indexDocument(doc.ID)
	return nil
}

// indexDocument 后台索引: 拉解析文本 -> 切块 -> 批量嵌入 -> 清旧块 -> 写 ClickHouse。
// 任何一步失败只更新 index_status/index_error, 不影响文档本身
func (s *UserDocumentService) indexDocument(docID uint) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[RAG] 索引协程 panic doc=%d: %v", docID, r)
			DB.Model(&model.UserDocument{}).Where("id = ?", docID).Updates(map[string]any{
				"index_status": "failed", "index_error": truncateRunes(fmt.Sprintf("内部错误: %v", r), 500),
			})
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var doc model.UserDocument
	if err := DB.Where("id = ?", docID).First(&doc).Error; err != nil {
		log.Printf("[RAG] 索引时找不到文档 doc=%d: %v", docID, err)
		return
	}
	fail := func(msg string) {
		log.Printf("[RAG] 文档索引失败 doc=%d(%s): %s", docID, doc.Filename, msg)
		DB.Model(&model.UserDocument{}).Where("id = ?", docID).Updates(map[string]any{
			"index_status": "failed", "index_error": truncateRunes(msg, 500), "chunk_count": 0,
		})
	}
	if err := DB.Model(&model.UserDocument{}).Where("id = ?", docID).Update("index_status", "indexing").Error; err != nil {
		log.Printf("[RAG] 标记索引中失败 doc=%d: %v", docID, err)
		return
	}

	s3s := GetS3()
	if s3s == nil {
		fail("文档存储未启用")
		return
	}
	data, err := s3s.GetObject(ctx, doc.TextKey)
	if err != nil {
		fail("读取解析文本失败: " + err.Error())
		return
	}
	text := string(data)
	if strings.TrimSpace(text) == "" {
		fail("解析文本为空")
		return
	}

	cfg := s.rag.snapshot()
	chunks := ChunkText(text, cfg.ChunkSize, cfg.Overlap)
	if len(chunks) == 0 {
		fail("切块结果为空")
		return
	}
	vecs, err := s.rag.Embed(ctx, chunks)
	if err != nil {
		fail("向量嵌入失败: " + err.Error())
		return
	}

	// 重建语义 = 清旧块再写新块 (删除/写入语义由 VectorStore 实现保证)
	if err := s.rag.DeleteDocumentChunks(ctx, doc.UserID, docID); err != nil {
		fail("清理旧向量块失败: " + err.Error())
		return
	}
	rows := make([]ChunkRow, 0, len(chunks))
	for i := range chunks {
		rows = append(rows, ChunkRow{
			UserID:         doc.UserID,
			DocumentID:     docID,
			ChunkIndex:     i,
			Content:        chunks[i],
			Embedding:      vecs[i],
			EmbeddingModel: cfg.Model,
			Dims:           len(vecs[i]),
		})
	}
	if err := s.rag.InsertChunks(ctx, rows); err != nil {
		fail(err.Error())
		return
	}

	if err := DB.Model(&model.UserDocument{}).Where("id = ?", docID).Updates(map[string]any{
		"index_status": "ok", "index_error": "", "chunk_count": len(chunks),
	}).Error; err != nil {
		log.Printf("[RAG] 索引成功但更新状态失败 doc=%d: %v", docID, err)
		return
	}
	log.Printf("[RAG] 文档索引完成 doc=%d(%s) chunks=%d model=%s", docID, doc.Filename, len(chunks), cfg.Model)
}

// SearchDocuments 在用户文档的向量索引中检索相关片段 (interfaces.DocumentSearcher)。
// 命中后按文档批量补文件名 (文档已删时文件名留空)
func (s *UserDocumentService) SearchDocuments(userID uint, query string, documentIDs []uint, topK int) ([]interfaces.ChunkHit, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("查询内容为空")
	}
	if !s.ragReady() {
		return nil, errors.New("文档检索未启用 (需开启 RAG 配置、配置嵌入模型, 且 ClickHouse 可用)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), ragSearchTimeout+10*time.Second)
	defer cancel()
	raw, err := s.rag.Search(ctx, userID, query, documentIDs, topK)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return []interfaces.ChunkHit{}, nil
	}
	seen := map[uint]bool{}
	ids := make([]uint, 0, len(raw))
	for _, h := range raw {
		if id := uint(h.DocumentID); id > 0 && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	names := map[uint]string{}
	if len(ids) > 0 {
		var docs []model.UserDocument
		if err := DB.Select("id", "filename").Where("user_id = ? AND id IN ?", userID, ids).Find(&docs).Error; err == nil {
			for _, d := range docs {
				names[d.ID] = d.Filename
			}
		}
	}
	out := make([]interfaces.ChunkHit, 0, len(raw))
	for _, h := range raw {
		out = append(out, interfaces.ChunkHit{
			DocumentID: uint(h.DocumentID),
			Filename:   names[uint(h.DocumentID)],
			ChunkIndex: h.ChunkIndex,
			Score:      h.Score,
			Content:    h.Content,
		})
	}
	return out, nil
}
