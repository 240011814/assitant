import { request } from '../request';

/**
 * Login
 *
 * @param userName User name
 * @param password Password
 */
export function fetchLogin(userName: string, password: string) {
  return request<Api.Auth.LoginToken>({
    url: '/auth/login',
    method: 'post',
    data: {
      userName,
      password
    }
  });
}

/**
 * Register
 *
 * @param userName User name
 * @param password Password
 */
export function fetchRegister(userName: string, password: string) {
  return request<Api.Auth.LoginToken>({
    url: '/auth/register',
    method: 'post',
    data: {
      userName,
      password
    }
  });
}

/** Get user info */
export function fetchGetUserInfo() {
  return request<Api.Auth.UserInfo>({ url: '/auth/getUserInfo' });
}

/** 注册开关状态 (登录页公开接口) */
export function fetchRegisterStatus() {
  return request<{ enabled: boolean }>({ url: '/auth/register-status' });
}

/**
 * Refresh token
 *
 * @param refreshToken Refresh token
 */
export function fetchRefreshToken(refreshToken: string) {
  return request<Api.Auth.LoginToken>({
    url: '/auth/refreshToken',
    method: 'post',
    data: {
      refreshToken
    }
  });
}

/**
 * return custom backend error
 *
 * @param code error code
 * @param msg error message
 */
export function fetchCustomBackendError(code: string, msg: string) {
  return request({ url: '/auth/error', params: { code, msg } });
}

/**
 * Verify 2FA code
 * Uses tempToken in Authorization header, code in body
 */
export function fetchTwoFactorVerify(tempToken: string, code: string) {
  return request<Api.Auth.LoginToken>({
    url: '/auth/2fa/verify',
    method: 'post',
    headers: {
      Authorization: `Bearer ${tempToken}`
    },
    data: {
      code
    }
  });
}

/**
 * 自助绑定两步验证第一步: 生成 TOTP 密钥 (返回二维码 URL + 密钥, 尚未生效)
 * 完整 JWT 认证, 用于个人中心
 */
export function fetchUser2FASetup() {
  return request<Api.Auth.TwoFactorSetupInfo>({ url: '/user/2fa/setup', method: 'post' });
}

/**
 * 自助绑定两步验证第二步: 校验验证码后正式开启
 */
export function fetchUser2FAEnable(secret: string, code: string) {
  return request({ url: '/user/2fa/enable', method: 'post', data: { secret, code } });
}

/**
 * 自助关闭两步验证: 校验验证码后清除密钥
 */
export function fetchUser2FADisable(code: string) {
  return request({ url: '/user/2fa/disable', method: 'post', data: { code } });
}
