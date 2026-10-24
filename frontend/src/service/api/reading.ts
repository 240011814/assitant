import { request } from '../request';

/** 慢 LLM 请求 (全文标注/课文生成) 的超时: 后端专用超时 4 分钟, 前端放宽到 250s 取后端结构化错误而非本地超时 */
const SLOW_LLM_TIMEOUT = 250 * 1000;

/** 阅读文档 (列表项) */
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

/** LLM 标注生词 (同步, 耗时与文章长度相关) */
export function fetchAnnotateReadingDoc(id: number) {
  return request<ReadingDocDetail>({
    url: `/api/reading/docs/${id}/annotate`,
    method: 'post',
    timeout: SLOW_LLM_TIMEOUT
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

/** AI 生成个性化课文 (落成课程包) */
export function fetchGeneratePassage(data: { wordIds?: number[]; sentenceCount?: number }) {
  return request<{ course: { id: number; title: string }; usedWords: string[] }>({
    url: '/api/reading/generate-passage',
    method: 'post',
    data,
    timeout: SLOW_LLM_TIMEOUT
  });
}
