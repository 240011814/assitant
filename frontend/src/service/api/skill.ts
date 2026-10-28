import { request } from '../request';

export interface AISkillItem {
  id: number;
  name: string;
  description: string;
  context: string;
  agent: string;
  model: string;
  content: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface AISkillCreateRequest {
  name: string;
  description: string;
  context: string;
  agent: string;
  model: string;
  content: string;
  enabled: boolean;
}

export interface DiscoveredSkill {
  name: string;
  description: string;
  context: string;
  agent: string;
  model: string;
  content: string;
  path: string;
  repo_url: string;
}

export function fetchSkills() {
  return request<AISkillItem[]>({ url: '/api/skills' });
}

export interface SkillDiscoverResult {
  repo: string;
  total: number;
  cached: boolean;
  warning?: string;
  skills: DiscoveredSkill[];
}

export function fetchDiscoverGithubSkills(data: { repo: string; ref?: string; path?: string }) {
  // 超时走后台配置 (system_config api_timeout_overrides, 种子 600s: 扫描仓库需逐个拉取 SKILL.md)
  return request<SkillDiscoverResult>({ url: '/api/skills/discover/github', method: 'post', data });
}

// 读取上次扫描成功的内存缓存 (不触发扫描)
export function fetchGithubSkillsCache(params: { repo: string; ref?: string; path?: string }) {
  return request<SkillDiscoverResult | null>({ url: '/api/skills/discover/github/cache', params });
}

export function fetchCreateSkill(data: AISkillCreateRequest) {
  return request<AISkillItem>({ url: '/api/skills', method: 'post', data });
}

export function fetchUpdateSkill(id: number, data: Partial<AISkillCreateRequest>) {
  return request<null>({ url: `/api/skills/${id}`, method: 'put', data });
}

export function fetchDeleteSkill(id: number) {
  return request<null>({ url: `/api/skills/${id}`, method: 'delete' });
}