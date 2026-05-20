import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { RadioPlayRow } from './RadioPlayRow'
import type { RadioPlay } from '../types'

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

function makePlay(overrides: Partial<RadioPlay> = {}): RadioPlay {
  return {
    id: 1,
    episode_id: 10,
    position: 3,
    artist_name: 'Gatecreeper',
    track_title: 'Sweltering Madness',
    album_title: 'Sonoran Depravation',
    label_name: 'Relapse',
    release_year: 2016,
    is_new: false,
    rotation_status: null,
    dj_comment: null,
    is_live_performance: false,
    is_request: false,
    artist_id: null,
    artist_slug: null,
    release_id: null,
    release_slug: null,
    label_id: null,
    label_slug: null,
    musicbrainz_artist_id: null,
    musicbrainz_recording_id: null,
    musicbrainz_release_id: null,
    air_timestamp: null,
    ...overrides,
  }
}

describe('RadioPlayRow', () => {
  it('renders the artist name and track title', () => {
    render(<RadioPlayRow play={makePlay()} />)
    expect(screen.getByText('Gatecreeper')).toBeInTheDocument()
    expect(screen.getByText('Sweltering Madness')).toBeInTheDocument()
  })

  it('renders album, label, and release year', () => {
    render(<RadioPlayRow play={makePlay()} />)
    expect(screen.getByText('Sonoran Depravation')).toBeInTheDocument()
    expect(screen.getByText('Relapse')).toBeInTheDocument()
    expect(screen.getByText('(2016)')).toBeInTheDocument()
  })

  it('renders the position number by default', () => {
    render(<RadioPlayRow play={makePlay({ position: 3 })} />)
    expect(screen.getByText('3')).toBeInTheDocument()
  })

  it('hides the position number when showPosition is false', () => {
    render(<RadioPlayRow play={makePlay({ position: 3 })} showPosition={false} />)
    expect(screen.queryByText('3')).not.toBeInTheDocument()
  })

  it('links the artist to its catalog page when a slug is present', () => {
    render(<RadioPlayRow play={makePlay({ artist_slug: 'gatecreeper' })} />)
    const link = screen.getByText('Gatecreeper').closest('a')
    expect(link).toHaveAttribute('href', '/artists/gatecreeper')
  })

  it('renders the artist as plain text when there is no slug', () => {
    render(<RadioPlayRow play={makePlay({ artist_slug: null })} />)
    expect(screen.getByText('Gatecreeper').closest('a')).toBeNull()
  })

  it('links the release and label to their catalog pages', () => {
    render(
      <RadioPlayRow
        play={makePlay({ release_slug: 'sonoran-depravation', label_slug: 'relapse' })}
      />
    )
    expect(screen.getByText('Sonoran Depravation').closest('a')).toHaveAttribute(
      'href',
      '/releases/sonoran-depravation'
    )
    expect(screen.getByText('Relapse').closest('a')).toHaveAttribute(
      'href',
      '/labels/relapse'
    )
  })

  it('renders a NEW badge for new plays', () => {
    render(<RadioPlayRow play={makePlay({ is_new: true })} />)
    expect(screen.getByText('NEW')).toBeInTheDocument()
  })

  it('renders a rotation-status badge with its label', () => {
    render(<RadioPlayRow play={makePlay({ rotation_status: 'heavy' })} />)
    expect(screen.getByText('Heavy Rotation')).toBeInTheDocument()
  })

  it('suppresses the rotation badge for library rotation', () => {
    render(<RadioPlayRow play={makePlay({ rotation_status: 'library' })} />)
    expect(screen.queryByText('Library')).not.toBeInTheDocument()
  })

  it('renders LIVE and REQ badges', () => {
    render(
      <RadioPlayRow play={makePlay({ is_live_performance: true, is_request: true })} />
    )
    expect(screen.getByText('LIVE')).toBeInTheDocument()
    expect(screen.getByText('REQ')).toBeInTheDocument()
  })

  it('renders a DJ comment when present', () => {
    render(<RadioPlayRow play={makePlay({ dj_comment: 'Local legends' })} />)
    expect(screen.getByText(/Local legends/)).toBeInTheDocument()
  })

  it('renders a formatted air timestamp when present', () => {
    render(
      <RadioPlayRow play={makePlay({ air_timestamp: '2026-05-01T06:32:00Z' })} />
    )
    // Exact rendered time depends on the runner TZ; assert the 12h-clock shape.
    expect(screen.getByText(/\d{1,2}:\d{2}\s?(AM|PM)/)).toBeInTheDocument()
  })

  it('does not render a separator dash when there is no track title', () => {
    render(<RadioPlayRow play={makePlay({ track_title: null })} />)
    expect(screen.queryByText('Sweltering Madness')).not.toBeInTheDocument()
  })
})
