import { request } from "../request";
import { getAuthorization, handleExpiredRequest } from "../request/shared";
import { getServiceBaseURL } from "@/utils/service";

export function fetchGetAIModels() {
  return request<Api.Admin.AIModel[]>({ url: "/api/ai/models" });
}

export function fetchGetUserPrompt(agentId: number, nodeKey?: string) {
  return request<{
    effective_prompt: string;
    default_prompt: string;
    versions: any[];
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
 * SSE 流式请求统一封装 (axios 无法在浏览器做流式读取, 必须直接 fetch)。
 * 返回原始 Response 交给调用方读取 ReadableStream; 仅当响应是 JSON
 * 且 code 命中过期 token 时刷新凭据重试一次, SSE 流原样透传不消费 body。
 */
async function fetchSSEWithAuthRetry(url: string, payload: unknown, signal?: AbortSignal): Promise<Response> {
  const isHttpProxy = import.meta.env.DEV && import.meta.env.VITE_HTTP_PROXY === "Y";
  const { baseURL } = getServiceBaseURL(import.meta.env, isHttpProxy);

  let authorization = getAuthorization();
  const doFetch = () =>
    fetch(`${baseURL}${url}`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: authorization || ""
      },
      body: JSON.stringify(payload),
      signal
    });

  let response = await doFetch();

  if (response.ok) {
    try {
      const contentType = response.headers.get("content-type");
      if (contentType?.includes("application/json")) {
        const errorData = await response.json();
        const expiredTokenCodes =
          import.meta.env.VITE_SERVICE_EXPIRED_TOKEN_CODES?.split(",") || [];

        if (expiredTokenCodes.includes(String(errorData.code))) {
          const success = await handleExpiredRequest(request.state);
          if (success) {
            authorization = getAuthorization();
            response = await doFetch();
          }
        }
      }
    } catch (err) {
      // 过期检查失败不影响流式读取, 但要留痕便于排查
      console.warn("[ai] SSE 请求 token 过期检查失败:", err);
    }
  }

  return response;
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
