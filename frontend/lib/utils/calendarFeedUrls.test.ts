import { describe, it, expect } from 'vitest'
import {
  webcalUrl,
  googleCalendarSubscribeUrl,
  googleCalendarEventUrl,
} from './calendarFeedUrls'

describe('webcalUrl', () => {
  it('swaps http and https schemes for webcal', () => {
    expect(webcalUrl('https://example.com/feed.ics')).toBe(
      'webcal://example.com/feed.ics'
    )
    expect(webcalUrl('http://localhost:8080/feed.ics')).toBe(
      'webcal://localhost:8080/feed.ics'
    )
  })
})

describe('googleCalendarSubscribeUrl', () => {
  it('wraps the webcal URL in the cid parameter, encoded', () => {
    expect(googleCalendarSubscribeUrl('https://example.com/feed.ics')).toBe(
      `https://calendar.google.com/calendar/r?cid=${encodeURIComponent(
        'webcal://example.com/feed.ics'
      )}`
    )
  })
})

describe('googleCalendarEventUrl', () => {
  it('builds a template URL with compact UTC stamps', () => {
    const url = new URL(
      googleCalendarEventUrl({
        title: 'Desert Doom Night',
        start: new Date('2026-08-15T03:00:00Z'),
        end: new Date('2026-08-15T06:00:00Z'),
        details: 'https://example.com/shows/x',
        location: 'The Rebel Lounge, Phoenix, AZ',
      })
    )
    expect(url.origin + url.pathname).toBe(
      'https://calendar.google.com/calendar/render'
    )
    expect(url.searchParams.get('action')).toBe('TEMPLATE')
    expect(url.searchParams.get('dates')).toBe(
      '20260815T030000Z/20260815T060000Z'
    )
  })

  it('omits empty optional fields', () => {
    const url = new URL(
      googleCalendarEventUrl({
        title: 'X',
        start: new Date('2026-08-15T03:00:00Z'),
        end: new Date('2026-08-15T06:00:00Z'),
      })
    )
    expect(url.searchParams.has('details')).toBe(false)
    expect(url.searchParams.has('location')).toBe(false)
  })
})
