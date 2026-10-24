import { request } from "../request";
import { getAuthorization, handleExpiredRequest } from "../request/shared";
import { getServiceBaseURL } from "@/utils/service";

/**
 * SSE 流式请求统一封装 (axios 无法在浏览器做流式读取, 必须直接 fetch)。
 * 返回原始 Response 交给调用方经 consumeSSEStream 读取; 仅当响应是 JSON
 * 且 code 命中过期 token 时刷新凭据重试一次, SSE 流原样透传不消费 body。
 */
export async function fetchSSEWithAuthRetry(
  url: string,
  payload: unknown,
  signal?: AbortSignal
): Promise<Response> {
  const isHttpProxy = import.meta.env.DEV && import.meta.env.VITE_HTTP_PROXY === "Y";
  const { baseURL } = getServiceBaseURL(import.meta.env, isHttpProxy);

  let authorization = getAuthorization();
  const doFetch = () =>
    fetch(`${baseURL}${url}`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: authorization || ""
      },
      body: JSON.stringify(payload),
      signal
    });

  let response = await doFetch();

  if (response.ok) {
    try {
      const contentType = response.headers.get("content-type");
      if (contentType?.includes("application/json")) {
        const errorData = await response.json();
        const expiredTokenCodes =
          import.meta.env.VITE_SERVICE_EXPIRED_TOKEN_CODES?.split(",") || [];

        if (expiredTokenCodes.includes(String(errorData.code))) {
          const success = await handleExpiredRequest(request.state);
          if (success) {
            authorization = getAuthorization();
            response = await doFetch();
          }
        }
      }
    } catch (err) {
      // 过期检查失败不影响流式读取, 但要留痕便于排查
      console.warn("[sse] SSE 请求 token 过期检查失败:", err);
    }
  }

  return response;
}
