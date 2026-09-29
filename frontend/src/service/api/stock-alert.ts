import { request } from '../request'

/** 获取预警规则列表 */
export function getStockAlerts() {
  return request<Api.Stock.StockAlert[]>({
    url: '/api/stock/alerts',
    method: 'get'
  })
}

/** 新建预警规则 */
export function createStockAlert(data: {
  code: string
  name?: string
  rule_type: Api.Stock.StockAlertRuleType
  threshold: number
}) {
  return request<Api.Stock.StockAlert>({
    url: '/api/stock/alerts',
    method: 'post',
    data
  })
}

/** 更新预警规则 (nil 字段不更新) */
export function updateStockAlert(id: number, data: {
  name?: string
  rule_type?: Api.Stock.StockAlertRuleType
  threshold?: number
  enabled?: boolean
}) {
  return request<null>({
    url: `/api/stock/alerts/${id}`,
    method: 'put',
    data
  })
}

/** 删除预警规则 */
export function deleteStockAlert(id: number) {
  return request<null>({
    url: `/api/stock/alerts/${id}`,
    method: 'delete'
  })
}
