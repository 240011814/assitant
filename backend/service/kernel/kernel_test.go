package kernel

import (
	"testing"

	"backend/model"
)

// TestDecodeLLMArray 标注场景回归: extractJSON 按 {..} 截取会把顶层数组削成
// 多对象碎片导致稳定解析失败, decodeLLMArray 必须吃下这些形态
func TestDecodeLLMArray(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"纯数组", `[{"word":"zipper","meaning":"拉链"}]`},
		{"老 extractJSON 切坏的形态", `[{"word":"zipper","meaning":"拉链"},{"word":"hinge","meaning":"铰链"}]`},
		{"markdown 围栏", "```json\n[{\"word\":\"zipper\",\"meaning\":\"拉链\"}]\n```"},
		{"前后说明文字", `好的, 以下是标注结果: [{"word":"zipper","meaning":"拉链"}] 请查收`},
		{"对象任意键包裹", `{"words":[{"word":"zipper","meaning":"拉链"}]}`},
	}
	for _, c := range cases {
		var out []model.DifficultWord
		if err := DecodeLLMArray(c.raw, &out); err != nil {
			t.Fatalf("%s: 解析失败: %v", c.name, err)
		}
		if len(out) == 0 || out[0].Word != "zipper" {
			t.Fatalf("%s: 解析结果异常: %+v", c.name, out)
		}
	}
}

// TestDecodeLLMObject 对象解析: 围栏/说明文字/首尾截取
func TestDecodeLLMObject(t *testing.T) {
	type obj struct {
		Example string `json:"example"`
	}
	cases := []struct {
		name string
		raw  string
	}{
		{"纯对象", `{"example":"hello world","mnemonic":"x"}`},
		{"markdown 围栏", "```json\n{\"example\":\"hello world\"}\n```"},
		{"前后说明文字", `结果如下: {"example":"hello world"} 以上`},
	}
	for _, c := range cases {
		var out obj
		if err := DecodeLLMObject(c.raw, &out); err != nil {
			t.Fatalf("%s: 解析失败: %v", c.name, err)
		}
		if out.Example != "hello world" {
			t.Fatalf("%s: 解析结果异常: %+v", c.name, out)
		}
	}
}
