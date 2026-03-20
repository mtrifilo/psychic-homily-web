import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useDensity, type Density } from './useDensity'

describe('useDensity', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.restoreAllMocks()
  })

  describe('initial state', () => {
    it('should default to comfortable density', () => {
      const { result } = renderHook(() => useDensity())
      expect(result.current.density).toBe('comfortable')
    })

    it('should read stored density from localStorage on mount', async () => {
      localStorage.setItem('ph-density', 'compact')

      const { result } = renderHook(() => useDensity())

      // useEffect reads from localStorage after mount
      await vi.waitFor(() => {
        expect(result.current.density).toBe('compact')
      })
    })

    it('should read stored density with custom suffix', async () => {
      localStorage.setItem('ph-density-shows', 'expanded')

      const { result } = renderHook(() => useDensity('shows'))

      await vi.waitFor(() => {
        expect(result.current.density).toBe('expanded')
      })
    })

    it('should fall back to comfortable for invalid stored value', async () => {
      localStorage.setItem('ph-density', 'invalid-value')

      const { result } = renderHook(() => useDensity())

      // After mount effect runs, it should still be comfortable
      await vi.waitFor(() => {
        expect(result.current.density).toBe('comfortable')
      })
    })

    it('should fall back to comfortable when localStorage is empty', () => {
      const { result } = renderHook(() => useDensity())
      expect(result.current.density).toBe('comfortable')
    })
  })

  describe('setDensity', () => {
    it('should update density to compact', () => {
      const { result } = renderHook(() => useDensity())

      act(() => {
        result.current.setDensity('compact')
      })

      expect(result.current.density).toBe('compact')
    })

    it('should update density to expanded', () => {
      const { result } = renderHook(() => useDensity())

      act(() => {
        result.current.setDensity('expanded')
      })

      expect(result.current.density).toBe('expanded')
    })

    it('should persist density to localStorage', () => {
      const { result } = renderHook(() => useDensity())

      act(() => {
        result.current.setDensity('compact')
      })

      expect(localStorage.getItem('ph-density')).toBe('compact')
    })

    it('should persist density with custom suffix', () => {
      const { result } = renderHook(() => useDensity('artists'))

      act(() => {
        result.current.setDensity('expanded')
      })

      expect(localStorage.getItem('ph-density-artists')).toBe('expanded')
    })

    it('should allow cycling through all density values', () => {
      const { result } = renderHook(() => useDensity())

      const densities: Density[] = ['compact', 'comfortable', 'expanded']
      for (const d of densities) {
        act(() => {
          result.current.setDensity(d)
        })
        expect(result.current.density).toBe(d)
        expect(localStorage.getItem('ph-density')).toBe(d)
      }
    })
  })

  describe('storage key isolation', () => {
    it('should use different storage keys for different suffixes', () => {
      const { result: showsResult } = renderHook(() => useDensity('shows'))
      const { result: artistsResult } = renderHook(() => useDensity('artists'))

      act(() => {
        showsResult.current.setDensity('compact')
      })
      act(() => {
        artistsResult.current.setDensity('expanded')
      })

      expect(localStorage.getItem('ph-density-shows')).toBe('compact')
      expect(localStorage.getItem('ph-density-artists')).toBe('expanded')
      expect(showsResult.current.density).toBe('compact')
      expect(artistsResult.current.density).toBe('expanded')
    })

    it('should not interfere between suffixed and unsuffixed keys', () => {
      const { result: globalResult } = renderHook(() => useDensity())
      const { result: showsResult } = renderHook(() => useDensity('shows'))

      act(() => {
        globalResult.current.setDensity('compact')
      })
      act(() => {
        showsResult.current.setDensity('expanded')
      })

      expect(localStorage.getItem('ph-density')).toBe('compact')
      expect(localStorage.getItem('ph-density-shows')).toBe('expanded')
    })
  })

  describe('localStorage error handling', () => {
    it('should handle localStorage.getItem throwing', async () => {
      vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
        throw new Error('localStorage disabled')
      })

      const { result } = renderHook(() => useDensity())

      // Should fall back to default without throwing
      await vi.waitFor(() => {
        expect(result.current.density).toBe('comfortable')
      })
    })

    it('should handle localStorage.setItem throwing', () => {
      vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
        throw new Error('QuotaExceededError')
      })

      const { result } = renderHook(() => useDensity())

      // Should update state even if localStorage write fails
      act(() => {
        result.current.setDensity('compact')
      })

      expect(result.current.density).toBe('compact')
    })
  })

  describe('return value stability', () => {
    it('should return a stable setDensity callback', () => {
      const { result, rerender } = renderHook(() => useDensity())

      const firstSetDensity = result.current.setDensity
      rerender()
      const secondSetDensity = result.current.setDensity

      expect(firstSetDensity).toBe(secondSetDensity)
    })
  })
})
