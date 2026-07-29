// Aliased from the generated OpenAPI types, not hand-written (PSY-1550/1600).
// Regenerate with `bun run api:types`; the "API Types Drift" CI gate fails if
// the committed types drift from the backend. Exported names are kept stable
// for callers.
//
// `AdminUser.auth_methods` and `AdminUsersResponse.users` are nullable on the
// wire: guard before iterating (PSY-1600).

import type { components } from '../../types/api'

export type UserSubmissionStats = components['schemas']['UserSubmissionStats']
export type AdminUser = components['schemas']['AdminUserResponse']
export type AdminUsersResponse = components['schemas']['GetAdminUsersResponseBody']
