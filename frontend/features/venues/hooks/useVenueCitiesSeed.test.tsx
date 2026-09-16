import { describe, it, expect } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { hashKey } from '@tanstack/react-query'
import { createWrapperWithClient, createTestQueryClient } from '@/test/utils'
import { venueQueryKeys } from '@/features/venues/api'
import { useVenueCities } from './useVenues'

/**
 * The one entry `/venues` is server-seeded on.
 *
 * A seed lands by KEY. Seed a key the hook does not ask for and nothing breaks
 * loudly: the hook misses the cache and fetches for itself, so the page keeps
 * working and silently stops being server-rendered. That failure is invisible by
 * construction, which is why the agreement is asserted against the REAL key
 * rather than reviewed. The sibling `useVenues.test.tsx` cannot do it: it mocks
 * this module, so it never sees the genuine constant.
 */
describe('useVenueCities cache key', () => {
  it('registers the key app/venues/page.tsx seeds', async () => {
    const queryClient = createTestQueryClient()
    const { result } = renderHook(() => useVenueCities(), {
      wrapper: createWrapperWithClient(queryClient),
    })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    const registered = queryClient.getQueryCache().getAll()
    expect(registered.map(q => q.queryHash)).toContain(
      hashKey(venueQueryKeys.cities)
    )
  })
})
