import { request } from '../request'

/** 开始扫码登录, 返回二维码内容与轮询 ticket */
export function startMihoyoQrLogin() {
  return request<Api.Mihoyo.QrStart>({
    url: '/api/mihoyo/qr/start',
    method: 'post'
  })
}

/** 轮询扫码状态 (Confirmed 时后端已保存凭据, 之后无需继续轮询) */
export function getMihoyoQrStatus(ticket: string) {
  return request<Api.Mihoyo.QrStatus>({
    url: '/api/mihoyo/qr/status',
    method: 'get',
    params: { ticket }
  })
}

/** 米游社账号绑定状态 */
export function getMihoyoBindStatus() {
  return request<Api.Mihoyo.BindStatus>({
    url: '/api/mihoyo/status',
    method: 'get'
  })
}

/** 立即签到一次 (返回每个角色的结果) */
export function runMihoyoCheckin() {
  return request<Api.Mihoyo.CheckinResult[]>({
    url: '/api/mihoyo/checkin',
    method: 'post'
  })
}

/** 解绑米游社账号 */
export function unbindMihoyo() {
  return request<{ unboundAt: string }>({
    url: '/api/mihoyo/bind',
    method: 'delete'
  })
}
