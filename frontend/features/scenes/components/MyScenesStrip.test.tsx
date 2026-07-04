import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'
import type { SceneListItem } from '../types'

const mockPush = vi.fn()
vi.mock('next/navigation', () => ({
  useRouter: () => ({ push: mockPush }),
}))

// useMyFollowing pulls AuthContext (unavailable here) — stub the follows hook.
const mockUseMyFollowing = vi.fn()
vi.mock('@/lib/hooks/common/useFollow', () => ({
  useMyFollowing: (opts: unknown) => mockUseMyFollowing(opts),
}))

import { MyScenesStrip } from './MyScenesStrip'

const scenes: SceneListItem[] = [
  {
    city: 'Phoenix',
    state: 'AZ',
    slug: 'phoenix-az',
    venue_count: 11,
    upcoming_show_count: 42,
    total_show_count: 180,
    shows_this_week: 0,
    latitude: 33.448,
    longitude: -112.074,
  },
  {
    city: 'Chicago',
    state: 'IL',
    slug: 'chicago-il',
    venue_count: 9,
    upcoming_show_count: 283,
    total_show_count: 337,
    shows_this_week: 0,
    latitude: 41.88,
    longitude: -87.63,
  },
]

function follows(entries: Array<{ slug: string; name: string }>) {
  return {
    data: {
      following: entries.map((e, i) => ({
        entity_type: 'scene',
        entity_id: i + 1,
        name: e.name,
        slug: e.slug,
        followed_at: '2026-07-01T00:00:00Z',
      })),
      total: entries.length,
    },
  }
}

describe('MyScenesStrip (PSY-1340)', () => {
  const onPick = vi.fn()

  beforeEach(() => {
    onPick.mockReset()
    mockPush.mockReset()
    mockUseMyFollowing.mockReset()
  })

  it('renders nothing while logged out / with no follows', () => {
    mockUseMyFollowing.mockReturnValue({ data: undefined })
    renderWithProviders(<MyScenesStrip scenes={scenes} onPick={onPick} />)
    expect(screen.queryByRole('navigation', { name: /my scenes/i })).not.toBeInTheDocument()
  })

  it('lists followed scenes liveliest-first and flies to a placeable pick', () => {
    mockUseMyFollowing.mockReturnValue(
      follows([
        { slug: 'phoenix-az', name: 'Phoenix, AZ' },
        { slug: 'chicago-il', name: 'Chicago, IL' },
      ]),
    )
    renderWithProviders(<MyScenesStrip scenes={scenes} onPick={onPick} />)

    const nav = screen.getByRole('navigation', { name: /my scenes/i })
    const chips = nav.querySelectorAll('button')
    // Chicago (283) outranks Phoenix (42) — the shared liveliest-first rule.
    expect(chips[0]).toHaveTextContent('Chicago, IL')
    expect(chips[1]).toHaveTextContent('Phoenix, AZ')

    fireEvent.click(chips[0])
    expect(onPick).toHaveBeenCalledTimes(1)
    expect(onPick.mock.calls[0][0]).toMatchObject({ slug: 'chicago-il' })
    expect(mockPush).not.toHaveBeenCalled()
  })

  it('caps the chips and routes the overflow to /following?tab=scene', () => {
    // 12 follows, total reported 30 (a truncated fetch page) — the strip shows
    // the cap, and "+N more" counts against the TOTAL so nothing is silently
    // invisible.
    const entries = Array.from({ length: 12 }, (_, i) => ({
      slug: `scene-${i}`,
      name: `Scene ${i}`,
    }))
    const resp = follows(entries)
    resp.data.total = 30
    mockUseMyFollowing.mockReturnValue(resp)
    renderWithProviders(<MyScenesStrip scenes={scenes} onPick={onPick} />)

    const nav = screen.getByRole('navigation', { name: /my scenes/i })
    expect(nav.querySelectorAll('button')).toHaveLength(8)
    const moreLink = screen.getByRole('link', { name: '+22 more' })
    expect(moreLink).toHaveAttribute('href', '/following?tab=scene')
  })

  it('navigates to the scene page for a follow the globe cannot place', () => {
    // e.g. a followed scene below this season's listing threshold, or with no
    // coords — it still deserves a way back.
    mockUseMyFollowing.mockReturnValue(
      follows([{ slug: 'faketown-zz', name: 'Faketown, ZZ' }]),
    )
    renderWithProviders(<MyScenesStrip scenes={scenes} onPick={onPick} />)

    fireEvent.click(screen.getByRole('button', { name: 'Faketown, ZZ' }))
    expect(mockPush).toHaveBeenCalledWith('/scenes/faketown-zz')
    expect(onPick).not.toHaveBeenCalled()
  })
})
