// Radix Select reserves the empty string for "no value", so admin list
// filters whose "All …" option means "no filter" (state held as '') cannot
// use value="" on a SelectItem. This sentinel represents the "All" option in
// the Select and maps back to '' in the filter state.
//
// Mirrors the PSY-907 precedent in
// app/admin/radio/_components/playlistSourceSelect.ts, generalized for the
// PSY-924 filter migration where four entity managers (releases, labels,
// festivals, tags) share the same ''-means-no-filter contract.
export const FILTER_SELECT_ALL = 'all'

export const toFilterSelectValue = (value: string) =>
  value || FILTER_SELECT_ALL

export const fromFilterSelectValue = (value: string) =>
  value === FILTER_SELECT_ALL ? '' : value
