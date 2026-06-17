import { describe, it, expect, vi, afterEach } from 'vitest'
import { formatTokenDate, formatTokenDateTime, isTokenExpiringSoon } from './api-token-utils'

describe('formatTokenDate', () => {
  it('formats a date string as "Mon D, YYYY"', () => {
    // Use a date that formats predictably in en-US
    const result = formatTokenDate('2026-03-15T00:00:00Z')
    expect(result).toContain('Mar')
    expect(result).toContain('2026')
  })

  it('formats a different date correctly', () => {
    const result = formatTokenDate('2025-12-25T12:00:00Z')
    expect(result).toContain('Dec')
    expect(result).toContain('2025')
  })
})

describe('formatTokenDateTime', () => {
  it('includes time in the formatted output', () => {
    const result = formatTokenDateTime('2026-03-15T14:30:00Z')
    // Should contain date parts and time parts
    expect(result).toContain('Mar')
    expect(result).toContain('2026')
    // Time formatting is locale-dependent but should contain some time info
    expect(result.length).toBeGreaterThan(formatTokenDate('2026-03-15T14:30:00Z').length)
  })
})

describe('isTokenExpiringSoon', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('returns false when token is already expired', () => {
    const past = new Date(Date.now() - 86400000).toISOString()
    expect(isTokenExpiringSoon(past, true)).toBe(false)
  })

  it('returns true when token expires within 7 days', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-10T00:00:00Z'))

    // Expires in 3 days
    const soon = '2026-03-13T00:00:00Z'
    expect(isTokenExpiringSoon(soon, false)).toBe(true)

    vi.useRealTimers()
  })

  it('returns false when token expires in more than 7 days', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-10T00:00:00Z'))

    // Expires in 30 days
    const later = '2026-04-09T00:00:00Z'
    expect(isTokenExpiringSoon(later, false)).toBe(false)

    vi.useRealTimers()
  })

  it('returns true at exactly 7 day boundary (less than threshold)', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-10T00:00:00Z'))

    // Expires in exactly 6 days, 23 hours (just under 7 days)
    const boundary = '2026-03-16T23:00:00Z'
    expect(isTokenExpiringSoon(boundary, false)).toBe(true)

    vi.useRealTimers()
  })

  it('returns false at just over 7 days', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-10T00:00:00Z'))

    // Expires in 7 days + 1 hour
    const over = '2026-03-17T01:00:00Z'
    expect(isTokenExpiringSoon(over, false)).toBe(false)

    vi.useRealTimers()
  })

  it('supports custom threshold', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-10T00:00:00Z'))

    // 3 days out, with 2-day threshold → not expiring soon
    const threeDays = '2026-03-13T00:00:00Z'
    const twoDaysMs = 2 * 24 * 60 * 60 * 1000
    expect(isTokenExpiringSoon(threeDays, false, twoDaysMs)).toBe(false)

    // 1 day out, with 2-day threshold → expiring soon
    const oneDay = '2026-03-11T00:00:00Z'
    expect(isTokenExpiringSoon(oneDay, false, twoDaysMs)).toBe(true)

    vi.useRealTimers()
  })
})
