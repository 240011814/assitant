import { request } from '../request';

export interface DashboardStats {
  today_messages: number;
  total_messages: number;
  total_vocabulary: number;
  total_notes: number;
  total_favorites: number;
  training_trend: TrendItem[];
  training_type_stats: TypeStatItem[];
}

export interface TrendItem {
  date: string;
  count: number;
}

export interface TypeStatItem {
  type: string;
  count: number;
}

export function fetchDashboardStats() {
  return request<DashboardStats>({
    url: '/api/dashboard/stats',
    method: 'get'
  });
}

export interface AdminDashboardStats {
  total_users: number;
  new_users_7d: number;
  recent_logins_24h: number;
  messages_today: number;
  total_conversations: number;
  enabled_jobs: number;
  failed_runs_24h: number;
  operations_today: number;
  failed_ops_today: number;
  ops_trend: TrendItem[];
}

export function fetchAdminDashboardStats() {
  return request<AdminDashboardStats>({
    url: '/api/admin/dashboard',
    method: 'get'
  });
}
