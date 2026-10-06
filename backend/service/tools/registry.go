package tools

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"gorm.io/gorm"
)

var db *gorm.DB

// SetDB sets the database connection for tools that need database access
func SetDB(database *gorm.DB) {
	db = database
}

// GetDB returns the database connection
func GetDB() *gorm.DB {
	return db
}

type ToolParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
}

type ToolMeta struct {
	Name        string      `json:"name"`
	DisplayName string      `json:"display_name"`
	Description string      `json:"description"`
	Params      []ToolParam `json:"params"`
}

type ToolFactory func(config map[string]any) (tool.BaseTool, error)

// registry 由 MCP 动态注册在 goroutine 里写、请求路径并发读, 必须持锁访问
var (
	registryMu sync.RWMutex
	registry   = map[string]*toolEntry{}
)

type toolEntry struct {
	meta    ToolMeta
	factory ToolFactory
	configType reflect.Type
}

func Register(name, displayName, description string, configType any, factory ToolFactory) {	var params []ToolParam
	if configType != nil {
		t := reflect.TypeOf(configType)
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		if t.Kind() == reflect.Struct {
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				param := ToolParam{
					Name:        f.Tag.Get("json"),
					Type:        f.Type.Kind().String(),
					Description: f.Tag.Get("description"),
					Required:    f.Tag.Get("required") == "true",
					Default:     f.Tag.Get("default"),
				}
				if param.Name == "" {
					param.Name = strings.ToLower(f.Name)
				}
				switch param.Type {
				case "int", "int64":
					param.Type = "integer"
				case "float32", "float64":
					param.Type = "number"
				case "bool":
					param.Type = "boolean"
				}
				params = append(params, param)
			}
		}
	}
	registryMu.Lock()
	registry[name] = &toolEntry{
		meta: ToolMeta{
			Name:        name,
			DisplayName: displayName,
			Description: description,
			Params:      params,
		},
		factory:     factory,
		configType:  reflect.TypeOf(configType),
	}
	registryMu.Unlock()
}

func GetAllToolMeta() []ToolMeta {
	registryMu.RLock()
	entries := make([]*toolEntry, 0, len(registry))
	for _, entry := range registry {
		entries = append(entries, entry)
	}
	registryMu.RUnlock()

	var result []ToolMeta
	for _, entry := range entries {
		result = append(result, entry.meta)
	}
	return result
}

// Unregister 注销动态注册的工具 (MCP server 删除/禁用时调用); 不存在时静默
func Unregister(names ...string) {
	registryMu.Lock()
	for _, n := range names {
		delete(registry, n)
	}
	registryMu.Unlock()
}

func GetToolMeta(name string) (ToolMeta, bool) {
	registryMu.RLock()
	entry, ok := registry[name]
	registryMu.RUnlock()
	if !ok {
		return ToolMeta{}, false
	}
	return entry.meta, true
}

func CreateTool(name string, configJSON string) (tool.BaseTool, error) {
	registryMu.RLock()
	entry, ok := registry[name]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	configMap := make(map[string]any)
	if configJSON != "" {
		if err := json.Unmarshal([]byte(configJSON), &configMap); err != nil {
			return nil, fmt.Errorf("invalid config JSON for tool %s: %w", name, err)
		}
	}
	return entry.factory(configMap)
}
