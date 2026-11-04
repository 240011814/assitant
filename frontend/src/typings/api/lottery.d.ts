declare namespace Api {
  namespace Lottery {
    /** 抽奖活动 */
    interface Activity {
      id: number;
      name: string;
      description: string;
      startTime: string;
      endTime: string;
      status: number; // 0-未开始, 1-进行中, 2-已结束
      drawMode: number; // 0-转盘, 1-原神抽卡
      allowAnonymous: boolean; // 是否允许匿名(未登录)抽奖
      maxParticipants: number;
      currentParticipants: number;
      dailyLimit: number;
      totalLimit: number;
      createdBy: number;
      createdAt: string;
      updatedAt: string;
    }

    /** 奖品 */
    interface Prize {
      id: number;
      activityId: number;
      name: string;
      description: string;
      imageUrl: string;
      prizeType: number; // 0-实物, 1-虚拟
      prizeLevel: number; // 0-未设置, 1-特等奖, 2-一等奖, 3-二等奖, 4-三等奖
      prizeValue: number;
      totalCount: number;
      remainingCount: number;
      probability: number;
      displayProbability: number;
      sortOrder: number;
      createdAt: string;
      updatedAt: string;
    }

    /** 抽奖记录 */
    interface Record {
      id: number;
      activityId: number;
      userId: number;
      userName: string;
      prizeId: number | null;
      prizeName: string;
      isWinner: boolean;
      createdAt: string;
    }

    /** 创建活动参数 */
    interface CreateActivityParams {
      name: string;
      description?: string;
      startTime: string;
      endTime: string;
      drawMode?: number;
      allowAnonymous?: boolean;
      maxParticipants?: number;
      dailyLimit?: number;
      totalLimit?: number;
    }

    /** 更新活动参数 */
    interface UpdateActivityParams {
      name?: string;
      description?: string;
      startTime?: string;
      endTime?: string;
      status?: number;
      drawMode?: number;
      allowAnonymous?: boolean;
      maxParticipants?: number;
      dailyLimit?: number;
      totalLimit?: number;
    }

    /** 创建奖品参数 */
    interface CreatePrizeParams {
      name: string;
      description?: string;
      imageUrl?: string;
      prizeType?: number;
      prizeLevel?: number;
      prizeValue?: number;
      totalCount: number;
      probability: number;
      displayProbability?: number;
      sortOrder?: number;
    }

    /** 更新奖品参数 */
    interface UpdatePrizeParams {
      name?: string;
      description?: string;
      imageUrl?: string;
      prizeType?: number;
      prizeLevel?: number;
      prizeValue?: number;
      totalCount?: number;
      probability?: number;
      displayProbability?: number;
      sortOrder?: number;
    }

    /** 抽奖结果 */
    interface DrawResult {
      isWinner: boolean;
      prize?: Prize;
      message: string;
    }

    /** 抽奖次数限制信息 */
    interface DrawLimits {
      dailyLimit: number;
      totalLimit: number;
      userDailyUsed: number;
      userTotalUsed: number;
      totalDailyUsed: number;
    }

    /** 分页响应 */
    interface PaginatedResponse<T> {
      list: T[];
      total: number;
      page: number;
      pageSize: number;
    }

    /** 查询参数 */
    interface ActivityListParams {
      keyword?: string;
      status?: number;
    }

    interface RecordListParams {
      activityId?: number;
      userId?: number;
      page?: number;
      pageSize?: number;
    }

    interface WinnerListParams {
      activityId?: number;
      page?: number;
      pageSize?: number;
    }
  }
}
