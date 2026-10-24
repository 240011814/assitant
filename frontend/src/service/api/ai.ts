import { request } from "../request";
import { fetchSSEWithAuthRetry } from "./sse";

/** 用户提示词版本 */
export interface UserPromptVersion {
  id: number;
  version: number;
  custom_prompt: string;
  remark: string;
  is_active: boolean;
  created_at: string;
}

export function fetchGetAIModels() {
  return request<Api.Admin.AIModel[]>({ url: "/api/ai/models" });
}

export function fetchGetUserPrompt(agentId: number, nodeKey?: string) {
  return request<{
    effective_prompt: string;
    default_prompt: string;
    versions: UserPromptVersion[];
    is_customized: boolean;
  }>({
    url: `/api/user-prompts/${agentId}`,
    // node_key: 编排 Agent 节点的提示词版本 (画布节点 id), 缺省为普通用户提示词
    params: nodeKey ? { node_key: nodeKey } : undefined
  });
}

export function fetchSaveUserPrompt(
  agentId: number,
  prompt: string,
  remark?: string,
  nodeKey?: string
) {
  return request({
    url: `/api/user-prompts/${agentId}`,
    method: "post",
    data: { prompt, remark },
    params: nodeKey ? { node_key: nodeKey } : undefined
  });
}

export function fetchSwitchUserPrompt(
  agentId: number,
  versionId: number,
  nodeKey?: string
) {
  return request({
    url: `/api/user-prompts/${agentId}/switch`,
    method: "put",
    data: { version_id: versionId },
    params: nodeKey ? { node_key: nodeKey } : undefined
  });
}

export function fetchDeleteUserPromptVersion(
  agentId: number,
  versionId: number,
  nodeKey?: string
) {
  return request({
    url: `/api/user-prompts/${agentId}/versions/${versionId}`,
    method: "delete",
    params: nodeKey ? { node_key: nodeKey } : undefined
  });
}

export function fetchResetUserPrompt(agentId: number) {
  return request({
    url: `/api/user-prompts/${agentId}`,
    method: "delete"
  });
}

/**
 * Chat streaming API - `signal` 用于中断流式请求(停止生成), 不会序列化进请求体
 */
export function fetchChatStream(data: {
  history_id: number;
  training_type: string;
  custom_training_id?: number;
  agent_id: number;
  model: string;
  messages: { role: string; content: string }[];
  signal?: AbortSignal;
}): Promise<Response> {
  const { signal, ...payload } = data;
  return fetchSSEWithAuthRetry("/api/chat", payload, signal);
}

/**
 * Tool approval API - 恢复被中断的工具调用 (SSE 流式返回)
 */
export function fetchToolApproval(data: {
  checkpoint_id: string;
  interrupt_id: string;
  approved: boolean;
  reason?: string;
  history_id?: number;
  training_type?: string;
  custom_training_id?: number;
  agent_id?: number;
  /** 发起被中断对话时使用的模型, 恢复时用同一模型续跑 */
  model?: string;
  messages?: { role: string; content: string }[];
}): Promise<Response> {
  return fetchSSEWithAuthRetry("/api/chat/tool-approval", data);
}
