import { request } from '../request';

/** 产品单列表 (本人全部, 新单在前) */
export function fetchCutProducts() {
  return request<Api.Cut.CutProduct[]>({
    url: '/api/cut/products',
    method: 'get'
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
