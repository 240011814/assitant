declare namespace Api {
  namespace Note {
    /** 笔记条目 */
    interface Item {
      id: number
      title: string
      category: string
      content: string
      createdAt: string
    }
  }
}
