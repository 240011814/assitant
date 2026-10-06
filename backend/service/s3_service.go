package service

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Storage S3 兼容对象存储客户端 (MinIO/阿里云 OSS/腾讯 COS 等)。
// 配置存 system_config (s3_* 键, SMTP 同款), 由 UserDocumentService.RefreshStorageConfig
// 构建与热刷新; 未启用或构建失败时 s3Storage 为 nil, 用户文档功能随之关闭
var (
	s3Mu      sync.RWMutex
	s3Storage *S3Storage
)

// S3StorageConfig 用户文档存储配置 (来自 system_config 的 s3_* 键)
type S3StorageConfig struct {
	Enabled      bool
	Endpoint     string
	Region       string
	Bucket       string
	AccessKey    string
	SecretKey    string
	Secure       bool
	UsePathStyle bool
}

// S3Storage 封装 minio 客户端 (自身并发安全)
type S3Storage struct {
	client *minio.Client
	bucket string
}

// GetS3 返回当前 S3 存储实例 (未启用/未配置时为 nil)
func GetS3() *S3Storage {
	s3Mu.RLock()
	defer s3Mu.RUnlock()
	return s3Storage
}

// rebuildS3Storage 用给定配置重建 S3 客户端并确保 bucket 存在。
// 构建失败时不改动现有实例 (旧配置继续服务), 错误由调用方记录
func rebuildS3Storage(cfg S3StorageConfig) error {
	if !cfg.Enabled {
		s3Mu.Lock()
		s3Storage = nil
		s3Mu.Unlock()
		log.Println("[S3] 未启用(s3_enabled=false), 用户文档功能关闭")
		return nil
	}
	if cfg.Endpoint == "" || cfg.Bucket == "" || cfg.AccessKey == "" || cfg.SecretKey == "" {
		return fmt.Errorf("S3 已启用但 endpoint/bucket/access_key/secret_key 存在空项")
	}

	lookup := minio.BucketLookupAuto
	if cfg.UsePathStyle {
		lookup = minio.BucketLookupPath
	}
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:       cfg.Secure,
		Region:       cfg.Region,
		BucketLookup: lookup,
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return fmt.Errorf("检查 bucket 失败: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
			return fmt.Errorf("bucket 不存在且自动创建失败: %w", err)
		}
		log.Printf("[S3] bucket %s 不存在, 已自动创建", cfg.Bucket)
	}

	s3Mu.Lock()
	s3Storage = &S3Storage{client: client, bucket: cfg.Bucket}
	s3Mu.Unlock()
	log.Printf("[S3] 已连接 %s (bucket=%s, pathStyle=%v)", cfg.Endpoint, cfg.Bucket, cfg.UsePathStyle)
	return nil
}

// PutObject 上传对象
func (s *S3Storage) PutObject(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, reader, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

// GetObject 读取对象全部内容 (调用方保证对象可信, 文档解析文本上限由写入侧控制)
func (s *S3Storage) GetObject(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = obj.Close() }()
	return io.ReadAll(obj)
}

// PresignGet 生成预签名下载 URL
func (s *S3Storage) PresignGet(ctx context.Context, key string, expiry time.Duration) (string, error) {
	u, err := s.client.PresignedGetObject(ctx, s.bucket, key, expiry, nil)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// RemoveObjects 批量删除对象 (单个失败即返回错误)
func (s *S3Storage) RemoveObjects(ctx context.Context, keys ...string) error {
	for _, key := range keys {
		if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
			return fmt.Errorf("删除对象 %s 失败: %w", key, err)
		}
	}
	return nil
}
