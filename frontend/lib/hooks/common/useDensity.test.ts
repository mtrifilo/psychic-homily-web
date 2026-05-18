import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useDensity, type Density } from './useDensity'

describe('useDensity', () => {
  let store: Record<string, string> = {}

  const localStorageMock = {
    getItem: vi.fn((key: string) => store[key] ?? null),
    setItem: vi.fn((key: string, value: string) => {
      store[key] = value
    }),
    clear: vi.fn(() => {
      store = {}
    }),
  }

  beforeEach(() => {
    store = {}
    // `mockReset` + re-apply default impl avoids `mockReturnValue` /
    // `mockImplementation` overrides leaking across tests.
    localStorageMock.getItem.mockReset().mockImplementation((key: string) => store[key] ?? null)
    localStorageMock.setItem.mockReset().mockImplementation((key: string, value: string) => {
      store[key] = value
    })
    Object.defineProperty(window, 'localStorage', {
      value: localStorageMock,
      writable: true,
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('returns default density "comfortable" initially', () => {
    const { result } = renderHook(() => useDensity())

    expect(result.current.density).toBe('comfortable')
  })

  it('provides setDensity function', () => {
    const { result } = renderHook(() => useDensity())

    expect(typeof result.current.setDensity).toBe('function')
  })

  it('persists density to localStorage when setDensity is called', () => {
    const { result } = renderHook(() => useDensity())

    act(() => {
      result.current.setDensity('compact')
    })

    expect(result.current.density).toBe('compact')
    expect(localStorageMock.setItem).toHaveBeenCalledWith('ph-density', 'compact')
  })

  it('reads stored density from localStorage on mount', () => {
    store['ph-density'] = 'expanded'

    const { result } = renderHook(() => useDensity())

    expect(localStorageMock.getItem).toHaveBeenCalledWith('ph-density')
    expect(result.current.density).toBe('expanded')
  })

  it('uses storage key suffix when provided', () => {
    const { result } = renderHook(() => useDensity('shows'))

    act(() => {
      result.current.setDensity('compact')
    })

    expect(localStorageMock.setItem).toHaveBeenCalledWith('ph-density-shows', 'compact')
  })

  it('reads from suffixed key on mount', () => {
    store['ph-density-artists'] = 'expanded'

    const { result } = renderHook(() => useDensity('artists'))

    expect(localStorageMock.getItem).toHaveBeenCalledWith('ph-density-artists')
    expect(result.current.density).toBe('expanded')
  })

  it('falls back to default for invalid stored values', () => {
    store['ph-density'] = 'invalid-value'

    const { result } = renderHook(() => useDensity())

    expect(result.current.density).toBe('comfortable')
  })

  it('handles localStorage errors gracefully on read', () => {
    localStorageMock.getItem.mockImplementation(() => {
      throw new Error('localStorage disabled')
    })

    const { result } = renderHook(() => useDensity())

    // Should fall back to default without throwing
    expect(result.current.density).toBe('comfortable')
  })

  it('handles localStorage errors gracefully on write', () => {
    localStorageMock.setItem.mockImplementation(() => {
      throw new Error('localStorage full')
    })

    const { result } = renderHook(() => useDensity())

    // Should not throw, but still update state in memory
    act(() => {
      result.current.setDensity('expanded')
    })

    expect(result.current.density).toBe('expanded')
  })

  it('supports all valid density values', () => {
    const validDensities: Density[] = ['compact', 'comfortable', 'expanded']

    for (const density of validDensities) {
      store = { 'ph-density': density }

      const { result } = renderHook(() => useDensity())

      expect(result.current.density).toBe(density)
    }
  })

  it('updates density when setDensity is called multiple times', () => {
    const { result } = renderHook(() => useDensity())

    act(() => {
      result.current.setDensity('compact')
    })
    expect(result.current.density).toBe('compact')

    act(() => {
      result.current.setDensity('expanded')
    })
    expect(result.current.density).toBe('expanded')

    act(() => {
      result.current.setDensity('comfortable')
    })
    expect(result.current.density).toBe('comfortable')
  })
})
