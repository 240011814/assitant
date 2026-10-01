import { request } from "../request";
import { getAuthorization, handleExpiredRequest } from "../request/shared";
import { getServiceBaseURL } from "@/utils/service";
import { request as requestInstance } from "../request";

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
    method: "delete",
  });
}

/**
 * Chat streaming API - direct fetch with proper auth interceptors
 * Returns raw Response object with ReadableStream for SSE handling
 * `signal` 用于中断流式请求(停止生成), 不会序列化进请求体
 */
export async function fetchChatStream(data: {
  history_id: number;
  training_type: string;
  custom_training_id?: number;
  agent_id: number;
  model: string;
  messages: { role: string; content: string }[];
  signal?: AbortSignal;
}): Promise<Response> {
  const { signal, ...payload } = data;
  const isHttpProxy =
    import.meta.env.DEV && import.meta.env.VITE_HTTP_PROXY === "Y";
  const { baseURL } = getServiceBaseURL(import.meta.env, isHttpProxy);

  let Authorization = getAuthorization();

  let response = await fetch(`${baseURL}/api/chat`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: Authorization || "",
    },
    body: JSON.stringify(payload),
    signal,
  });

  // Handle token expiration - check first chunk for error codes
  if (response.ok) {
    try {
      const contentType = response.headers.get("content-type");
      if (contentType?.includes("text/event-stream")) {
        // For SSE streams, we check the response but don't consume the body
        // Just return the response as-is for streaming
        return response;
      }

      if (contentType?.includes("application/json")) {
        const errorData = await response.json();
        const expiredTokenCodes =
          import.meta.env.VITE_SERVICE_EXPIRED_TOKEN_CODES?.split(",") || [];

        if (expiredTokenCodes.includes(String(errorData.code))) {
          // Try to refresh token
          const success = await handleExpiredRequest(requestInstance.state);
          if (success) {
            // Retry with new token
            Authorization = getAuthorization();
            response = await fetch(`${baseURL}/api/chat`, {
              method: "POST",
              headers: {
                "Content-Type": "application/json",
                Authorization: Authorization || "",
              },
              body: JSON.stringify(payload),
              signal,
            });
          }
        }
      }
    } catch {
      // Continue normally if checking fails
    }
  }

  return response;
}

/**
 * Tool approval API - direct fetch with proper auth interceptors
 * Returns raw Response object with ReadableStream for SSE handling
 */
export async function fetchToolApproval(data: {
  checkpoint_id: string;
  interrupt_id: string;
  approved: boolean;
  reason?: string;
  history_id?: number;
  training_type?: string;
  custom_training_id?: number;
  agent_id?: number;
  messages?: { role: string; content: string }[];
}): Promise<Response> {
  const isHttpProxy =
    import.meta.env.DEV && import.meta.env.VITE_HTTP_PROXY === "Y";
  const { baseURL } = getServiceBaseURL(import.meta.env, isHttpProxy);

  let Authorization = getAuthorization();

  let response = await fetch(`${baseURL}/api/chat/tool-approval`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: Authorization || "",
    },
    body: JSON.stringify(data),
  });

  if (response.ok) {
    try {
      const contentType = response.headers.get("content-type");
      if (contentType?.includes("text/event-stream")) {
        return response;
      }

      if (contentType?.includes("application/json")) {
        const errorData = await response.json();
        const expiredTokenCodes =
          import.meta.env.VITE_SERVICE_EXPIRED_TOKEN_CODES?.split(",") || [];

        if (expiredTokenCodes.includes(String(errorData.code))) {
          const success = await handleExpiredRequest(requestInstance.state);
          if (success) {
            Authorization = getAuthorization();
            response = await fetch(`${baseURL}/api/chat/tool-approval`, {
              method: "POST",
              headers: {
                "Content-Type": "application/json",
                Authorization: Authorization || "",
              },
              body: JSON.stringify(data),
            });
          }
        }
      }
    } catch {
      // Continue normally if checking fails
    }
  }

  return response;
}
