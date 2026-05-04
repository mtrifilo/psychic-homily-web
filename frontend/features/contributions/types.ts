// ──────────────────────────────────────────────
// Data Quality / Contribution Opportunities
// ──────────────────────────────────────────────

export interface DataQualityCategory {
  key: string
  label: string
  entity_type: string
  count: number
  description: string
}

export interface DataQualitySummary {
  categories: DataQualityCategory[]
  total_items: number
}

export interface DataQualityItem {
  entity_type: string
  entity_id: number
  name: string
  slug: string
  reason: string
  show_count: number
}

// ──────────────────────────────────────────────
// Pending Edits
// ──────────────────────────────────────────────

export type PendingEditStatus = 'pending' | 'approved' | 'rejected'

export type EditableEntityType = 'artist' | 'venue' | 'festival' | 'release' | 'label'

export interface FieldChange {
  field: string
  old_value: unknown
  new_value: unknown
}

export interface PendingEditResponse {
  id: number
  entity_type: string
  entity_id: number
  submitted_by: number
  submitter_name: string
  field_changes: FieldChange[]
  summary: string
  status: PendingEditStatus
  reviewed_by?: number
  reviewer_name?: string
  reviewed_at?: string
  rejection_reason?: string
  created_at: string
  updated_at: string
}

export interface SuggestEditResponse {
  pending_edit?: PendingEditResponse
  applied: boolean
  message: string
}

export interface SuggestEditRequest {
  changes: FieldChange[]
  summary: string
}

/** Field configuration for the edit drawer. */
export interface EditableField {
  key: string
  label: string
  type: 'text' | 'textarea' | 'url'
  placeholder?: string
  group?: 'info' | 'social' | 'details'
}

export type ReportableEntityType = 'artist' | 'venue' | 'festival' | 'show' | 'comment' | 'collection'

export interface ReportTypeOption {
  value: string
  label: string
  description: string
}

/** Report type options per entity type — matches backend entity_reports report_type values. */
export const REPORT_TYPES: Record<ReportableEntityType, ReportTypeOption[]> = {
  artist: [
    { value: 'inaccurate', label: 'Inaccurate Information', description: 'Name, bio, social links, or other info is wrong' },
    { value: 'duplicate', label: 'Duplicate Artist', description: 'This artist already exists under a different name' },
    { value: 'wrong_image', label: 'Wrong Image', description: 'The artist image is incorrect' },
    { value: 'removal_request', label: 'Removal Request', description: 'This artist page should be removed' },
    { value: 'missing_info', label: 'Missing Information', description: 'Important information is missing' },
  ],
  venue: [
    { value: 'closed_permanently', label: 'Permanently Closed', description: 'This venue has permanently closed' },
    { value: 'wrong_address', label: 'Wrong Address', description: 'The address or location is incorrect' },
    { value: 'duplicate', label: 'Duplicate Venue', description: 'This venue already exists under a different name' },
    { value: 'inaccurate', label: 'Inaccurate Information', description: 'Name, details, or other info is wrong' },
    { value: 'missing_info', label: 'Missing Information', description: 'Important information is missing' },
  ],
  festival: [
    { value: 'cancelled', label: 'Cancelled', description: 'This festival has been cancelled' },
    { value: 'wrong_dates', label: 'Wrong Dates', description: 'The festival dates are incorrect' },
    { value: 'duplicate', label: 'Duplicate Festival', description: 'This festival already exists' },
    { value: 'inaccurate', label: 'Inaccurate Information', description: 'Information is wrong or outdated' },
  ],
  show: [
    { value: 'cancelled', label: 'Cancelled', description: 'This show has been cancelled' },
    { value: 'sold_out', label: 'Sold Out', description: 'This show is sold out' },
    { value: 'inaccurate', label: 'Inaccurate Information', description: 'Date, time, venue, or other info is wrong' },
    { value: 'wrong_venue', label: 'Wrong Venue', description: 'This show is listed at the wrong venue' },
    { value: 'wrong_date', label: 'Wrong Date', description: 'The show date or time is incorrect' },
  ],
  comment: [
    { value: 'spam', label: 'Spam', description: 'This comment is spam or advertising' },
    { value: 'harassment', label: 'Harassment', description: 'This comment is abusive or harassing' },
    { value: 'off_topic', label: 'Off Topic', description: 'This comment is irrelevant to the discussion' },
    { value: 'inaccurate', label: 'Inaccurate', description: 'This comment contains incorrect information' },
    { value: 'other', label: 'Other', description: 'Another issue not listed above' },
  ],
  // PSY-357: collection reuses the comment vocabulary verbatim — both are
  // user-generated content surfaces with the same abuse vectors.
  collection: [
    { value: 'spam', label: 'Spam', description: 'This collection is spam or advertising' },
    { value: 'harassment', label: 'Harassment', description: 'This collection is abusive or harassing' },
    { value: 'off_topic', label: 'Off Topic', description: 'This collection is irrelevant or misplaced' },
    { value: 'inaccurate', label: 'Inaccurate', description: 'This collection contains incorrect information' },
    { value: 'other', label: 'Other', description: 'Another issue not listed above' },
  ],
}

/** Editable fields per entity type — matches backend allowedEditFields. */
export const EDITABLE_FIELDS: Record<EditableEntityType, EditableField[]> = {
  artist: [
    { key: 'name', label: 'Name', type: 'text', group: 'info' },
    { key: 'city', label: 'City', type: 'text', group: 'info' },
    { key: 'state', label: 'State', type: 'text', group: 'info' },
    { key: 'country', label: 'Country', type: 'text', group: 'info' },
    { key: 'image_url', label: 'Image URL', type: 'url', placeholder: 'https://...', group: 'info' },
    { key: 'description', label: 'Description', type: 'textarea', group: 'details' },
    { key: 'instagram', label: 'Instagram', type: 'url', placeholder: 'https://instagram.com/...', group: 'social' },
    { key: 'facebook', label: 'Facebook', type: 'url', placeholder: 'https://facebook.com/...', group: 'social' },
    { key: 'twitter', label: 'X / Twitter', type: 'url', placeholder: 'https://x.com/...', group: 'social' },
    { key: 'youtube', label: 'YouTube', type: 'url', placeholder: 'https://youtube.com/...', group: 'social' },
    { key: 'spotify', label: 'Spotify', type: 'url', placeholder: 'https://open.spotify.com/...', group: 'social' },
    { key: 'soundcloud', label: 'SoundCloud', type: 'url', placeholder: 'https://soundcloud.com/...', group: 'social' },
    { key: 'bandcamp', label: 'Bandcamp', type: 'url', placeholder: 'https://....bandcamp.com', group: 'social' },
    { key: 'website', label: 'Website', type: 'url', placeholder: 'https://...', group: 'social' },
  ],
  venue: [
    { key: 'name', label: 'Name', type: 'text', group: 'info' },
    { key: 'address', label: 'Address', type: 'text', group: 'info' },
    { key: 'city', label: 'City', type: 'text', group: 'info' },
    { key: 'state', label: 'State', type: 'text', group: 'info' },
    { key: 'country', label: 'Country', type: 'text', group: 'info' },
    { key: 'zipcode', label: 'Zipcode', type: 'text', group: 'info' },
    { key: 'image_url', label: 'Image URL', type: 'url', placeholder: 'https://...', group: 'info' },
    { key: 'description', label: 'Description', type: 'textarea', group: 'details' },
    { key: 'instagram', label: 'Instagram', type: 'url', placeholder: 'https://instagram.com/...', group: 'social' },
    { key: 'facebook', label: 'Facebook', type: 'url', placeholder: 'https://facebook.com/...', group: 'social' },
    { key: 'twitter', label: 'X / Twitter', type: 'url', placeholder: 'https://x.com/...', group: 'social' },
    { key: 'youtube', label: 'YouTube', type: 'url', placeholder: 'https://youtube.com/...', group: 'social' },
    { key: 'spotify', label: 'Spotify', type: 'url', placeholder: 'https://open.spotify.com/...', group: 'social' },
    { key: 'soundcloud', label: 'SoundCloud', type: 'url', placeholder: 'https://soundcloud.com/...', group: 'social' },
    { key: 'bandcamp', label: 'Bandcamp', type: 'url', placeholder: 'https://....bandcamp.com', group: 'social' },
    { key: 'website', label: 'Website', type: 'url', placeholder: 'https://...', group: 'social' },
  ],
  festival: [
    { key: 'name', label: 'Name', type: 'text', group: 'info' },
    { key: 'description', label: 'Description', type: 'textarea', group: 'details' },
    { key: 'location_name', label: 'Location Name', type: 'text', group: 'info' },
    { key: 'city', label: 'City', type: 'text', group: 'info' },
    { key: 'state', label: 'State', type: 'text', group: 'info' },
    { key: 'country', label: 'Country', type: 'text', group: 'info' },
    { key: 'website', label: 'Website', type: 'url', placeholder: 'https://...', group: 'info' },
    { key: 'ticket_url', label: 'Ticket URL', type: 'url', placeholder: 'https://...', group: 'info' },
    { key: 'flyer_url', label: 'Flyer URL', type: 'url', placeholder: 'https://...', group: 'info' },
  ],
  release: [
    { key: 'title', label: 'Title', type: 'text', group: 'info' },
    { key: 'release_type', label: 'Release Type', type: 'text', placeholder: 'lp, ep, single, compilation, live, remix, demo', group: 'info' },
    { key: 'release_year', label: 'Release Year', type: 'text', placeholder: '1991', group: 'info' },
    { key: 'release_date', label: 'Release Date', type: 'text', placeholder: 'YYYY-MM-DD', group: 'info' },
    { key: 'cover_art_url', label: 'Cover Art URL', type: 'url', placeholder: 'https://...', group: 'info' },
    { key: 'description', label: 'Description', type: 'textarea', group: 'details' },
  ],
  label: [
    { key: 'name', label: 'Name', type: 'text', group: 'info' },
    { key: 'founded_year', label: 'Founded Year', type: 'text', placeholder: '1985', group: 'info' },
    { key: 'city', label: 'City', type: 'text', group: 'info' },
    { key: 'state', label: 'State', type: 'text', group: 'info' },
    { key: 'country', label: 'Country', type: 'text', group: 'info' },
    { key: 'image_url', label: 'Image URL', type: 'url', placeholder: 'https://...', group: 'info' },
    { key: 'description', label: 'Description', type: 'textarea', group: 'details' },
    { key: 'instagram', label: 'Instagram', type: 'url', placeholder: 'https://instagram.com/...', group: 'social' },
    { key: 'facebook', label: 'Facebook', type: 'url', placeholder: 'https://facebook.com/...', group: 'social' },
    { key: 'twitter', label: 'X / Twitter', type: 'url', placeholder: 'https://x.com/...', group: 'social' },
    { key: 'youtube', label: 'YouTube', type: 'url', placeholder: 'https://youtube.com/...', group: 'social' },
    { key: 'spotify', label: 'Spotify', type: 'url', placeholder: 'https://open.spotify.com/...', group: 'social' },
    { key: 'soundcloud', label: 'SoundCloud', type: 'url', placeholder: 'https://soundcloud.com/...', group: 'social' },
    { key: 'bandcamp', label: 'Bandcamp', type: 'url', placeholder: 'https://....bandcamp.com', group: 'social' },
    { key: 'website', label: 'Website', type: 'url', placeholder: 'https://...', group: 'social' },
  ],
}
