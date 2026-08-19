import { describe, it, expect } from 'vitest'
import { flyerImageSrc, sourceVenueName } from './showFlyer'
import type { ShowResponse, VenueResponse } from '../types'

function venue(overrides: Partial<VenueResponse> = {}): VenueResponse {
  return {
    id: 1,
    slug: 'the-venue',
    name: 'The Venue',
    city: 'Phoenix',
    state: 'AZ',
    verified: true,
    ...overrides,
  }
}

describe('flyerImageSrc', () => {
  it.each([
    ['a plain https url', 'https://flyers.example/poster.jpg'],
    ['a url with a query and fragment', 'https://flyers.example/p.jpg?v=2#x'],
  ])('accepts %s', (_label, value) => {
    expect(flyerImageSrc({ image_url: value })).toBe(value)
  })

  // The backend stores the column untrimmed and `new URL` normalises the rest,
  // so the returned value is the parsed href rather than what was stored.
  it.each([
    [
      'strips surrounding whitespace',
      '  https://flyers.example/poster.jpg  ',
      'https://flyers.example/poster.jpg',
    ],
    [
      'strips an embedded newline',
      'https://flyers.example/pos\nter.jpg',
      'https://flyers.example/poster.jpg',
    ],
    [
      'adds the implicit root path',
      'https://flyers.example',
      'https://flyers.example/',
    ],
  ])('%s', (_label, value, expected) => {
    expect(flyerImageSrc({ image_url: value })).toBe(expected)
  })

  // `image_url` is writable by any email-verified submitter, so everything
  // that is not an https URL has to resolve to "no flyer" rather than reach
  // an <img src>.
  it.each([
    ['null', null],
    ['undefined', undefined],
    ['an empty string', ''],
    ['whitespace only', '   '],
    ['a relative path', '/uploads/flyer.jpg'],
    // The backend accepts http and such rows exist, but the app's CSP is
    // `img-src ... https:`, so an http flyer can only ever be blocked.
    ['an http url the CSP would block', 'http://flyers.example/poster.jpg'],
    ['a protocol-relative url', '//flyers.example/poster.jpg'],
    ['a bare host, which is NOT repaired into https', 'flyers.example/p.jpg'],
    ['a data url', 'data:image/png;base64,AAAA'],
    ['a javascript url', 'javascript:alert(1)'],
    ['a file url', 'file:///etc/passwd'],
    ['an unparseable value', 'not a url at all'],
  ])('rejects %s', (_label, value) => {
    expect(flyerImageSrc({ image_url: value })).toBeNull()
  })
})

describe('sourceVenueName', () => {
  it('names the venue whose slug the listing was scraped from', () => {
    expect(
      sourceVenueName({ source_venue: 'the-venue', venues: [venue()] })
    ).toBe('The Venue')
  })

  it('picks the matching venue out of several', () => {
    expect(
      sourceVenueName({
        source_venue: 'second-venue',
        venues: [
          venue(),
          venue({ id: 2, slug: 'second-venue', name: 'Second Venue' }),
        ],
      })
    ).toBe('Second Venue')
  })

  // Every branch below prints NO credit rather than a guess: a credit is a
  // factual claim about where an image came from.
  it.each([
    ['a user-submitted show with no source', undefined, [venue()]],
    ['a whitespace-only source', '   ', [venue()]],
    ['a source that matches no venue', 'somewhere-else', [venue()]],
    ['a show with no venues at all', 'the-venue', []],
    [
      'a matched venue with a blank name',
      'the-venue',
      [venue({ name: '   ' })],
    ],
  ])(
    'credits nobody for %s',
    (_label, sourceVenue: string | undefined, venues: VenueResponse[]) => {
      expect(
        sourceVenueName({
          source_venue: sourceVenue,
          venues: venues as ShowResponse['venues'],
        })
      ).toBeNull()
    }
  )
})
