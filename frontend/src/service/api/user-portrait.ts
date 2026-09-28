import { request } from '../request';

export interface UserPortrait {
  summary: string;
  dimensions: Record<string, string>;
  tags: string[];
  confidence: number;
  is_user_edited: boolean;
  extraction_enabled: boolean;
  updated_at: string | null;
}

export interface UserExperience {
  id: number;
  history_id: number | null;
  domain: string;
  event_type: string;
  title: string;
  content: string;
  time_range: string;
  tags: string[];
  occurred_at: string | null;
  confidence: number;
  importance: number;
  memory_level: string;
  evidence: string;
  status: string;
  is_user_edited: boolean;
  created_at: string;
}

export interface UserPortraitResponse {
  portrait: UserPortrait | null;
  experiences: UserExperience[];
}

export interface ListUserExperienceParams {
  page: number;
  pageSize: number;
  domain?: string;
  keyword?: string;
}

export interface ListUserExperienceResponse {
  total: number;
  items: UserExperience[];
}

export interface UpdateUserPortraitParams {
  summary?: string;
  dimensions?: Record<string, string>;
  tags?: string[];
  extraction_enabled?: boolean;
}

export interface CreateUserExperienceParams {
  domain?: string;
  event_type?: string;
  title: string;
  content?: string;
  time_range?: string;
  occurred_at?: string | null;
  tags?: string[];
  importance?: number;
  memory_level?: string;
  evidence?: string;
  status?: string;
}

export interface UpdateUserExperienceParams {
  domain?: string;
  event_type?: string;
  title?: string;
  content?: string;
  time_range?: string;
  occurred_at?: string | null;
  tags?: string[];
  importance?: number;
  memory_level?: string;
  evidence?: string;
  status?: string;
}

/** 获取用户画像与经历 */
export function fetchGetUserPortrait() {
  return request<UserPortraitResponse>({ url: '/api/user/portrait', method: 'get' });
}

/** 保存用户画像 */
export function fetchUpdateUserPortrait(data: UpdateUserPortraitParams) {
  return request<null>({ url: '/api/user/portrait', method: 'put', data });
}

/** 手动触发画像/经历抽取 */
export function fetchTriggerPortraitExtract() {
  return request<{ extracted: number }>({ url: '/api/user/portrait/extract', method: 'post' });
}

/** 分页获取经历 */
export function fetchListUserExperiences(params: ListUserExperienceParams) {
  return request<ListUserExperienceResponse>({ url: '/api/user/experiences', method: 'get', params });
}

/** 新增经历 */
export function fetchCreateUserExperience(data: CreateUserExperienceParams) {
  return request<UserExperience>({ url: '/api/user/experiences', method: 'post', data });
}

/** 更新经历 */
export function fetchUpdateUserExperience(id: number, data: UpdateUserExperienceParams) {
  return request<null>({ url: `/api/user/experiences/${id}`, method: 'put', data });
}

/** 删除经历 */
export function fetchDeleteUserExperience(id: number) {
  return request<null>({ url: `/api/user/experiences/${id}`, method: 'delete' });
}