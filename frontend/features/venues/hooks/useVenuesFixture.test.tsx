import { describe, it, expect } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '@/test/utils'
import { useVenues } from './useVenues'

/**
 * The directory row order, against the shared `/venues` MSW fixture.
 *
 * The sibling `useVenues.test.tsx` mocks `apiRequest`, so it can only assert
 * what the hook ASKS for. This file lets a real response through, which is the
 * only place in frontend CI where the contract the table depends on is visible:
 * the API hands back quiet rooms after active ones under every sort, and the
 * table inserts its divider where that block begins rather than re-sorting.
 */
describe('useVenues row order (MSW fixture)', () => {
  it('returns quiet rooms after every room with something booked', async () => {
    const { result } = renderHook(() => useVenues({ limit: 50 }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const counts = result.current.data!.venues.map(v => v.upcoming_show_count)
    const firstQuiet = counts.indexOf(0)
    expect(firstQuiet).toBeGreaterThan(0)
    expect(counts.slice(firstQuiet).every(c => c === 0)).toBe(true)
  })

  it('keeps the quiet block last under an alphabetical sort too', async () => {
    const { result } = renderHook(() => useVenues({ limit: 50, sort: 'name' }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const venues = result.current.data!.venues
    // Alphabetically "Arizona Financial Theatre" leads, and it is quiet, so a
    // name sort that ignored the two blocks would put it first.
    expect(venues[0].name).toBe('Crescent Ballroom')
    expect(venues[venues.length - 1].upcoming_show_count).toBe(0)
  })

  it('carries next_show for an active room and last_show for a quiet one', async () => {
    const { result } = renderHook(() => useVenues({ limit: 50 }), {
      wrapper: createWrapper(),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const venues = result.current.data!.venues
    const active = venues.find(v => v.upcoming_show_count > 0)!
    const quiet = venues.find(v => v.upcoming_show_count === 0)!
    // The two fields partition the room's shows on one boundary, so neither
    // row ever carries both.
    expect(active.next_show).toBeDefined()
    expect(active.last_show).toBeUndefined()
    expect(quiet.next_show).toBeUndefined()
    expect(quiet.last_show).toBeDefined()
  })

  it('answers empty for a city the fixture does not list', async () => {
    const { result } = renderHook(
      () => useVenues({ cities: [{ city: 'Chicago', state: 'IL' }] }),
      { wrapper: createWrapper() }
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data!.venues).toEqual([])
    expect(result.current.data!.total).toBe(0)
  })
})
