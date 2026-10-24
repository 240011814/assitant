declare namespace Api {
  namespace ErrorBook {
    /** 错题条目 */
    interface Item {
      id: number
      contentType: 'word' | 'sentence'
      content: string
      translation: string
      sourceType: string
      sourceId: number
      errorCount: number
      isMastered: boolean
      createdAt: string
    }
  }
}
