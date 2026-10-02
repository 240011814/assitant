import { request } from '../request';
import { getAuthorization, handleExpiredRequest } from '../request/shared';
import { getServiceBaseURL } from '@/utils/service';
import { request as requestInstance } from '../request';

/** 编排 DSL 节点 */
export interface OrchNode {
  id: string;
  type: 'agent' | 'tool' | 'template' | 'branch' | 'router' | 'extract' | 'merge' | 'end' | 'subagent' | 'suborch';
  name: string;
  config: Record<string, any>;
  position?: { x: number; y: number };
}

/** 编排 DSL 连线 */
export interface OrchEdge {
  source: string;
  target: string;
  label?: string;
  /** kind 连线类型: flow(默认) 主流边; loop 循环回边 (分支/LLM路由指向更早节点, 限次执行) */
  kind?: 'flow' | 'loop';
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
  /** error_items 结构化错误: node_id 非空时可定位画布节点 (高亮/选中) */
  error_items?: { node_id?: string; message: string }[];
}

export interface OrchestrationResource {
  tools: { name: string; display_name: string; description: string; confirm_required: boolean }[];
  models: { model_code: string; display_name: string; is_default: boolean }[];
  /** agents 仅包含允许作为编排子Agent 的 Agent (agent_type=subagent 或有委派说明) */
  agents: {
    id: number;
    title: string;
    description: string;
    system_prompt: string;
    agent_type?: string;
    delegation_description?: string;
  }[];
  skills: { name: string; description: string }[];
  /** orchestrations 子编排节点可引用的已保存编排 (含未启用的, 运行时会报错) */
  orchestrations: { id: number; name: string; description: string; enabled: boolean }[];
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
 * 事件: start / delta / reasoning / node / summary / error / done
 * `history` 为之前轮次 (不含本轮 input), 用于多轮对话式调试
 * `signal` 用于中断调试运行
 */
export async function fetchOrchestrationDebugRun(data: {
  id?: number;
  definition: string;
  input: string;
  history?: { role: string; content: string }[];
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

/** 训练中心「编排对话」精简信息 */
export interface OrchestrationChatItem {
  id: number;
  name: string;
  description: string;
}

/** 训练中心可对话的编排列表 (仅已启用) */
export function fetchOrchestrationChatList() {
  return request<OrchestrationChatItem[]>({ url: '/api/ai-orchestrations' });
}

/** 训练中心编排详情 (精简) */
export function fetchOrchestrationChatItem(id: number) {
  return request<OrchestrationChatItem>({ url: `/api/ai-orchestrations/${id}` });
}

/**
 * 训练中心「编排对话」- SSE 流式运行 (复用编排调试事件: start/delta/reasoning/node/summary/error/done)
 * `history` 为之前轮次 (不含本轮 input), `signal` 用于中断
 */
export async function fetchOrchestrationChatRun(data: {
  id: number;
  input: string;
  history?: { role: string; content: string }[];
  /** 已有会话的 history_id (0/undefined 表示新会话) */
  historyId?: number;
  signal?: AbortSignal;
}): Promise<Response> {
  const { signal } = data;
  const isHttpProxy = import.meta.env.DEV && import.meta.env.VITE_HTTP_PROXY === 'Y';
  const { baseURL } = getServiceBaseURL(import.meta.env, isHttpProxy);

  const url = `${baseURL}/api/ai-orchestrations/${data.id}/chat`;
  const body = JSON.stringify({
    input: data.input,
    history: data.history,
    history_id: data.historyId || 0,
  });

  let Authorization = getAuthorization();

  let response = await fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: Authorization || '',
    },
    body,
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
            response = await fetch(url, {
              method: 'POST',
              headers: {
                'Content-Type': 'application/json',
                Authorization: Authorization || '',
              },
              body,
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
