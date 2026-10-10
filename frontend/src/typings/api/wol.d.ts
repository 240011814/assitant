declare namespace Api {
  namespace Wol {
    /** MQTT 连接配置 (密码不回显, 仅有/无标记) */
    interface Config {
      host: string
      port: number
      username: string
      has_password: boolean
    }

    /** 设备状态 (协议状态主题 JSON) */
    interface Device {
      id: string
      ip: string
      ssid: string
      rssi: number
      heap: number
      broadcast: string
      mqtt_connected: boolean
      mac_count: number
      temp_c: number
      time: string
      last_seen: string
    }

    /** 地址簿条目 */
    interface MacEntry {
      mac: string
      ip: string
    }

    /** 最近收到的命令结果 / 状态消息 */
    interface Message {
      device_id: string
      cmd: string
      payload: string
      received_at: string
    }

    /** 服务运行状态 */
    interface Status {
      connected: boolean
      host: string
      port: number
      device_count: number
      last_error: string
      messages: Message[]
    }
  }
}