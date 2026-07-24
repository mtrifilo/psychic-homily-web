import type { RadioPlay } from '../types'

/**
 * Test-only RadioPlay factory shared by the playlist-page component tests
 * (PlaylistTable, EpisodeDateDetail). An unmatched play by default; pass
 * artist_id/artist_slug for the matched variant.
 */
export function makeRadioPlay(overrides: Partial<RadioPlay> = {}): RadioPlay {
  return {
    id: 1,
    episode_id: 10,
    position: 1,
    artist_name: 'CAN',
    track_title: 'Mother Sky',
    album_title: 'Soundtracks',
    label_name: 'United Artists',
    release_year: 1970,
    is_new: false,
    rotation_status: null,
    dj_comment: null,
    is_live_performance: false,
    is_request: false,
    artist_id: null,
    artist_slug: null,
    release_id: null,
    release_slug: null,
    label_id: null,
    label_slug: null,
    musicbrainz_artist_id: null,
    musicbrainz_recording_id: null,
    musicbrainz_release_id: null,
    air_timestamp: null,
    ...overrides,
  }
}
