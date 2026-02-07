export interface AuditLogEntry {
  id: number
  actor_id: number | null
  actor_email?: string
  action: string
  entity_type: string
  entity_id: number
  metadata?: Record<string, unknown>
  created_at: string
}

export interface AuditLogsResponse {
  logs: AuditLogEntry[]
  total: number
}
