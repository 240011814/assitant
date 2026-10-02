import { request } from '../request';

function buildQuery(params: Api.TokenUsage.StatsParams, extra: Record<string, string | number> = {}) {
  const query: Record<string, string | number> = {
    granularity: params.granularity || 'day',
    ...extra
  };
  if (params.model) query.model = params.model;
  if (params.userId != null) query.userId = params.userId;
  if (params.startTime) query.startTime = params.startTime;
  if (params.endTime) query.endTime = params.endTime;
  return query;
}

/** Token 用量统计: 汇总 + 趋势 + 模型分布 + 用户排行 */
export function fetchTokenUsageStats(params: Api.TokenUsage.StatsParams) {
  return request<Api.TokenUsage.Stats>({
    url: '/api/admin/token-usages/stats',
    method: 'get',
    params: buildQuery(params)
  });
}

/** Token 用量明细分页 */
export function fetchTokenUsageRecords(params: Api.TokenUsage.ListParams) {
  return request<Api.TokenUsage.ListResult>({
    url: '/api/admin/token-usages',
    method: 'get',
    params: buildQuery(params, { page: params.page, page_size: params.page_size })
  });
}
