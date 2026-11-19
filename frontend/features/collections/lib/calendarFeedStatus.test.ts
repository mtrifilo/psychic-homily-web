import { describe, it, expect } from 'vitest'
import {
  calendarCheckedLine,
  calendarStoppedLine,
  classifyCalendarApps,
  summarizeCalendar,
} from './calendarFeedStatus'
import type {
  CalendarFeedClient,
  CalendarFeedRead,
} from '@/features/shows/types'

const NOW = new Date('2026-11-19T12:00:00Z')
const MINUTE = 60 * 1000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

const ago = (ms: number) => new Date(NOW.getTime() - ms).toISOString()

function read(
  client: CalendarFeedClient,
  lastAgo: number,
  firstAgo = lastAgo + 2 * DAY
): CalendarFeedRead {
  return { client, first_read_at: ago(firstAgo), last_read_at: ago(lastAgo) }
}

function stateOf(r: CalendarFeedRead) {
  const { connected, stopped } = classifyCalendarApps({ reads: [r] }, NOW)
  if (connected.length) return 'connected'
  if (stopped.length) return 'stopped'
  return 'ignored'
}

describe('classifyCalendarApps: the connected windows', () => {
  it.each([
    ['google, exactly 3 days', read('google', 3 * DAY), 'connected'],
    ['google, 3 days and a second', read('google', 3 * DAY + 1000), 'stopped'],
    ['apple_ios, exactly 8 days', read('apple_ios', 8 * DAY), 'connected'],
    ['apple_ios, 8 days and a second', read('apple_ios', 8 * DAY + 1000), 'stopped'],
    ['apple_mac, 5 days', read('apple_mac', 5 * DAY), 'connected'],
    ['apple_mac, 8 days and a second', read('apple_mac', 8 * DAY + 1000), 'stopped'],
    ['other, exactly 8 days', read('other', 8 * DAY), 'connected'],
    ['other, 8 days and a second', read('other', 8 * DAY + 1000), 'stopped'],
  ] as const)('%s: %s', (_label, r, want) => {
    expect(stateOf(r)).toBe(want)
  })
})

describe('classifyCalendarApps: an unnamed app needs checks an hour apart', () => {
  it.each([
    ['one preview fetch (first equals last)', 0, 'ignored'],
    ['59 minutes apart', 59 * MINUTE, 'ignored'],
    ['exactly one hour apart', HOUR, 'connected'],
    ['two hours apart', 2 * HOUR, 'connected'],
  ] as const)('%s', (_label, span, want) => {
    expect(stateOf(read('other', 10 * MINUTE, 10 * MINUTE + span))).toBe(want)
  })

  it('an unnamed app that never spanned an hour is never stopped either', () => {
    expect(stateOf(read('other', 20 * DAY, 20 * DAY))).toBe('ignored')
  })

  it('a named app counts from its first check', () => {
    expect(stateOf(read('google', 10 * MINUTE, 10 * MINUTE))).toBe('connected')
  })
})

describe('classifyCalendarApps', () => {
  it('orders each group most recent first and names every app', () => {
    const apps = classifyCalendarApps(
      {
        reads: [
          read('apple_ios', DAY),
          read('google', 2 * HOUR),
          read('apple_mac', 12 * DAY),
          read('other', 3 * HOUR),
        ],
      },
      NOW
    )
    expect(apps.connected.map(a => a.name)).toEqual([
      'Google Calendar',
      'Another calendar app',
      'Apple Calendar (iPhone or iPad)',
    ])
    expect(apps.stopped.map(a => a.name)).toEqual(['Apple Calendar (Mac)'])
  })

  it('reads nothing into a missing or null reads list', () => {
    expect(classifyCalendarApps(undefined, NOW)).toEqual({
      connected: [],
      stopped: [],
    })
    expect(classifyCalendarApps({ reads: null }, NOW)).toEqual({
      connected: [],
      stopped: [],
    })
  })
})

describe('summarizeCalendar', () => {
  it.each([
    ['nothing known', { has_token: false }, 'none'],
    ['a link nobody checked', { has_token: true, reads: [] }, 'none'],
    [
      'one connected beside one stopped',
      { has_token: true, reads: [read('google', HOUR), read('apple_mac', 20 * DAY)] },
      'connected',
    ],
    [
      'every known app stopped',
      { has_token: true, reads: [read('google', 4 * DAY)] },
      'stopped',
    ],
    [
      'the link was reset',
      {
        has_token: false,
        link_reset_at: ago(DAY),
        link_reset_reason: 'password_reset' as const,
      },
      'stopped',
    ],
  ])('%s', (_label, status, want) => {
    expect(summarizeCalendar(status, NOW)).toBe(want)
  })
})

describe('calendarCheckedLine', () => {
  it('says the check came since the newest save', () => {
    expect(calendarCheckedLine(ago(2 * HOUR), ago(5 * HOUR), NOW)).toBe(
      'Checked 2h ago, since your last save.'
    )
  })

  it('counts a check at the same instant as the save as since it', () => {
    expect(calendarCheckedLine(ago(2 * HOUR), ago(2 * HOUR), NOW)).toBe(
      'Checked 2h ago, since your last save.'
    )
  })

  it('says the check came before the newest save', () => {
    expect(calendarCheckedLine(ago(9 * HOUR), ago(HOUR), NOW)).toBe(
      'Checked 9h ago, before your last save. It appears at its next check.'
    )
  })

  it('drops the comparison when there are no saved shows', () => {
    expect(calendarCheckedLine(ago(2 * HOUR), null, NOW)).toBe(
      'Checked 2h ago.'
    )
  })

  it('uses the short form for minutes and days', () => {
    expect(calendarCheckedLine(ago(20 * MINUTE), null, NOW)).toBe(
      'Checked 20m ago.'
    )
    expect(calendarCheckedLine(ago(DAY + HOUR), null, NOW)).toBe(
      'Checked 1d ago.'
    )
  })
})

describe('calendarStoppedLine', () => {
  it('names the app and its last check in the long form', () => {
    const [app] = classifyCalendarApps(
      { reads: [read('google', 12 * DAY)] },
      NOW
    ).stopped
    expect(calendarStoppedLine(app, NOW)).toBe(
      'Google Calendar last checked 12 days ago. It may have been removed there.'
    )
  })
})
