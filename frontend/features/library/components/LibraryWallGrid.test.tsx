import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { LibraryWallGrid } from './LibraryWallGrid'
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
      screen.getByRole('button', { name: /Remove Bar Italia from saved shows/i })
    )
    expect(onRemove).toHaveBeenCalledWith(3)
  })
})
