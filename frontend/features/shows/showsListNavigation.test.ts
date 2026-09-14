import { describe, it, expect } from 'vitest'
import { SHOWS_PAGE_SIZE, showsPageHref } from './showsListNavigation'

describe('showsPageHref', () => {
  it('writes no page param for page 1, so the list has one canonical address', () => {
    expect(showsPageHref(new URLSearchParams(), 1)).toBe('/shows')
  })

  it('writes the page number for every later page', () => {
    expect(showsPageHref(new URLSearchParams(), 2)).toBe('/shows?page=2')
  })

  // The list shares its query string with the city filter, the tag filter, and
  // whatever a campaign link brought along. A builder that minted fresh params
  // would drop all of it on every page click.
  it('carries every other param through untouched', () => {
    const href = showsPageHref(
      new URLSearchParams({
        cities: 'Phoenix,AZ',
        tags: 'post-punk',
        tag_match: 'any',
        utm_source: 'newsletter',
      }),
      3
    )

    const query = new URLSearchParams(href.split('?')[1])
    expect(query.get('cities')).toBe('Phoenix,AZ')
    expect(query.get('tags')).toBe('post-punk')
    expect(query.get('tag_match')).toBe('any')
    expect(query.get('utm_source')).toBe('newsletter')
    expect(query.get('page')).toBe('3')
  })

  it('strips an existing page param on the way back to page 1', () => {
    const href = showsPageHref(
      new URLSearchParams({ cities: 'Phoenix,AZ', page: '4' }),
      1
    )

    expect(href).toBe('/shows?cities=Phoenix%2CAZ')
  })

  it('replaces rather than appends an existing page param', () => {
    const href = showsPageHref(new URLSearchParams({ page: '4' }), 2)

    expect(href).toBe('/shows?page=2')
  })
})

describe('SHOWS_PAGE_SIZE', () => {
  // Stated rather than inherited from the endpoint's own default: the pager's
  // arithmetic has to agree with the limit the request actually carried.
  it('is the page size the list requests', () => {
    expect(SHOWS_PAGE_SIZE).toBe(50)
  })
})
