import { describe, it, expect } from 'vitest'
import {
  generateOrganizationSchema,
  generateWebSiteSchema,
  generateBreadcrumbSchema,
  generateMusicEventSchema,
  generateBlogPostingSchema,
  generateMusicVenueSchema,
  generateMusicGroupSchema,
  generateMusicRecordingSchema,
  generateItemListSchema,
  renderJsonLd,
} from './jsonld'

describe('generateOrganizationSchema', () => {
  it('returns correct static fields', () => {
    const schema = generateOrganizationSchema()
    expect(schema['@context']).toBe('https://schema.org')
    expect(schema['@type']).toBe('Organization')
    expect(schema.name).toBe('Psychic Homily')
    expect(schema.url).toBe('https://psychichomily.com')
    expect(schema.description).toBeDefined()
    expect(schema.logo).toBe('https://psychichomily.com/og-image.jpg')
  })
})

describe('generateWebSiteSchema', () => {
  it('returns correct static fields', () => {
    const schema = generateWebSiteSchema()
    expect(schema['@context']).toBe('https://schema.org')
    expect(schema['@type']).toBe('WebSite')
    expect(schema.name).toBe('Psychic Homily')
    expect(schema.url).toBe('https://psychichomily.com')
  })
})

describe('generateBreadcrumbSchema', () => {
  it('generates correct positions starting at 1', () => {
    const schema = generateBreadcrumbSchema([
      { name: 'Home', url: 'https://psychichomily.com' },
      { name: 'Shows', url: 'https://psychichomily.com/shows' },
    ])
    expect(schema['@type']).toBe('BreadcrumbList')
    expect(schema.itemListElement).toHaveLength(2)
    expect(schema.itemListElement[0].position).toBe(1)
    expect(schema.itemListElement[1].position).toBe(2)
  })

  it('maps name and item correctly', () => {
    const schema = generateBreadcrumbSchema([
      { name: 'Home', url: 'https://psychichomily.com' },
      { name: 'Venues', url: 'https://psychichomily.com/venues' },
      { name: 'The Rebel Lounge', url: 'https://psychichomily.com/venues/the-rebel-lounge' },
    ])
    expect(schema.itemListElement[2]).toEqual({
      '@type': 'ListItem',
      position: 3,
      name: 'The Rebel Lounge',
      item: 'https://psychichomily.com/venues/the-rebel-lounge',
    })
  })

  it('handles empty items array', () => {
    const schema = generateBreadcrumbSchema([])
    expect(schema.itemListElement).toHaveLength(0)
  })
})

describe('generateMusicEventSchema', () => {
  const baseShow = {
    date: '2026-03-15T20:00:00Z',
    // `state` carries the zone: without it the venue's zone is a GUESS and
    // `startDate` degrades to a bare date, which would make every assertion
    // below about the offset form vacuous.
    venue: { name: 'The Rebel Lounge', state: 'AZ' },
    artists: [{ name: 'Test Band', is_headliner: true }],
  }

  // Required fields
  it('includes all Google-required fields', () => {
    const schema = generateMusicEventSchema(baseShow)
    expect(schema['@context']).toBe('https://schema.org')
    expect(schema['@type']).toBe('MusicEvent')
    expect(schema.name).toBeDefined()
    // startDate is rendered in the venue's local time with offset (PSY-986):
    // 20:00Z at The Rebel Lounge (AZ → America/Phoenix, UTC-7) = 1:00 PM.
    expect(schema.startDate).toBe('2026-03-15T13:00:00-07:00')
    expect(schema.location).toBeDefined()
    expect(schema.eventAttendanceMode).toBe('https://schema.org/OfflineEventAttendanceMode')
    expect(schema.eventStatus).toBeDefined()
  })

  describe('startDate says only as much as the zone supports', () => {
    it('carries the venue-local offset when the venue supplies a zone', () => {
      const schema = generateMusicEventSchema({
        ...baseShow,
        venue: { name: 'Berghain', state: '', timezone: 'Europe/Berlin' },
      })
      expect(schema.startDate).toBe('2026-03-15T21:00:00+01:00')
    })

    it('degrades to a bare calendar date when the state is blank', () => {
      const schema = generateMusicEventSchema({
        ...baseShow,
        venue: { name: 'Hall Ohne Zone', state: '' },
      })
      expect(schema.startDate).toBe('2026-03-15')
    })

    it('degrades to a bare calendar date for a state outside the US map', () => {
      // Naming a region is not naming a zone: `resolveShowTimezone` answers
      // FALLBACK_SHOW_TIMEZONE for 'England' exactly as it does for ''.
      const schema = generateMusicEventSchema({
        ...baseShow,
        venue: { name: 'The Windmill', state: 'England' },
      })
      expect(schema.startDate).toBe('2026-03-15')
    })

    it('is the day the fallback reads, not the UTC day', () => {
      // 03:00Z is the evening BEFORE in the fallback zone (UTC-7), which is the
      // day the submit path composed. The date-only form keeps that day rather
      // than moving the show forward by one.
      const schema = generateMusicEventSchema({
        ...baseShow,
        date: '2026-03-15T03:00:00Z',
        venue: { name: 'Hall Ohne Zone', state: '' },
      })
      expect(schema.startDate).toBe('2026-03-14')
    })

    it('emits the raw instant for a show with no venue at all', () => {
      const schema = generateMusicEventSchema({
        ...baseShow,
        venue: undefined,
      })
      expect(schema.startDate).toBe('2026-03-15T20:00:00Z')
    })
  })

  // Name generation
  it('uses explicit name when provided', () => {
    const schema = generateMusicEventSchema({ ...baseShow, name: 'Spring Fest 2026' })
    expect(schema.name).toBe('Spring Fest 2026')
  })

  it('falls back to headliner at venue', () => {
    const schema = generateMusicEventSchema(baseShow)
    expect(schema.name).toBe('Test Band at The Rebel Lounge')
  })

  it('falls back to first artist when no headliner', () => {
    const schema = generateMusicEventSchema({
      date: '2026-03-15T20:00:00Z',
      venue: { name: 'Crescent Ballroom' },
      artists: [
        { name: 'Opener', is_headliner: false },
        { name: 'Closer', is_headliner: false },
      ],
    })
    expect(schema.name).toBe('Opener at Crescent Ballroom')
  })

  it('falls back to "Live Music" when no artists', () => {
    const schema = generateMusicEventSchema({
      date: '2026-03-15T20:00:00Z',
      venue: { name: 'Valley Bar' },
    })
    expect(schema.name).toBe('Live Music at Valley Bar')
  })

  it('falls back to "TBA" when no venue', () => {
    const schema = generateMusicEventSchema({
      date: '2026-03-15T20:00:00Z',
      artists: [{ name: 'Test Band', is_headliner: true }],
    })
    expect(schema.name).toBe('Test Band at TBA')
  })

  // Event status
  it('sets EventCancelled when is_cancelled is true', () => {
    const schema = generateMusicEventSchema({ ...baseShow, is_cancelled: true })
    expect(schema.eventStatus).toBe('https://schema.org/EventCancelled')
  })

  it('sets EventScheduled when not cancelled', () => {
    const schema = generateMusicEventSchema(baseShow)
    expect(schema.eventStatus).toBe('https://schema.org/EventScheduled')
  })

  // Venue / location
  it('sets venue name in location', () => {
    const schema = generateMusicEventSchema(baseShow)
    expect(schema.location.name).toBe('The Rebel Lounge')
  })

  it('sets location name to TBA when no venue', () => {
    const schema = generateMusicEventSchema({ date: '2026-03-15T20:00:00Z' })
    expect(schema.location.name).toBe('TBA')
  })

  it('generates venue URL from slug', () => {
    const schema = generateMusicEventSchema({
      ...baseShow,
      venue: { name: 'The Rebel Lounge', slug: 'the-rebel-lounge' },
    })
    expect(schema.location.url).toBe('https://psychichomily.com/venues/the-rebel-lounge')
  })

  it('includes PostalAddress with addressCountry US', () => {
    const schema = generateMusicEventSchema({
      ...baseShow,
      venue: {
        name: 'The Rebel Lounge',
        address: '2303 E Indian School Rd',
        city: 'Phoenix',
        state: 'AZ',
        zip_code: '85016',
      },
    })
    expect(schema.location.address).toEqual({
      '@type': 'PostalAddress',
      streetAddress: '2303 E Indian School Rd',
      addressLocality: 'Phoenix',
      addressRegion: 'AZ',
      postalCode: '85016',
      addressCountry: 'US',
    })
  })

  it('omits address when no address or city', () => {
    const schema = generateMusicEventSchema(baseShow)
    expect(schema.location.address).toBeUndefined()
  })

  // Performers
  it('includes performers with MusicGroup type', () => {
    const schema = generateMusicEventSchema(baseShow)
    expect(schema.performer).toHaveLength(1)
    expect(schema.performer![0]['@type']).toBe('MusicGroup')
    expect(schema.performer![0].name).toBe('Test Band')
  })

  it('generates artist URL from slug', () => {
    const schema = generateMusicEventSchema({
      ...baseShow,
      artists: [{ name: 'Test Band', is_headliner: true, slug: 'test-band' }],
    })
    expect(schema.performer![0].url).toBe('https://psychichomily.com/artists/test-band')
  })

  it('includes non-null social links as sameAs', () => {
    const schema = generateMusicEventSchema({
      ...baseShow,
      artists: [{
        name: 'Test Band',
        is_headliner: true,
        socials: {
          instagram: 'https://instagram.com/testband',
          spotify: 'https://open.spotify.com/artist/123',
          facebook: null,
          twitter: undefined,
        },
      }],
    })
    expect(schema.performer![0].sameAs).toEqual([
      'https://instagram.com/testband',
      'https://open.spotify.com/artist/123',
    ])
  })

  it('omits sameAs when all socials are null', () => {
    const schema = generateMusicEventSchema({
      ...baseShow,
      artists: [{
        name: 'Test Band',
        is_headliner: true,
        socials: { instagram: null, facebook: null },
      }],
    })
    expect(schema.performer![0].sameAs).toBeUndefined()
  })

  it('omits performer when no artists', () => {
    const schema = generateMusicEventSchema({ date: '2026-03-15T20:00:00Z' })
    expect(schema.performer).toBeUndefined()
  })

  it('omits performer when artists array is empty', () => {
    const schema = generateMusicEventSchema({ date: '2026-03-15T20:00:00Z', artists: [] })
    expect(schema.performer).toBeUndefined()
  })

  // Offers
  it('includes offers when price is provided', () => {
    const schema = generateMusicEventSchema({ ...baseShow, price: 25 })
    expect(schema.offers).toBeDefined()
    expect(schema.offers!.price).toBe(25)
    expect(schema.offers!.priceCurrency).toBe('USD')
  })

  it('sets SoldOut availability when is_sold_out', () => {
    const schema = generateMusicEventSchema({ ...baseShow, price: 25, is_sold_out: true })
    expect(schema.offers!.availability).toBe('https://schema.org/SoldOut')
  })

  it('sets InStock availability when not sold out', () => {
    const schema = generateMusicEventSchema({ ...baseShow, price: 25 })
    expect(schema.offers!.availability).toBe('https://schema.org/InStock')
  })

  // The offer carries NO url — see PSY-1669's Linear thread for the decision
  // trail. Original AC wanted the vendor's ticket URL; that was reversed
  // because this site has no referral arrangement and would be handing over
  // sales for free; the self-referencing URL that replaced it was then dropped
  // too, because Google's bar for the field is a "landing page that clearly
  // and predominantly provides the opportunity to buy" and our show page is
  // not that. Google marks `offers.url` Recommended, not required, and only
  // the "ticket purchase option" placement is lost by omitting it — price and
  // sold-out status still surface. Do not "fix" this by adding either URL.
  it('emits no offer URL at all', () => {
    const schema = generateMusicEventSchema({
      ...baseShow,
      price: 25,
      slug: 'test-show',
      ticket_url: 'https://dice.fm/event/abc',
    })
    expect(schema.offers).toBeDefined()
    expect('url' in schema.offers!).toBe(false)
  })

  // The description is emitted verbatim, so a vendor URL stored there would
  // reach structured data past the offer gate. The discovery writer stores the
  // URL in `ticket_url` and keeps the description free of it; this pins the
  // whole document, not just `offers`.
  it('carries the vendor URL nowhere for an ingested show', () => {
    const schema = generateMusicEventSchema({
      ...baseShow,
      price: 25,
      slug: 'test-show',
      description: 'Doors: 7ish | Show: 8ish',
      ticket_url: 'https://dice.fm/event/abc',
    })
    expect(schema.description).toBe('Doors: 7ish | Show: 8ish')
    expect(JSON.stringify(schema)).not.toContain('dice.fm')
  })

  // `seller` names the vendor without linking to them.
  it.each([
    ['https://dice.fm/event/abc', 'DICE'],
    ['https://www.eventbrite.com/e/123', 'Eventbrite'],
    ['ticketmaster.com/event/1', 'Ticketmaster'],
    ['https://www.ticketweb.com/event/2', 'TicketWeb'],
    ['https://seetickets.us/event/3', 'See Tickets'],
    ['https://event.etix.com/ticket/4', 'Etix'],
  ])('names the seller for %s', (ticketUrl, expected) => {
    const schema = generateMusicEventSchema({ ...baseShow, price: 25, ticket_url: ticketUrl })
    expect(schema.offers!.seller).toEqual({ '@type': 'Organization', name: expected })
  })

  // Host-anchored: a lookalike domain must not borrow a real vendor's name.
  it.each([
    ['a lookalike prefix', 'https://evil-dice.fm/event/abc'],
    ['a lookalike suffix', 'https://dice.fm.evil.test/event/abc'],
    ['an unknown vendor', 'https://tix.some-venue.example/e/1'],
    ['an unparseable value', 'not a url'],
  ])('omits the seller for %s', (_label, ticketUrl) => {
    const schema = generateMusicEventSchema({ ...baseShow, price: 25, ticket_url: ticketUrl })
    expect(schema.offers!.seller).toBeUndefined()
  })

  it('omits the seller when the show records no ticket URL', () => {
    const schema = generateMusicEventSchema({ ...baseShow, price: 25 })
    expect(schema.offers!.seller).toBeUndefined()
  })

  // Hard rules: the two claims that must never be made.
  it('never says InStock for a started or sold-out show', () => {
    const started = generateMusicEventSchema({ ...baseShow, price: 25, has_started: true })
    expect(started.offers).toBeUndefined()

    const soldOut = generateMusicEventSchema({ ...baseShow, price: 25, is_sold_out: true })
    expect(soldOut.offers!.availability).toBe('https://schema.org/SoldOut')

    const cancelled = generateMusicEventSchema({ ...baseShow, price: 25, is_cancelled: true })
    expect(cancelled.offers).toBeUndefined()
  })

  it('never invents a price the show did not record', () => {
    const soldOut = generateMusicEventSchema({ ...baseShow, is_sold_out: true })
    expect(soldOut.offers!.price).toBeUndefined()
    expect(soldOut.offers!.priceCurrency).toBeUndefined()

    // Nothing to convey without a price or a sold-out flag, so no offer at all.
    const bare = generateMusicEventSchema({ ...baseShow, ticket_url: 'https://dice.fm/event/abc' })
    expect(bare.offers).toBeUndefined()
  })

  it('omits offers when no price', () => {
    const schema = generateMusicEventSchema(baseShow)
    expect(schema.offers).toBeUndefined()
  })

  // Otherwise the same block said EventCancelled and InStock at once.
  it('omits offers for a cancelled show even when priced', () => {
    const schema = generateMusicEventSchema({ ...baseShow, price: 25, is_cancelled: true })
    expect(schema.eventStatus).toBe('https://schema.org/EventCancelled')
    expect(schema.offers).toBeUndefined()
  })

  // An offer is a claim about what a reader can still buy, and doors closing
  // ends it. The show need not be over for the claim to have expired.
  it('omits offers for a show that has already started', () => {
    const schema = generateMusicEventSchema({ ...baseShow, price: 25, has_started: true })
    expect(schema.eventStatus).toBe('https://schema.org/EventScheduled')
    expect(schema.offers).toBeUndefined()
  })

  // `availability` is schema.org's only channel for sold-out — there is no
  // EventSoldOut status — so it must not depend on a price the show may not
  // have. Google marks offers.price Recommended, not required.
  it('says SoldOut without a price', () => {
    const schema = generateMusicEventSchema({ ...baseShow, is_sold_out: true })
    expect(schema.offers?.availability).toBe('https://schema.org/SoldOut')
    expect(schema.offers?.price).toBeUndefined()
    expect(schema.offers?.priceCurrency).toBeUndefined()
  })

  it('uses the venue country when given, and US otherwise', () => {
    const abroad = generateMusicEventSchema({
      ...baseShow,
      venue: { name: 'Horseshoe Tavern', city: 'Toronto', state: 'ON', country: 'CA' },
    })
    expect(abroad.location.address?.addressCountry).toBe('CA')

    const home = generateMusicEventSchema({
      ...baseShow,
      venue: { name: 'Valley Bar', city: 'Phoenix', state: 'AZ' },
    })
    expect(home.location.address?.addressCountry).toBe('US')
  })

  it('includes offers when price is 0 (free show)', () => {
    const schema = generateMusicEventSchema({ ...baseShow, price: 0 })
    expect(schema.offers).toBeDefined()
    expect(schema.offers!.price).toBe(0)
  })

  // Optional fields
  it('includes description when provided', () => {
    const schema = generateMusicEventSchema({ ...baseShow, description: 'A great show!' })
    expect(schema.description).toBe('A great show!')
  })

  it('omits description when not provided', () => {
    const schema = generateMusicEventSchema(baseShow)
    expect(schema.description).toBeUndefined()
  })

  it('includes URL and image from slug', () => {
    const schema = generateMusicEventSchema({ ...baseShow, slug: 'test-show' })
    expect(schema.url).toBe('https://psychichomily.com/shows/test-show')
    expect(schema.image).toEqual(['https://psychichomily.com/shows/test-show/opengraph-image'])
  })

  it('omits URL and image when no slug', () => {
    const schema = generateMusicEventSchema(baseShow)
    expect(schema.url).toBeUndefined()
    expect(schema.image).toBeUndefined()
  })

  describe('the flyer in the image array', () => {
    // Generated card FIRST: Google reads the array as ranked preference, and
    // the card is the one we control the dimensions and content of.
    it('lists the flyer after the generated card', () => {
      const schema = generateMusicEventSchema({
        ...baseShow,
        slug: 'test-show',
        image_url: 'https://cdn.example.com/flyer.jpg',
      })
      expect(schema.image).toEqual([
        'https://psychichomily.com/shows/test-show/opengraph-image',
        'https://cdn.example.com/flyer.jpg',
      ])
    })

    it('leaves the array at just the card when there is no flyer', () => {
      for (const image_url of [undefined, null, '']) {
        const schema = generateMusicEventSchema({ ...baseShow, slug: 'test-show', image_url })
        expect(schema.image, String(image_url)).toEqual([
          'https://psychichomily.com/shows/test-show/opengraph-image',
        ])
      }
    })

    // The two are independent claims: whatever made the OG route fall back to a
    // text card has no bearing on a consumer fetching the flyer directly.
    it('emits the flyer even with no slug to build a card URL from', () => {
      const schema = generateMusicEventSchema({
        ...baseShow,
        image_url: 'https://cdn.example.com/flyer.jpg',
      })
      expect(schema.image).toEqual(['https://cdn.example.com/flyer.jpg'])
    })

    // `image_url` is writable by any email-verified user and the backend
    // validates only the scheme and a length cap, so junk reaches this builder.
    // A broken URL in machine-readable output is worse than silence.
    it('drops a value that is not an absolute http(s) URL', () => {
      const junk = [
        '/uploads/flyer.jpg',
        'flyer.jpg',
        'javascript:alert(1)',
        'data:image/png;base64,AAAA',
        'not a url at all',
      ]
      for (const image_url of junk) {
        const schema = generateMusicEventSchema({ ...baseShow, slug: 'test-show', image_url })
        expect(schema.image, image_url).toEqual([
          'https://psychichomily.com/shows/test-show/opengraph-image',
        ])
      }
    })

    // Parsing is far more lenient than the value it yields, and the backend
    // stores `image_url` untrimmed — so the padded form really does arrive
    // here. Emitting the raw string would put whitespace and control characters
    // inside a machine-readable claim.
    it('emits the normalised URL, not the raw stored string', () => {
      for (const [raw, expected] of [
        ['  https://cdn.example.com/f.jpg ', 'https://cdn.example.com/f.jpg'],
        ['https://cdn.example.com/f\njpg', 'https://cdn.example.com/fjpg'],
        ['https:evil', 'https://evil/'],
      ] as const) {
        const schema = generateMusicEventSchema({ ...baseShow, image_url: raw })
        expect(schema.image, raw).toEqual([expected])
      }
    })

    // The backend accepts plain http, so the field genuinely carries it. It is
    // a real, resolvable claim, unlike the cases above.
    it('keeps a plain http flyer URL', () => {
      const schema = generateMusicEventSchema({
        ...baseShow,
        slug: 'test-show',
        image_url: 'http://cdn.example.com/flyer.jpg',
      })
      expect(schema.image).toContain('http://cdn.example.com/flyer.jpg')
    })
  })

  it('always includes organizer', () => {
    const schema = generateMusicEventSchema(baseShow)
    expect(schema.organizer).toEqual({
      '@type': 'Organization',
      name: 'Psychic Homily',
      url: 'https://psychichomily.com',
    })
  })
})

describe('generateBlogPostingSchema', () => {
  const basePost = {
    title: 'Test Blog Post',
    date: '2026-01-15',
    slug: 'test-blog-post',
  }

  it('includes all required fields', () => {
    const schema = generateBlogPostingSchema(basePost)
    expect(schema['@context']).toBe('https://schema.org')
    expect(schema['@type']).toBe('BlogPosting')
    expect(schema.headline).toBe('Test Blog Post')
    expect(schema.datePublished).toBe('2026-01-15')
    expect(schema.dateModified).toBe('2026-01-15')
  })

  it('generates URL from slug', () => {
    const schema = generateBlogPostingSchema(basePost)
    expect(schema.url).toBe('https://psychichomily.com/blog/test-blog-post')
  })

  it('includes description when provided', () => {
    const schema = generateBlogPostingSchema({ ...basePost, description: 'A blog about music.' })
    expect(schema.description).toBe('A blog about music.')
  })

  it('includes Organization author', () => {
    const schema = generateBlogPostingSchema(basePost)
    expect(schema.author).toEqual({
      '@type': 'Organization',
      name: 'Psychic Homily',
    })
  })
})

describe('generateMusicVenueSchema', () => {
  it('includes name and type', () => {
    const schema = generateMusicVenueSchema({ name: 'The Rebel Lounge' })
    expect(schema['@context']).toBe('https://schema.org')
    expect(schema['@type']).toBe('MusicVenue')
    expect(schema.name).toBe('The Rebel Lounge')
  })

  it('includes address when provided', () => {
    const schema = generateMusicVenueSchema({
      name: 'The Rebel Lounge',
      address: '2303 E Indian School Rd',
      city: 'Phoenix',
      state: 'AZ',
      zip_code: '85016',
    })
    expect(schema.address).toEqual({
      '@type': 'PostalAddress',
      streetAddress: '2303 E Indian School Rd',
      addressLocality: 'Phoenix',
      addressRegion: 'AZ',
      postalCode: '85016',
    })
  })

  it('omits address when no address or city', () => {
    const schema = generateMusicVenueSchema({ name: 'The Rebel Lounge' })
    expect(schema.address).toBeUndefined()
  })

  it('generates URL from slug', () => {
    const schema = generateMusicVenueSchema({ name: 'Valley Bar', slug: 'valley-bar' })
    expect(schema.url).toBe('https://psychichomily.com/venues/valley-bar')
  })

  it('omits URL when no slug', () => {
    const schema = generateMusicVenueSchema({ name: 'Valley Bar' })
    expect(schema.url).toBeUndefined()
  })
})

describe('generateMusicGroupSchema', () => {
  it('includes name and type', () => {
    const schema = generateMusicGroupSchema({ name: 'Test Band' })
    expect(schema['@context']).toBe('https://schema.org')
    expect(schema['@type']).toBe('MusicGroup')
    expect(schema.name).toBe('Test Band')
  })

  it('generates URL from slug', () => {
    const schema = generateMusicGroupSchema({ name: 'Test Band', slug: 'test-band' })
    expect(schema.url).toBe('https://psychichomily.com/artists/test-band')
  })

  it('filters null social links and includes valid ones as sameAs', () => {
    const schema = generateMusicGroupSchema({
      name: 'Test Band',
      social: {
        instagram: 'https://instagram.com/testband',
        spotify: null,
        website: 'https://testband.com',
      },
    })
    expect(schema.sameAs).toEqual([
      'https://instagram.com/testband',
      'https://testband.com',
    ])
  })

  it('omits sameAs when all socials are null', () => {
    const schema = generateMusicGroupSchema({
      name: 'Test Band',
      social: { instagram: null, spotify: null },
    })
    expect(schema.sameAs).toBeUndefined()
  })

  it('omits sameAs when no social provided', () => {
    const schema = generateMusicGroupSchema({ name: 'Test Band' })
    expect(schema.sameAs).toBeUndefined()
  })

  it('includes foundingLocation when city or state provided', () => {
    const schema = generateMusicGroupSchema({
      name: 'Test Band',
      city: 'Phoenix',
      state: 'AZ',
    })
    expect(schema.foundingLocation).toEqual({
      '@type': 'Place',
      address: {
        '@type': 'PostalAddress',
        addressLocality: 'Phoenix',
        addressRegion: 'AZ',
      },
    })
  })

  it('omits foundingLocation when no city or state', () => {
    const schema = generateMusicGroupSchema({ name: 'Test Band' })
    expect(schema.foundingLocation).toBeUndefined()
  })

  it('handles null city with valid state', () => {
    const schema = generateMusicGroupSchema({ name: 'Test Band', city: null, state: 'AZ' })
    expect(schema.foundingLocation!.address!.addressLocality).toBeUndefined()
    expect(schema.foundingLocation!.address!.addressRegion).toBe('AZ')
  })
})

describe('generateMusicRecordingSchema', () => {
  const baseMix = {
    title: 'Summer Vibes Mix',
    artist: 'DJ Test',
    date: '2026-01-10',
    slug: 'summer-vibes-mix',
  }

  it('includes all fields', () => {
    const schema = generateMusicRecordingSchema(baseMix)
    expect(schema['@context']).toBe('https://schema.org')
    expect(schema['@type']).toBe('MusicRecording')
    expect(schema.name).toBe('Summer Vibes Mix')
    expect(schema.datePublished).toBe('2026-01-10')
    expect(schema.url).toBe('https://psychichomily.com/dj-sets/summer-vibes-mix')
  })

  it('includes byArtist with MusicGroup type', () => {
    const schema = generateMusicRecordingSchema(baseMix)
    expect(schema.byArtist).toEqual({
      '@type': 'MusicGroup',
      name: 'DJ Test',
    })
  })
})

describe('generateItemListSchema', () => {
  it('generates correct 1-based positions', () => {
    const schema = generateItemListSchema({
      listItems: [
        { url: 'https://psychichomily.com/shows/a', name: 'Show A' },
        { url: 'https://psychichomily.com/shows/b', name: 'Show B' },
        { url: 'https://psychichomily.com/shows/c', name: 'Show C' },
      ],
    })
    expect(schema.itemListElement[0].position).toBe(1)
    expect(schema.itemListElement[1].position).toBe(2)
    expect(schema.itemListElement[2].position).toBe(3)
  })

  it('sets numberOfItems to match array length', () => {
    const schema = generateItemListSchema({
      listItems: [
        { url: 'https://psychichomily.com/shows/a' },
        { url: 'https://psychichomily.com/shows/b' },
      ],
    })
    expect(schema.numberOfItems).toBe(2)
  })

  it('includes name and description when provided', () => {
    const schema = generateItemListSchema({
      name: 'Upcoming Shows',
      description: 'Live music shows in Phoenix.',
      listItems: [{ url: 'https://psychichomily.com/shows/a' }],
    })
    expect(schema.name).toBe('Upcoming Shows')
    expect(schema.description).toBe('Live music shows in Phoenix.')
  })

  it('omits name and description when not provided', () => {
    const schema = generateItemListSchema({
      listItems: [{ url: 'https://psychichomily.com/shows/a' }],
    })
    expect(schema.name).toBeUndefined()
    expect(schema.description).toBeUndefined()
  })

  it('handles empty items array', () => {
    const schema = generateItemListSchema({ listItems: [] })
    expect(schema.numberOfItems).toBe(0)
    expect(schema.itemListElement).toHaveLength(0)
  })

  it('includes item name when provided', () => {
    const schema = generateItemListSchema({
      listItems: [{ url: 'https://psychichomily.com/shows/a', name: 'Show A' }],
    })
    expect(schema.itemListElement[0].name).toBe('Show A')
  })

  it('omits item name when not provided', () => {
    const schema = generateItemListSchema({
      listItems: [{ url: 'https://psychichomily.com/shows/a' }],
    })
    expect(schema.itemListElement[0].name).toBeUndefined()
  })

  it('has correct context and type', () => {
    const schema = generateItemListSchema({ listItems: [] })
    expect(schema['@context']).toBe('https://schema.org')
    expect(schema['@type']).toBe('ItemList')
  })
})

describe('renderJsonLd', () => {
  it('serializes schema to JSON string', () => {
    const schema = generateOrganizationSchema()
    const json = renderJsonLd(schema)
    const parsed = JSON.parse(json)
    expect(parsed['@type']).toBe('Organization')
    expect(parsed.name).toBe('Psychic Homily')
  })

  it('handles nested objects', () => {
    const schema = generateMusicEventSchema({
      date: '2026-03-15T20:00:00Z',
      venue: { name: 'Valley Bar', slug: 'valley-bar', city: 'Phoenix' },
      artists: [{ name: 'Band', is_headliner: true }],
    })
    const json = renderJsonLd(schema)
    const parsed = JSON.parse(json)
    expect(parsed.location.name).toBe('Valley Bar')
    expect(parsed.performer[0].name).toBe('Band')
  })
})
