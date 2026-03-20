import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useDensity, type Density } from './useDensity'

describe('useDensity', () => {
  const localStorageMock = (() => {
    let store: Record<string, string> = {}
    return {
      getItem: vi.fn((key: string) => store[key] ?? null),
      setItem: vi.fn((key: string, value: string) => {
        store[key] = value
      }),
      clear: () => {
        store = {}
      },
    }
  })()

  beforeEach(() => {
    localStorageMock.clear()
    localStorageMock.getItem.mockClear()
    localStorageMock.setItem.mockClear()
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

    // Before the useEffect runs, density is the default
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
    localStorageMock.getItem.mockReturnValue('expanded')

    const { result } = renderHook(() => useDensity())

    // After the effect runs, it should read from localStorage
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
    localStorageMock.getItem.mockImplementation((key: string) => {
      if (key === 'ph-density-artists') return 'expanded'
      return null
    })

    const { result } = renderHook(() => useDensity('artists'))

    expect(localStorageMock.getItem).toHaveBeenCalledWith('ph-density-artists')
    expect(result.current.density).toBe('expanded')
  })

  it('falls back to default for invalid stored values', () => {
    localStorageMock.getItem.mockReturnValue('invalid-value')

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
      localStorageMock.getItem.mockReturnValue(density)

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
