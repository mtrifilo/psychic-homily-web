import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { LatestPlaylistsTable } from './LatestPlaylistsTable'
import type { RadioStationEpisodeRow } from '@/features/radio'
import { localIso } from '@/features/radio/lib/localIso.testutil'

function makeRow(
  overrides: Partial<RadioStationEpisodeRow> = {}
): RadioStationEpisodeRow {
  return {
    id: 1,
    title: null,
    air_date: '2026-06-09',
    starts_at: null,
    ends_at: null,
    play_count: 24,
    archive_url: null,
    show_id: 3,
    show_name: 'The Night Owl Show',
    show_slug: 'night-owl',
    station_id: 2,
    station_name: 'WFMU',
    station_slug: 'wfmu',
    artist_preview: [
      { artist_name: 'CAN', artist_id: 9, artist_slug: 'can' },
      { artist_name: "it's all meat", artist_id: null, artist_slug: null },
    ],
    ...overrides,
  }
}

describe('LatestPlaylistsTable', () => {
  it('renders a loading spinner while the feed is in flight', () => {
    render(<LatestPlaylistsTable rows={undefined} isLoading error={null} />)
    expect(screen.getByText('Loading latest playlists')).toBeInTheDocument()
  })

  it('renders an error message when the feed fails', () => {
    render(
      <LatestPlaylistsTable
        rows={undefined}
        isLoading={false}
        error={new Error('boom')}
      />
    )
    expect(
      screen.getByText("Couldn't load the latest playlists.")
    ).toBeInTheDocument()
  })

  it('renders an empty state when there are no episodes', () => {
    render(<LatestPlaylistsTable rows={[]} isLoading={false} error={null} />)
    expect(screen.getByText('No playlists tracked yet.')).toBeInTheDocument()
  })

  it('renders date, station, show link, and track count for each row', () => {
    render(
      <LatestPlaylistsTable rows={[makeRow()]} isLoading={false} error={null} />
    )

    expect(screen.getByText('Jun 9')).toBeInTheDocument()
    expect(screen.getByText('WFMU')).toBeInTheDocument()
    expect(screen.getByText('24')).toBeInTheDocument()

    const showLink = screen.getByRole('link', { name: 'The Night Owl Show' })
    expect(showLink).toHaveAttribute('href', '/radio/wfmu/night-owl')
  })

  it('renders the viewer-local air-time block stacked under the date for windowed rows (PSY-1298)', () => {
    // air_date differs from the window's local day so this discriminates the
    // starts_at-derived date; a fallback to air_date would render Jun 8.
    render(
      <LatestPlaylistsTable
        rows={[
          makeRow({
            air_date: '2026-06-08',
            starts_at: localIso(2026, 5, 9, 15),
            ends_at: localIso(2026, 5, 9, 18),
          }),
        ]}
        isLoading={false}
        error={null}
      />
    )
    expect(screen.getByText('Jun 9')).toBeInTheDocument()
    expect(screen.getByText('3–6 PM')).toBeInTheDocument()
    expect(screen.queryByText('Jun 8')).not.toBeInTheDocument()
  })

  it('falls back to the raw air_date string when the date is unparsable (never a blank cell)', () => {
    render(
      <LatestPlaylistsTable
        rows={[makeRow({ air_date: 'not-a-date' })]}
        isLoading={false}
        error={null}
      />
    )
    expect(screen.getByText('not-a-date')).toBeInTheDocument()
  })

  it('renders NO time line for windowless rows (locked decision 4)', () => {
    render(<LatestPlaylistsTable rows={[makeRow()]} isLoading={false} error={null} />)
    const dateCell = screen.getByText('Jun 9').closest('td')
    expect(dateCell?.textContent).toBe('Jun 9')
  })

  it('links matched preview artists and renders unmatched ones as plain text', () => {
    render(
      <LatestPlaylistsTable rows={[makeRow()]} isLoading={false} error={null} />
    )

    const matched = screen.getByRole('link', { name: 'CAN' })
    expect(matched).toHaveAttribute('href', '/artists/can')

    // Unmatched artist: visible, but NOT a link (no dead links).
    expect(screen.getByText("it's all meat")).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: "it's all meat" })
    ).not.toBeInTheDocument()
  })

  it('renders a dash for rows with an empty artist preview', () => {
    render(
      <LatestPlaylistsTable
        rows={[makeRow({ artist_preview: [] })]}
        isLoading={false}
        error={null}
      />
    )
    expect(screen.getByText('—')).toBeInTheDocument()
  })
})
