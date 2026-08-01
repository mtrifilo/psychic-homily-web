import { describe, it, expect } from 'vitest'
import { webcalUrl, googleCalendarSubscribeUrl } from './calendarFeedUrls'

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
