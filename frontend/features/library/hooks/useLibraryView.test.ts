import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useLibraryView } from './useLibraryView'

describe('useLibraryView', () => {
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
    localStorageMock.getItem
      .mockReset()
      .mockImplementation((key: string) => store[key] ?? null)
    localStorageMock.setItem
      .mockReset()
      .mockImplementation((key: string, value: string) => {
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

  it('defaults to table', () => {
    const { result } = renderHook(() => useLibraryView())
    expect(result.current.view).toBe('table')
  })

  it('persists wall to localStorage', () => {
    const { result } = renderHook(() => useLibraryView())

    act(() => {
      result.current.setView('wall')
    })

    expect(result.current.view).toBe('wall')
    expect(localStorageMock.setItem).toHaveBeenCalledWith(
      'ph-library-view',
      'wall'
    )
  })

  it('reads a stored wall preference', () => {
    store['ph-library-view'] = 'wall'
    const { result } = renderHook(() => useLibraryView())
    expect(result.current.view).toBe('wall')
  })

  it('falls back to table for invalid storage', () => {
    store['ph-library-view'] = 'mosaic'
    const { result } = renderHook(() => useLibraryView())
    expect(result.current.view).toBe('table')
  })
})
