import { request } from '../request';

/** 余料库存列表 (分页+筛选; 不传分页参数 = 返回全部, 上限 500) */
export function fetchCutScraps(params?: Api.Cut.CutScrapSearchParams) {
  return request<Api.Cut.CutScrapListResponse>({
    url: '/api/cut/scraps',
    method: 'get',
    params
  });
}

/** 余料批量入库 (请求体为数组) */
export function addCutScraps(data: Api.Cut.AddCutScrapRequest[]) {
  return request<Api.Cut.CutScrap[]>({
    url: '/api/cut/scraps',
    method: 'post',
    data
  });
}

/** 删除余料库存 */
export function deleteCutScrap(id: number) {
  return request<null>({
    url: `/api/cut/scraps/${id}`,
    method: 'delete'
  });
}


/** 修改库存余料 (数量/名称/材料类型/备注) */
export function updateCutScrap(id: number, data: Api.Cut.UpdateCutScrapRequest) {
  return request<null>({
    url: `/api/cut/scraps/${id}`,
    method: 'put',
    data
  });
}

/** 批量删除库存余料 (返回实际删除条数) */
export function batchDeleteCutScraps(data: Api.Cut.BatchDeleteCutScrapRequest) {
  return request<{ deleted: number }>({
    url: '/api/cut/scraps/batch-delete',
    method: 'post',
    data
  });
}
