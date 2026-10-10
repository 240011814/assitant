import { request } from '../request'

/** 读取 MQTT 连接配置 */
export function getWolConfig() {
  return request<Api.Wol.Config>({
    url: '/api/wol/config',
    method: 'get'
  })
}

/** 保存 MQTT 连接配置 (密码留空则保持不变) */
export function updateWolConfig(data: { host: string; port?: number; username?: string; password?: string }) {
  return request<Api.Wol.Config>({
    url: '/api/wol/config',
    method: 'put',
    data
  })
}

/** 建立 MQTT 连接 */
export function connectWol() {
  return request<Api.Wol.Status>({
    url: '/api/wol/connect',
    method: 'post'
  })
}

/** 断开 MQTT 连接 */
export function disconnectWol() {
  return request<Api.Wol.Status>({
    url: '/api/wol/disconnect',
    method: 'post'
  })
}

/** 查询连接状态与最近消息 */
export function getWolStatus() {
  return request<Api.Wol.Status>({
    url: '/api/wol/status',
    method: 'get'
  })
}

/** 获取设备列表 (discover=true 时先广播发现) */
export function getWolDevices(discover = true) {
  return request<Api.Wol.Device[]>({
    url: '/api/wol/devices',
    method: 'get',
    params: { discover }
  })
}

/** 获取指定设备的地址簿列表 */
export function getWolMacList(deviceId: string) {
  return request<Api.Wol.MacEntry[]>({
    url: `/api/wol/devices/${deviceId}/macs`,
    method: 'get'
  })
}

/** 地址簿增删改 (action: set/del/clear) */
export function updateWolMac(deviceId: string, data: { action: 'set' | 'del' | 'clear'; mac?: string; ip?: string }) {
  return request<null>({
    url: `/api/wol/devices/${deviceId}/mac`,
    method: 'post',
    data
  })
}

/** 发送唤醒命令 */
export function wakeWolDevice(deviceId: string, data: { mac: string; ip?: string }) {
  return request<null>({
    url: `/api/wol/devices/${deviceId}/wake`,
    method: 'post',
    data
  })
}

/** 调试: 向任意主题下发原始报文 */
export function publishWolRaw(data: { topic: string; payload: string }) {
  return request<null>({
    url: '/api/wol/publish',
    method: 'post',
    data
  })
}