import { request } from '../request';

export interface UserDocumentItem {
  id: number;
  user_id: number;
  filename: string;
  ext: string;
  mime_type: string;
  size_bytes: number;
  parse_status: 'none' | 'ok' | 'failed';
  parse_error: string;
  text_chars: number;
  created_at: string;
  updated_at: string;
}

export interface UserDocumentListResponse {
  items: UserDocumentItem[];
  total: number;
}

export interface DocumentTextChunk {
  chunk: string;
  total_chars: number;
  filename: string;
  offset: number;
}

/** 上传文档 (multipart; 后端限制类型与大小) */
export function fetchUploadDocument(file: File) {
  const formData = new FormData();
  formData.append('file', file);
  return request<UserDocumentItem>({
    url: '/api/documents/upload',
    method: 'post',
    data: formData,
    headers: { 'Content-Type': 'multipart/form-data' },
    // 覆盖默认超时: 大文件 + 解析耗时较长
    timeout: 120000
  });
}

/** 分页获取文档列表 */
export function fetchDocuments(page = 1, pageSize = 20) {
  return request<UserDocumentListResponse>({ url: '/api/documents', method: 'get', params: { page, page_size: pageSize } });
}

/** 删除文档 */
export function fetchDeleteDocument(id: number) {
  return request<null>({ url: `/api/documents/${id}`, method: 'delete' });
}

/** 获取预签名下载 URL (短期有效) */
export function fetchDocumentDownloadUrl(id: number) {
  return request<{ url: string; expires_in: number }>({ url: `/api/documents/${id}/download`, method: 'get' });
}

/** 分页读取解析文本 (与 AI 读取工具同一套分页语义) */
export function fetchDocumentText(id: number, offset = 0, length = 6000) {
  return request<DocumentTextChunk>({ url: `/api/documents/${id}/text`, method: 'get', params: { offset, length } });
}
