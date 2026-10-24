import { request } from '../request';

/**
 * 添加错题
 * @param data 错题信息
 */
export function fetchAddErrorBook(data: {
  contentType: 'word' | 'sentence';
  content: string;
  translation?: string;
  sourceType?: string;
  sourceId?: number;
}) {
  return request<Api.ErrorBook.Item>({
    url: '/api/error-book',
    method: 'post',
    data
  });
}

/**
 * 获取错题本列表
 * @param params 查询参数
 */
export function fetchGetErrorBookList(params?: {
  sourceType?: string;
  keyword?: string;
  isMastered?: boolean;
}) {
  return request<Api.ErrorBook.Item[]>({
    url: '/api/error-book',
    method: 'get',
    params
  });
}

/**
 * 获取错题练习数据
 * @param params 查询参数
 */
export function fetchGetErrorBookForPractice(params?: {
  contentType?: string;
}) {
  return request<Api.ErrorBook.Item[]>({
    url: '/api/error-book/practice',
    method: 'get',
    params
  });
}

/**
 * 获取错题统计
 */
export function fetchGetErrorBookStats() {
  return request<{
    total: number;
    mastered: number;
    unmastered: number;
    wordCount: number;
    sentenceCount: number;
  }>({
    url: '/api/error-book/stats',
    method: 'get'
  });
}

/**
 * 更新错题状态
 * @param id 错题 ID
 * @param data 更新内容
 */
export function fetchUpdateErrorBook(id: number, data: {
  isMastered?: boolean;
}) {
  return request({
    url: `/api/error-book/${id}`,
    method: 'put',
    data
  });
}

/**
 * 删除错题
 * @param id 错题 ID
 */
export function fetchDeleteErrorBook(id: number) {
  return request({
    url: `/api/error-book/${id}`,
    method: 'delete'
  });
}

/**
 * 随机获取指定数量的错题
 * @param params 查询参数
 */
export function fetchGetRandomErrorBooks(params: { count?: number; contentType?: string }) {
  return request<Api.ErrorBook.Item[]>({
    url: '/api/error-book/random',
    method: 'get',
    params
  });
}
