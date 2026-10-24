/**
 * SSE 流式消费工具 (axios 无法在浏览器做流式读取, 各调用方直接 fetch 后经此解析)。
 * 按 SSE 规范以空行分块, 块内 `event:` 行定事件名、`data:` 行可多行拼接。
 */
export type SSEEventHandler = (event: string, data: string) => void;

/** 读取 Response 的 ReadableStream 并逐事件回调; 处理跨 chunk 的半行/半块分片 */
export async function consumeSSEStream(response: Response, onEvent: SSEEventHandler): Promise<void> {
  const reader = response.body?.getReader();
  if (!reader) throw new Error("无法获取响应流");

  const decoder = new TextDecoder("utf-8");
  let buffer = "";

  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const blocks = buffer.split(/\n\n/);
    buffer = blocks.pop() || "";

    for (const block of blocks) {
      let eventName = "message";
      const dataLines: string[] = [];
      for (const line of block.split(/\r?\n/)) {
        if (line.startsWith("event:")) {
          eventName = line.replace(/^event:\s*/, "").trim();
        } else if (line.startsWith("data:")) {
          dataLines.push(line.replace(/^data:\s*/, ""));
        }
      }
      if (dataLines.length > 0) {
        onEvent(eventName, dataLines.join("\n"));
      }
    }
  }
}

/** 解析 SSE 事件 data 为 JSON 对象; 解析失败或空串返回 null */
export function parseSSEPayload<T = Record<string, any>>(data: string): T | null {
  if (!data) return null;
  try {
    return JSON.parse(data) as T;
  } catch {
    return null;
  }
}

/** 从非 2xx 的流式响应中提取错误信息 (后端可能返回 JSON 错误体而非 SSE 流) */
export async function extractSSEResponseError(response: Response): Promise<Error> {
  let errorMessage = `请求失败 (HTTP ${response.status})`;
  try {
    const errorData = await response.json();
    if (errorData?.error) errorMessage = errorData.error;
    else if (errorData?.message) errorMessage = errorData.message;
  } catch {
    // 非 JSON 错误体时保留 HTTP 状态码信息
  }
  return new Error(errorMessage);
}
