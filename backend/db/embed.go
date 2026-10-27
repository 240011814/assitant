// Package db 提供数据库迁移 SQL 资源 (MySQL Goose + ClickHouse)。
package db

import "embed"

// Migrations MySQL Goose 迁移文件。
//
//go:embed migrations/*.sql
var Migrations embed.FS

// ChMigrations ClickHouse 迁移文件 (与 MySQL 迁移相互独立)。
//
//go:embed ch_migrations/*.sql
var ChMigrations embed.FS
