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
      createdAt: string
      updatedAt: string
    }

    /** SRS 复习统计 */
    interface ReviewStats {
      dueCount: number
      learningCount: number
      boxDist: Record<string, number>
    }
  }
}
