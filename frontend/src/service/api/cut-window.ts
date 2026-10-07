import { request } from '../request';

/** 窗户单列表 (本人全部, 新单在前) */
export function fetchCutWindows() {
  return request<Api.Cut.CutWindow[]>({
    url: '/api/cut/windows',
    method: 'get'
  });
}

/** 新增/更新窗户单 (带 id 为更新) */
export function saveCutWindow(data: Api.Cut.SaveCutWindowRequest) {
  return request<Api.Cut.CutWindow>({
    url: data.id ? `/api/cut/windows/${data.id}` : '/api/cut/windows',
    method: data.id ? 'put' : 'post',
    data
  });
}

/** 删除窗户单 */
export function deleteCutWindow(id: number) {
  return request<null>({
    url: `/api/cut/windows/${id}`,
    method: 'delete'
  });
}
