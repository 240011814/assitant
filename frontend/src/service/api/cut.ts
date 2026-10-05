import { request } from '../request';

export function cutBar(data: Api.Cut.BarRequest) {
  return request<Api.Cut.BarCutResponse>({
    url: '/api/cut/bar',
    method: 'post',
    data
  });
}

export function cutBin(data: Api.Cut.BinRequest) {
  return request<Api.Cut.PlaneCutResponse>({
    url: '/api/cut/plane',
    method: 'post',
    data,
    // 精确模式 (OR-Tools) 服务端限时 2 分钟, 放宽到 3 分钟留出传输余量
    timeout: 180 * 1000
  });
}

export function addRecord(data: Api.Cut.RecordRequest) {
  return request<Api.Cut.CutRecord>({
    url: '/api/cutRecord/add',
    method: 'post',
    data
  });
}

export function cutList(params?: Api.Cut.CutRecordSearchParams) {
  return request<Api.Common.PaginatingQueryRecord<Api.Common.CommonRecord<Api.Cut.CutRecord>>>({
    url: '/api/cutRecord/list',
    method: 'get',
    params
  });
}

export function deleteRecod(id: string) {
  return request<boolean>({
    url: `/api/cutRecord/delete/${id}`,
    method: 'post'
  });
}
