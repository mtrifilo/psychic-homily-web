import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useLocalStorageEnum } from './useLocalStorageEnum'

const COLORS = ['red', 'green', 'blue'] as const
type Color = (typeof COLORS)[number]

describe('useLocalStorageEnum', () => {
  let store: Record<string, string> = {}

  const localStorageMock = {
    getItem: vi.fn((key: string) => store[key] ?? null),
    setItem: vi.fn((key: string, value: string) => {
      store[key] = value
    }),
    removeItem: vi.fn((key: string) => {
      delete store[key]
    }),
    clear: vi.fn(() => {
      store = {}
    }),
  }

  beforeEach(() => {
    store = {}
    // Reset mocks AND re-apply default impls. `mockClear()` would preserve
    // any per-test `mockImplementation` / `mockReturnValue` overrides, which
    // would leak state across tests.
    localStorageMock.getItem.mockReset().mockImplementation((key: string) => store[key] ?? null)
    localStorageMock.setItem.mockReset().mockImplementation((key: string, value: string) => {
      store[key] = value
    })
    localStorageMock.removeItem.mockReset().mockImplementation((key: string) => {
      delete store[key]
    })
    Object.defineProperty(window, 'localStorage', {
      value: localStorageMock,
      writable: true,
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('returns the default value when no stored value exists', () => {
    const { result } = renderHook(() =>
      useLocalStorageEnum<Color>('test-color', 'red', COLORS)
    )
    expect(result.current[0]).toBe('red')
  })

  it('returns the stored value when localStorage has a valid entry', () => {
    store['test-color'] = 'green'
    const { result } = renderHook(() =>
      useLocalStorageEnum<Color>('test-color', 'red', COLORS)
    )
    expect(result.current[0]).toBe('green')
  })

  it('falls back to default when stored value is not in allowed list', () => {
    store['test-color'] = 'purple'
    const { result } = renderHook(() =>
      useLocalStorageEnum<Color>('test-color', 'red', COLORS)
    )
    expect(result.current[0]).toBe('red')
  })

  it('persists the new value to localStorage when setter is called', () => {
    const { result } = renderHook(() =>
      useLocalStorageEnum<Color>('test-color', 'red', COLORS)
    )
    act(() => {
      result.current[1]('blue')
    })
    expect(result.current[0]).toBe('blue')
    expect(localStorageMock.setItem).toHaveBeenCalledWith('test-color', 'blue')
  })

  it('re-renders the same component when the setter is called', () => {
    const { result } = renderHook(() =>
      useLocalStorageEnum<Color>('test-color', 'red', COLORS)
    )
    expect(result.current[0]).toBe('red')
    act(() => {
      result.current[1]('green')
    })
    expect(result.current[0]).toBe('green')
    act(() => {
      result.current[1]('blue')
    })
    expect(result.current[0]).toBe('blue')
  })

  it('keeps two components reading the same key in sync within a tab', () => {
    const a = renderHook(() =>
      useLocalStorageEnum<Color>('shared-color', 'red', COLORS)
    )
    const b = renderHook(() =>
      useLocalStorageEnum<Color>('shared-color', 'red', COLORS)
    )
    expect(a.result.current[0]).toBe('red')
    expect(b.result.current[0]).toBe('red')

    act(() => {
      a.result.current[1]('green')
    })

    expect(a.result.current[0]).toBe('green')
    expect(b.result.current[0]).toBe('green')
  })

  it('re-renders when a cross-tab storage event fires', () => {
    const { result } = renderHook(() =>
      useLocalStorageEnum<Color>('test-color', 'red', COLORS)
    )
    expect(result.current[0]).toBe('red')

    act(() => {
      store['test-color'] = 'blue'
      window.dispatchEvent(new StorageEvent('storage', { key: 'test-color' }))
    })

    expect(result.current[0]).toBe('blue')
  })

  it('returns default when localStorage.getItem throws', () => {
    localStorageMock.getItem.mockImplementation(() => {
      throw new Error('localStorage disabled')
    })
    const { result } = renderHook(() =>
      useLocalStorageEnum<Color>('test-color', 'red', COLORS)
    )
    expect(result.current[0]).toBe('red')
  })

  it('keeps the calling component responsive when localStorage.setItem throws', () => {
    localStorageMock.setItem.mockImplementation(() => {
      throw new Error('localStorage full')
    })
    const { result } = renderHook(() =>
      useLocalStorageEnum<Color>('test-color', 'red', COLORS)
    )
    act(() => {
      result.current[1]('green')
    })
    // The setter doesn't throw, and the per-component intent layer keeps the
    // UI live even though localStorage persistence failed.
    expect(result.current[0]).toBe('green')
  })

  it('treats different keys as independent', () => {
    const a = renderHook(() =>
      useLocalStorageEnum<Color>('key-a', 'red', COLORS)
    )
    const b = renderHook(() =>
      useLocalStorageEnum<Color>('key-b', 'green', COLORS)
    )

    act(() => {
      a.result.current[1]('blue')
    })

    expect(a.result.current[0]).toBe('blue')
    expect(b.result.current[0]).toBe('green')
  })
})
