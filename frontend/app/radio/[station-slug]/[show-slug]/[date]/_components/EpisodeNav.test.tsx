import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { EpisodeNav } from './EpisodeNav'
import type { EpisodeNeighbors, RadioEpisodeListItem } from '@/features/radio'

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

function makeEpisode(id: number, airDate: string): RadioEpisodeListItem {
  return {
    id,
    show_id: 1,
    title: null,
    air_date: airDate,
    air_time: null,
    duration_minutes: null,
    archive_url: null,
    play_count: 10,
    created_at: '2026-01-01T00:00:00Z',
    artist_preview: [],
  }
}

const SHOW_URL = '/radio/wfmu/the-night-owl-show'

describe('EpisodeNav', () => {
  it('links both neighbors when present', () => {
    const neighbors: EpisodeNeighbors = {
      older: makeEpisode(1, '2026-05-26'),
      newer: makeEpisode(3, '2026-06-09'),
    }
    render(<EpisodeNav neighbors={neighbors} showUrl={SHOW_URL} />)

    expect(
      screen.getByRole('link', { name: 'Previous episode, May 26' })
    ).toHaveAttribute('href', `${SHOW_URL}/2026-05-26`)
    expect(
      screen.getByRole('link', { name: 'Next episode, Jun 9' })
    ).toHaveAttribute('href', `${SHOW_URL}/2026-06-09`)
  })

  it('disables the newer bracket at the newest episode', () => {
    const neighbors: EpisodeNeighbors = {
      older: makeEpisode(1, '2026-05-26'),
      newer: null,
    }
    render(<EpisodeNav neighbors={neighbors} showUrl={SHOW_URL} />)

    expect(
      screen.getByRole('button', { name: 'No newer episode' })
    ).toBeDisabled()
    expect(
      screen.getByRole('link', { name: 'Previous episode, May 26' })
    ).toBeInTheDocument()
  })

  it('disables the older bracket at the oldest episode', () => {
    const neighbors: EpisodeNeighbors = {
      older: null,
      newer: makeEpisode(3, '2026-06-09'),
    }
    render(<EpisodeNav neighbors={neighbors} showUrl={SHOW_URL} />)

    expect(
      screen.getByRole('button', { name: 'No older episode' })
    ).toBeDisabled()
    expect(
      screen.getByRole('link', { name: 'Next episode, Jun 9' })
    ).toBeInTheDocument()
  })

  it('disables both brackets while neighbors are loading', () => {
    render(<EpisodeNav neighbors={undefined} showUrl={SHOW_URL} />)

    expect(screen.getByRole('button', { name: 'No older episode' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'No newer episode' })).toBeDisabled()
  })

  it('always links back to the full archive', () => {
    render(<EpisodeNav neighbors={undefined} showUrl={SHOW_URL} />)
    expect(screen.getByRole('link', { name: 'all episodes' })).toHaveAttribute(
      'href',
      SHOW_URL
    )
  })
})
