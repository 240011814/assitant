import { request } from "@/service/request";

/** system_config 表条目 (value 恒为字符串, 布尔/数字序列化后存入) */
export interface ConfigItem {
  key: string;
  value: string;
  remark?: string;
}

/** 配置条目数组转 key -> value 映射, 消除逐条 data.find 样板 */
export function toConfigMap(items: ConfigItem[]): Record<string, string> {
  const map: Record<string, string> = {};
  for (const item of items) map[item.key] = item.value;
  return map;
}

/** "true" 才视为开 (register/smtp/s3/rag 等缺省关闭的开关) */
export function isTrue(value: string | undefined): boolean {
  return value === "true";
}

/** 仅 "false" 视为关 (telegram/memory 等缺省开启的开关) */
export function notFalse(value: string | undefined): boolean {
  return value !== "false";
}

export function toNumber(value: string | undefined, fallback: number): number {
  if (value === undefined || value === "") return fallback;
  return Number(value);
}

/** 保存单个配置项; 失败时全局拦截器已 toast, 返回 false 供调用方决定是否提示成功/回滚 */
export async function saveConfig(key: string, value: string, remark: string): Promise<boolean> {
  const { error } = await request({
    url: "/api/admin/system-config",
    method: "put",
    data: { key, value, remark },
  });
  return !error;
}

/** 依次保存多个配置项, 全部成功才返回 true (沿用逐条 PUT 的既有写法) */
export async function saveConfigs(entries: Array<[key: string, value: string, remark: string]>): Promise<boolean> {
  for (const [key, value, remark] of entries) {
    if (!(await saveConfig(key, value, remark))) return false;
  }
  return true;
}

/** 各分区子组件共用: 父页面加载 system_config 后整体下发, 子组件 watch 应用自己的字段 */
export type ConfigMap = Record<string, string>;
