declare namespace Api {
  namespace Vocabulary {
    interface Item {
      id: number
      userId: number
      word: string
      phonetic: string
      definition: string
      example: string
      sourceContext: string
      confusingWords: string
      isMastered: boolean
      srsBox: number
      nextReviewAt: string | null
      lastReviewedAt: string | null
      fsrsStability: number
      fsrsDifficulty: number
      /** 0=New 1=Learning 2=Review 3=Relearning */
      fsrsState: number
      fsrsReps: number
      fsrsLapses: number
      assistExample: string
      assistMnemonic: string
      /** 来源: reading=分级阅读 / rss=RSS 条目 (空=手动) */
      sourceKind: string
      /** 来源文档/条目 ID (回链原文) */
      sourceRef: number
      /** 阅读中再次相遇次数 */
      encounters: number
      /** 加入生词本时所在的原句 (语境复习方向) */
      contextSentence: string
      createdAt: string
      updatedAt: string
    }

    /** 复习统计 (FSRS): 到期数 / 学习中词数 / 各状态分布 */
    interface ReviewStats {
      dueCount: number
      learningCount: number
      stateDist: Record<string, number>
    }

    /** 复习方向: en2zh 再认 / zh2en 主动回忆 / listen 听音辨词 / context 原文语境 */
    type ReviewDirection = 'en2zh' | 'zh2en' | 'listen' | 'context'

    /** 复习卡: 词 + 本卡方向 */
    interface ReviewCard {
      direction: ReviewDirection
      word: Item
    }

    /** 复习会话: 到期复习 + 新词 (受每日限额约束) */
    interface ReviewSession {
      reviews: ReviewCard[]
      newWords: ReviewCard[]
      newLimit: number
      newStartedToday: number
      estimatedMin: number
    }

    /** 到期预测 */
    interface ForecastPoint {
      date: string
      count: number
    }

    interface Forecast {
      days: number
      forecast: ForecastPoint[]
      newWords: number
    }

    /** 学习看板 */
    interface LearningStats {
      total: number
      mastered: number
      newWords: number
      heatmap: ForecastPoint[]
      retention30: number
      reviewCount30: number
      streak: number
      troubleWords: Item[]
      totalReviews: number
      avgReviewsPerMastered: number
      /** 阅读维度 (近90天: 打开/查词/加词/阅读分钟) */
      reading?: {
        totalOpens: number
        totalLookups: number
        totalWordsAdded: number
        totalReadMinutes: number
        daily: Array<{ date: string; opens: number; lookups: number; wordsAdded: number; readMinutes: number }>
      }
      /** 练习维度 (近30天) */
      quiz?: {
        sessions: number
        answered: number
        correct: number
        accuracy: number
      }
    }

    /** 词书导入条目 */
    interface ImportItem {
      word: string
      phonetic?: string
      definition?: string
      example?: string
    }

    interface ImportResult {
      added: number
      skipped: number
      failed: string[]
    }
  }
}
