import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { EpisodeArchiveTable } from './EpisodeArchiveTable'
import type { RadioEpisodeListItem } from '@/features/radio'

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

function makeEpisode(overrides: Partial<RadioEpisodeListItem> = {}): RadioEpisodeListItem {
  return {
    id: 1,
    show_id: 1,
    title: null,
    air_date: '2026-06-02',
    air_time: null,
    duration_minutes: null,
    archive_url: null,
    starts_at: null,
    ends_at: null,
    status: 'aired',
    is_upcoming: false,
    play_count: 24,
    created_at: '2026-06-02T00:00:00Z',
    artist_preview: [],
    ...overrides,
  }
}

const defaultProps = {
  stationSlug: 'wfmu',
  showSlug: 'the-night-owl-show',
}

describe('EpisodeArchiveTable', () => {
  it('links the date cell to the episode playlist page', () => {
    render(<EpisodeArchiveTable {...defaultProps} episodes={[makeEpisode()]} />)
    const dateLink = screen.getByRole('link', { name: 'Jun 2 2026' })
    expect(dateLink).toHaveAttribute(
      'href',
      '/radio/wfmu/the-night-owl-show/2026-06-02'
    )
  })

  it('renders the episode title as a link when present', () => {
    render(
      <EpisodeArchiveTable
        {...defaultProps}
        episodes={[makeEpisode({ title: 'with guest Alabaster DePlume' })]}
      />
    )
    expect(
      screen.getByRole('link', { name: 'with guest Alabaster DePlume' })
    ).toHaveAttribute('href', '/radio/wfmu/the-night-owl-show/2026-06-02')
  })

  it('renders an em dash when the episode has no title', () => {
    render(<EpisodeArchiveTable {...defaultProps} episodes={[makeEpisode()]} />)
    expect(screen.getByText('—')).toBeInTheDocument()
  })

  it('renders matched preview artists as links and unmatched as plain text', () => {
    render(
      <EpisodeArchiveTable
        {...defaultProps}
        episodes={[
          makeEpisode({
            artist_preview: [
              { artist_name: 'CAN', artist_id: 5, artist_slug: 'can' },
              { artist_name: 'The Tweeters', artist_id: null, artist_slug: null },
            ],
          }),
        ]}
      />
    )
    expect(screen.getByRole('link', { name: 'CAN' })).toHaveAttribute(
      'href',
      '/artists/can'
    )
    const unmatched = screen.getByText('The Tweeters')
    expect(unmatched.closest('a')).toBeNull()
  })

  it('renders the track count', () => {
    render(<EpisodeArchiveTable {...defaultProps} episodes={[makeEpisode({ play_count: 41 })]} />)
    expect(screen.getByText('41')).toBeInTheDocument()
  })

  it('renders an [mp3] link when the episode has an archive_url', () => {
    render(
      <EpisodeArchiveTable
        {...defaultProps}
        episodes={[makeEpisode({ archive_url: 'https://example.com/ep.mp3' })]}
      />
    )
    const mp3 = screen.getByRole('link', {
      name: 'Listen to the Jun 2 2026 archive',
    })
    expect(mp3).toHaveAttribute('href', 'https://example.com/ep.mp3')
    expect(mp3).toHaveAttribute('target', '_blank')
  })

  it('omits the [mp3] link when there is no archive_url', () => {
    render(<EpisodeArchiveTable {...defaultProps} episodes={[makeEpisode()]} />)
    expect(screen.queryByText('[ mp3 ]')).not.toBeInTheDocument()
  })

  it('marks a currently-live episode (now inside its air window) as live, not [mp3]', () => {
    const now = Date.now()
    render(
      <EpisodeArchiveTable
        {...defaultProps}
        episodes={[
          makeEpisode({
            starts_at: new Date(now - 60 * 60 * 1000).toISOString(),
            ends_at: new Date(now + 60 * 60 * 1000).toISOString(),
            archive_url: 'https://example.com/ep.mp3',
          }),
        ]}
      />
    )
    expect(screen.getByText('live')).toBeInTheDocument()
    expect(screen.queryByText('[ mp3 ]')).not.toBeInTheDocument()
  })

  // PSY-1128 regression: an episode whose air window has ENDED is NOT live, even
  // if it aired earlier today (the old air_date-equality bug marked it live all
  // day). It shows the archive link instead.
  it('does not mark an episode whose window has ended as live', () => {
    const now = Date.now()
    render(
      <EpisodeArchiveTable
        {...defaultProps}
        episodes={[
          makeEpisode({
            starts_at: new Date(now - 3 * 60 * 60 * 1000).toISOString(),
            ends_at: new Date(now - 60 * 60 * 1000).toISOString(),
            archive_url: 'https://example.com/ep.mp3',
          }),
        ]}
      />
    )
    expect(screen.queryByText('live')).not.toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: /archive/ })
    ).toHaveAttribute('href', 'https://example.com/ep.mp3')
  })

  // PSY-1205: an upcoming (not-yet-aired) episode is labeled "upcoming", and its
  // date/title/[mp3] do NOT link — there's no playlist yet, so every link would
  // lead to an empty, aired-looking page.
  it('labels an upcoming episode and suppresses all links to its empty page', () => {
    render(
      <EpisodeArchiveTable
        {...defaultProps}
        episodes={[
          makeEpisode({
            title: 'Next Week',
            is_upcoming: true,
            // upcoming WFMU pages carry an archive_url but no playlist yet —
            // the label must win over the misleading [mp3] link
            archive_url: 'https://wfmu.org/playlists/shows/999999',
          }),
        ]}
      />
    )
    expect(screen.getByText('upcoming')).toBeInTheDocument()
    expect(screen.queryByText('[ mp3 ]')).not.toBeInTheDocument()
    expect(screen.queryByText('live')).not.toBeInTheDocument()
    // date + title render as plain text, not links to the not-yet-aired page
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
    expect(screen.getByText('Next Week')).toBeInTheDocument()
  })
})
