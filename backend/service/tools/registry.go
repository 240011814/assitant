package tools

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

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

var registry = map[string]*toolEntry{}

type toolEntry struct {
	meta    ToolMeta
	factory ToolFactory
	configType reflect.Type
}

func Register(name, displayName, description string, configType any, factory ToolFactory) {	var params []ToolParam
	if configType != nil {
		t := reflect.TypeOf(configType)
		if t.Kind() == reflect.Ptr {
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
				if param.Type == "string" {
					param.Type = "string"
				} else if param.Type == "int" || param.Type == "int64" {
					param.Type = "integer"
				} else if param.Type == "float32" || param.Type == "float64" {
					param.Type = "number"
				} else if param.Type == "bool" {
					param.Type = "boolean"
				}
				params = append(params, param)
			}
		}
	}
	registry[name] = &toolEntry{
		meta: ToolMeta{
			Name:        name,
			DisplayName: displayName,
			Description: description,
			Params:      params,
		},
		factory:     factory,
		configType: reflect.TypeOf(configType),
	}
}

func GetAllToolMeta() []ToolMeta {
	var result []ToolMeta
	for _, entry := range registry {
		result = append(result, entry.meta)
	}
	return result
}

// Unregister 注销动态注册的工具 (MCP server 删除/禁用时调用); 不存在时静默
func Unregister(names ...string) {
	for _, n := range names {
		delete(registry, n)
	}
}

func GetToolMeta(name string) (ToolMeta, bool) {
	entry, ok := registry[name]
	if !ok {
		return ToolMeta{}, false
	}
	return entry.meta, true
}

func CreateTool(name string, configJSON string) (tool.BaseTool, error) {
	entry, ok := registry[name]
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
