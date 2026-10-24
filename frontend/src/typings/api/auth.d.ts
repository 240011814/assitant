declare namespace Api {
  /**
   * namespace Auth
   *
   * backend api module: "auth"
   */
  namespace Auth {
    interface LoginToken {
      token: string;
      refreshToken: string;
    }

    /** 开启 2FA 的账号登录时的中间响应: 需先验码换取正式 token */
    interface LoginTwoFAChallenge {
      need2fa: true;
      tempToken: string;
    }

    interface UserInfo {
      userId: string;
      userName: string;
      nickname: string;
      roles: string[];
      buttons: string[];
      permissions?: string[];
    }

    interface TwoFactorSetupInfo {
      qrCodeUrl: string;
      secret: string;
    }
  }
}
