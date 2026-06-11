import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { RadioEpisodeRow } from './RadioEpisodeRow'
import type { RadioEpisodeListItem } from '../types'

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

function makeEpisode(overrides: Partial<RadioEpisodeListItem> = {}): RadioEpisodeListItem {
  return {
    id: 1,
    show_id: 10,
    title: 'Episode Title',
    air_date: '2026-05-01',
    air_time: '21:30:00',
    duration_minutes: 120,
    archive_url: null,
    play_count: 18,
    created_at: '2026-05-01T00:00:00Z',
    artist_preview: [],
    ...overrides,
  }
}

describe('RadioEpisodeRow', () => {
  it('links to the episode detail page keyed by air_date', () => {
    render(
      <RadioEpisodeRow episode={makeEpisode()} stationSlug="wfmu" showSlug="drummer" />
    )
    const link = screen.getByRole('link')
    expect(link).toHaveAttribute('href', '/radio/wfmu/drummer/2026-05-01')
  })

  it('renders the episode title when present', () => {
    render(
      <RadioEpisodeRow episode={makeEpisode()} stationSlug="wfmu" showSlug="drummer" />
    )
    expect(screen.getByText('Episode Title')).toBeInTheDocument()
  })

  it('formats a 24h air_time into a 12h clock time (PM)', () => {
    render(
      <RadioEpisodeRow
        episode={makeEpisode({ air_time: '21:30:00' })}
        stationSlug="wfmu"
        showSlug="drummer"
      />
    )
    expect(screen.getByText(/9:30 PM/)).toBeInTheDocument()
  })

  it('formats midnight as 12:00 AM', () => {
    render(
      <RadioEpisodeRow
        episode={makeEpisode({ air_time: '00:00:00' })}
        stationSlug="wfmu"
        showSlug="drummer"
      />
    )
    expect(screen.getByText(/12:00 AM/)).toBeInTheDocument()
  })

  it('renders the formatted air date', () => {
    render(
      <RadioEpisodeRow episode={makeEpisode()} stationSlug="wfmu" showSlug="drummer" />
    )
    // date-only constructor parses in local time → no TZ drift.
    expect(screen.getByText(/May 1, 2026/)).toBeInTheDocument()
  })

  it('renders the track/play count', () => {
    render(
      <RadioEpisodeRow
        episode={makeEpisode({ play_count: 18 })}
        stationSlug="wfmu"
        showSlug="drummer"
      />
    )
    expect(screen.getByText(/18 tracks/)).toBeInTheDocument()
  })

  it('renders the duration in minutes', () => {
    render(
      <RadioEpisodeRow
        episode={makeEpisode({ duration_minutes: 120 })}
        stationSlug="wfmu"
        showSlug="drummer"
      />
    )
    expect(screen.getByText(/120 min/)).toBeInTheDocument()
  })

  it('falls back to the date as the primary line when there is no title', () => {
    render(
      <RadioEpisodeRow
        episode={makeEpisode({ title: null })}
        stationSlug="wfmu"
        showSlug="drummer"
      />
    )
    // Title-less episodes promote the date into the bold primary slot.
    expect(screen.getByText(/May 1, 2026/)).toBeInTheDocument()
    expect(screen.queryByText('Episode Title')).not.toBeInTheDocument()
  })
})
