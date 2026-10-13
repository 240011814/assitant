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
      boot_time: string
      last_seen: string
    }

    /** 地址簿条目 */
    interface MacEntry {
      mac: string
      ip: string
    }

    /** 最近收发的报文 (send=下发 recv=上报) */
    interface Message {
      device_id: string
      direction: 'send' | 'recv'
      topic: string
      cmd: string
      payload: string
      received_at: string
    }

    /** 中转站条目 (订阅 wakemanager/relay 收到的文本, 最多 50 条 FIFO) */
    interface RelayItem {
      id: number
      text: string
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
      relay: RelayItem[]
    }
  }
}