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
 * Setup 2FA (get QR code URL)
 * Uses tempToken in Authorization header
 */
export function fetchTwoFactorSetup(tempToken: string) {
  return request<Api.Auth.TwoFactorSetupInfo>({
    url: '/auth/2fa/setup',
    method: 'post',
    headers: {
      Authorization: `Bearer ${tempToken}`
    }
  });
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
