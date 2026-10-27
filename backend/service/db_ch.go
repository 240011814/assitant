package service

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net"
	"sort"
	"strings"
	"time"

	"backend/config"
	dbmigrations "backend/db"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// CH ClickHouse 连接 (未启用或初始化失败时为 nil, 相关同步直接跳过)
var CH clickhouse.Conn

// InitClickHouse 连接 ClickHouse 并执行迁移; 未启用时返回 nil 且 CH 保持 nil
func InitClickHouse(cfg *config.Config) error {
	if !cfg.ClickHouse.Enabled {
		log.Println("[ClickHouse] 未启用(config.clickhouse.enabled=false), 跳过初始化")
		return nil
	}

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr:            []string{net.JoinHostPort(cfg.ClickHouse.Host, cfg.ClickHouse.Port)},
		Auth:            clickhouse.Auth{Database: cfg.ClickHouse.Database, Username: cfg.ClickHouse.User, Password: cfg.ClickHouse.Password},
		DialTimeout:     5 * time.Second,
		MaxOpenConns:    4,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Hour,
		Compression:     &clickhouse.Compression{Method: clickhouse.CompressionLZ4},
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		return err
	}
	if err := applyChMigrations(ctx, conn); err != nil {
		return err
	}

	CH = conn
	log.Printf("[ClickHouse] 连接成功 %s:%s/%s, 迁移就绪", cfg.ClickHouse.Host, cfg.ClickHouse.Port, cfg.ClickHouse.Database)
	return nil
}

// applyChMigrations 执行 ch_migrations 下未应用的迁移
// 与 MySQL 的 Goose 迁移相互独立(CH 是另一个库); 按文件名排序, 已应用项记录在 CH 的 schema_migrations 表
func applyChMigrations(ctx context.Context, conn clickhouse.Conn) error {
	if err := conn.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (name String, applied_at DateTime DEFAULT now()) ENGINE = MergeTree ORDER BY name"); err != nil {
		return fmt.Errorf("创建 schema_migrations 失败: %v", err)
	}

	entries, err := fs.ReadDir(dbmigrations.ChMigrations, "ch_migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var cnt uint64
		if err := conn.QueryRow(ctx, "SELECT count() FROM schema_migrations WHERE name = ?", name).Scan(&cnt); err != nil {
			return fmt.Errorf("查询 ClickHouse 迁移记录失败: %v", err)
		}
		if cnt > 0 {
			continue
		}
		content, err := dbmigrations.ChMigrations.ReadFile("ch_migrations/" + name)
		if err != nil {
			return err
		}
		for _, stmt := range splitSQLStatements(string(content)) {
			if err := conn.Exec(ctx, stmt); err != nil {
				return fmt.Errorf("执行 ClickHouse 迁移 %s 失败: %v", name, err)
			}
		}
		if err := conn.Exec(ctx, "INSERT INTO schema_migrations (name) VALUES (?)", name); err != nil {
			return fmt.Errorf("记录 ClickHouse 迁移 %s 失败: %v", name, err)
		}
		log.Printf("[ClickHouse] 迁移已应用: %s", name)
	}
	return nil
}

// splitSQLStatements 按分号切分迁移文件为单条语句, 跳过空语句(含纯注释块)
func splitSQLStatements(content string) []string {
	parts := strings.Split(content, ";")
	stmts := make([]string, 0, len(parts))
	for _, p := range parts {
		var b strings.Builder
		for _, line := range strings.Split(p, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "--") {
				continue
			}
			b.WriteString(line)
			b.WriteByte('\n')
		}
		if s := strings.TrimSpace(b.String()); s != "" {
			stmts = append(stmts, s)
		}
	}
	return stmts
}
