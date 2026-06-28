import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fireEvent, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'
import { renderWithProviders } from '@/test/utils'
import type { SceneListItem } from '../types'

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string
    children: ReactNode
  }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}))

const mockUseSceneArtists = vi.fn()
vi.mock('../hooks', () => ({
  useSceneArtists: (opts: unknown) => mockUseSceneArtists(opts),
}))

import { ScenePreviewPanel } from './ScenePreviewPanel'

const scene: SceneListItem = {
  city: 'Chicago',
  state: 'IL',
  slug: 'chicago-il',
  venue_count: 9,
  upcoming_show_count: 283,
  total_show_count: 337,
  latitude: 41.88,
  longitude: -87.63,
}

describe('ScenePreviewPanel', () => {
  beforeEach(() => {
    mockUseSceneArtists.mockReset()
  })

  it('renders the city, counts, active artists, and a link into the scene', () => {
    mockUseSceneArtists.mockReturnValue({
      data: { artists: [{ id: 1, slug: 'band-a', name: 'Band A' }], total: 1 },
      isLoading: false,
    })
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={() => {}} />)

    expect(screen.getByText('Chicago, IL')).toBeInTheDocument()
    expect(screen.getByText(/283 upcoming · 9 venues/)).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Band A' }),
    ).toHaveAttribute('href', '/artists/band-a')
    expect(
      screen.getByRole('link', { name: /open scene/i }),
    ).toHaveAttribute('href', '/scenes/chicago-il')
  })

  it('flags active roster members with an accessible "(active)" marker', () => {
    mockUseSceneArtists.mockReturnValue({
      data: {
        artists: [
          { id: 1, slug: 'band-a', name: 'Band A', is_active: true },
          { id: 2, slug: 'band-b', name: 'Band B', is_active: false },
        ],
        total: 2,
      },
      isLoading: false,
    })
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={() => {}} />)

    const bandA = screen.getByText('Band A').closest('li')!
    expect(within(bandA).getByText('(active)')).toBeInTheDocument()
    const bandB = screen.getByText('Band B').closest('li')!
    expect(within(bandB).queryByText('(active)')).not.toBeInTheDocument()
  })

  it('calls onClose when the close button is clicked', () => {
    mockUseSceneArtists.mockReturnValue({ data: undefined, isLoading: false })
    const onClose = vi.fn()
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={onClose} />)

    fireEvent.click(screen.getByRole('button', { name: /close scene preview/i }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('shows a loading state while artists load', () => {
    mockUseSceneArtists.mockReturnValue({ data: undefined, isLoading: true })
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={() => {}} />)
    expect(screen.getByText('Loading…')).toBeInTheDocument()
  })

  it('handles a scene with an empty roster', () => {
    mockUseSceneArtists.mockReturnValue({
      data: { artists: [], total: 0 },
      isLoading: false,
    })
    renderWithProviders(<ScenePreviewPanel scene={scene} onClose={() => {}} />)
    expect(screen.getByText(/no artists based here yet/i)).toBeInTheDocument()
  })
})
