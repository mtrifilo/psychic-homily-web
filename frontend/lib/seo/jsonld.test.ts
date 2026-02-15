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
    venue: { name: 'The Rebel Lounge' },
    artists: [{ name: 'Test Band', is_headliner: true }],
  }

  // Required fields
  it('includes all Google-required fields', () => {
    const schema = generateMusicEventSchema(baseShow)
    expect(schema['@context']).toBe('https://schema.org')
    expect(schema['@type']).toBe('MusicEvent')
    expect(schema.name).toBeDefined()
    expect(schema.startDate).toBe('2026-03-15T20:00:00Z')
    expect(schema.location).toBeDefined()
    expect(schema.eventAttendanceMode).toBe('https://schema.org/OfflineEventAttendanceMode')
    expect(schema.eventStatus).toBeDefined()
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

  it('includes offer URL when show has slug', () => {
    const schema = generateMusicEventSchema({ ...baseShow, price: 25, slug: 'test-show' })
    expect(schema.offers!.url).toBe('https://psychichomily.com/shows/test-show')
  })

  it('omits offers when no price', () => {
    const schema = generateMusicEventSchema(baseShow)
    expect(schema.offers).toBeUndefined()
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
