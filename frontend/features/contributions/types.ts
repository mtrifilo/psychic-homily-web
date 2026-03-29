export type PendingEditStatus = 'pending' | 'approved' | 'rejected'

export type EditableEntityType = 'artist' | 'venue' | 'festival'

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

/** Editable fields per entity type — matches backend allowedEditFields. */
export const EDITABLE_FIELDS: Record<EditableEntityType, EditableField[]> = {
  artist: [
    { key: 'name', label: 'Name', type: 'text', group: 'info' },
    { key: 'city', label: 'City', type: 'text', group: 'info' },
    { key: 'state', label: 'State', type: 'text', group: 'info' },
    { key: 'country', label: 'Country', type: 'text', group: 'info' },
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
}
