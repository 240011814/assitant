declare namespace Api {
  namespace AsyncJob {
    /** 异步任务行 (审计日志页「异步任务」Tab, 管理员视角) */
    interface AdminJob {
      id: number;
      userId: number;
      userName: string;
      kind: 'passage' | 'vocab_test' | 'rss_annotate' | 'rss_translate' | string;
      status: 'running' | 'done' | 'failed';
      /** 进度文本 (如 "翻译中 2/8 批") */
      progress?: string;
      resultId: number;
      resultJson?: string;
      error?: string;
      /** 输入快照 (排查用, 仅管理员下发) */
      paramsJson?: string;
      startedAt?: string | null;
      finishedAt?: string | null;
      createdAt: string;
    }

    interface AdminListParams {
      page: number;
      page_size: number;
      userId?: number | null;
      kind?: string;
      status?: string;
    }
  }
}
