import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'

const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({
    push: mockPush,
    replace: vi.fn(),
    back: vi.fn(),
    forward: vi.fn(),
    refresh: vi.fn(),
    prefetch: vi.fn(),
  }),
}))

import { useFilterNavigation } from './useFilterNavigation'

describe('useFilterNavigation', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockPush.mockReset()
  })

  it('returns navigate function and isPending state', () => {
    const { result } = renderHook(() => useFilterNavigation('/shows'))

    expect(typeof result.current.navigate).toBe('function')
    expect(typeof result.current.isPending).toBe('boolean')
  })

  it('navigates with query params', () => {
    const { result } = renderHook(() => useFilterNavigation('/shows'))

    act(() => {
      result.current.navigate({ city: 'Phoenix', state: 'AZ' })
    })

    expect(mockPush).toHaveBeenCalledWith('/shows?city=Phoenix&state=AZ')
  })

  it('navigates to base path when all params are null', () => {
    const { result } = renderHook(() => useFilterNavigation('/artists'))

    act(() => {
      result.current.navigate({ city: null, state: null })
    })

    expect(mockPush).toHaveBeenCalledWith('/artists')
  })

  it('omits null values from query string', () => {
    const { result } = renderHook(() => useFilterNavigation('/shows'))

    act(() => {
      result.current.navigate({ city: 'Phoenix', state: null, genre: 'rock' })
    })

    expect(mockPush).toHaveBeenCalledWith('/shows?city=Phoenix&genre=rock')
  })

  it('uses the provided basePath', () => {
    const { result } = renderHook(() => useFilterNavigation('/custom/path'))

    act(() => {
      result.current.navigate({ filter: 'value' })
    })

    expect(mockPush).toHaveBeenCalledWith('/custom/path?filter=value')
  })
})
