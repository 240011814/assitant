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

/** 扣减一维余料库存 (每条 quantity-1, 归零自动删除), 返回扣减后的一维库存列表 */
export function fetchConsumeCutScraps(ids: number[]) {
  return request<Api.Cut.CutScrap[]>({
    url: '/api/cut/scraps/consume',
    method: 'post',
    data: { ids }
  });
}
