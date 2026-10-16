package service

import (
	"encoding/json"
	"fmt"
	"testing"
)

// wolDeviceIDFromTopic 覆盖各命令主题前缀; mac/get 须排在 mac 之前, 否则被截成 get/<id>
func TestWolDeviceIDFromTopic(t *testing.T) {
	tests := []struct {
		topic string
		want  string
	}{
		{"wakemanager/cmd/A1B2C3D4E5F6", "A1B2C3D4E5F6"},
		{"wakemanager/mac/get/A1B2C3D4E5F6", "A1B2C3D4E5F6"},
		{"wakemanager/mac/A1B2C3D4E5F6", "A1B2C3D4E5F6"},
		{"wakemanager/ota/A1B2C3D4E5F6", "A1B2C3D4E5F6"},
		{"wakemanager/temp/A1B2C3D4E5F6", "A1B2C3D4E5F6"},
		{"wakemanager/status", ""},
		{"texthub/web", ""},
	}
	for _, tt := range tests {
		if got := wolDeviceIDFromTopic(tt.topic); got != tt.want {
			t.Errorf("wolDeviceIDFromTopic(%q) = %q, want %q", tt.topic, got, tt.want)
		}
	}
}

// 状态快照的 version/ota 字段须被解析, 且仅带这两个字段也能识别为设备状态上报
func TestWolInboundParsesVersionAndOta(t *testing.T) {
	var in wolInbound
	payload := []byte(`{"id":"A1B2C3D4E5F6","version":"c4f2a91","ota":{"state":"update_available","current":"c4f2a91","latest":"9be03d7","progress":42}}`)
	if err := json.Unmarshal(payload, &in); err != nil {
		t.Fatalf("unmarshal 状态快照: %v", err)
	}
	if in.Version != "c4f2a91" {
		t.Errorf("version = %q, want %q", in.Version, "c4f2a91")
	}
	if in.Ota == nil {
		t.Fatal("ota 字段未被解析")
	}
	if in.Ota.State != "update_available" || in.Ota.Latest != "9be03d7" {
		t.Errorf("ota = %+v, want state=update_available latest=9be03d7", *in.Ota)
	}
	if in.Ota.Progress == nil || *in.Ota.Progress != 42 {
		t.Errorf("progress = %v, want 42", in.Ota.Progress)
	}
	if !isWolDeviceStatus(in) {
		t.Error("仅含 version/ota 的快照应被识别为设备状态上报")
	}

	// 失败态: error 有值, progress/latest 可缺省
	var failed wolInbound
	if err := json.Unmarshal([]byte(`{"id":"A1B2C3D4E5F6","ota":{"state":"failed","current":"c4f2a91","error":"download failed"}}`), &failed); err != nil {
		t.Fatalf("unmarshal 失败态快照: %v", err)
	}
	if failed.Ota == nil || failed.Ota.Error != "download failed" || failed.Ota.Progress != nil {
		t.Errorf("failed ota = %+v, want error=download failed 且 progress 为空", failed.Ota)
	}
}

// 中转站报文仅收 {"deviceId","text"}: 文本去除首尾空白, 非法 JSON 与空文本忽略
func TestWolAppendRelayParsesJSON(t *testing.T) {
	s := &WolMqttService{}
	s.appendRelay([]byte(`{"deviceId":"A1B2C3D4E5F6","text":" hello "}`))
	s.appendRelay([]byte(`{"text":"缺 deviceId 也保留"}`))
	s.appendRelay([]byte(`not-json`))
	s.appendRelay([]byte(`{"deviceId":"A1B2C3D4E5F6","text":"   "}`))

	if len(s.relay) != 2 {
		t.Fatalf("relay 条数 = %d, want 2 (非法 JSON 与空文本应被忽略)", len(s.relay))
	}
	if s.relay[0].DeviceID != "A1B2C3D4E5F6" || s.relay[0].Text != "hello" {
		t.Errorf("首条 = %+v, want deviceId=A1B2C3D4E5F6 text=hello", s.relay[0])
	}
	if s.relay[1].DeviceID != "" || s.relay[1].Text != "缺 deviceId 也保留" {
		t.Errorf("次条 = %+v, want deviceId 为空且保留文本", s.relay[1])
	}
}

// 中转站 FIFO: 超出 wolMaxRelayItems 时淘汰最旧的, ID 单调递增
func TestWolAppendRelayFIFO(t *testing.T) {
	s := &WolMqttService{}
	total := wolMaxRelayItems + 5
	for i := 0; i < total; i++ {
		s.appendRelay([]byte(fmt.Sprintf(`{"deviceId":"web","text":"m%d"}`, i)))
	}
	if len(s.relay) != wolMaxRelayItems {
		t.Fatalf("relay 条数 = %d, want %d", len(s.relay), wolMaxRelayItems)
	}
	if s.relay[0].Text != "m5" || s.relay[0].ID != 6 {
		t.Errorf("淘汰后首条 = %+v, want m5 (ID=6)", s.relay[0])
	}
	if s.relay[len(s.relay)-1].ID != uint64(total) {
		t.Errorf("末条 ID = %d, want %d", s.relay[len(s.relay)-1].ID, total)
	}
}
