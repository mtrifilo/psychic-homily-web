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
    play_count: 24,
    created_at: '2026-06-02T00:00:00Z',
    artist_preview: [],
    ...overrides,
  }
}

function localToday(): string {
  const now = new Date()
  return [
    now.getFullYear(),
    String(now.getMonth() + 1).padStart(2, '0'),
    String(now.getDate()).padStart(2, '0'),
  ].join('-')
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

  it("marks today's episode as live instead of showing an [mp3] link", () => {
    render(
      <EpisodeArchiveTable
        {...defaultProps}
        episodes={[
          makeEpisode({
            air_date: localToday(),
            archive_url: 'https://example.com/ep.mp3',
          }),
        ]}
      />
    )
    expect(screen.getByText('live')).toBeInTheDocument()
    expect(screen.queryByText('[ mp3 ]')).not.toBeInTheDocument()
  })
})
