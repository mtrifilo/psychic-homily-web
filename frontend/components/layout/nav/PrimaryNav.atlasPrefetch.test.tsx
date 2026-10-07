import { describe, it, expect, vi } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import { PrimaryNav } from './PrimaryNav'
import { markAtlasMapReady } from '@/lib/atlasMapReady'

vi.mock('next/navigation', () => ({
  usePathname: () => '/atlas',
}))

vi.mock('@/lib/context/AuthContext', () => ({
  useAuthContext: () => ({ isAuthenticated: false, user: null }),
}))

// A plain anchor that surfaces the prefetch prop the link was rendered with.
vi.mock('next/link', () => import('@/test/mocks/nextLink'))

describe('PrimaryNav on /atlas', () => {
  // PrimaryNav's links are primary destinations: on /atlas they re-arm their
  // prefetch once the map is up.
  it('holds its links until the map is up, then re-arms them', () => {
    const { rerender } = render(<PrimaryNav />)
    const anchors = () => screen.getAllByRole('link')
    expect(anchors().length).toBeGreaterThan(0)
    for (const anchor of anchors()) expect(anchor).toHaveAttribute('data-prefetch', 'false')

    act(() => markAtlasMapReady())
    rerender(<PrimaryNav />)

    for (const anchor of anchors()) expect(anchor).toHaveAttribute('data-prefetch', 'default')
  })
})
