// Aliased from the generated OpenAPI types, not hand-written (PSY-1550/1600).
// Regenerate with `bun run api:types`; the "API Types Drift" CI gate fails if
// the committed types drift from the backend. Exported names are kept stable
// for callers.
//
// `logs` is nullable on the wire: guard before iterating (PSY-1600).

import type { components } from '../../types/api'

export type AuditLogEntry = components['schemas']['AuditLogResponse']
export type AuditLogsResponse = components['schemas']['GetAuditLogsResponseBody']
