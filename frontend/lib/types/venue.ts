/**
 * Venue-related TypeScript types
 *
 * These types match the backend API response structures
 * from backend/internal/services/venue.go
 */

import type { ArtistResponse } from './show'

export interface Venue {
  id: number
  slug: string
  name: string
  address: string | null
  city: string
  state: string
  zipcode?: string | null
  verified: boolean
  submitted_by?: number | null
  social?: {
    instagram?: string | null
    facebook?: string | null
    twitter?: string | null
    youtube?: string | null
    spotify?: string | null
    soundcloud?: string | null
    bandcamp?: string | null
    website?: string | null
  }
  created_at: string
  updated_at: string
}

export interface VenueSearchParams {
  query: string
}

export interface VenueSearchResponse {
  venues: Venue[]
  count: number
}

/**
 * Venue with upcoming show count for the venues list
 */
export interface VenueWithShowCount extends Venue {
  upcoming_show_count: number
}

/**
 * Response for the venues list endpoint
 */
export interface VenuesListResponse {
  venues: VenueWithShowCount[]
  total: number
  limit: number
  offset: number
}

/**
 * Show response in the venue shows endpoint
 */
export interface VenueShow {
  id: number
  slug: string
  title: string
  event_date: string
  city: string | null
  state: string | null
  price: number | null
  age_requirement: string | null
  artists: ArtistResponse[]
}

/**
 * Response for the venue shows endpoint
 */
export interface VenueShowsResponse {
  shows: VenueShow[]
  venue_id: number
  total: number
}

/**
 * City with venue count for filtering
 */
export interface VenueCity {
  city: string
  state: string
  venue_count: number
}

/**
 * Response for the venue cities endpoint
 */
export interface VenueCitiesResponse {
  cities: VenueCity[]
}

/**
 * Get a formatted location string for a venue
 */
export const getVenueLocation = (venue: Venue): string => {
  return `${venue.city}, ${venue.state}`
}

// ============================================================================
// Venue Editing Types
// ============================================================================

/**
 * Status of a pending venue edit
 */
export type VenueEditStatus = 'pending' | 'approved' | 'rejected'

/**
 * Request to update a venue
 */
export interface VenueEditRequest {
  name?: string
  address?: string
  city?: string
  state?: string
  zipcode?: string
  instagram?: string
  facebook?: string
  twitter?: string
  youtube?: string
  spotify?: string
  soundcloud?: string
  bandcamp?: string
  website?: string
}

/**
 * Pending venue edit awaiting admin approval
 */
export interface PendingVenueEdit {
  id: number
  venue_id: number
  submitted_by: number
  status: VenueEditStatus

  // Proposed changes
  name?: string | null
  address?: string | null
  city?: string | null
  state?: string | null
  zipcode?: string | null
  instagram?: string | null
  facebook?: string | null
  twitter?: string | null
  youtube?: string | null
  spotify?: string | null
  soundcloud?: string | null
  bandcamp?: string | null
  website?: string | null

  // Workflow fields
  rejection_reason?: string | null
  reviewed_by?: number | null
  reviewed_at?: string | null

  created_at: string
  updated_at: string

  // Embedded venue info
  venue?: Venue
  submitter_name?: string
  reviewer_name?: string
}

/**
 * Response from updating a venue (admin or non-admin)
 */
export interface UpdateVenueResponse {
  venue?: Venue
  pending_edit?: PendingVenueEdit
  status: 'updated' | 'pending'
  message: string
}

/**
 * Response for getting user's pending edit
 */
export interface MyPendingEditResponse {
  pending_edit: PendingVenueEdit | null
}

/**
 * Response for admin listing pending venue edits
 */
export interface PendingVenueEditsResponse {
  edits: PendingVenueEdit[]
  total: number
}

