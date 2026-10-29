import { request } from '../request'

/** 操作审计日志列表 (分页 + 筛选) */
export function fetchAuditLogs(params: Api.AuditLog.AuditLogListParams) {
  return request<{
    list: Api.AuditLog.OperationAuditLog[]
    total: number
    page: number
    page_size: number
  }>({
    url: '/api/admin/audit-logs',
    method: 'get',
    params
  })
}

/** 异步任务列表 (审计日志页「异步任务」Tab, 全量用户, 含输入快照) */
export function fetchAdminAsyncJobs(params: Api.AsyncJob.AdminListParams) {
  return request<{
    list: Api.AsyncJob.AdminJob[]
    total: number
    page: number
    page_size: number
  }>({
    url: '/api/admin/async-jobs',
    method: 'get',
    params
  })
}
