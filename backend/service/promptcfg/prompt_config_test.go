package promptcfg

import (
	"os"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"backend/model"
)

// setupPromptTestDB 用纯 Go SQLite 起真库, 把 service.DB 指过去; 用完还原全局 DB 并清缓存
func setupPromptTestDB(t *testing.T) {
	t.Helper()
	origDB := DB
	dbFile := t.Name() + ".db"
	db, err := gorm.Open(sqlite.Open(dbFile), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.SystemConfig{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	DB = db
	RefreshPromptCache()
	t.Cleanup(func() {
		DB = origDB
		RefreshPromptCache()
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		_ = os.Remove(dbFile)
	})
}

// TestGetPromptFallbackAndOverride 未配置时回落内置默认; 配置后带缓存读覆盖值
func TestGetPromptFallbackAndOverride(t *testing.T) {
	setupPromptTestDB(t)

	def := builtinPrompts[PromptKeyTranslate].Default
	if got := GetPrompt(PromptKeyTranslate); got != def {
		t.Fatalf("未配置应回落默认值, got %q", got)
	}

	if err := DB.Create(&model.SystemConfig{Key: PromptKeyTranslate, Value: "自定义翻译提示词"}).Error; err != nil {
		t.Fatalf("insert config: %v", err)
	}
	RefreshPromptCache()
	if got := GetPrompt(PromptKeyTranslate); got != "自定义翻译提示词" {
		t.Fatalf("配置后应读覆盖值, got %q", got)
	}

	// 覆盖值被清空 → 回落默认
	if err := DB.Model(&model.SystemConfig{}).Where("`key` = ?", PromptKeyTranslate).Update("value", "").Error; err != nil {
		t.Fatalf("update config: %v", err)
	}
	RefreshPromptCache()
	if got := GetPrompt(PromptKeyTranslate); got != def {
		t.Fatalf("清空后应回落默认值, got %q", got)
	}
}

// TestGetPromptLazyLoadAndNegativeCache 未刷新缓存时首次访问也能读到已有覆盖值;
// 无行的键做负缓存, 不会每次查库
func TestGetPromptLazyLoadAndNegativeCache(t *testing.T) {
	setupPromptTestDB(t)

	if err := DB.Create(&model.SystemConfig{Key: PromptKeySentenceReview, Value: "自定义批改提示词"}).Error; err != nil {
		t.Fatalf("insert config: %v", err)
	}
	// 不调 RefreshPromptCache, 依赖 GetPrompt 的首次查库
	if got := GetPrompt(PromptKeySentenceReview); got != "自定义批改提示词" {
		t.Fatalf("懒加载应读到覆盖值, got %q", got)
	}

	// 无行的键: 首次访问后进负缓存, 之后删行/加行需靠 RefreshPromptCache 生效
	if got := GetPrompt(PromptKeyVocabTest); got != builtinPrompts[PromptKeyVocabTest].Default {
		t.Fatal("无行键应回落默认值")
	}
	if err := DB.Create(&model.SystemConfig{Key: PromptKeyVocabTest, Value: "后补的出题提示词"}).Error; err != nil {
		t.Fatalf("insert config: %v", err)
	}
	if got := GetPrompt(PromptKeyVocabTest); got == "后补的出题提示词" {
		t.Fatal("负缓存生效期间不应读到新行, 需 RefreshPromptCache")
	}
	RefreshPromptCache()
	if got := GetPrompt(PromptKeyVocabTest); got != "后补的出题提示词" {
		t.Fatalf("刷新后应读到新行, got %q", got)
	}
}

// TestListPromptsMerge 注册表与 DB 覆盖值合并, overridden 标记正确
func TestListPromptsMerge(t *testing.T) {
	setupPromptTestDB(t)

	if err := DB.Create(&model.SystemConfig{Key: PromptKeyDictLLM, Value: "自定义查词"}).Error; err != nil {
		t.Fatalf("insert config: %v", err)
	}
	items := ListPrompts()
	if len(items) != len(promptOrder) {
		t.Fatalf("条目数应为 %d, got %d", len(promptOrder), len(items))
	}
	for _, it := range items {
		switch it.Key {
		case PromptKeyDictLLM:
			if !it.Overridden || it.Value != "自定义查词" {
				t.Fatalf("覆盖项标记/取值错误: %+v", it)
			}
		default:
			if it.Overridden || it.Value != it.DefaultValue {
				t.Fatalf("未覆盖项应回默认值: %+v", it)
			}
		}
	}
}

// TestGetPromptPassageLevelPlaceholder 写课文提示词的 {level} 占位符可替换
func TestGetPromptPassageLevelPlaceholder(t *testing.T) {
	setupPromptTestDB(t)

	got := GetPrompt(PromptKeyReadingPassage)
	if !strings.Contains(got, "{level}") {
		t.Fatal("默认提示词应包含 {level} 占位符")
	}
	replaced := strings.ReplaceAll(got, "{level}", "B1")
	if strings.Contains(replaced, "{level}") || !strings.Contains(replaced, "B1") {
		t.Fatal("{level} 替换失败")
	}
}
