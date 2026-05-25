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
 * A close-but-not-exact match suggestion from the database
 */
export interface MatchSuggestion {
  id: number
  name: string
  slug: string
}

/**
 * Venue match suggestion with location info
 */
export interface VenueMatchSuggestion extends MatchSuggestion {
  city: string
  state: string
}

/**
 * Extracted artist with optional database match
 */
export interface ExtractedArtist {
  /** Artist name as extracted from input */
  name: string
  /** Whether this artist is the headliner */
  is_headliner: boolean
  /** Instagram handle extracted from flyer (only for new/unmatched artists) */
  instagram_handle?: string
  /** Database ID if matched to existing artist */
  matched_id?: number
  /** Canonical name from database (may differ in casing) */
  matched_name?: string
  /** Slug from database if matched */
  matched_slug?: string
  /** Close matches when no exact match found */
  suggestions?: MatchSuggestion[]
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
  /** Close matches when no exact match found */
  suggestions?: VenueMatchSuggestion[]
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

// ─────────────────────────────────────────────────────────────
// AI-assisted collection extraction
// ─────────────────────────────────────────────────────────────

/**
 * Request to extract a collection's items from pasted article text or a
 * screenshot of one. Same input shape as show extraction so AICollectionFiller
 * stays close to the AIFormFiller chrome.
 */
export interface ExtractCollectionRequest {
  type: 'text' | 'image' | 'both'
  text?: string
  image_data?: string
  media_type?: 'image/jpeg' | 'image/png' | 'image/gif' | 'image/webp'
}

/**
 * One row in an AI-extracted collection. Mirrors the shape ExtractedArtist
 * uses for show extraction so consumers can apply the same matched/suggestion
 * UX pattern. Carries an OPTIONAL release_title — many canon lists are
 * "best releases" so most rows will have one, but pure artist lists (e.g.
 * "100 best artists of the decade") have just the artist.
 *
 * V1 matches against artists.name (case-insensitive) + artist_aliases.alias;
 * release_title is preserved verbatim in the response but NOT matched against
 * the releases table (deferred to a follow-up — release matching needs the
 * artist match to scope correctly, and same-title releases by different
 * artists is common enough to need its own picker UX).
 */
export interface ExtractedCollectionItem {
  /** Artist name as extracted from input */
  artist_name: string
  /** Release/album title when the source is a "best releases" list */
  release_title?: string
  /** Database ID if the artist was matched to an existing entity */
  matched_artist_id?: number
  /** Canonical artist name from database (may differ in casing) */
  matched_artist_name?: string
  /** Slug from database if matched */
  matched_artist_slug?: string
  /** Close matches when no exact artist match found */
  artist_suggestions?: MatchSuggestion[]
}

/**
 * Full extraction result. Items are returned in source order — for canon
 * lists ("100 best albums of the 2010s") order is the user's primary
 * curation signal.
 */
export interface ExtractedCollectionData {
  /** Optional source label captured from the article (e.g. "Pitchfork's 200 Best Albums of the 2010s") */
  source?: string
  /** Optional description for the collection (suggested title / summary from the article) */
  description?: string
  /** Per-row items in source order. */
  items: ExtractedCollectionItem[]
}

/**
 * API response wrapper. Same envelope shape as ExtractShowResponse so
 * AICollectionFiller's error/warning handling can mirror AIFormFiller's.
 */
export interface ExtractCollectionResponse {
  success: boolean
  data?: ExtractedCollectionData
  error?: string
  warnings?: string[]
}
