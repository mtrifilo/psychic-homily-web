/**
 * API Configuration Utility
 *
 * This module provides centralized API configuration that automatically
 * selects the correct backend URL based on the environment.
 *
 * In development, requests go through a Next.js API proxy (/api/*)
 * to handle cookie same-origin requirements.
 *
 * Feature-specific endpoints are defined in their feature modules
 * (e.g., features/artists/api.ts) and re-exported here for backward compat.
 */

import { authLogger } from './utils/authLogger'
import { AuthError, AuthErrorCode } from './errors'
import * as Sentry from '@sentry/nextjs'
import { artistEndpoints } from '@/features/artists/api'
import { venueEndpoints } from '@/features/venues/api'
import { showEndpoints } from '@/features/shows/api'
import { releaseEndpoints } from '@/features/releases/api'
import { labelEndpoints } from '@/features/labels/api'
import { festivalEndpoints } from '@/features/festivals/api'

// Re-export API_BASE_URL from api-base for backward compatibility
export { API_BASE_URL } from './api-base'
import { API_BASE_URL } from './api-base'

// Request ID header name (must match backend middleware)
const REQUEST_ID_HEADER = 'X-Request-ID'

// API endpoint configuration
export const API_ENDPOINTS = {
  // Authentication endpoints
  AUTH: {
    LOGIN: `${API_BASE_URL}/auth/login`,
    LOGOUT: `${API_BASE_URL}/auth/logout`,
    REGISTER: `${API_BASE_URL}/auth/register`,
    PROFILE: `${API_BASE_URL}/auth/profile`,
    REFRESH: `${API_BASE_URL}/auth/refresh`,
    // Email verification endpoints
    VERIFY_EMAIL_SEND: `${API_BASE_URL}/auth/verify-email/send`,
    VERIFY_EMAIL_CONFIRM: `${API_BASE_URL}/auth/verify-email/confirm`,
    // Password management endpoints
    CHANGE_PASSWORD: `${API_BASE_URL}/auth/change-password`,
    // Magic link endpoints
    MAGIC_LINK_SEND: `${API_BASE_URL}/auth/magic-link/send`,
    MAGIC_LINK_VERIFY: `${API_BASE_URL}/auth/magic-link/verify`,
    // OAuth endpoints
    OAUTH_LOGIN: (provider: string) => `${API_BASE_URL}/auth/login/${provider}`,
    OAUTH_CALLBACK: (provider: string) =>
      `${API_BASE_URL}/auth/callback/${provider}`,
    OAUTH_ACCOUNTS: `${API_BASE_URL}/auth/oauth/accounts`,
    OAUTH_UNLINK: (provider: string) =>
      `${API_BASE_URL}/auth/oauth/accounts/${provider}`,
    // Account deletion endpoints
    DELETION_SUMMARY: `${API_BASE_URL}/auth/account/deletion-summary`,
    DELETE_ACCOUNT: `${API_BASE_URL}/auth/account/delete`,
    // Data export endpoint (GDPR Right to Portability)
    EXPORT_DATA: `${API_BASE_URL}/auth/account/export`,
    // Account recovery endpoints
    RECOVER_ACCOUNT: `${API_BASE_URL}/auth/recover-account`,
    RECOVER_ACCOUNT_REQUEST: `${API_BASE_URL}/auth/recover-account/request`,
    RECOVER_ACCOUNT_CONFIRM: `${API_BASE_URL}/auth/recover-account/confirm`,
    // CLI token generation (admin only)
    CLI_TOKEN: `${API_BASE_URL}/auth/cli-token`,
    // User preferences
    FAVORITE_CITIES: `${API_BASE_URL}/auth/preferences/favorite-cities`,
    SHOW_REMINDERS: `${API_BASE_URL}/auth/preferences/show-reminders`,
    UNSUBSCRIBE_SHOW_REMINDERS: `${API_BASE_URL}/auth/unsubscribe/show-reminders`,
    // PSY-350 / PSY-515: weekly digest of new items in collections you follow.
    COLLECTION_DIGEST: `${API_BASE_URL}/auth/preferences/collection-digest`,
  },

  // Feature module endpoints (defined in features/*/api.ts, re-exported here)
  SHOWS: showEndpoints,
  ARTISTS: artistEndpoints,
  VENUES: venueEndpoints,
  RELEASES: releaseEndpoints,
  LABELS: labelEndpoints,
  FESTIVALS: festivalEndpoints,

  // Calendar feed endpoints
  CALENDAR: {
    TOKEN: `${API_BASE_URL}/calendar/token`,
  },

  // Saved shows (user's "My List") endpoints
  SAVED_SHOWS: {
    LIST: `${API_BASE_URL}/saved-shows`,
    SAVE: (showId: string | number) => `${API_BASE_URL}/saved-shows/${showId}`,
    UNSAVE: (showId: string | number) =>
      `${API_BASE_URL}/saved-shows/${showId}`,
    CHECK: (showId: string | number) =>
      `${API_BASE_URL}/saved-shows/${showId}/check`,
    CHECK_BATCH: `${API_BASE_URL}/saved-shows/check-batch`,
  },

  // Favorite venues endpoints
  FAVORITE_VENUES: {
    LIST: `${API_BASE_URL}/favorite-venues`,
    FAVORITE: (venueId: string | number) =>
      `${API_BASE_URL}/favorite-venues/${venueId}`,
    UNFAVORITE: (venueId: string | number) =>
      `${API_BASE_URL}/favorite-venues/${venueId}`,
    CHECK: (venueId: string | number) =>
      `${API_BASE_URL}/favorite-venues/${venueId}/check`,
    SHOWS: `${API_BASE_URL}/favorite-venues/shows`,
  },

  // Admin endpoints
  ADMIN: {
    SHOWS: {
      PENDING: `${API_BASE_URL}/admin/shows/pending`,
      REJECTED: `${API_BASE_URL}/admin/shows/rejected`,
      APPROVE: (showId: string | number) =>
        `${API_BASE_URL}/admin/shows/${showId}/approve`,
      REJECT: (showId: string | number) =>
        `${API_BASE_URL}/admin/shows/${showId}/reject`,
      BATCH_APPROVE: `${API_BASE_URL}/admin/shows/batch-approve`,
      BATCH_REJECT: `${API_BASE_URL}/admin/shows/batch-reject`,
      IMPORT_PREVIEW: `${API_BASE_URL}/admin/shows/import/preview`,
      IMPORT_CONFIRM: `${API_BASE_URL}/admin/shows/import/confirm`,
      SET_SOLD_OUT: (showId: string | number) =>
        `${API_BASE_URL}/admin/shows/${showId}/sold-out`,
      SET_CANCELLED: (showId: string | number) =>
        `${API_BASE_URL}/admin/shows/${showId}/cancelled`,
    },
    VENUES: {
      UNVERIFIED: `${API_BASE_URL}/admin/venues/unverified`,
      VERIFY: (venueId: string | number) =>
        `${API_BASE_URL}/admin/venues/${venueId}/verify`,
      PENDING_EDITS: `${API_BASE_URL}/admin/venues/pending-edits`,
      APPROVE_EDIT: (editId: string | number) =>
        `${API_BASE_URL}/admin/venues/pending-edits/${editId}/approve`,
      REJECT_EDIT: (editId: string | number) =>
        `${API_BASE_URL}/admin/venues/pending-edits/${editId}/reject`,
    },
    ARTISTS: {
      UPDATE: (artistId: string | number) =>
        `${API_BASE_URL}/admin/artists/${artistId}`,
      ALIASES: (artistId: string | number) =>
        `${API_BASE_URL}/admin/artists/${artistId}/aliases`,
      DELETE_ALIAS: (artistId: string | number, aliasId: string | number) =>
        `${API_BASE_URL}/admin/artists/${artistId}/aliases/${aliasId}`,
      MERGE: `${API_BASE_URL}/admin/artists/merge`,
    },
    REPORTS: {
      LIST: `${API_BASE_URL}/admin/reports`,
      DISMISS: (reportId: string | number) =>
        `${API_BASE_URL}/admin/reports/${reportId}/dismiss`,
      RESOLVE: (reportId: string | number) =>
        `${API_BASE_URL}/admin/reports/${reportId}/resolve`,
    },
    ARTIST_REPORTS: {
      LIST: `${API_BASE_URL}/admin/artist-reports`,
      DISMISS: (reportId: string | number) =>
        `${API_BASE_URL}/admin/artist-reports/${reportId}/dismiss`,
      RESOLVE: (reportId: string | number) =>
        `${API_BASE_URL}/admin/artist-reports/${reportId}/resolve`,
    },
    TOKENS: {
      LIST: `${API_BASE_URL}/admin/tokens`,
      CREATE: `${API_BASE_URL}/admin/tokens`,
      REVOKE: (tokenId: string | number) =>
        `${API_BASE_URL}/admin/tokens/${tokenId}`,
    },
    AUDIT_LOGS: {
      LIST: `${API_BASE_URL}/admin/audit-logs`,
    },
    USERS: {
      LIST: `${API_BASE_URL}/admin/users`,
    },
    STATS: `${API_BASE_URL}/admin/stats`,
    ACTIVITY: `${API_BASE_URL}/admin/activity`,
    DISCOVERY: {
      IMPORT: `${API_BASE_URL}/admin/discovery/import`,
    },
    DATA_QUALITY: {
      SUMMARY: `${API_BASE_URL}/admin/data-quality`,
      CATEGORY: (category: string) =>
        `${API_BASE_URL}/admin/data-quality/${category}`,
    },
    ANALYTICS: {
      GROWTH: `${API_BASE_URL}/admin/analytics/growth`,
      ENGAGEMENT: `${API_BASE_URL}/admin/analytics/engagement`,
      COMMUNITY: `${API_BASE_URL}/admin/analytics/community`,
      DATA_QUALITY: `${API_BASE_URL}/admin/analytics/data-quality`,
    },
    PENDING_EDITS: {
      LIST: `${API_BASE_URL}/admin/pending-edits`,
      GET: (editId: string | number) =>
        `${API_BASE_URL}/admin/pending-edits/${editId}`,
      APPROVE: (editId: string | number) =>
        `${API_BASE_URL}/admin/pending-edits/${editId}/approve`,
      REJECT: (editId: string | number) =>
        `${API_BASE_URL}/admin/pending-edits/${editId}/reject`,
    },
    ENTITY_REPORTS: {
      LIST: `${API_BASE_URL}/admin/entity-reports`,
      GET: (reportId: string | number) =>
        `${API_BASE_URL}/admin/entity-reports/${reportId}`,
      RESOLVE: (reportId: string | number) =>
        `${API_BASE_URL}/admin/entity-reports/${reportId}/resolve`,
      DISMISS: (reportId: string | number) =>
        `${API_BASE_URL}/admin/entity-reports/${reportId}/dismiss`,
    },
    PIPELINE: {
      VENUES: `${API_BASE_URL}/admin/pipeline/venues`,
      IMPORTS: `${API_BASE_URL}/admin/pipeline/imports`,
      EXTRACT: (venueId: string | number) =>
        `${API_BASE_URL}/admin/pipeline/extract/${venueId}`,
      VENUE_STATS: (venueId: string | number) =>
        `${API_BASE_URL}/admin/pipeline/venues/${venueId}/stats`,
      VENUE_NOTES: (venueId: string | number) =>
        `${API_BASE_URL}/admin/pipeline/venues/${venueId}/notes`,
      VENUE_CONFIG: (venueId: string | number) =>
        `${API_BASE_URL}/admin/pipeline/venues/${venueId}/config`,
      VENUE_RUNS: (venueId: string | number) =>
        `${API_BASE_URL}/admin/pipeline/venues/${venueId}/runs`,
      VENUE_RESET_RENDER: (venueId: string | number) =>
        `${API_BASE_URL}/admin/pipeline/venues/${venueId}/reset-render-method`,
    },
  },

  // Contributor profile endpoints (public)
  USERS: {
    PROFILE: (username: string) => `${API_BASE_URL}/users/${username}`,
    CONTRIBUTIONS: (username: string) =>
      `${API_BASE_URL}/users/${username}/contributions`,
    SECTIONS: (username: string) =>
      `${API_BASE_URL}/users/${username}/sections`,
    ACTIVITY_HEATMAP: (username: string) =>
      `${API_BASE_URL}/users/${username}/activity-heatmap`,
    RANKINGS: (username: string) =>
      `${API_BASE_URL}/users/${username}/rankings`,
  },

  // Contributor profile endpoints (authenticated)
  CONTRIBUTOR: {
    OWN_PROFILE: `${API_BASE_URL}/auth/profile/contributor`,
    OWN_CONTRIBUTIONS: `${API_BASE_URL}/auth/profile/contributions`,
    VISIBILITY: `${API_BASE_URL}/auth/profile/visibility`,
    PRIVACY: `${API_BASE_URL}/auth/profile/privacy`,
    OWN_SECTIONS: `${API_BASE_URL}/auth/profile/sections`,
    SECTION: (sectionId: number) =>
      `${API_BASE_URL}/auth/profile/sections/${sectionId}`,
  },

  // Collection endpoints
  COLLECTIONS: {
    LIST: `${API_BASE_URL}/collections`,
    DETAIL: (slug: string) => `${API_BASE_URL}/collections/${slug}`,
    STATS: (slug: string) => `${API_BASE_URL}/collections/${slug}/stats`,
    ITEMS: (slug: string) => `${API_BASE_URL}/collections/${slug}/items`,
    ITEM: (slug: string, itemId: number) =>
      `${API_BASE_URL}/collections/${slug}/items/${itemId}`,
    UPDATE_ITEM: (slug: string, itemId: number) =>
      `${API_BASE_URL}/collections/${slug}/items/${itemId}`,
    REORDER: (slug: string) =>
      `${API_BASE_URL}/collections/${slug}/items/reorder`,
    SUBSCRIBE: (slug: string) => `${API_BASE_URL}/collections/${slug}/subscribe`,
    FEATURE: (slug: string) => `${API_BASE_URL}/collections/${slug}/feature`,
    // PSY-351: clone/fork an existing public collection.
    CLONE: (slug: string) => `${API_BASE_URL}/collections/${slug}/clone`,
    // PSY-352: like/unlike toggle. POST creates, DELETE removes; both
    // idempotent and return the post-mutation aggregate.
    LIKE: (slug: string) => `${API_BASE_URL}/collections/${slug}/like`,
    // PSY-354: collection-level tag management. Same body shape as the
    // entity-tag endpoints (tag_id OR tag_name + optional category).
    TAGS: (slug: string) => `${API_BASE_URL}/collections/${slug}/tags`,
    TAG: (slug: string, tagId: number) =>
      `${API_BASE_URL}/collections/${slug}/tags/${tagId}`,
    // PSY-366: artist-relationship subgraph for the collection's artist items.
    GRAPH: (slug: string) => `${API_BASE_URL}/collections/${slug}/graph`,
    MY: `${API_BASE_URL}/auth/collections`,
    // PSY-359: which of the user's own collections already contain a given
    // entity (entity_type + entity_id). Single round-trip pre-check that
    // backs the multi-select Add-to-Collection popover.
    CONTAINS: `${API_BASE_URL}/auth/collections/contains`,
    ENTITY: (entityType: string, entityId: number) =>
      `${API_BASE_URL}/collections/entity/${entityType}/${entityId}`,
    USER_PUBLIC: (username: string) =>
      `${API_BASE_URL}/users/${username}/collections`,
  },

  // Request endpoints
  REQUESTS: {
    LIST: `${API_BASE_URL}/requests`,
    GET: (requestId: string | number) => `${API_BASE_URL}/requests/${requestId}`,
    VOTE: (requestId: string | number) =>
      `${API_BASE_URL}/requests/${requestId}/vote`,
    FULFILL: (requestId: string | number) =>
      `${API_BASE_URL}/requests/${requestId}/fulfill`,
    CLOSE: (requestId: string | number) =>
      `${API_BASE_URL}/requests/${requestId}/close`,
  },

  // Tag endpoints
  TAGS: {
    LIST: `${API_BASE_URL}/tags`,
    SEARCH: `${API_BASE_URL}/tags/search`,
    GET: (idOrSlug: string | number) => `${API_BASE_URL}/tags/${idOrSlug}`,
    DETAIL: (idOrSlug: string | number) => `${API_BASE_URL}/tags/${idOrSlug}/detail`,
    ALIASES: (idOrSlug: string | number) => `${API_BASE_URL}/tags/${idOrSlug}/aliases`,
    DELETE_ALIAS: (tagId: number, aliasId: number) => `${API_BASE_URL}/tags/${tagId}/aliases/${aliasId}`,
    ENTITIES: (idOrSlug: string | number) => `${API_BASE_URL}/tags/${idOrSlug}/entities`,
    ADMIN_ALIASES_ALL: `${API_BASE_URL}/admin/tags/aliases`,
    ADMIN_ALIASES_BULK: `${API_BASE_URL}/admin/tags/aliases/bulk`,
    ADMIN_MERGE: (sourceId: number) => `${API_BASE_URL}/admin/tags/${sourceId}/merge`,
    ADMIN_MERGE_PREVIEW: (sourceId: number, targetId: number) =>
      `${API_BASE_URL}/admin/tags/${sourceId}/merge-preview?target_id=${targetId}`,
    ADMIN_LOW_QUALITY: `${API_BASE_URL}/admin/tags/low-quality`,
    ADMIN_SNOOZE: (tagId: number) => `${API_BASE_URL}/admin/tags/${tagId}/snooze`,
    // Admin bulk action on the low-quality queue (PSY-487).
    ADMIN_LOW_QUALITY_BULK: `${API_BASE_URL}/admin/tags/low-quality/bulk-action`,
    // Admin genre-hierarchy editor (PSY-311).
    ADMIN_HIERARCHY: `${API_BASE_URL}/admin/tags/hierarchy`,
    ADMIN_SET_PARENT: (tagId: number) => `${API_BASE_URL}/admin/tags/${tagId}/parent`,
  },

  // Entity tag endpoints
  ENTITY_TAGS: {
    LIST: (entityType: string, entityId: number) =>
      `${API_BASE_URL}/entities/${entityType}/${entityId}/tags`,
    ADD: (entityType: string, entityId: number) =>
      `${API_BASE_URL}/entities/${entityType}/${entityId}/tags`,
    REMOVE: (entityType: string, entityId: number, tagId: number) =>
      `${API_BASE_URL}/entities/${entityType}/${entityId}/tags/${tagId}`,
    VOTE: (tagId: number, entityType: string, entityId: number) =>
      `${API_BASE_URL}/tags/${tagId}/entities/${entityType}/${entityId}/votes`,
  },

  // Revision history endpoints
  REVISIONS: {
    ENTITY_HISTORY: (entityType: string, entityId: string | number) =>
      `${API_BASE_URL}/revisions/${entityType}/${entityId}`,
    DETAIL: (revisionId: number) =>
      `${API_BASE_URL}/revisions/${revisionId}`,
    USER_REVISIONS: (userId: string | number) =>
      `${API_BASE_URL}/users/${userId}/revisions`,
    ROLLBACK: (revisionId: number) =>
      `${API_BASE_URL}/admin/revisions/${revisionId}/rollback`,
  },

  // Attendance (going/interested) endpoints
  ATTENDANCE: {
    SHOW: (showId: number) => `${API_BASE_URL}/shows/${showId}/attendance`,
    BATCH: `${API_BASE_URL}/shows/attendance/batch`,
    MY_SHOWS: `${API_BASE_URL}/attendance/my-shows`,
  },

  // Follow endpoints
  FOLLOW: {
    ENTITY: (entityType: string, entityId: number) =>
      `${API_BASE_URL}/${entityType}/${entityId}/follow`,
    FOLLOWERS: (entityType: string, entityId: number) =>
      `${API_BASE_URL}/${entityType}/${entityId}/followers`,
    FOLLOWERS_LIST: (entityType: string, entityId: number) =>
      `${API_BASE_URL}/${entityType}/${entityId}/followers/list`,
    BATCH: `${API_BASE_URL}/follows/batch`,
    MY_FOLLOWING: `${API_BASE_URL}/me/following`,
  },

  // Scene endpoints
  SCENES: {
    LIST: `${API_BASE_URL}/scenes`,
    DETAIL: (slug: string) => `${API_BASE_URL}/scenes/${slug}`,
    ARTISTS: (slug: string) => `${API_BASE_URL}/scenes/${slug}/artists`,
    GENRES: (slug: string) => `${API_BASE_URL}/scenes/${slug}/genres`,
    GRAPH: (slug: string) => `${API_BASE_URL}/scenes/${slug}/graph`,
  },

  // Community endpoints (public)
  COMMUNITY: {
    LEADERBOARD: `${API_BASE_URL}/community/leaderboard`,
  },

  // Charts endpoints (public)
  CHARTS: {
    OVERVIEW: `${API_BASE_URL}/charts/overview`,
    TRENDING_SHOWS: `${API_BASE_URL}/charts/trending-shows`,
    POPULAR_ARTISTS: `${API_BASE_URL}/charts/popular-artists`,
    ACTIVE_VENUES: `${API_BASE_URL}/charts/active-venues`,
    HOT_RELEASES: `${API_BASE_URL}/charts/hot-releases`,
  },

  // System endpoints
  HEALTH: `${API_BASE_URL}/health`,
  OPENAPI: `${API_BASE_URL}/openapi.json`,
} as const

/**
 * Extended error type for API errors
 */
export interface ApiError extends Error {
  status?: number
  statusText?: string
  requestId?: string
  errorCode?: string
  details?: unknown
}

/**
 * Base response type that includes request ID (optional fields for compatibility)
 */
export interface ApiResponse {
  success?: boolean
  message?: string
  error_code?: string
  request_id?: string
  [key: string]: unknown // Allow additional properties
}

/**
 * Make API requests with proper configuration, error handling, and request ID extraction
 */
export const apiRequest = async <T = unknown>(
  endpoint: string,
  options: RequestInit = {}
): Promise<T> => {
  const defaultHeaders: Record<string, string> = {
    'Content-Type': 'application/json',
  }

  const config: RequestInit = {
    credentials: 'include', // Always include cookies for HTTP-only auth
    ...options,
    headers: {
      ...defaultHeaders,
      ...options.headers,
    },
  }

  const endpointPath = endpoint.replace(API_BASE_URL, '')
  const isAuthEndpoint = endpointPath.startsWith('/auth/')

  authLogger.debug('API request', {
    endpoint: endpointPath,
    method: config.method || 'GET',
  })

  let response: Response
  try {
    response = await fetch(endpoint, config)
  } catch (networkError) {
    // Network failure (backend unreachable, DNS failure, etc.)
    if (isAuthEndpoint) {
      Sentry.captureException(networkError, {
        level: 'error',
        tags: { service: 'auth', error_type: 'network_failure' },
        extra: { endpoint: endpointPath },
      })
    }
    throw networkError
  }

  // Extract request ID from response headers
  const requestId = response.headers.get(REQUEST_ID_HEADER) || undefined

  if (!response.ok) {
    const errorBody = await response.json().catch(() => ({
      message: `HTTP ${response.status}: ${response.statusText}`,
    }))

    // Extract error message - Huma uses 'detail', standard APIs use 'message'
    const errorMessage =
      errorBody.detail || errorBody.message || response.statusText

    // Log the API error with request ID
    authLogger.error(
      'API request failed',
      new Error(errorMessage),
      {
        endpoint: endpointPath,
        status: response.status,
        errorCode: errorBody.error_code,
      },
      requestId || errorBody.request_id
    )

    // Capture 5xx errors on auth endpoints to Sentry (service failures)
    if (response.status >= 500 && isAuthEndpoint) {
      Sentry.captureMessage(`Auth service error: ${response.status}`, {
        level: 'error',
        tags: {
          service: 'auth',
          error_type: 'service_error',
          status: response.status,
        },
        extra: {
          endpoint: endpointPath,
          errorCode: errorBody.error_code,
          requestId: requestId || errorBody.request_id,
        },
      })
    }

    // Check if this is an auth-related error
    if (response.status === 401 || response.status === 403) {
      throw new AuthError(
        errorMessage || 'Authentication failed',
        errorBody.error_code || AuthErrorCode.UNAUTHORIZED,
        {
          requestId: requestId || errorBody.request_id,
          status: response.status,
        }
      )
    }

    // Create a standard API error
    const apiError: ApiError = new Error(
      errorMessage || `HTTP ${response.status}: ${response.statusText}`
    )
    apiError.status = response.status
    apiError.statusText = response.statusText
    apiError.requestId = requestId || errorBody.request_id
    apiError.errorCode = errorBody.error_code
    apiError.details = errorBody.details || errorBody.errors || errorBody

    throw apiError
  }

  // Handle 204 No Content responses (e.g., DELETE operations)
  if (response.status === 204) {
    authLogger.debug(
      'API response',
      {
        endpoint: endpointPath,
        success: true,
      },
      requestId
    )
    return undefined as T
  }

  // Parse successful response
  const data = (await response.json()) as T

  // Inject request ID from header if not in response body (if data is an object)
  if (requestId && data && typeof data === 'object') {
    const dataObj = data as Record<string, unknown>
    if (!dataObj.request_id) {
      dataObj.request_id = requestId
    }
  }

  // Log response (safely access success property if it exists)
  const dataObj = data as Record<string, unknown> | null
  authLogger.debug(
    'API response',
    {
      endpoint: endpointPath,
      success: dataObj?.success,
    },
    requestId
  )

  return data
}

/**
 * Environment information for debugging
 */
export const getEnvironmentInfo = () => ({
  apiBaseUrl: API_BASE_URL,
  environment: process.env.NODE_ENV,
  isDevelopment: process.env.NODE_ENV === 'development',
  isProduction: process.env.NODE_ENV === 'production',
})

/**
 * Extract request ID from an error object
 */
export function getRequestIdFromError(error: unknown): string | undefined {
  if (error instanceof AuthError) {
    return error.requestId
  }
  if (error && typeof error === 'object' && 'requestId' in error) {
    return (error as ApiError).requestId
  }
  return undefined
}
