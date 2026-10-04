import { request } from '../request';

/** MCP 服务 (动态注册: 连接 -> 发现工具 -> 注册 mcp_<server>_<tool>) */
export interface MCPServerItem {
  id: number;
  name: string;
  transport: 'stdio' | 'sse' | 'http';
  url: string;
  command: string;
  args: string;
  env: string;
  headers: string;
  timeout_seconds: number;
  enabled: boolean;
  last_status: 'none' | 'ok' | 'failed';
  last_error: string;
  tool_count: number;
  registered_tools: string[];
  created_at: string;
  updated_at: string;
}

export interface MCPServerPayload {
  name: string;
  transport: string;
  url: string;
  command: string;
  args: string;
  env: string;
  headers: string;
  timeout_seconds: number;
  enabled: boolean;
}

/** 获取服务列表 */
export function fetchMCPServers() {
  return request<{ items: MCPServerItem[] }>({ url: '/api/admin/mcp-servers', method: 'get' });
}

/** 新增服务 (启用时自动连接并注册工具) */
export function fetchCreateMCPServer(data: MCPServerPayload) {
  return request<MCPServerItem>({ url: '/api/admin/mcp-servers', method: 'post', data });
}

/** 修改服务 (禁用时自动禁用其工具行; 启用时重新连接) */
export function fetchUpdateMCPServer(id: number, data: MCPServerPayload) {
  return request<null>({ url: `/api/admin/mcp-servers/${id}`, method: 'put', data });
}

/** 手动重连 (重新发现并注册工具) */
export function fetchConnectMCPServer(id: number) {
  return request<{ message: string }>({ url: `/api/admin/mcp-servers/${id}/connect`, method: 'post' });
}

/** 删除服务 (自动注销工具并删除其工具行) */
export function fetchDeleteMCPServer(id: number) {
  return request<null>({ url: `/api/admin/mcp-servers/${id}`, method: 'delete' });
}
