import { request } from '../request';

/** RSS 订阅源 */
export interface RSSSource {
  id: number;
  title: string;
  url: string;
  enabled: boolean;
  lastFetchedAt: string | null;
  lastError: string;
  createdAt: string;
  /** 未读数 (列表接口附带) */
  unread?: number;
}

/** RSS 文章条目 (正文按需拉取) */
export interface RSSItem {
  id: number;
  sourceId: number;
  title: string;
  link: string;
  summary: string;
  /** 摘要预估生词率 (个性化难度, 0=未算) */
  estUnknownRate?: number;
  publishedAt: string | null;
  status: 'unread' | 'read' | 'imported';
  importedDocId: number;
  createdAt: string;
  /** 净化后的 HTML 正文 (仅详情接口返回) */
  contentHtml?: string;
  /** AI 提取的生词 JSON 字符串 (仅详情接口返回, 一键提取结果) */
  difficultWords?: string;
}

/** 添加订阅 (先验证 feed 可解析) */
export function fetchRSSAddSource(data: { title?: string; url: string }) {
  return request<RSSSource>({
    url: '/api/rss/sources',
    method: 'post',
    data
  });
}

/** 订阅源列表 */
export function fetchRSSListSources() {
  return request<RSSSource[]>({
    url: '/api/rss/sources',
    method: 'get'
  });
}

/** 编辑订阅源 (改名 / 换 feed 地址, 空字段保持原值; 地址变更时后端会重新验证 feed) */
export function fetchRSSUpdateSource(id: number, data: { title?: string; url?: string }) {
  return request<RSSSource>({
    url: `/api/rss/sources/${id}`,
    method: 'put',
    data
  });
}

/** 删除订阅源及其条目 */
export function fetchRSSDeleteSource(id: number) {
  return request({
    url: `/api/rss/sources/${id}`,
    method: 'delete'
  });
}

/** 手动抓取单个源 */
export function fetchRSSFetchSource(id: number) {
  return request<{ inserted: number }>({
    url: `/api/rss/sources/${id}/fetch`,
    method: 'post'
  });
}

/** 文章列表 (分页 + 源/状态/难度过滤, status=unread|read, sort=time_desc|time_asc) */
export function fetchRSSListItems(params?: {
  sourceId?: number;
  status?: 'unread' | 'read';
  easyOnly?: boolean;
  sort?: 'time_desc' | 'time_asc';
  page?: number;
  page_size?: number;
}) {
  return request<{ list: RSSItem[]; total: number }>({
    url: '/api/rss/items',
    method: 'get',
    params
  });
}

/** 标记已读 */
export function fetchRSSMarkRead(id: number) {
  return request({
    url: `/api/rss/items/${id}/read`,
    method: 'post'
  });
}

/** 取条目正文 (按需抓取原文页并缓存, 返回纯文本 + 净化 HTML 双格式) */
export function fetchRSSItemContent(id: number) {
  return request<RSSItem & { content: string; contentHtml?: string }>({
    url: `/api/rss/items/${id}/content`,
    method: 'get'
  });
}

/** 把条目导入分级阅读 */
export function fetchRSSImportItem(id: number) {
  return request<{ id: number; title: string }>({
    url: `/api/rss/items/${id}/import`,
    method: 'post'
  });
}

/** 一键提取本文生词 (异步 job, 轮询 /api/reading/generation-jobs/:id) */
export function fetchRSSAnnotateItem(id: number) {
  return request<{ id: number; kind: string; status: string; resultId: number }>({
    url: `/api/rss/items/${id}/annotate`,
    method: 'post'
  });
}
