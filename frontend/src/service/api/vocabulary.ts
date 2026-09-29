import { request } from '../request';

/**
 * 添加生词
 * @param data 生词信息
 */
export function fetchAddVocabulary(data: {
  word: string;
  phonetic?: string;
  definition?: string;
  example?: string;
  sourceContext?: string;
}) {
  return request<any>({
    url: '/api/vocabulary',
    method: 'post',
    data
  });
}

/**
 * 获取生词列表
 * @param params 查询参数, ids 为逗号分隔的主键串(如 '1,2,3'), 后端按主键过滤
 */
export function fetchGetVocabularyList(params?: { keyword?: string; isMastered?: boolean; ids?: string }) {
  return request<any[]>({
    url: '/api/vocabulary',
    method: 'get',
    params
  });
}

/**
 * 删除生词
 * @param id 生词 ID
 */
export function fetchDeleteVocabulary(id: number) {
  return request({
    url: `/api/vocabulary/${id}`,
    method: 'delete'
  });
}

/**
 * 更新生词
 * @param id 生词 ID
 * @param data 更新内容
 */
export function fetchUpdateVocabulary(id: number, data: {
  phonetic?: string;
  definition?: string;
  example?: string;
  isMastered?: boolean;
}) {
  return request({
    url: `/api/vocabulary/${id}`,
    method: 'put',
    data
  });
}

/**
 * 随机获取指定数量的生词
 * @param params 查询参数
 */
export function fetchGetRandomVocabulary(params: { count?: number; isMastered?: boolean }) {
  return request<any[]>({
    url: '/api/vocabulary/random',
    method: 'get',
    params
  });
}

/** 获取 SRS 到期复习词汇 */
export function fetchGetDueVocabulary(params?: { limit?: number }) {
  return request<Api.Vocabulary.Item[]>({
    url: '/api/vocabulary/review/due',
    method: 'get',
    params
  });
}

/** 获取 SRS 复习统计: 到期数 / 复习中词数 / 各盒子分布 */
export function fetchGetVocabularyReviewStats() {
  return request<Api.Vocabulary.ReviewStats>({
    url: '/api/vocabulary/review/stats',
    method: 'get'
  });
}

/**
 * 提交 SRS 复习结果, 返回更新后的词汇(含新盒子)
 * @param id 生词 ID
 * @param known 是否认识
 */
export function fetchSubmitVocabularyReview(id: number, known: boolean) {
  return request<Api.Vocabulary.Item>({
    url: `/api/vocabulary/review/${id}`,
    method: 'post',
    data: { known }
  });
}
