declare namespace Api {
  namespace AuditLog {
    interface OperationAuditLog {
      id: number
      userId: number | null
      userName: string
      method: string
      path: string
      statusCode: string
      success: boolean
      errorMsg: string
      requestBody: string | null
      ip: string
      userAgent: string
      latencyMs: number
      createdAt: string
    }

    interface AuditLogListParams {
      page: number
      page_size: number
      userId?: number | null
      method?: string
      path?: string
      success?: string
      startTime?: string
      endTime?: string
    }
  }
}
