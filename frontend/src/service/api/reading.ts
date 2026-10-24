import { request } from '../request';

export interface ReadingDoc {
  id: number;
  title: string;
  sourceType: 'url' | 'text';
  sourceUrl: string;
  wordCount: number;
  unknownRate: number;
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
}

/** 慢 LLM 生成任务 (Cloudflare 对源站有 100s 上限, 此类操作一律异步 + 轮询) */
export interface AITextJob {
  id: number;
  kind: 'passage' | string;
  status: 'running' | 'done' | 'failed';
  resultId: number;
  error: string;
}

/** 导入阅读材料 (URL 或文本) */
export function fetchImportReading(data: { title?: string; sourceType: 'url' | 'text'; sourceUrl?: string; content?: string }) {
  return request<ReadingDoc>({
    url: '/api/reading/import',
    method: 'post',
    data
  });
}

/** 阅读文档列表 */
export function fetchGetReadingDocs() {
  return request<ReadingDoc[]>({
    url: '/api/reading/docs',
    method: 'get'
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

/** 点词即查 (词典缓存) */
export function fetchLookupWord(word: string) {
  return request<DictionaryEntry>({
    url: '/api/reading/lookup',
    method: 'get',
    params: { word }
  });
}

/** AI 写课文 (异步任务, 立即返回; 轮询 fetchGetGenerationJob 取结果) */
export function fetchGeneratePassage(data: { wordIds?: number[]; sentenceCount?: number }) {
  return request<AITextJob>({
    url: '/api/reading/generate-passage',
    method: 'post',
    data
  });
}

/** 查询慢生成任务状态 */
export function fetchGetGenerationJob(id: number) {
  return request<AITextJob>({
    url: `/api/reading/generation-jobs/${id}`,
    method: 'get'
  });
}
