/**
 * Comments API Configuration
 *
 * Co-located endpoint definitions and query keys for the comments feature.
 */

import { API_BASE_URL } from '@/lib/api-base'

// ============================================================================
// Endpoints
// ============================================================================

export const commentEndpoints = {
  LIST: (entityType: string, entityId: number) =>
    `${API_BASE_URL}/entities/${entityType}/${entityId}/comments`,
  CREATE: (entityType: string, entityId: number) =>
    `${API_BASE_URL}/entities/${entityType}/${entityId}/comments`,
  REPLY: (commentId: number) =>
    `${API_BASE_URL}/comments/${commentId}/replies`,
  UPDATE: (commentId: number) =>
    `${API_BASE_URL}/comments/${commentId}`,
  DELETE: (commentId: number) =>
    `${API_BASE_URL}/comments/${commentId}`,
  VOTE: (commentId: number) =>
    `${API_BASE_URL}/comments/${commentId}/vote`,
  THREAD: (commentId: number) =>
    `${API_BASE_URL}/comments/${commentId}/thread`,
} as const

export const fieldNoteEndpoints = {
  LIST: (showId: number) =>
    `${API_BASE_URL}/shows/${showId}/field-notes`,
  CREATE: (showId: number) =>
    `${API_BASE_URL}/shows/${showId}/field-notes`,
} as const

// ============================================================================
// Query Keys
// ============================================================================

export const commentQueryKeys = {
  all: ['comments'] as const,
  entity: (entityType: string, entityId: number) =>
    ['comments', entityType, entityId] as const,
  thread: (commentId: number) =>
    ['comments', 'thread', commentId] as const,
} as const

export const fieldNoteQueryKeys = {
  all: ['field-notes'] as const,
  show: (showId: number) =>
    ['field-notes', showId] as const,
} as const
