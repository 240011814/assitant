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
