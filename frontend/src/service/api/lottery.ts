import { request } from '../request';
import { localStg } from '@/utils/storage';

// ==================== 活动管理 ====================

/**
 * 获取抽奖活动列表
 * @param params 查询参数
 */
export function fetchGetLotteryActivities(params?: Api.Lottery.ActivityListParams) {
  return request<Api.Lottery.Activity[]>({
    url: '/api/lottery/activities',
    method: 'get',
    params
  });
}

/**
 * 获取抽奖活动详情
 * @param id 活动ID
 */
export function fetchGetLotteryActivity(id: number) {
  return request<Api.Lottery.Activity>({
    url: `/api/lottery/activities/${id}`,
    method: 'get'
  });
}

/**
 * 创建抽奖活动
 * @param data 活动信息
 */
export function fetchCreateLotteryActivity(data: Api.Lottery.CreateActivityParams) {
  return request<Api.Lottery.Activity>({
    url: '/api/lottery/activities',
    method: 'post',
    data
  });
}

/**
 * 更新抽奖活动
 * @param id 活动ID
 * @param data 更新内容
 */
export function fetchUpdateLotteryActivity(id: number, data: Api.Lottery.UpdateActivityParams) {
  return request({
    url: `/api/lottery/activities/${id}`,
    method: 'put',
    data
  });
}

/**
 * 删除抽奖活动
 * @param id 活动ID
 */
export function fetchDeleteLotteryActivity(id: number) {
  return request({
    url: `/api/lottery/activities/${id}`,
    method: 'delete'
  });
}

// ==================== 奖品管理 ====================

/**
 * 获取奖品列表
 * @param activityId 活动ID
 */
export function fetchGetLotteryPrizes(activityId: number) {
  return request<Api.Lottery.Prize[]>({
    url: `/api/lottery/activities/${activityId}/prizes`,
    method: 'get'
  });
}

/**
 * 创建奖品
 * @param activityId 活动ID
 * @param data 奖品信息
 */
export function fetchCreateLotteryPrize(activityId: number, data: Api.Lottery.CreatePrizeParams) {
  return request<Api.Lottery.Prize>({
    url: `/api/lottery/activities/${activityId}/prizes`,
    method: 'post',
    data
  });
}

/**
 * 更新奖品
 * @param id 奖品ID
 * @param data 更新内容
 */
export function fetchUpdateLotteryPrize(id: number, data: Api.Lottery.UpdatePrizeParams) {
  return request({
    url: `/api/lottery/prizes/${id}`,
    method: 'put',
    data
  });
}

/**
 * 删除奖品
 * @param id 奖品ID
 */
export function fetchDeleteLotteryPrize(id: number) {
  return request({
    url: `/api/lottery/prizes/${id}`,
    method: 'delete'
  });
}

// ==================== 抽奖操作 ====================

/**
 * 执行抽奖
 * @param activityId 活动ID
 * @param userName 用户姓名
 */
export function fetchDrawLottery(activityId: number, userName: string) {
  return request<Api.Lottery.DrawResult>({
    url: `/api/lottery/draw/${activityId}`,
    method: 'post',
    data: { userName }
  });
}

// ==================== 记录查询 ====================

/**
 * 获取抽奖记录列表
 * @param params 查询参数
 */
export function fetchGetLotteryRecords(params?: Api.Lottery.RecordListParams) {
  return request<Api.Lottery.PaginatedResponse<Api.Lottery.Record>>({
    url: '/api/lottery/records',
    method: 'get',
    params
  });
}

/**
 * 获取中奖名单
 * @param params 查询参数
 */
export function fetchGetLotteryWinners(params?: Api.Lottery.WinnerListParams) {
  return request<Api.Lottery.PaginatedResponse<Api.Lottery.Record>>({
    url: '/api/lottery/winners',
    method: 'get',
    params
  });
}

/**
 * 删除抽奖记录
 * @param id 记录ID
 */
export function fetchDeleteLotteryRecord(id: number) {
  return request({
    url: `/api/lottery/records/${id}`,
    method: 'delete'
  });
}

/**
 * 设置中奖记录核销状态 (缺省参数为取反)
 * @param id 记录ID
 * @param redeemed 目标状态, 不传则取反
 */
export function fetchSetRecordRedeemed(id: number, redeemed?: boolean) {
  return request<{ id: number; redeemed: boolean }>({
    url: `/api/lottery/records/${id}/redeemed`,
    method: 'put',
    data: redeemed === undefined ? {} : { redeemed }
  });
}

/**
 * 导出活动中奖名单 CSV (浏览器触发下载)
 * @param activityId 活动ID
 */
export async function fetchExportLotteryWinners(activityId: number) {
  const token = localStg.get('token');
  const resp = await fetch(`/api/lottery/activities/${activityId}/winners/export`, {
    headers: token ? { Authorization: `Bearer ${token}` } : {}
  });
  if (!resp.ok) {
    throw new Error(`导出失败 (${resp.status})`);
  }
  const blob = await resp.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = 'winners.csv';
  a.click();
  URL.revokeObjectURL(url);
}

/**
 * 删除活动的所有抽奖记录
 * @param activityId 活动ID
 */
export function fetchDeleteLotteryRecordsByActivity(activityId: number) {
  return request({
    url: `/api/lottery/activities/${activityId}/records`,
    method: 'delete'
  });
}

/**
 * 获取抽奖次数限制信息
 * @param activityId 活动ID
 * @param userName 用户姓名
 */
export function fetchGetDrawLimits(activityId: number, userName?: string) {
  return request<{
    dailyLimit: number;
    totalLimit: number;
    userDailyUsed: number;
    userTotalUsed: number;
    totalDailyUsed: number;
  }>({
    url: `/api/lottery/activities/${activityId}/limits`,
    method: 'get',
    params: { userName }
  });
}
