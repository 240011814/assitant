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
  publishedAt: string | null;
  status: 'unread' | 'read' | 'imported';
  importedDocId: number;
  createdAt: string;
  /** 净化后的 HTML 正文 (仅详情接口返回) */
  contentHtml?: string;
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

/** 文章列表 (分页 + 源/未读过滤, sort=time_desc|time_asc) */
export function fetchRSSListItems(params?: {
  sourceId?: number;
  unreadOnly?: boolean;
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
