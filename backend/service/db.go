package service

import (
	"embed"
	"fmt"
	"log"
	"os"
	"time"

	"backend/config"

	"github.com/pressly/goose/v3"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

//go:embed db/migrations/*.sql
var embedMigrations embed.FS

// InitDB 初始化数据库连接并自动运行迁移
func InitDB(cfg *config.Config) (*gorm.DB, error) {
	tls:= "false"	

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local&tls=%s",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.DBName,
		tls,
	)

	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold:             500 * time.Millisecond, // 慢 SQL 阈值调高到 500ms
			LogLevel:                  logger.Warn,            // 仅告警/慢查询 (Info 会打印全部 SQL 及绑定参数, 生产泄露数据且拖性能)
			IgnoreRecordNotFoundError: true,                   // 忽略未找到记录的错误
			Colorful:                  true,                   // 彩色打印
		},
	)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: newLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %v", err)
	}

	// 运行数据库迁移 (Goose)
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %v", err)
	}

	// 连接池配置 (默认无上限连接, 高并发下会打爆 MySQL)
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(time.Hour)

	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("mysql"); err != nil {
		return nil, fmt.Errorf("failed to set goose dialect: %v", err)
	}

	log.Println("Running database migrations...")
	if err := goose.Up(sqlDB, "db/migrations"); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %v", err)
	}

	DB = db
	log.Println("Database connection established and migrations completed")
	return db, nil
}
