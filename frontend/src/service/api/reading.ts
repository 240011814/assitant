import { request } from '../request';

export interface ReadingDoc {
  id: number;
  title: string;
  sourceType: 'url' | 'text';
  sourceUrl: string;
  wordCount: number;
  unknownRate: number;
  /** 导入时预估生词率 (免 LLM 个性化难度) */
  estUnknownRate: number;
  /** 分块标注进度 0-100 */
  annotateProgress: number;
  status: 'pending' | 'annotating' | 'annotated' | 'failed';
  createdAt: string;
}

export interface DifficultWord {
  word: string;
  meaning: string;
  reason?: string;
  /** 是否已在用户生词本 */
  known: boolean;
}

export interface ReadingDocDetail {
  doc: ReadingDoc & { content: string };
  difficultWords: DifficultWord[];
}

export interface DictionaryEntry {
  word: string;
  phonetic: string;
  definition: string;
  translation: string;
  example: string;
  /** 是否已在生词本 (查词命中时后端顺带计一次阅读相遇) */
  inVocabulary?: boolean;
  /** 阅读中已相遇次数 */
  encounters?: number;
}

/** 通用异步任务 (Cloudflare 对源站有 100s 上限, 慢 LLM 操作一律异步 + 轮询 /api/async-jobs/:id) */
export interface AsyncJob {
  id: number;
  kind: 'passage' | 'vocab_test' | 'vocab_adaptive' | 'rss_annotate' | 'rss_translate' | string;
  status: 'running' | 'done' | 'failed';
  resultId: number;
  /** 进度文本 (如 "生成题目 8/25") */
  progress?: string;
  /** 任务结果 JSON (vocab_adaptive 下发前已剥离答案: {questions:[...]}) */
  resultJson?: string;
  error?: string;
}

/** 词汇量测试题 (下发版, 无答案) */
export interface VocabTestQuestion {
  id: string;
  band: string;
  word: string;
  options: string[];
}

/** 词汇量测试结果 */
export interface VocabTestResult {
  cefr: string;
  estSize: number;
  bands: Record<string, string>;
}

/** 阅读维度统计 (近90天) */
export interface ReadingStats {
  totalOpens: number;
  totalLookups: number;
  totalWordsAdded: number;
  totalReadMinutes: number;
  daily: Array<{ date: string; opens: number; lookups: number; wordsAdded: number; readMinutes: number }>;
}

/** 导入阅读材料 (URL 或文本) */
export function fetchImportReading(data: { title?: string; sourceType: 'url' | 'text'; sourceUrl?: string; content?: string }) {
  return request<ReadingDoc>({
    url: '/api/reading/import',
    method: 'post',
    data
  });
}

/** 阅读文档列表 (keyword 模糊匹配标题与正文) */
export function fetchGetReadingDocs(params?: { keyword?: string }) {
  return request<ReadingDoc[]>({
    url: '/api/reading/docs',
    method: 'get',
    params
  });
}

/** 阅读文档详情 (含标注) */
export function fetchGetReadingDoc(id: number) {
  return request<ReadingDocDetail>({
    url: `/api/reading/docs/${id}`,
    method: 'get'
  });
}

/** 删除阅读文档 */
export function fetchDeleteReadingDoc(id: number) {
  return request({
    url: `/api/reading/docs/${id}`,
    method: 'delete'
  });
}

/** LLM 标注生词 (异步启动, 立即返回 annotating; 轮询 fetchGetReadingDoc 取结果) */
export function fetchAnnotateReadingDoc(id: number) {
  return request<ReadingDoc>({
    url: `/api/reading/docs/${id}/annotate`,
    method: 'post'
  });
}

/** 点词即查 (词典缓存优先; 命中生词本返回 inVocabulary + encounters) */
export function fetchLookupWord(word: string) {
  return request<DictionaryEntry>({
    url: '/api/reading/lookup',
    method: 'get',
    params: { word }
  });
}

/** AI 写课文 (异步任务, 立即返回; 轮询 fetchGetAsyncJob 取结果) */
export function fetchGeneratePassage(data: { wordIds?: number[]; sentenceCount?: number }) {
  return request<AsyncJob>({
    url: '/api/reading/generate-passage',
    method: 'post',
    data
  });
}

/** 查询通用异步任务状态 (AI 写课文/词汇测试/RSS 生词提取/RSS 对照翻译等) */
export function fetchGetAsyncJob(id: number) {
  return request<AsyncJob>({
    url: `/api/async-jobs/${id}`,
    method: 'get'
  });
}

/** 划词翻译 (双向: 中文→英文 / 其他→中文) */
export function fetchTranslateText(text: string) {
  return request<{ translation: string }>({
    url: '/api/reading/translate',
    method: 'post',
    data: { text }
  });
}

/** 阅读时长心跳 (每 30s 上报停留秒数, 聚合进学习统计) */
export function fetchReadingHeartbeat(seconds: number) {
  return request({
    url: '/api/reading/heartbeat',
    method: 'post',
    data: { seconds }
  });
}

/** 启动自适应词汇量测试 (锚定词库抽样 + 题目池生成, 异步出题) */
export function fetchStartAdaptiveVocabTest() {
  return request<AsyncJob>({
    url: '/api/reading/vocab-test/adaptive/start',
    method: 'post'
  });
}

/** 自适应测试: 单题下发版 (无答案) */
export interface AdaptiveQuestion {
  id: string;
  band: string;
  word: string;
  options: string[];
}

/** 自适应测试: 单题作答 */
export interface AdaptiveRoundAnswer {
  questionId: string;
  choice: number;
}

/** 自适应测试: 单档作答明细 */
export interface VocabBandStat {
  asked: number;
  correct: number;
}

/** 自适应测试结果 (区间 + 中点, 不给假精度) */
export interface AdaptiveVocabResult {
  cefr: string;
  estSize: number;
  estLow: number;
  estHigh: number;
  bands: Record<string, VocabBandStat>;
  rounds: number;
  suspicious: boolean;
}

/** 自适应测试: 逐轮响应 (未收敛给下一轮题目, 收敛给最终结果) */
export interface AdaptiveRoundResponse {
  finished: boolean;
  round: number;
  probeBand: string;
  questions?: AdaptiveQuestion[];
  result?: AdaptiveVocabResult;
}

/** 自适应测试: 逐轮提交 (rounds 为历史各轮作答, 含本轮; 服务端重放二分路径) */
export function fetchSubmitAdaptiveRound(data: { jobId: number; durationMs: number; rounds: AdaptiveRoundAnswer[][] }) {
  return request<AdaptiveRoundResponse>({
    url: '/api/reading/vocab-test/adaptive/round',
    method: 'post',
    data
  });
}

/** 行为数据滚动校准结果 */
export interface VocabBehaviorEstimate {
  frontierBand: string;
  estCefr: string;
  confidence: 'low' | 'mid' | 'high' | string;
  taggedCount: number;
  bandCounts: Record<string, number>;
  trend: '' | 'up' | 'down' | 'flat' | string;
  note: string;
}

/** 行为数据滚动校准 (生词本档位分布 + 查词密度趋势) */
export function fetchVocabBehavior() {
  return request<VocabBehaviorEstimate>({
    url: '/api/reading/vocab-test/behavior',
    method: 'get'
  });
}

/** 最近一次词汇量评估 (无记录返回 null) */
export function fetchLatestVocabTest() {
  return request<{ cefr: string; estSize: number; createdAt: string } | null>({
    url: '/api/reading/vocab-test/latest',
    method: 'get'
  });
}
