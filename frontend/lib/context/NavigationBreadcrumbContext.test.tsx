import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { NavigationBreadcrumbProvider, useNavigationBreadcrumbs } from './NavigationBreadcrumbContext'

// Mock sessionStorage
const sessionStorageMock = (() => {
  let store: Record<string, string> = {}
  return {
    getItem: vi.fn((key: string) => store[key] ?? null),
    setItem: vi.fn((key: string, value: string) => { store[key] = value }),
    removeItem: vi.fn((key: string) => { delete store[key] }),
    clear: vi.fn(() => { store = {} }),
  }
})()
Object.defineProperty(window, 'sessionStorage', { value: sessionStorageMock })

function wrapper({ children }: { children: React.ReactNode }) {
  return <NavigationBreadcrumbProvider>{children}</NavigationBreadcrumbProvider>
}

describe('NavigationBreadcrumbContext', () => {
  beforeEach(() => {
    sessionStorageMock.clear()
    vi.clearAllMocks()
  })

  it('starts with empty breadcrumbs', () => {
    const { result } = renderHook(() => useNavigationBreadcrumbs(), { wrapper })
    expect(result.current.breadcrumbs).toEqual([])
  })

  it('pushes a breadcrumb entry', () => {
    const { result } = renderHook(() => useNavigationBreadcrumbs(), { wrapper })

    act(() => {
      result.current.pushBreadcrumb('Artists', '/artists')
    })

    expect(result.current.breadcrumbs).toEqual([
      { label: 'Artists', href: '/artists' },
    ])
  })

  it('pushes multiple breadcrumb entries', () => {
    const { result } = renderHook(() => useNavigationBreadcrumbs(), { wrapper })

    act(() => {
      result.current.pushBreadcrumb('Shows', '/shows')
    })
    act(() => {
      result.current.pushBreadcrumb('Jeff Tweedy at Van Buren', '/shows/jeff-tweedy')
    })
    act(() => {
      result.current.pushBreadcrumb('Macie Stewart', '/artists/macie-stewart')
    })

    expect(result.current.breadcrumbs).toHaveLength(3)
    expect(result.current.breadcrumbs[0].label).toBe('Shows')
    expect(result.current.breadcrumbs[1].label).toBe('Jeff Tweedy at Van Buren')
    expect(result.current.breadcrumbs[2].label).toBe('Macie Stewart')
  })

  it('pops back to existing entry when href already in stack', () => {
    const { result } = renderHook(() => useNavigationBreadcrumbs(), { wrapper })

    act(() => {
      result.current.pushBreadcrumb('Shows', '/shows')
    })
    act(() => {
      result.current.pushBreadcrumb('Artist A', '/artists/a')
    })
    act(() => {
      result.current.pushBreadcrumb('Venue B', '/venues/b')
    })

    // Navigate back to /shows - should pop back
    act(() => {
      result.current.pushBreadcrumb('Shows', '/shows')
    })

    expect(result.current.breadcrumbs).toHaveLength(1)
    expect(result.current.breadcrumbs[0]).toEqual({ label: 'Shows', href: '/shows' })
  })

  it('limits stack to 4 entries', () => {
    const { result } = renderHook(() => useNavigationBreadcrumbs(), { wrapper })

    act(() => {
      result.current.pushBreadcrumb('Entry 1', '/1')
    })
    act(() => {
      result.current.pushBreadcrumb('Entry 2', '/2')
    })
    act(() => {
      result.current.pushBreadcrumb('Entry 3', '/3')
    })
    act(() => {
      result.current.pushBreadcrumb('Entry 4', '/4')
    })
    act(() => {
      result.current.pushBreadcrumb('Entry 5', '/5')
    })

    expect(result.current.breadcrumbs).toHaveLength(4)
    expect(result.current.breadcrumbs[0].label).toBe('Entry 2')
    expect(result.current.breadcrumbs[3].label).toBe('Entry 5')
  })

  it('persists to sessionStorage', () => {
    const { result } = renderHook(() => useNavigationBreadcrumbs(), { wrapper })

    act(() => {
      result.current.pushBreadcrumb('Shows', '/shows')
    })

    expect(sessionStorageMock.setItem).toHaveBeenCalledWith(
      'ph-breadcrumbs',
      JSON.stringify([{ label: 'Shows', href: '/shows' }])
    )
  })

  it('throws when used outside provider', () => {
    expect(() => {
      renderHook(() => useNavigationBreadcrumbs())
    }).toThrow('useNavigationBreadcrumbs must be used within NavigationBreadcrumbProvider')
  })
})
