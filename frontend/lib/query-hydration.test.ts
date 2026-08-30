import { describe, expect, it, vi } from 'vitest'

// `seedFirstScreen` marks its boundary dynamic via next/server's `connection`.
// There is no request scope in vitest, so stub it; the assertion that it is
// actually called lives below, because dropping it is a BUILD failure under
// `cacheComponents` and nothing else here would notice.
const { connection } = vi.hoisted(() => ({ connection: vi.fn() }))
vi.mock('next/server', () => ({ connection }))

import { hashKey, type DehydratedState } from '@tanstack/react-query'
import { prefetchEntities, seedFirstScreen } from './query-hydration'

// `getQueryClient()` returns a per-request client on the server but a
// SINGLETON under jsdom, so a dehydrated state here also carries entries
// seeded by earlier tests in this file. Look entries up by hash rather than
// position — which is how TanStack matches them anyway.
const entryFor = (state: DehydratedState, queryKey: readonly unknown[]) =>
  state.queries.find(q => q.queryHash === hashKey(queryKey))

describe('seedFirstScreen', () => {
  it('seeds every key into ONE dehydrated state', async () => {
    // Two `prefetchEntity` calls cannot do this — each mints its own client on
    // the server — and a list page that seeds its rows but not its facets
    // renders the skeleton anyway.
    const rowsKey = ['shows', 'list', {}]
    const facetsKey = ['shows', 'cities', undefined]

    const state = await seedFirstScreen([
      { queryKey: rowsKey, data: { shows: [{ id: 1 }], total: 1 } },
      { queryKey: facetsKey, data: { cities: [] } },
    ])

    expect(entryFor(state, rowsKey)).toBeDefined()
    expect(entryFor(state, facetsKey)).toBeDefined()
  })

  it('stamps dataUpdatedAt: 0 so the client revalidates instead of trusting it', async () => {
    // The payload came out of Next's Data Cache and can be an hour old, so
    // "fetched just now" would buy a staleTime it never earned. 0 means paint
    // it, then go check.
    const key = ['scenes', 'list', 'stale-marker']
    const state = await seedFirstScreen([
      { queryKey: key, data: { scenes: [], count: 0 } },
    ])

    expect(entryFor(state, key)?.state.dataUpdatedAt).toBe(0)
    expect(entryFor(state, key)?.state.status).toBe('success')
  })

  it('preserves the seeded data verbatim', async () => {
    const key = ['scenes', 'list', 'verbatim']
    const data = { scenes: [{ slug: 'phoenix-az' }], count: 1 }
    const state = await seedFirstScreen([{ queryKey: key, data }])

    expect(entryFor(state, key)?.state.data).toEqual(data)
  })

  it('marks the calling boundary dynamic', async () => {
    connection.mockClear()
    await seedFirstScreen([{ queryKey: ['scenes', 'list', 'dynamic'], data: {} }])
    expect(connection).toHaveBeenCalled()
  })
})

describe('prefetchEntities', () => {
  it('seeds every key into ONE dehydrated state', async () => {
    // A page whose first paint depends on two keys gets one boundary, and one
    // boundary takes one state; two `prefetchEntity` calls mint two clients.
    const detailKey = ['shows', 'detail', 'multi-seed']
    const timelineKey = ['shows', 'timeline', 'multi-seed']

    const state = await prefetchEntities([
      { queryKey: detailKey, data: { id: 1 } },
      { queryKey: timelineKey, data: { previous: null, next: null } },
    ])

    expect(entryFor(state, detailKey)?.state.data).toEqual({ id: 1 })
    expect(entryFor(state, timelineKey)?.state.data).toEqual({
      previous: null,
      next: null,
    })
  })

  // A cached empty is indistinguishable from a real one at the client hook, so
  // a failed server read leaves the key unseeded and the client fetches it.
  it('skips a null seed and still seeds the rest', async () => {
    const failedKey = ['shows', 'timeline', 'null-seed']
    const survivingKey = ['shows', 'detail', 'null-seed']

    const state = await prefetchEntities([
      { queryKey: failedKey, data: null },
      { queryKey: survivingKey, data: { id: 2 } },
    ])

    expect(entryFor(state, failedKey)).toBeUndefined()
    expect(entryFor(state, survivingKey)?.state.data).toEqual({ id: 2 })
  })
})
