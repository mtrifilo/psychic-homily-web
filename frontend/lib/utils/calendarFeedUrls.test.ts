import { describe, it, expect } from 'vitest'
import {
  webcalUrl,
  googleCalendarSubscribeUrl,
  googleCalendarEventUrl,
  outlookComSubscribeUrl,
  outlookWorkSubscribeUrl,
  GOOGLE_CALENDAR_ADD_BY_URL_PAGE,
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

describe('Outlook subscribe URLs', () => {
  it.each([
    ['Outlook.com', outlookComSubscribeUrl, 'https://outlook.live.com'],
    ['work or school', outlookWorkSubscribeUrl, 'https://outlook.office.com'],
  ])(
    '%s: addfromweb page with the webcal address and the name, encoded',
    (_label, build, origin) => {
      const url = new URL(
        build('https://example.com/feed.ics?a=1&b=2')
      )
      expect(url.origin + url.pathname).toBe(
        `${origin}/calendar/0/addfromweb/`
      )
      expect(url.searchParams.get('url')).toBe(
        'webcal://example.com/feed.ics?a=1&b=2'
      )
      expect(url.searchParams.get('name')).toBe('Psychic Homily saved shows')
    }
  )
})

describe('GOOGLE_CALENDAR_ADD_BY_URL_PAGE', () => {
  it("is Google's Add from URL settings page and carries no address", () => {
    expect(GOOGLE_CALENDAR_ADD_BY_URL_PAGE).toBe(
      'https://calendar.google.com/calendar/r/settings/addbyurl'
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
