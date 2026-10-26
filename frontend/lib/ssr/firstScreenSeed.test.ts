import { beforeEach, describe, expect, it, vi } from 'vitest'

const { seedFirstScreen } = vi.hoisted(() => ({ seedFirstScreen: vi.fn() }))
vi.mock('@/lib/query-hydration', () => ({ seedFirstScreen }))

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

  beforeEach(() => {
    vi.clearAllMocks()
    seedFirstScreen.mockResolvedValue(seeded)
  })

  it('seeds every key with its own payload, in order', async () => {
    const state = await dehydrateListFirstScreen({}, DEFEATING, [
      { queryKey: ['a'], read: async () => ({ rows: [1] }) },
      { queryKey: ['b'], read: async () => ({ rows: [2] }) },
    ])

    expect(state).toBe(seeded)
    expect(seedFirstScreen).toHaveBeenCalledWith([
      { queryKey: ['a'], data: { rows: [1] } },
      { queryKey: ['b'], data: { rows: [2] } },
    ])
  })

  it('seeds nothing when any read returns null', async () => {
    const state = await dehydrateListFirstScreen({}, DEFEATING, [
      { queryKey: ['a'], read: async () => ({ rows: [1] }) },
      { queryKey: ['b'], read: async () => null },
    ])

    expect(state).toEqual(EMPTY_DEHYDRATED_STATE)
    expect(seedFirstScreen).not.toHaveBeenCalled()
  })

  it('neither reads nor seeds off the first screen', async () => {
    const read = vi.fn()

    const state = await dehydrateListFirstScreen({ page: '3' }, DEFEATING, [
      { queryKey: ['a'], read },
    ])

    expect(state).toEqual(EMPTY_DEHYDRATED_STATE)
    expect(read).not.toHaveBeenCalled()
    expect(seedFirstScreen).not.toHaveBeenCalled()
  })
})
