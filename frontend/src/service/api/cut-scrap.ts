import { request } from '../request';

/** 余料库存列表 (scrapType=0或不传=全部, 1=一维余料, 2=二维余料) */
export function fetchCutScraps(params?: { scrapType?: 0 | 1 | 2 }) {
  return request<Api.Cut.CutScrap[]>({
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
