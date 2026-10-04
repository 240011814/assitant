import { request } from '../request';

/** 定时 Agent 任务 (到点让 Agent/编排带着输入跑一遍, 结果落训练历史) */
export interface AgentTaskItem {
  id: number;
  name: string;
  scheduleType: 'once' | 'cron';
  cronExpr: string;
  runAt: string | null;
  enabled: boolean;
  lastRunError?: string;
  next_run_at: string | null;
  params_data: {
    agent_id: number;
    agent_type: 'chat' | 'orchestration';
    input: string;
    notify_email: boolean;
    notify_telegram: boolean;
  };
  createdAt: string;
  updatedAt: string;
}

export interface CreateAgentTaskPayload {
  agent_id: number;
  agent_type: string;
  input: string;
  schedule_type: 'once' | 'cron';
  run_at?: string | null;
  cron_expr?: string;
  notify_email?: boolean;
  notify_telegram?: boolean;
  enabled?: boolean;
}

export type UpdateAgentTaskPayload = Partial<CreateAgentTaskPayload>;

/** 任务列表 (含下次执行时间) */
export function fetchAgentTasks() {
  return request<{ items: AgentTaskItem[] }>({ url: '/api/agent-tasks', method: 'get' });
}

export function fetchCreateAgentTask(data: CreateAgentTaskPayload) {
  return request<AgentTaskItem>({ url: '/api/agent-tasks', method: 'post', data });
}

export function fetchUpdateAgentTask(id: number, data: UpdateAgentTaskPayload) {
  return request<AgentTaskItem>({ url: `/api/agent-tasks/${id}`, method: 'put', data });
}

export function fetchDeleteAgentTask(id: number) {
  return request<null>({ url: `/api/agent-tasks/${id}`, method: 'delete' });
}

/** 手动立即执行一次 */
export function fetchRunAgentTask(id: number) {
  return request<{ message: string }>({ url: `/api/agent-tasks/${id}/run`, method: 'post' });
}
