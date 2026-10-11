import { request } from '../request';

/** 产品单列表 (仅本人, 新单在前; 带分页参数时服务端分页, 省略时返回全部) */
export function fetchCutProducts(params?: Api.Cut.CutProductSearchParams) {
  return request<Api.Cut.CutProductListResponse>({
    url: '/api/cut/products',
    method: 'get',
    params
  });
}

/** 新增/更新产品单 (带 id 为更新) */
export function saveCutProduct(data: Api.Cut.SaveCutProductRequest) {
  return request<Api.Cut.CutProduct>({
    url: data.id ? `/api/cut/products/${data.id}` : '/api/cut/products',
    method: data.id ? 'put' : 'post',
    data
  });
}

/** 删除产品单 */
export function deleteCutProduct(id: number) {
  return request<null>({
    url: `/api/cut/products/${id}`,
    method: 'delete'
  });
}
