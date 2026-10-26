import { beforeEach, describe, expect, it, vi } from 'vitest'

const { seedFirstScreen, fetchListPayload } = vi.hoisted(() => ({
  seedFirstScreen: vi.fn(),
  fetchListPayload: vi.fn(),
}))
vi.mock('@/lib/query-hydration', () => ({ seedFirstScreen }))
vi.mock('@/lib/ssr/fetchListPayload', () => ({ fetchListPayload }))

import {
  EMPTY_DEHYDRATED_STATE,
  dehydrateListFirstScreen,
  isFirstScreenRequest,
} from './firstScreenSeed'

const DEFEATING = ['page', 'tags'] as const

describe('isFirstScreenRequest', () => {
  it('is true for a bare URL', () => {
    expect(isFirstScreenRequest({}, DEFEATING)).toBe(true)
  })

  it('is false when a defeating param carries a value', () => {
    expect(isFirstScreenRequest({ page: '2' }, DEFEATING)).toBe(false)
    expect(isFirstScreenRequest({ tags: 'shoegaze' }, DEFEATING)).toBe(false)
  })

  // A param restating the default still skips: the safe direction, since a
  // skipped seed costs a server-rendered loader and nothing else.
  it('is false for a defeating param that restates the default', () => {
    expect(isFirstScreenRequest({ page: '1' }, DEFEATING)).toBe(false)
  })

  it('reads an empty value as absent', () => {
    expect(isFirstScreenRequest({ page: '', tags: '' }, DEFEATING)).toBe(true)
  })

  it('reads a repeated param by its non-empty members', () => {
    expect(isFirstScreenRequest({ tags: ['', ''] }, DEFEATING)).toBe(true)
    expect(isFirstScreenRequest({ tags: ['', 'shoegaze'] }, DEFEATING)).toBe(false)
  })

  it('ignores params outside the list', () => {
    expect(isFirstScreenRequest({ utm_source: 'news' }, DEFEATING)).toBe(true)
  })
})

describe('dehydrateListFirstScreen', () => {
  const seeded = { mutations: [], queries: [{ queryHash: 'seeded' }] }
  const read = (name: string) => ({
    url: `https://api.test/${name}`,
    queryKey: [name],
    collection: 'tags' as const,
    service: `${name}-first-screen`,
  })

  beforeEach(() => {
    vi.clearAllMocks()
    seedFirstScreen.mockResolvedValue(seeded)
  })

  it('reads every URL and seeds each key with its own payload, in order', async () => {
    fetchListPayload.mockImplementation(async ({ url }: { url: string }) => ({ url }))

    const state = await dehydrateListFirstScreen({}, DEFEATING, [read('a'), read('b')])

    expect(state).toBe(seeded)
    expect(fetchListPayload).toHaveBeenCalledWith({
      url: 'https://api.test/a',
      collection: 'tags',
      service: 'a-first-screen',
    })
    expect(seedFirstScreen).toHaveBeenCalledWith([
      { queryKey: ['a'], data: { url: 'https://api.test/a' } },
      { queryKey: ['b'], data: { url: 'https://api.test/b' } },
    ])
  })

  it('seeds nothing when any read returns null', async () => {
    fetchListPayload.mockImplementation(async ({ url }: { url: string }) =>
      url.endsWith('/b') ? null : { url }
    )

    const state = await dehydrateListFirstScreen({}, DEFEATING, [read('a'), read('b')])

    expect(state).toEqual(EMPTY_DEHYDRATED_STATE)
    expect(seedFirstScreen).not.toHaveBeenCalled()
  })

  it('neither reads nor seeds off the first screen', async () => {
    const state = await dehydrateListFirstScreen({ page: '3' }, DEFEATING, [read('a')])

    expect(state).toEqual(EMPTY_DEHYDRATED_STATE)
    expect(fetchListPayload).not.toHaveBeenCalled()
    expect(seedFirstScreen).not.toHaveBeenCalled()
  })
})
