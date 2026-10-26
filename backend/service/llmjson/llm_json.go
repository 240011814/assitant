// Package llmjson LLM JSON 输出的宽容解析: 模型会加 markdown 围栏、前后说明文字、
// 或把数组包进对象, 严格 Unmarshal 经常失败。这里提供数组/对象两个兜底入口, 全部失败时
// 把模型原文 (截断) 记入日志, 避免排障时盲猜。纯标准库依赖, 任何业务包可安全引用。
// 注意: ExtractJSON 按 {..} 截取, 会把顶层数组削成多个对象的碎片, 数组解析不能走它。
package llmjson

import (
	"encoding/json"
	"errors"
	"log"
	"strings"
)

// DecodeLLMArray 解析 LLM 输出中的 JSON 数组到 v (v 必须是切片指针)
func DecodeLLMArray(raw string, v any) error {
	s := stripJSONFence(raw)
	if err := json.Unmarshal([]byte(s), v); err == nil {
		return nil
	}
	// 前后有说明文字: 取首个 [ 到末个 ] 的片段重试
	if i := strings.Index(s, "["); i >= 0 {
		if j := strings.LastIndex(s, "]"); j > i {
			if err := json.Unmarshal([]byte(s[i:j+1]), v); err == nil {
				return nil
			}
		}
	}
	// {"words": [...]} 等任意键包裹: 取出对象里第一个数组值
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			var m map[string]json.RawMessage
			if err := json.Unmarshal([]byte(s[i:j+1]), &m); err == nil {
				for _, val := range m {
					var arr []json.RawMessage
					if err := json.Unmarshal(val, &arr); err == nil {
						if data, err := json.Marshal(arr); err == nil {
							if err := json.Unmarshal(data, v); err == nil {
								return nil
							}
						}
					}
				}
			}
		}
	}
	err := errors.New("AI 输出不是合法 JSON 数组")
	logLLMParseFailure(err, raw)
	return err
}

// DecodeLLMObject 解析 LLM 输出中的 JSON 对象到 v (v 必须是结构体/map 指针)
func DecodeLLMObject(raw string, v any) error {
	s := ExtractJSON(raw)
	if err := json.Unmarshal([]byte(s), v); err == nil {
		return nil
	}
	// extractJSON 截取失败时按首尾大括号原样重试
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			if err := json.Unmarshal([]byte(s[i:j+1]), v); err == nil {
				return nil
			}
		}
	}
	err := errors.New("AI 输出不是合法 JSON 对象")
	logLLMParseFailure(err, raw)
	return err
}

// ExtractJSON 去掉 LLM 输出中可能包裹的 markdown 代码块, 并截取首个 JSON 对象
func ExtractJSON(raw string) string {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[i+3:]
		s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "json"))
		if j := strings.Index(s, "```"); j >= 0 {
			s = s[:j]
		}
	}
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "{"); i > 0 {
		s = s[i:]
	}
	if j := strings.LastIndex(s, "}"); j >= 0 && j < len(s)-1 {
		s = s[:j+1]
	}
	return s
}

// stripJSONFence 只剥 markdown 代码围栏, 不动结构本身 (数组/对象都安全)
func stripJSONFence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[i+3:]
		s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "json"))
		if j := strings.Index(s, "```"); j >= 0 {
			s = s[:j]
		}
	}
	return strings.TrimSpace(s)
}

// logLLMParseFailure 记录解析失败与模型原文片段 (rune 截断防多字节切断)
func logLLMParseFailure(err error, raw string) {
	runes := []rune(strings.TrimSpace(raw))
	if len(runes) > 800 {
		runes = runes[:800]
	}
	log.Printf("LLM 输出解析失败: %v; 模型原文: %s", err, string(runes))
}
