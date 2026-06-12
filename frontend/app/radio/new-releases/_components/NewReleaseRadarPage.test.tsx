import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import type { RadioNewReleaseRadarEntry } from '@/features/radio'

const mockUseNewReleaseRadar = vi.fn()

vi.mock('@/features/radio', async importOriginal => {
  const actual = await importOriginal<typeof import('@/features/radio')>()
  return {
    ...actual,
    useNewReleaseRadar: (...args: unknown[]) => mockUseNewReleaseRadar(...args),
  }
})

import NewReleaseRadarPage from './NewReleaseRadarPage'

function makeEntry(
  overrides: Partial<RadioNewReleaseRadarEntry> = {}
): RadioNewReleaseRadarEntry {
  return {
    artist_name: 'Wet Leg',
    artist_id: 4,
    artist_slug: 'wet-leg',
    album_title: 'Moisturizer',
    label_name: 'Domino',
    release_id: 8,
    release_slug: 'wet-leg-moisturizer',
    label_id: 2,
    label_slug: 'domino',
    play_count: 24,
    station_count: 2,
    ...overrides,
  }
}

function setReleases(releases: RadioNewReleaseRadarEntry[]) {
  mockUseNewReleaseRadar.mockReturnValue({
    data: { releases, count: releases.length },
    isLoading: false,
    isFetching: false,
    error: null,
  })
}

describe('NewReleaseRadarPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders breadcrumb, heading, and a row with release + label links', () => {
    setReleases([makeEntry()])
    render(<NewReleaseRadarPage />)

    expect(screen.getByRole('link', { name: 'Radio' })).toHaveAttribute(
      'href',
      '/radio'
    )
    expect(
      screen.getByRole('heading', { level: 1, name: 'New release radar' })
    ).toBeInTheDocument()

    expect(
      screen.getByRole('link', { name: 'Wet Leg — Moisturizer' })
    ).toHaveAttribute('href', '/releases/wet-leg-moisturizer')
    expect(screen.getByRole('link', { name: 'Domino' })).toHaveAttribute(
      'href',
      '/labels/domino'
    )
    expect(screen.getByText('24')).toBeInTheDocument()
    expect(screen.getByText('2')).toBeInTheDocument()
  })

  it('falls back to the artist page, then plain text (no dead links)', () => {
    setReleases([
      makeEntry({ release_slug: null }),
      makeEntry({
        artist_name: 'Florry',
        album_title: 'Sounds Like...',
        artist_slug: null,
        release_slug: null,
        label_name: null,
        label_slug: null,
      }),
    ])
    render(<NewReleaseRadarPage />)

    expect(
      screen.getByRole('link', { name: 'Wet Leg — Moisturizer' })
    ).toHaveAttribute('href', '/artists/wet-leg')

    expect(screen.getByText('Florry — Sounds Like...')).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Florry — Sounds Like...' })
    ).not.toBeInTheDocument()
  })

  it('grows the in-place limit on "More releases" while pages come back full', () => {
    // A full first page (20 of 20) means more may exist.
    setReleases(
      Array.from({ length: 20 }, (_, i) =>
        makeEntry({ artist_name: `Artist ${i}` })
      )
    )
    render(<NewReleaseRadarPage />)

    expect(screen.getByText('showing 20 releases')).toBeInTheDocument()
    expect(mockUseNewReleaseRadar).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 20 })
    )

    fireEvent.click(screen.getByRole('button', { name: 'More releases' }))
    expect(mockUseNewReleaseRadar).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 40 })
    )
  })

  it('keeps a disabled "Loading…" control mounted while a limit bump is in flight', () => {
    // keepPreviousData serves the OLD full page (20 rows) against the NEW
    // limit (40) while fetching — the control must not flicker out.
    const fullPage = {
      releases: Array.from({ length: 20 }, (_, i) =>
        makeEntry({ artist_name: `Artist ${i}` })
      ),
      count: 20,
    }
    mockUseNewReleaseRadar.mockImplementation((opts: { limit?: number }) => ({
      data: fullPage,
      isLoading: false,
      // The bumped-limit query is in flight; the old page is the placeholder.
      isFetching: (opts?.limit ?? 20) > 20,
      isPlaceholderData: (opts?.limit ?? 20) > 20,
      error: null,
    }))
    render(<NewReleaseRadarPage />)

    fireEvent.click(screen.getByRole('button', { name: 'More releases' }))
    expect(screen.getByRole('button', { name: 'Loading…' })).toBeDisabled()
  })

  it('hides the load-more control when the radar comes back short', () => {
    setReleases([makeEntry()])
    render(<NewReleaseRadarPage />)

    expect(screen.getByText('showing 1 release')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'More releases' })
    ).not.toBeInTheDocument()
  })

  it('hides the load-more control at the API limit cap of 100', () => {
    setReleases(
      Array.from({ length: 100 }, (_, i) =>
        makeEntry({ artist_name: `Artist ${i}` })
      )
    )
    render(<NewReleaseRadarPage />)

    // 20 → 40 → 60 → 80 → 100: four clicks reach the cap.
    for (let i = 0; i < 4; i++) {
      fireEvent.click(screen.getByRole('button', { name: 'More releases' }))
    }
    expect(mockUseNewReleaseRadar).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 100 })
    )

    expect(screen.getByText('showing 100 releases')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'More releases' })
    ).not.toBeInTheDocument()
  })

  it('renders an empty state when nothing is on the radar', () => {
    setReleases([])
    render(<NewReleaseRadarPage />)
    expect(screen.getByText('Nothing on the radar yet.')).toBeInTheDocument()
  })

  it('renders an error state when the radar fails to load', () => {
    mockUseNewReleaseRadar.mockReturnValue({
      data: undefined,
      isLoading: false,
      isFetching: false,
      error: new Error('boom'),
    })
    render(<NewReleaseRadarPage />)
    expect(
      screen.getByText("Couldn't load the new release radar.")
    ).toBeInTheDocument()
  })
})
