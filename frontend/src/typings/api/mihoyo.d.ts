declare namespace Api {
  namespace Mihoyo {
    /** 开始扫码登录: 返回二维码内容与轮询凭证 */
    interface QrStart {
      ticket: string
      qrUrl: string
      expiresIn: number
    }

    /** 扫码轮询状态 (Confirmed 时服务端已自动绑定凭据) */
    interface QrStatus {
      status: 'Created' | 'Scanned' | 'Confirmed' | 'Expired' | string
      message?: string
      nickname?: string
      uid?: string
      bound: boolean
    }

    /** 绑定状态 */
    interface BindStatus {
      bound: boolean
      nickname: string
      uid: string
      mid: string
    }

    /** 单角色签到结果 */
    interface CheckinResult {
      gameUid: string
      nickname: string
      status: 'success' | 'already' | 'blocked' | 'failed' | 'no_role' | string
      message: string
    }

    /** 签到信息查询单角色状态 */
    interface CheckinRoleInfo {
      gameUid: string
      nickname: string
      signed: boolean
      totalSignDay: number
      awardName: string
      awardCnt: string
      awardIcon: string
    }

    /** 签到信息查询 (不触发签到) */
    interface CheckinStatus {
      bound: boolean
      roles: CheckinRoleInfo[]
    }
  }
}
