import { describe, it, expect } from 'vitest'
import { formatShowDateBadge } from './showDateBadge'

describe('formatShowDateBadge', () => {
  it('formats a date string into badge parts for Arizona (default)', () => {
    // 2026-03-17T03:00:00Z in America/Phoenix (UTC-7, no DST) = March 16 at 8:00 PM
    // Use a date that maps cleanly to avoid cross-day issues
    const result = formatShowDateBadge('2026-03-17T19:00:00Z')
    // In Phoenix (UTC-7): March 17 at 12:00 PM
    expect(result.dayOfWeek).toMatch(/^[A-Z]{3}$/) // e.g. "TUE"
    expect(result.monthDay).toMatch(/^[A-Z]{3} \d{1,2}$/) // e.g. "MAR 17"
  })

  it('returns uppercase day of week', () => {
    const result = formatShowDateBadge('2026-01-05T20:00:00Z', 'AZ')
    // Jan 5 2026 at 20:00 UTC = Jan 5 at 13:00 Phoenix time = Monday
    expect(result.dayOfWeek).toBe('MON')
  })

  it('returns uppercase month in monthDay', () => {
    const result = formatShowDateBadge('2026-06-15T03:00:00Z', 'AZ')
    // June 14 in Phoenix (UTC-7)
    expect(result.monthDay).toMatch(/^JUN/)
  })

  it('uses Arizona timezone by default when state is not provided', () => {
    const result = formatShowDateBadge('2026-03-20T02:00:00Z')
    // In Phoenix: March 19 at 7:00 PM
    expect(result.dayOfWeek).toBe('THU')
    expect(result.monthDay).toBe('MAR 19')
  })

  it('uses Arizona timezone when state is null', () => {
    const result = formatShowDateBadge('2026-03-20T02:00:00Z', null)
    // Same as default — should use AZ
    expect(result.dayOfWeek).toBe('THU')
    expect(result.monthDay).toBe('MAR 19')
  })

  it('handles California timezone', () => {
    // March 2026: CA is on PDT (UTC-7), same as AZ (no DST)
    // Actually, DST starts March 8 2026 in CA, so PDT = UTC-7
    const result = formatShowDateBadge('2026-03-20T02:00:00Z', 'CA')
    // In LA (PDT, UTC-7): March 19 at 7:00 PM
    expect(result.dayOfWeek).toBe('THU')
    expect(result.monthDay).toBe('MAR 19')
  })

  it('handles New York timezone', () => {
    // March 2026: NY is on EDT (UTC-4), DST starts March 8 2026
    const result = formatShowDateBadge('2026-03-20T02:00:00Z', 'NY')
    // In NY (EDT, UTC-4): March 19 at 10:00 PM
    expect(result.dayOfWeek).toBe('THU')
    expect(result.monthDay).toBe('MAR 19')
  })

  it('handles Texas timezone', () => {
    // March 2026: TX is on CDT (UTC-5)
    const result = formatShowDateBadge('2026-03-20T02:00:00Z', 'TX')
    // In TX (CDT, UTC-5): March 19 at 9:00 PM
    expect(result.dayOfWeek).toBe('THU')
    expect(result.monthDay).toBe('MAR 19')
  })

  it('handles date crossing midnight in venue timezone', () => {
    // A show at 6:00 AM UTC in Phoenix (UTC-7) would be March 19 at 11:00 PM
    const result = formatShowDateBadge('2026-03-20T06:00:00Z', 'AZ')
    // Phoenix: March 19 at 11:00 PM
    expect(result.dayOfWeek).toBe('THU')
    expect(result.monthDay).toBe('MAR 19')
  })

  it('handles date not crossing midnight', () => {
    // A show at 8:00 PM UTC in Phoenix (UTC-7) would be March 20 at 1:00 PM
    const result = formatShowDateBadge('2026-03-20T20:00:00Z', 'AZ')
    // Phoenix: March 20 at 1:00 PM
    expect(result.dayOfWeek).toBe('FRI')
    expect(result.monthDay).toBe('MAR 20')
  })

  it('defaults to AZ for unknown state codes', () => {
    // Unknown state should fall back to America/Phoenix
    const resultUnknown = formatShowDateBadge('2026-03-20T20:00:00Z', 'XX')
    const resultAZ = formatShowDateBadge('2026-03-20T20:00:00Z', 'AZ')

    expect(resultUnknown.dayOfWeek).toBe(resultAZ.dayOfWeek)
    expect(resultUnknown.monthDay).toBe(resultAZ.monthDay)
  })

  it('handles winter dates (no DST difference for AZ)', () => {
    // December 15 2026 at 3:00 AM UTC in Phoenix (UTC-7) = Dec 14 at 8:00 PM
    const result = formatShowDateBadge('2026-12-15T03:00:00Z', 'AZ')
    expect(result.dayOfWeek).toBe('MON')
    expect(result.monthDay).toBe('DEC 14')
  })

  it('handles New Year boundary', () => {
    // January 1 2026 at 10:00 UTC in Phoenix = Dec 31 at 3:00 AM... actually
    // Jan 1 at 10:00 UTC = Jan 1 at 3:00 AM Phoenix
    const result = formatShowDateBadge('2026-01-01T10:00:00Z', 'AZ')
    expect(result.dayOfWeek).toBe('THU')
    expect(result.monthDay).toBe('JAN 1')
  })

  it('returns correct structure shape', () => {
    const result = formatShowDateBadge('2026-07-04T19:00:00Z', 'AZ')

    expect(result).toHaveProperty('dayOfWeek')
    expect(result).toHaveProperty('monthDay')
    expect(typeof result.dayOfWeek).toBe('string')
    expect(typeof result.monthDay).toBe('string')
  })

  it('formats single-digit days without leading zero', () => {
    // March 5 2026
    const result = formatShowDateBadge('2026-03-05T20:00:00Z', 'AZ')
    // Phoenix: March 5 at 1:00 PM
    expect(result.monthDay).toBe('MAR 5')
  })
})
