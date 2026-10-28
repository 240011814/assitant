/**
 * 接口请求超时后台配置 (system_config: api_timeout_default + api_timeout_overrides)。
 * 登录后的首个请求触发拉取 (接口在鉴权组内, 未登录拉取只会 401); 幂等, 只拉一次。
 * 匹配规则: method + path, 后端路由的 :param 段匹配任意非 "/" 字符。
 * 优先级: 单请求显式 timeout > 路由覆盖 > 默认 (未加载时保持 @sa/axios 实例默认 10s)。
 */
import { request } from './index';

interface ApiRouteOverride {
  method: string;
  path: string;
  timeoutMs: number;
}

interface ApiTimeoutConfig {
  defaultMs: number;
  overrides: ApiRouteOverride[];
}

let loaded = false;
let loading = false;
let defaultMs = 10 * 1000;
const matchers: Array<{ method: string; re: RegExp; timeoutMs: number }> = [];

// 路径段转正则: :id 参数段 → [^/]+, 其余按字面转义
const segRe = (seg: string) => (seg.startsWith(':') ? '[^/]+' : seg.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'));
const pathToRe = (path: string) => new RegExp(`^/${path.split('/').filter(Boolean).map(segRe).join('/')}/?$`);

const rebuild = (cfg: ApiTimeoutConfig) => {
  defaultMs = cfg.defaultMs > 0 ? cfg.defaultMs : 10 * 1000;
  matchers.length = 0;
  for (const o of cfg.overrides || []) {
    if (!o?.path || !o?.method) continue;
    try {
      matchers.push({ method: o.method.toUpperCase(), re: pathToRe(o.path), timeoutMs: o.timeoutMs });
    } catch {
      // 非法路径忽略, 不影响其它项
    }
  }
  loaded = true;
};

/** 拉取超时配置 (幂等; force=true 强制刷新, 管理页保存后调用使本页立即生效) */
export function ensureTimeoutConfigLoaded(force = false) {
  if (loading || (loaded && !force)) return;
  loading = true;
  request<ApiTimeoutConfig>({ url: '/api/api-timeouts', method: 'get' })
    .then(({ data }) => {
      if (data) rebuild(data);
    })
    .catch(() => {
      // 拉取失败保持实例默认, 下次会话再试 (不打断请求)
    })
    .finally(() => {
      loading = false;
    });
}

/** 请求期应用超时: 显式 > 路由覆盖 > 默认; 未加载时不干预 (实例默认 10s) */
export function applyRequestTimeout(config: { url?: string; method?: string; timeout?: number }) {
  if (config.timeout !== undefined) return;
  if (!loaded || matchers.length === 0) return;
  const method = (config.method || 'get').toUpperCase();
  const path = (config.url || '').split('?')[0];
  for (const m of matchers) {
    if (m.method === method && m.re.test(path)) {
      config.timeout = m.timeoutMs;
      return;
    }
  }
  config.timeout = defaultMs;
}
