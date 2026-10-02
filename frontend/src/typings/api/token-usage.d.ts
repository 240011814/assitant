declare namespace Api {
  namespace TokenUsage {
    interface Summary {
      prompt_tokens: number
      completion_tokens: number
      total_tokens: number
      calls: number
      users: number
    }

    interface TrendItem {
      bucket: string
      prompt_tokens: number
      completion_tokens: number
      total_tokens: number
      calls: number
    }

    interface ModelItem {
      model: string
      prompt_tokens: number
      completion_tokens: number
      total_tokens: number
      calls: number
    }

    interface UserItem {
      user_id: number
      username: string
      nickname: string
      prompt_tokens: number
      completion_tokens: number
      total_tokens: number
      calls: number
      quota_month: number | null
    }

    interface Stats {
      summary: Summary
      trend: TrendItem[]
      by_model: ModelItem[]
      by_user: UserItem[]
    }

    type Granularity = 'hour' | 'day' | 'month' | 'year'

    interface StatsParams {
      granularity?: Granularity
      model?: string
      userId?: number | null
      startTime?: string
      endTime?: string
    }

    interface UsageRecord {
      id: number
      user_id: number
      username: string
      nickname: string | null
      model: string
      source: string
      prompt_tokens: number
      completion_tokens: number
      total_tokens: number
      created_at: string
    }

    interface ListParams extends StatsParams {
      page: number
      page_size: number
    }

    interface MyUsage {
      month_used: number
      quota_month: number | null
      trend: TrendItem[]
      by_model: ModelItem[]
    }

    interface ListResult {
      list: UsageRecord[]
      total: number
      page: number
      page_size: number
    }
  }
}
