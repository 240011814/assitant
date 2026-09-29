import { request } from '../request';

export interface AIAgent {
  id: number;
  user_id: number;
  is_public: boolean;
  permission_code: string;
  title: string;
  description: string;
  code: string;
  system_prompt: string;
  icon: string;
  color: string;
  initial_message: string;
  input_placeholder: string;
  speech_lang: string;
  speech_rate: number;
  is_favorite: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateAIAgentParams {
  title: string;
  description?: string;
  code?: string;
  system_prompt: string;
  icon?: string;
  color?: string;
  initial_message?: string;
  input_placeholder?: string;
  speech_lang?: string;
  speech_rate?: number;
}

export interface UpdateAIAgentParams {
  title?: string;
  description?: string;
  system_prompt?: string;
  icon?: string;
  color?: string;
  initial_message?: string;
  input_placeholder?: string;
  speech_lang?: string;
  speech_rate?: number;
}

export function fetchAIAgentList() {
  return request<AIAgent[]>({
    url: '/api/ai-agents',
    method: 'get'
  });
}

export function fetchAIAgentDetail(id: number) {
  return request<AIAgent>({
    url: `/api/ai-agents/${id}`,
    method: 'get'
  });
}

export function fetchCreateAIAgent(data: CreateAIAgentParams) {
  return request<AIAgent>({
    url: '/api/ai-agents',
    method: 'post',
    data
  });
}

export function fetchUpdateAIAgent(id: number, data: UpdateAIAgentParams) {
  return request({
    url: `/api/ai-agents/${id}`,
    method: 'put',
    data
  });
}

export function fetchDeleteAIAgent(id: number) {
  return request({
    url: `/api/ai-agents/${id}`,
    method: 'delete'
  });
}
