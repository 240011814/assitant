declare namespace Api {
  namespace Admin {
    interface User {
      userId: number;
      userName: string;
      nickname: string;
      role: string;
      token_quota_month: number | null;
      createdAt: string;
      updatedAt: string;
    }

    interface UserSearchParams {
      keyword?: string;
      role?: string;
      page?: number;
      page_size?: number;
    }

    interface UserListResult {
      list: User[];
      total: number;
      page: number;
      page_size: number;
    }

    interface CreateUserParams {
      userName: string;
      password: string;
      nickname: string;
      role: string;
    }

    interface UpdateUserParams {
      password?: string;
      nickname: string;
      role: string;
      /** 月度 Token 限额: 不传=不修改, 0=不限, 正数=每月上限 */
      token_quota_month?: number | null;
    }

    /** 管理员重置用户密码: password 为空时由服务端生成随机密码 */
    interface ResetUserPasswordParams {
      password?: string;
    }

    /** 重置密码响应: 仅随机生成时回传明文一次 */
    interface ResetUserPasswordResult {
      password: string;
    }

    interface UserProfile {
      userId: number;
      userName: string;
      nickname: string;
      email: string;
      role: string;
      lastLoginAt: string | null;
      createdAt: string;
      updatedAt: string;
    }

    interface UpdateProfileParams {
      nickname: string;
      email: string;
    }

    interface ChangePasswordParams {
      oldPassword: string;
      newPassword: string;
    }

    interface Role {
      id: number;
      code: string;
      name: string;
      description: string;
    }

    interface Permission {
      id: number;
      code: string;
      name: string;
      groupName: string;
    }

    interface AIProvider {
      id: number;
      name: string;
      api_key: string;
      masked_api_key?: string;
      base_url: string;
      is_active: boolean;
      created_at: string;
      updated_at: string;
      models?: AIModel[];
    }

    interface AIModel {
      id: number;
      provider_id: number;
      model_code: string;
      display_name: string;
      is_default: boolean;
      config_json: string;
      created_at: string;
      updated_at: string;
    }

    interface AITool {
      id: number;
      name: string;
      display_name: string;
      description: string;
      enabled: boolean;
      confirm_required: boolean;
      config_json: string;
      created_at: string;
      updated_at: string;
    }

    interface AIToolParam {
      name: string;
      type: string;
      description: string;
      required: boolean;
      default?: string;
    }

    interface AIToolMeta {
      name: string;
      display_name: string;
      description: string;
      params: AIToolParam[];
    }
  }
}
