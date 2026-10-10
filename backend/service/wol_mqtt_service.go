package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// WOL MQTT 协议主题 (components/wol_mqtt, 主题固定, <id> 为设备 12 位大写十六进制 MAC)
const (
	wolBroadcastTopic    = "wakemanager/broadcast" // 广播 (共享): 设备发现, {"cmd":"status"}
	wolStatusTopic       = "wakemanager/status"    // 状态上报 + 命令结果 + 地址簿列表 (共享)
	wolCmdTopicPrefix    = "wakemanager/cmd/"      // 唤醒: {"cmd":"wake","mac":"...","ip":"..."}
	wolMacTopicPrefix    = "wakemanager/mac/"      // 地址簿设置: set/del/clear
	wolMacGetTopicPrefix = "wakemanager/mac/get/"  // 地址簿获取: {"cmd":"get"}
	wolTempTopicPrefix   = "wakemanager/temp/"     // 芯片温度 (每设备, 每 60s)
)

// system_config 键
const (
	wolConfigHost     = "wol_mqtt_host"
	wolConfigPort     = "wol_mqtt_port"
	wolConfigUsername = "wol_mqtt_username"
	wolConfigPassword = "wol_mqtt_password"
)

const (
	wolDefaultPort  = 1883
	wolDiscoverWait = 2 * time.Second
	wolMacListWait  = 3 * time.Second
	wolMaxResultLog = 50
)

// WolMqttConfig MQTT 连接配置 (存 system_config, 密码仅落库不回显)
type WolMqttConfig struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	HasPassword bool   `json:"has_password"`
}

// WolDeviceInfo 设备状态 (协议状态主题 JSON)
type WolDeviceInfo struct {
	ID            string    `json:"id"`
	IP            string    `json:"ip"`
	SSID          string    `json:"ssid"`
	RSSI          int       `json:"rssi"`
	Heap          int       `json:"heap"`
	Broadcast     string    `json:"broadcast"`
	MQTTConnected bool      `json:"mqtt_connected"`
	MacCount      int       `json:"mac_count"`
	TempC         float64   `json:"temp_c"`
	Time          string    `json:"time"`
	LastSeen      time.Time `json:"last_seen"`
}

// WolMacEntry 地址簿条目
type WolMacEntry struct {
	MAC string `json:"mac"`
	IP  string `json:"ip"`
}

// WolMessage 最近收到的命令结果 / 状态消息 (供页面反馈)
type WolMessage struct {
	DeviceID   string    `json:"device_id"`
	Cmd        string    `json:"cmd"`
	Payload    string    `json:"payload"`
	ReceivedAt time.Time `json:"received_at"`
}

// WolStatus 服务运行状态
type WolStatus struct {
	Connected   bool         `json:"connected"`
	Host        string       `json:"host"`
	Port        int          `json:"port"`
	DeviceCount int          `json:"device_count"`
	LastError   string       `json:"last_error"`
	Messages    []WolMessage `json:"messages"`
}

// wolInbound 状态主题/温度主题来包的宽松解析 (用指针区分"字段是否存在")
type wolInbound struct {
	ID            string        `json:"id"`
	Cmd           string        `json:"cmd"`
	IP            string        `json:"ip"`
	SSID          string        `json:"ssid"`
	Broadcast     string        `json:"broadcast"`
	Time          string        `json:"time"`
	RSSI          *int          `json:"rssi"`
	Heap          *int          `json:"heap"`
	MacCount      *int          `json:"mac_count"`
	TempC         *float64      `json:"temp_c"`
	MQTTConnected *bool         `json:"mqtt_connected"`
	Macs          []WolMacEntry `json:"macs"`
}

// WolMqttService 维护与 MQTT Broker 的长连接会话, 聚合设备状态并提供设备管理能力
type WolMqttService struct {
	configSvc *SystemConfigService

	mu        sync.Mutex
	client    mqtt.Client
	connected bool
	lastError string
	devices   map[string]*WolDeviceInfo
	macLists  map[string][]WolMacEntry
	messages  []WolMessage
	waiters   map[string]chan []byte
}

// NewWolMqttService 构造设备管理服务
func NewWolMqttService(configSvc *SystemConfigService) *WolMqttService {
	return &WolMqttService{
		configSvc: configSvc,
		devices:   make(map[string]*WolDeviceInfo),
		macLists:  make(map[string][]WolMacEntry),
		waiters:   make(map[string]chan []byte),
	}
}

// LoadConfig 从 system_config 读取 MQTT 配置
func (s *WolMqttService) LoadConfig() WolMqttConfig {
	host, _ := s.configSvc.GetValue(wolConfigHost)
	portStr, _ := s.configSvc.GetValue(wolConfigPort)
	port := wolDefaultPort
	if n, err := strconv.Atoi(strings.TrimSpace(portStr)); err == nil && n > 0 {
		port = n
	}
	username, _ := s.configSvc.GetValue(wolConfigUsername)
	password, _ := s.configSvc.GetValue(wolConfigPassword)
	return WolMqttConfig{
		Host:        strings.TrimSpace(host),
		Port:        port,
		Username:    username,
		HasPassword: password != "",
	}
}

// SaveConfig 保存 MQTT 配置到 system_config (密码为空时保留原值)
func (s *WolMqttService) SaveConfig(cfg WolMqttConfig, password string) error {
	cfg.Host = strings.TrimSpace(cfg.Host)
	if cfg.Host == "" {
		return errors.New("MQTT 地址不能为空")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		cfg.Port = wolDefaultPort
	}
	if err := s.configSvc.SetValue(wolConfigHost, cfg.Host, "设备管理 MQTT 服务器地址"); err != nil {
		return err
	}
	if err := s.configSvc.SetValue(wolConfigPort, strconv.Itoa(cfg.Port), "设备管理 MQTT 服务器端口"); err != nil {
		return err
	}
	if err := s.configSvc.SetValue(wolConfigUsername, cfg.Username, "设备管理 MQTT 用户名"); err != nil {
		return err
	}
	if password != "" {
		if err := s.configSvc.SetValue(wolConfigPassword, password, "设备管理 MQTT 密码"); err != nil {
			return err
		}
	}
	return nil
}

// Connect 按配置建立 MQTT 连接并订阅状态/温度主题
func (s *WolMqttService) Connect() error {
	cfg := s.LoadConfig()
	if cfg.Host == "" {
		return errors.New("请先配置 MQTT 服务器地址")
	}

	s.Disconnect()

	password, _ := s.configSvc.GetValue(wolConfigPassword)
	broker := fmt.Sprintf("tcp://%s:%d", cfg.Host, cfg.Port)

	opts := mqtt.NewClientOptions()
	opts.AddBroker(broker)
	opts.SetClientID(fmt.Sprintf("wakemanager-backend-%d", time.Now().UnixNano()))
	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
		opts.SetPassword(password)
	}
	opts.SetConnectTimeout(5 * time.Second)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetAutoReconnect(true)
	opts.SetCleanSession(true)

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		s.setConnected(true, "")
		if err := s.subscribeAll(c); err != nil {
			log.Printf("[wol_mqtt] 订阅主题失败: %v", err)
			s.setConnected(true, "订阅主题失败: "+err.Error())
		}
	})
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		s.setConnected(false, "连接已断开: "+err.Error())
		log.Printf("[wol_mqtt] 连接断开: %v", err)
	})

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(6 * time.Second) {
		return errors.New("连接 MQTT 超时, 请检查地址与端口")
	}
	if err := token.Error(); err != nil {
		s.setConnected(false, err.Error())
		return fmt.Errorf("连接 MQTT 失败: %w", err)
	}

	s.mu.Lock()
	s.client = client
	s.connected = true
	s.mu.Unlock()

	return nil
}

// Disconnect 断开 MQTT 连接
func (s *WolMqttService) Disconnect() {
	s.mu.Lock()
	client := s.client
	s.client = nil
	s.connected = false
	s.mu.Unlock()

	if client != nil && client.IsConnected() {
		client.Disconnect(250)
	}
}

// IsConnected 当前是否已连接
func (s *WolMqttService) IsConnected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connected && s.client != nil && s.client.IsConnected()
}

// Status 返回运行状态与最近消息
func (s *WolMqttService) Status() WolStatus {
	cfg := s.LoadConfig()

	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := make([]WolMessage, len(s.messages))
	copy(msgs, s.messages)
	return WolStatus{
		Connected:   s.connected && s.client != nil && s.client.IsConnected(),
		Host:        cfg.Host,
		Port:        cfg.Port,
		DeviceCount: len(s.devices),
		LastError:   s.lastError,
		Messages:    msgs,
	}
}

// Discover 向广播主题请求状态并在超时后返回已发现设备 (先清空缓存)
func (s *WolMqttService) Discover() ([]WolDeviceInfo, error) {
	if !s.IsConnected() {
		return nil, errors.New("MQTT 未连接, 请先连接")
	}
	s.mu.Lock()
	s.devices = make(map[string]*WolDeviceInfo)
	s.mu.Unlock()

	if err := s.publish(wolBroadcastTopic, map[string]string{"cmd": "status"}); err != nil {
		return nil, err
	}
	time.Sleep(wolDiscoverWait)
	return s.ListDevices(), nil
}

// ListDevices 返回当前缓存的设备列表 (按 ID 排序)
func (s *WolMqttService) ListDevices() []WolDeviceInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]WolDeviceInfo, 0, len(s.devices))
	for _, d := range s.devices {
		list = append(list, *d)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	return list
}

// GetMacList 向设备请求地址簿并等待 mac_list 应答
func (s *WolMqttService) GetMacList(deviceID string) ([]WolMacEntry, error) {
	if !s.IsConnected() {
		return nil, errors.New("MQTT 未连接, 请先连接")
	}
	if deviceID == "" {
		return nil, errors.New("设备 ID 不能为空")
	}

	ch := s.registerWaiter(deviceID, "mac_list")
	defer s.unregisterWaiter(deviceID, "mac_list")

	if err := s.publish(wolMacGetTopicPrefix+deviceID, map[string]string{"cmd": "get"}); err != nil {
		return nil, err
	}

	select {
	case <-ch:
	case <-time.After(wolMacListWait):
	}
	return s.cachedMacList(deviceID), nil
}

// UpdateMac 设置地址簿: set(新增/更新) / del(删除) / clear(清空), 结果由设备异步上报到状态主题
func (s *WolMqttService) UpdateMac(deviceID, action, mac, ip string) error {
	if !s.IsConnected() {
		return errors.New("MQTT 未连接, 请先连接")
	}
	if deviceID == "" {
		return errors.New("设备 ID 不能为空")
	}
	mac = strings.ToUpper(strings.TrimSpace(mac))

	var body map[string]string
	switch action {
	case "set", "add", "update":
		if mac == "" {
			return errors.New("MAC 不能为空")
		}
		body = map[string]string{"cmd": "set", "mac": mac}
		if ip != "" {
			body["ip"] = strings.TrimSpace(ip)
		}
	case "del", "delete", "remove":
		if mac == "" {
			return errors.New("MAC 不能为空")
		}
		body = map[string]string{"cmd": "del", "mac": mac}
	case "clear":
		body = map[string]string{"cmd": "clear"}
	default:
		return fmt.Errorf("不支持的操作: %s", action)
	}
	return s.publish(wolMacTopicPrefix+deviceID, body)
}

// Wake 发送唤醒命令 (mac 必传, ip 可选)
func (s *WolMqttService) Wake(deviceID, mac, ip string) error {
	if !s.IsConnected() {
		return errors.New("MQTT 未连接, 请先连接")
	}
	if deviceID == "" {
		return errors.New("设备 ID 不能为空")
	}
	mac = strings.ToUpper(strings.TrimSpace(mac))
	if mac == "" {
		return errors.New("MAC 不能为空")
	}
	body := map[string]string{"cmd": "wake", "mac": mac}
	if ip != "" {
		body["ip"] = strings.TrimSpace(ip)
	}
	return s.publish(wolCmdTopicPrefix+deviceID, body)
}

// subscribeAll 订阅状态主题与温度主题
func (s *WolMqttService) subscribeAll(client mqtt.Client) error {
	if token := client.Subscribe(wolStatusTopic, 1, s.handleMessage); token.WaitTimeout(5*time.Second) && token.Error() != nil {
		return token.Error()
	}
	if token := client.Subscribe(wolTempTopicPrefix+"+", 0, s.handleMessage); token.WaitTimeout(5*time.Second) && token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (s *WolMqttService) publish(topic string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.publishBytes(topic, data)
}

// PublishRaw 调试用: 向指定主题发送原始报文 (不做 JSON 校验)
func (s *WolMqttService) PublishRaw(topic, payload string) error {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return errors.New("主题不能为空")
	}
	return s.publishBytes(topic, []byte(payload))
}

func (s *WolMqttService) publishBytes(topic string, data []byte) error {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if client == nil || !client.IsConnected() {
		return errors.New("MQTT 未连接, 请先连接")
	}
	token := client.Publish(topic, 0, false, data)
	if !token.WaitTimeout(3 * time.Second) {
		return errors.New("发送命令超时")
	}
	return token.Error()
}

// handleMessage 处理所有订阅到的消息 (状态主题与温度主题)
func (s *WolMqttService) handleMessage(_ mqtt.Client, msg mqtt.Message) {
	payload := append([]byte(nil), msg.Payload()...)
	topic := msg.Topic()

	var in wolInbound
	if err := json.Unmarshal(payload, &in); err != nil {
		s.recordMessage("", "", string(payload))
		return
	}

	// 温度主题: 只更新温度 (id 优先取 payload, 缺失时从主题后缀取)
	if strings.HasPrefix(topic, wolTempTopicPrefix) {
		deviceID := in.ID
		if deviceID == "" {
			deviceID = strings.TrimPrefix(topic, wolTempTopicPrefix)
		}
		s.applyTemp(deviceID, in.TempC)
		s.recordMessage(deviceID, in.Cmd, string(payload))
		return
	}

	// 地址簿列表应答
	if in.Cmd == "mac_list" {
		s.applyMacList(in.ID, in.Macs)
		s.recordMessage(in.ID, in.Cmd, string(payload))
		s.notifyWaiter(in.ID, "mac_list", payload)
		return
	}

	// 设备状态上报 (含状态字段)
	if in.ID != "" && isWolDeviceStatus(in) {
		s.applyDeviceStatus(in)
		s.recordMessage(in.ID, in.Cmd, string(payload))
		return
	}

	// 其余按命令结果记录并唤醒等待者
	s.recordMessage(in.ID, in.Cmd, string(payload))
	s.notifyWaiter(in.ID, "result", payload)
}

func isWolDeviceStatus(in wolInbound) bool {
	return in.IP != "" || in.SSID != "" || in.Broadcast != "" || in.Time != "" ||
		in.RSSI != nil || in.Heap != nil || in.MacCount != nil || in.MQTTConnected != nil
}

func (s *WolMqttService) applyDeviceStatus(in wolInbound) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.devices[in.ID]
	if d == nil {
		d = &WolDeviceInfo{ID: in.ID}
		s.devices[in.ID] = d
	}
	if in.IP != "" {
		d.IP = in.IP
	}
	if in.SSID != "" {
		d.SSID = in.SSID
	}
	if in.Broadcast != "" {
		d.Broadcast = in.Broadcast
	}
	if in.Time != "" {
		d.Time = in.Time
	}
	if in.RSSI != nil {
		d.RSSI = *in.RSSI
	}
	if in.Heap != nil {
		d.Heap = *in.Heap
	}
	if in.MacCount != nil {
		d.MacCount = *in.MacCount
	}
	if in.TempC != nil {
		d.TempC = *in.TempC
	}
	if in.MQTTConnected != nil {
		d.MQTTConnected = *in.MQTTConnected
	}
	d.LastSeen = time.Now()
}

func (s *WolMqttService) applyTemp(id string, temp *float64) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.devices[id]
	if d == nil {
		d = &WolDeviceInfo{ID: id}
		s.devices[id] = d
	}
	if temp != nil {
		d.TempC = *temp
	}
	d.LastSeen = time.Now()
}

func (s *WolMqttService) applyMacList(id string, macs []WolMacEntry) {
	if id == "" {
		return
	}
	if macs == nil {
		macs = []WolMacEntry{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.macLists[id] = macs
	if d := s.devices[id]; d != nil {
		d.MacCount = len(macs)
	}
}

func (s *WolMqttService) cachedMacList(id string) []WolMacEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.macLists[id]
	if list == nil {
		return []WolMacEntry{}
	}
	out := make([]WolMacEntry, len(list))
	copy(out, list)
	return out
}

func (s *WolMqttService) recordMessage(deviceID, cmd, payload string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, WolMessage{
		DeviceID:   deviceID,
		Cmd:        cmd,
		Payload:    payload,
		ReceivedAt: time.Now(),
	})
	if len(s.messages) > wolMaxResultLog {
		s.messages = s.messages[len(s.messages)-wolMaxResultLog:]
	}
}

func (s *WolMqttService) setConnected(connected bool, lastErr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connected = connected
	if lastErr != "" {
		s.lastError = lastErr
	} else if connected {
		s.lastError = ""
	}
}

func wolWaiterKey(deviceID, kind string) string {
	return deviceID + "|" + kind
}

func (s *WolMqttService) registerWaiter(deviceID, kind string) chan []byte {
	ch := make(chan []byte, 1)
	s.mu.Lock()
	s.waiters[wolWaiterKey(deviceID, kind)] = ch
	s.mu.Unlock()
	return ch
}

func (s *WolMqttService) unregisterWaiter(deviceID, kind string) {
	s.mu.Lock()
	delete(s.waiters, wolWaiterKey(deviceID, kind))
	s.mu.Unlock()
}

func (s *WolMqttService) notifyWaiter(deviceID, kind string, payload []byte) {
	s.mu.Lock()
	ch := s.waiters[wolWaiterKey(deviceID, kind)]
	s.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- payload:
	default:
	}
}
