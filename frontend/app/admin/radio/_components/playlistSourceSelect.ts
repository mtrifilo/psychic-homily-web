// Playlist-source options offered in the Radio station create/edit forms.
export const PLAYLIST_SOURCES = [
  { value: 'kexp_api', label: 'KEXP API' },
  { value: 'wfmu_scrape', label: 'WFMU Scrape' },
  { value: 'nts_api', label: 'NTS API' },
  { value: 'manual', label: 'Manual' },
] as const

// Radix Select reserves the empty string for "no value", so the "None"
// (no playlist source) state is represented by this sentinel in the Select
// and mapped back to '' in component state. These two helpers are the only
// non-mechanical part of the PSY-907 Select migration, so they live here with
// a focused test rather than inline.
export const PLAYLIST_SOURCE_NONE = 'none'

export const toPlaylistSelectValue = (source: string) =>
  source || PLAYLIST_SOURCE_NONE

export const fromPlaylistSelectValue = (value: string) =>
  value === PLAYLIST_SOURCE_NONE ? '' : value
