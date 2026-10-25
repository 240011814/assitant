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
  /** 来源: reading / rss (回链原文) */
  sourceKind?: string;
  sourceRef?: number;
  /** 加入时所在原句 (语境复习方向) */
  contextSentence?: string;
}) {
  return request<Api.Vocabulary.Item>({
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
  return request<Api.Vocabulary.Item[]>({
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
  return request<Api.Vocabulary.Item[]>({
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
 * 提交 SRS 复习结果 (旧接口: 认识/不认识), 返回更新后的词汇
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

/** 获取复习会话: 到期复习 + 新词 (受每日限额约束, 卡片带方向) */
export function fetchGetReviewSession(params?: { limit?: number }) {
  return request<Api.Vocabulary.ReviewSession>({
    url: '/api/vocabulary/review/session',
    method: 'get',
    params
  });
}

/** 碎片时间模式: 按 ROI 挑词 (?minutes=5) */
export function fetchGetQuickSession(params?: { minutes?: number }) {
  return request<Api.Vocabulary.ReviewSession>({
    url: '/api/vocabulary/quick-session',
    method: 'get',
    params
  });
}

/** 未来 N 天到期预测 */
export function fetchGetReviewForecast(params?: { days?: number }) {
  return request<Api.Vocabulary.Forecast>({
    url: '/api/vocabulary/review/forecast',
    method: 'get',
    params
  });
}

/** 学习看板: 热力图/保持率/连续天数/易错词 */
export function fetchGetLearningStats() {
  return request<Api.Vocabulary.LearningStats>({
    url: '/api/vocabulary/learning-stats',
    method: 'get'
  });
}

/**
 * 提交 FSRS 复习结果 (1=忘记 2=困难 3=良好 4=简单)
 * @param direction 复习方向 en2zh/zh2en/listen
 */
export function fetchSubmitReviewFSRS(
  id: number,
  rating: 1 | 2 | 3 | 4,
  direction?: Api.Vocabulary.ReviewDirection,
  durationMs?: number
) {
  return request<Api.Vocabulary.Item>({
    url: `/api/vocabulary/review/${id}`,
    method: 'post',
    data: { rating, direction, durationMs }
  });
}

/** 遗忘干预: 为反复遗忘的词生成新例句与助记提示 */
export function fetchAssistVocabulary(id: number) {
  return request<Api.Vocabulary.Item>({
    url: `/api/vocabulary/${id}/assist`,
    method: 'get'
  });
}

/** 词书批量导入 (JSON 结构化条目) */
export function fetchImportVocabulary(data: { words: Api.Vocabulary.ImportItem[]; sourceTag?: string }) {
  return request<Api.Vocabulary.ImportResult>({
    url: '/api/vocabulary/import',
    method: 'post',
    data
  });
}

/** 词书文件导入 (txt/csv, 每行一词, 制表符或逗号分隔) */
export function fetchImportVocabularyFile(file: File, sourceTag?: string) {
  const formData = new FormData();
  formData.append('file', file);
  if (sourceTag) {
    formData.append('sourceTag', sourceTag);
  }
  return request<Api.Vocabulary.ImportResult>({
    url: '/api/vocabulary/import',
    method: 'post',
    data: formData,
    headers: { 'Content-Type': 'multipart/form-data' }
  });
}

/** 知识库文档提词入库 */
export function fetchExtractVocabularyFromDocument(data: { documentId: number; limit?: number }) {
  return request<Api.Vocabulary.ImportResult>({
    url: '/api/vocabulary/import/document',
    method: 'post',
    data
  });
}
