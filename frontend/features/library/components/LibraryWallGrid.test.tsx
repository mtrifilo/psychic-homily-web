import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { LibraryWallGrid } from './LibraryWallGrid'
import { stubImageLoadState } from '@/test/imageLoadState'
import type { SavedShowResponse } from '@/features/shows'

function makeShow(
  overrides: Partial<SavedShowResponse> & { id: number; title: string }
): SavedShowResponse {
  return {
    slug: `show-${overrides.id}`,
    event_date: '2026-07-25T20:00:00Z',
    status: 'approved',
    is_sold_out: false,
    is_cancelled: false,
    venues: [
      {
        id: 1,
        name: 'Valley Bar',
        slug: 'valley-bar',
        city: 'Phoenix',
        state: 'AZ',
      },
    ],
    artists: [{ id: 1, name: 'Militarie Gun', slug: 'militarie-gun' } as SavedShowResponse['artists'][number]],
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    saved_at: '2026-07-01T00:00:00Z',
    image_url: null,
    ...overrides,
  } as SavedShowResponse
}

describe('LibraryWallGrid', () => {
  it('renders typographic fallback when image_url is missing', () => {
    render(
      <LibraryWallGrid
        shows={[makeShow({ id: 1, title: 'Militarie Gun' })]}
        onRemove={vi.fn()}
        isRemovalPending={false}
      />
    )

    expect(screen.getByTestId('library-wall-tile-fallback')).toBeInTheDocument()
    expect(screen.queryByTestId('library-wall-tile-image')).not.toBeInTheDocument()
    expect(
      screen.getAllByText('Militarie Gun').length
    ).toBeGreaterThanOrEqual(1)
  })

  it('renders cover art when image_url is present', () => {
    render(
      <LibraryWallGrid
        shows={[
          makeShow({
            id: 2,
            title: 'Wednesday',
            image_url: 'https://example.com/flyer.jpg',
            artists: [
              {
                id: 2,
                name: 'Wednesday',
                slug: 'wednesday',
              } as SavedShowResponse['artists'][number],
            ],
          }),
        ]}
        onRemove={vi.fn()}
        isRemovalPending={false}
      />
    )

    expect(screen.getByTestId('library-wall-tile-image')).toHaveAttribute(
      'src',
      'https://example.com/flyer.jpg'
    )
    expect(
      screen.queryByTestId('library-wall-tile-fallback')
    ).not.toBeInTheDocument()
  })

  it('keeps remove action available on wall tiles', () => {
    const onRemove = vi.fn()
    render(
      <LibraryWallGrid
        shows={[makeShow({ id: 3, title: 'Bar Italia' })]}
        onRemove={onRemove}
        isRemovalPending={false}
      />
    )

    fireEvent.click(
      screen.getByRole('button', {
        name: /Remove Bar Italia from saved shows/i,
      })
    )
    expect(onRemove).toHaveBeenCalledWith(3)
  })

  it('falls back to typographic tile when the image fails to load', async () => {
    render(
      <LibraryWallGrid
        shows={[
          makeShow({
            id: 4,
            title: 'Broken Flyer',
            image_url: 'https://example.com/missing.jpg',
            artists: [
              {
                id: 4,
                name: 'Broken Flyer',
                slug: 'broken-flyer',
              } as SavedShowResponse['artists'][number],
            ],
          }),
        ]}
        onRemove={vi.fn()}
        isRemovalPending={false}
      />
    )

    const img = screen.getByTestId('library-wall-tile-image')
    fireEvent.error(img)
    expect(await screen.findByTestId('library-wall-tile-fallback')).toBeTruthy()
  })

  // `onError` structurally cannot see a failure that happened before React
  // attached it, which is every failure on a surface that server-renders the
  // tile: the browser starts fetching the flyer while it parses the HTML, so a
  // dead hotlink 404s and fires its error event at nobody. Without the
  // mount-time read this tile keeps a blank bordered square instead of the
  // typographic fallback. `complete` + zero `naturalWidth` is what the element
  // still reports afterwards.
  it('falls back for an image that already failed before the handler attached', () => {
    const img = stubImageLoadState({ complete: true, naturalWidth: 0 })

    try {
      render(
        <LibraryWallGrid
          shows={[
            makeShow({
              id: 5,
              title: 'Already Dead Flyer',
              image_url: 'https://example.com/gone.jpg',
            }),
          ]}
          onRemove={vi.fn()}
          isRemovalPending={false}
        />
      )

      expect(
        screen.getByTestId('library-wall-tile-fallback')
      ).toBeInTheDocument()
      expect(
        screen.queryByTestId('library-wall-tile-image')
      ).not.toBeInTheDocument()
    } finally {
      img.restore()
    }
  })

  // The other half of the predicate, and the only test that pins it. Loosening
  // the check to `complete` alone would blank every flyer the browser HAS
  // decoded; nothing else here catches that, because jsdom reports
  // `complete: false` for an http src, so the tests above never reach that
  // branch. (A loosening to a bare `naturalWidth === 0` is already caught —
  // jsdom reports 0 for every image, so those same tests would fail.)
  it('keeps an image that already finished loading before mount', () => {
    const img = stubImageLoadState({ complete: true, naturalWidth: 600 })

    try {
      render(
        <LibraryWallGrid
          shows={[
            makeShow({
              id: 6,
              title: 'Cached Flyer',
              image_url: 'https://example.com/cached.jpg',
            }),
          ]}
          onRemove={vi.fn()}
          isRemovalPending={false}
        />
      )

      expect(screen.getByTestId('library-wall-tile-image')).toBeInTheDocument()
      expect(
        screen.queryByTestId('library-wall-tile-fallback')
      ).not.toBeInTheDocument()
    } finally {
      img.restore()
    }
  })
})
