import { request } from '../request';

export function fetchGetUsers(params?: Api.Admin.UserSearchParams) {
  return request<Api.Admin.UserListResult>({ url: '/api/admin/users', params });
}

export function fetchCreateUser(data: Api.Admin.CreateUserParams) {
  return request({ url: '/api/admin/users', method: 'post', data });
}

export function fetchUpdateUser(userId: number, data: Api.Admin.UpdateUserParams) {
  return request({ url: `/api/admin/users/${userId}`, method: 'put', data });
}

export function fetchDeleteUser(userId: number) {
  return request({ url: `/api/admin/users/${userId}`, method: 'delete' });
}

export function fetchProxyLogin(userId: number) {
  return request<Api.Auth.LoginToken>({ url: `/api/admin/users/${userId}/proxy-login`, method: 'post' });
}

export function fetchGetRoles() {
  return request<Api.Admin.Role[]>({ url: '/api/admin/roles' });
}

export function fetchGetPermissions() {
  return request<Api.Admin.Permission[]>({ url: '/api/admin/permissions' });
}

export function fetchGetRolePermissions(roleCode: string) {
  return request<string[]>({ url: `/api/admin/roles/${roleCode}/permissions` });
}

export function fetchUpdateRolePermissions(roleCode: string, permissions: string[]) {
  return request({ url: `/api/admin/roles/${roleCode}/permissions`, method: 'put', data: { permissions } });
}

export function fetchCreateRole(data: Partial<Api.Admin.Role>) {
  return request({ url: '/api/admin/roles', method: 'post', data });
}

export function fetchDeleteRole(roleCode: string) {
  return request({ url: `/api/admin/roles/${roleCode}`, method: 'delete' });
}

export function fetchCreatePermission(data: Partial<Api.Admin.Permission>) {
  return request({ url: '/api/admin/permissions', method: 'post', data });
}

export function fetchUpdatePermission(id: number, data: Partial<Api.Admin.Permission>) {
  return request({ url: `/api/admin/permissions/${id}`, method: 'put', data });
}

export function fetchDeletePermission(id: number) {
  return request({ url: `/api/admin/permissions/${id}`, method: 'delete' });
}

export function fetchGetAIProviders() {
  return request<Api.Admin.AIProvider[]>({ url: '/api/admin/ai-providers' });
}

export function fetchCreateAIProvider(data: Partial<Api.Admin.AIProvider>) {
  return request({ url: '/api/admin/ai-providers', method: 'post', data });
}

export function fetchUpdateAIProvider(id: number, data: Partial<Api.Admin.AIProvider>) {
  return request({ url: `/api/admin/ai-providers/${id}`, method: 'put', data });
}

export function fetchDeleteAIProvider(id: number) {
  return request({ url: `/api/admin/ai-providers/${id}`, method: 'delete' });
}

export function fetchGetAIModels() {
  return request<Api.Admin.AIModel[]>({ url: '/api/admin/ai-models' });
}

export function fetchCreateAIModel(data: Partial<Api.Admin.AIModel>) {
  return request({ url: '/api/admin/ai-models', method: 'post', data });
}

export function fetchUpdateAIModel(id: number, data: Partial<Api.Admin.AIModel>) {
  return request({ url: `/api/admin/ai-models/${id}`, method: 'put', data });
}

export function fetchDeleteAIModel(id: number) {
  return request({ url: `/api/admin/ai-models/${id}`, method: 'delete' });
}

export function fetchTestAIConnection(data: { api_key: string; base_url?: string; model_code?: string }) {
  return request<string>({ url: '/api/admin/ai-test', method: 'post', data });
}

export function fetchGetAITools() {
  return request<Api.Admin.AITool[]>({ url: '/api/admin/ai-tools' });
}

export function fetchGetAITool(id: number) {
  return request<Api.Admin.AITool>({ url: `/api/admin/ai-tools/${id}` });
}

export function fetchCreateAITool(data: Partial<Api.Admin.AITool>) {
  return request({ url: '/api/admin/ai-tools', method: 'post', data });
}

export function fetchUpdateAITool(id: number, data: Partial<Api.Admin.AITool>) {
  return request({ url: `/api/admin/ai-tools/${id}`, method: 'put', data });
}

export function fetchDeleteAITool(id: number) {
  return request({ url: `/api/admin/ai-tools/${id}`, method: 'delete' });
}

export function fetchGetAIToolMetas() {
  return request<Api.Admin.AIToolMeta[]>({ url: '/api/admin/ai-tools/meta' });
}
