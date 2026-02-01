/**
 * Types for AI-powered show information extraction
 */

/**
 * Request to extract show information from text, image, or both
 */
export interface ExtractShowRequest {
  /** Type of input: 'text' for pasted text, 'image' for uploaded image, 'both' for image with additional context */
  type: 'text' | 'image' | 'both'
  /** Text content (required when type is 'text', optional context when type is 'both') */
  text?: string
  /** Base64-encoded image data (required when type is 'image' or 'both') */
  image_data?: string
  /** MIME type of the image (required when type is 'image' or 'both') */
  media_type?: 'image/jpeg' | 'image/png' | 'image/gif' | 'image/webp'
}

/**
 * Extracted artist with optional database match
 */
export interface ExtractedArtist {
  /** Artist name as extracted from input */
  name: string
  /** Whether this artist is the headliner */
  is_headliner: boolean
  /** Database ID if matched to existing artist */
  matched_id?: number
  /** Canonical name from database (may differ in casing) */
  matched_name?: string
  /** Slug from database if matched */
  matched_slug?: string
}

/**
 * Extracted venue with optional database match
 */
export interface ExtractedVenue {
  /** Venue name as extracted from input */
  name: string
  /** City if found in input */
  city?: string
  /** State if found in input (2-letter code) */
  state?: string
  /** Database ID if matched to existing venue */
  matched_id?: number
  /** Canonical name from database (may differ in casing) */
  matched_name?: string
  /** Slug from database if matched */
  matched_slug?: string
}

/**
 * Full extraction result from AI processing
 */
export interface ExtractedShowData {
  /** List of extracted artists */
  artists: ExtractedArtist[]
  /** Extracted venue information */
  venue?: ExtractedVenue
  /** Event date in YYYY-MM-DD format */
  date?: string
  /** Event time in HH:MM format (24-hour) */
  time?: string
  /** Ticket cost as string (e.g., "$20", "Free") */
  cost?: string
  /** Age requirement (e.g., "21+", "All Ages") */
  ages?: string
  /** Any additional description or notes */
  description?: string
}

/**
 * API response wrapper for extraction endpoint
 */
export interface ExtractShowResponse {
  /** Whether extraction was successful */
  success: boolean
  /** Extracted show data */
  data?: ExtractedShowData
  /** Error message if extraction failed */
  error?: string
  /** Warnings about partial extraction or uncertain matches */
  warnings?: string[]
}
