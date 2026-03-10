import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useCommandPalette, openCommandPalette } from './useCommandPalette'

describe('useCommandPalette', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  describe('open/close state', () => {
    it('should start closed', () => {
      const { result } = renderHook(() => useCommandPalette())
      expect(result.current.open).toBe(false)
    })

    it('should toggle on Cmd+K', () => {
      const { result } = renderHook(() => useCommandPalette())

      act(() => {
        document.dispatchEvent(
          new KeyboardEvent('keydown', { key: 'k', metaKey: true })
        )
      })
      expect(result.current.open).toBe(true)

      act(() => {
        document.dispatchEvent(
          new KeyboardEvent('keydown', { key: 'k', metaKey: true })
        )
      })
      expect(result.current.open).toBe(false)
    })

    it('should toggle on Ctrl+K', () => {
      const { result } = renderHook(() => useCommandPalette())

      act(() => {
        document.dispatchEvent(
          new KeyboardEvent('keydown', { key: 'k', ctrlKey: true })
        )
      })
      expect(result.current.open).toBe(true)
    })

    it('should not toggle on plain K key', () => {
      const { result } = renderHook(() => useCommandPalette())

      act(() => {
        document.dispatchEvent(
          new KeyboardEvent('keydown', { key: 'k' })
        )
      })
      expect(result.current.open).toBe(false)
    })

    it('should open via setOpen', () => {
      const { result } = renderHook(() => useCommandPalette())

      act(() => {
        result.current.setOpen(true)
      })
      expect(result.current.open).toBe(true)
    })

    it('should open via custom event', () => {
      const { result } = renderHook(() => useCommandPalette())

      act(() => {
        openCommandPalette()
      })
      expect(result.current.open).toBe(true)
    })
  })

  describe('recent searches', () => {
    it('should return empty array when no recent searches', () => {
      const { result } = renderHook(() => useCommandPalette())
      expect(result.current.getRecentSearches()).toEqual([])
    })

    it('should add and retrieve recent searches', () => {
      const { result } = renderHook(() => useCommandPalette())

      act(() => {
        result.current.addRecentSearch('Shows')
      })
      expect(result.current.getRecentSearches()).toEqual(['Shows'])
    })

    it('should deduplicate recent searches and move to front', () => {
      const { result } = renderHook(() => useCommandPalette())

      act(() => {
        result.current.addRecentSearch('Shows')
        result.current.addRecentSearch('Artists')
        result.current.addRecentSearch('Shows')
      })

      const recent = result.current.getRecentSearches()
      expect(recent).toEqual(['Shows', 'Artists'])
    })

    it('should limit to 5 recent searches', () => {
      const { result } = renderHook(() => useCommandPalette())

      act(() => {
        result.current.addRecentSearch('A')
        result.current.addRecentSearch('B')
        result.current.addRecentSearch('C')
        result.current.addRecentSearch('D')
        result.current.addRecentSearch('E')
        result.current.addRecentSearch('F')
      })

      const recent = result.current.getRecentSearches()
      expect(recent).toHaveLength(5)
      expect(recent[0]).toBe('F')
      expect(recent).not.toContain('A')
    })

    it('should not add empty/whitespace searches', () => {
      const { result } = renderHook(() => useCommandPalette())

      act(() => {
        result.current.addRecentSearch('')
        result.current.addRecentSearch('   ')
      })
      expect(result.current.getRecentSearches()).toEqual([])
    })

    it('should clear recent searches', () => {
      const { result } = renderHook(() => useCommandPalette())

      act(() => {
        result.current.addRecentSearch('Shows')
        result.current.addRecentSearch('Artists')
        result.current.clearRecentSearches()
      })

      expect(result.current.getRecentSearches()).toEqual([])
    })

    it('should handle corrupted localStorage gracefully', () => {
      localStorage.setItem('command-palette-recent', 'not-valid-json')
      const { result } = renderHook(() => useCommandPalette())
      expect(result.current.getRecentSearches()).toEqual([])
    })
  })

  describe('cleanup', () => {
    it('should remove keyboard listener on unmount', () => {
      const removeEventListenerSpy = vi.spyOn(document, 'removeEventListener')
      const { unmount } = renderHook(() => useCommandPalette())

      unmount()

      expect(removeEventListenerSpy).toHaveBeenCalledWith(
        'keydown',
        expect.any(Function)
      )
    })
  })
})
