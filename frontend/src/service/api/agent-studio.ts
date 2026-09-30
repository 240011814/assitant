import { request } from '../request';
import { getAuthorization, handleExpiredRequest } from '../request/shared';
import { getServiceBaseURL } from '@/utils/service';
import { request as requestInstance } from '../request';

/** 编排 DSL 节点 */
export interface OrchNode {
  id: string;
  type: 'agent' | 'tool' | 'template' | 'branch' | 'merge' | 'end';
  name: string;
  config: Record<string, any>;
  position?: { x: number; y: number };
}

/** 编排 DSL 连线 */
export interface OrchEdge {
  source: string;
  target: string;
  label?: string;
}

/** 编排 DSL (与后端 model.OrchestrationDSL 对应) */
export interface OrchestrationDSL {
  version: number;
  nodes: OrchNode[];
  edges: OrchEdge[];
}

export interface AIOrchestrationItem {
  id: number;
  name: string;
  description: string;
  definition: string;
  version: number;
  enabled: boolean;
  last_debug_summary?: string;
  created_at: string;
  updated_at: string;
}

export interface OrchestrationValidateResult {
  valid: boolean;
  mode: string;
  errors?: string[];
  warnings?: string[];
}

export interface OrchestrationResource {
  tools: { name: string; display_name: string; description: string; confirm_required: boolean }[];
  models: { model_code: string; display_name: string; is_default: boolean }[];
  agents: { id: number; title: string; description: string; system_prompt: string }[];
  skills: { name: string; description: string }[];
  node_types: { type: string; label: string; desc: string }[];
}

export function fetchOrchestrations() {
  return request<AIOrchestrationItem[]>({ url: '/api/agent-studio/orchestrations' });
}

export function fetchOrchestration(id: number) {
  return request<AIOrchestrationItem>({ url: `/api/agent-studio/orchestrations/${id}` });
}

export function fetchCreateOrchestration(data: { name: string; description: string; definition: string; enabled: boolean }) {
  return request<AIOrchestrationItem>({ url: '/api/agent-studio/orchestrations', method: 'post', data });
}

export function fetchUpdateOrchestration(id: number, data: { name?: string; description?: string; definition?: string; enabled?: boolean }) {
  return request<null>({ url: `/api/agent-studio/orchestrations/${id}`, method: 'put', data });
}

export function fetchDeleteOrchestration(id: number) {
  return request<null>({ url: `/api/agent-studio/orchestrations/${id}`, method: 'delete' });
}

export function fetchValidateOrchestration(definition: string) {
  return request<OrchestrationValidateResult>({
    url: '/api/agent-studio/orchestrations/validate',
    method: 'post',
    data: { definition },
  });
}

export function fetchOrchestrationResources() {
  return request<OrchestrationResource>({ url: '/api/agent-studio/resources' });
}

/**
 * 编排调试运行 - SSE 流式返回节点级事件
 * 事件: start / node / delta / summary / error / done
 * `signal` 用于中断调试运行
 */
export async function fetchOrchestrationDebugRun(data: {
  id?: number;
  definition: string;
  input: string;
  signal?: AbortSignal;
}): Promise<Response> {
  const { signal, ...payload } = data;
  const isHttpProxy = import.meta.env.DEV && import.meta.env.VITE_HTTP_PROXY === 'Y';
  const { baseURL } = getServiceBaseURL(import.meta.env, isHttpProxy);

  let Authorization = getAuthorization();

  let response = await fetch(`${baseURL}/api/agent-studio/debug`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: Authorization || '',
    },
    body: JSON.stringify(payload),
    signal,
  });

  if (response.ok) {
    try {
      const contentType = response.headers.get('content-type');
      if (contentType?.includes('text/event-stream')) {
        return response;
      }
      if (contentType?.includes('application/json')) {
        const errorData = await response.json();
        const expiredTokenCodes = import.meta.env.VITE_SERVICE_EXPIRED_TOKEN_CODES?.split(',') || [];
        if (expiredTokenCodes.includes(String(errorData.code))) {
          const success = await handleExpiredRequest(requestInstance.state);
          if (success) {
            Authorization = getAuthorization();
            response = await fetch(`${baseURL}/api/agent-studio/debug`, {
              method: 'POST',
              headers: {
                'Content-Type': 'application/json',
                Authorization: Authorization || '',
              },
              body: JSON.stringify(payload),
              signal,
            });
          }
        }
      }
    } catch {
      // token 检查失败不阻断流式请求
    }
  }

  return response;
}
