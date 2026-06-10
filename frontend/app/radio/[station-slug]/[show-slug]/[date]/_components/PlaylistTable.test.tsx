import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { PlaylistTable } from './PlaylistTable'
import type { RadioPlay } from '@/features/radio'

vi.mock('next/link', () => ({
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode; [key: string]: unknown }) => (
    <a href={href} {...props}>{children}</a>
  ),
}))

function makePlay(overrides: Partial<RadioPlay> = {}): RadioPlay {
  return {
    id: 1,
    episode_id: 10,
    position: 1,
    artist_name: 'CAN',
    track_title: 'Mother Sky',
    album_title: 'Soundtracks',
    label_name: 'United Artists',
    release_year: 1970,
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

describe('PlaylistTable', () => {
  it('renders a matched artist as a link with the filled dot', () => {
    render(
      <PlaylistTable
        plays={[makePlay({ artist_id: 5, artist_slug: 'can' })]}
      />
    )
    expect(screen.getByRole('link', { name: 'CAN' })).toHaveAttribute(
      'href',
      '/artists/can'
    )
    // One ● in the row + one in the legend
    expect(screen.getAllByText('●')).toHaveLength(2)
  })

  it('renders an unmatched artist as plain text with the open dot', () => {
    render(<PlaylistTable plays={[makePlay({ artist_name: 'The Tweeters' })]} />)
    const artist = screen.getByText('The Tweeters')
    expect(artist.closest('a')).toBeNull()
    // One ○ in the row + one in the legend
    expect(screen.getAllByText('○')).toHaveLength(2)
  })

  it('renders track, album, label, and year', () => {
    render(<PlaylistTable plays={[makePlay()]} />)
    expect(screen.getByText('Mother Sky')).toBeInTheDocument()
    expect(screen.getByText('Soundtracks')).toBeInTheDocument()
    expect(screen.getByText('United Artists')).toBeInTheDocument()
    expect(screen.getByText('1970')).toBeInTheDocument()
  })

  it('links the label when label_slug is present', () => {
    render(
      <PlaylistTable
        plays={[makePlay({ label_id: 3, label_slug: 'united-artists' })]}
      />
    )
    expect(screen.getByRole('link', { name: 'United Artists' })).toHaveAttribute(
      'href',
      '/labels/united-artists'
    )
  })

  it('renders the TIME cell from air_timestamp and leaves it blank when null', () => {
    render(
      <PlaylistTable
        plays={[
          makePlay({ id: 1, air_timestamp: '2026-06-02T21:02:00' }),
          makePlay({ id: 2, artist_name: 'Neu!', air_timestamp: null }),
        ]}
      />
    )
    expect(screen.getByText('9:02 PM')).toBeInTheDocument()
    // Only one populated TIME cell across both rows
    const timeCells = document.querySelectorAll('tbody td:first-child')
    expect(timeCells).toHaveLength(2)
    expect(timeCells[1].textContent).toBe('')
  })

  it('keeps rows in playlist order', () => {
    render(
      <PlaylistTable
        plays={[
          makePlay({ id: 1, position: 1, artist_name: 'CAN' }),
          makePlay({ id: 2, position: 2, artist_name: 'Neu!' }),
          makePlay({ id: 3, position: 3, artist_name: 'Harmonia' }),
        ]}
      />
    )
    const rows = screen.getAllByRole('row').slice(1) // skip thead
    expect(rows[0]).toHaveTextContent('CAN')
    expect(rows[1]).toHaveTextContent('Neu!')
    expect(rows[2]).toHaveTextContent('Harmonia')
  })

  it('renders LIVE, NEW, rotation, and REQ badges in the NOTES column', () => {
    render(
      <PlaylistTable
        plays={[
          makePlay({
            is_live_performance: true,
            is_new: true,
            rotation_status: 'recommended_new',
            is_request: true,
          }),
        ]}
      />
    )
    expect(screen.getByText('LIVE')).toBeInTheDocument()
    expect(screen.getByText('NEW')).toBeInTheDocument()
    expect(screen.getByText('REC NEW')).toBeInTheDocument()
    expect(screen.getByText('REQ')).toBeInTheDocument()
  })

  it('does not render a rotation tag for library rotation', () => {
    render(<PlaylistTable plays={[makePlay({ rotation_status: 'library' })]} />)
    expect(screen.queryByText('LIBRARY')).not.toBeInTheDocument()
  })

  it('renders a dj_comment as an indented sub-row under its track', () => {
    render(
      <PlaylistTable
        plays={[makePlay({ dj_comment: 'recorded in Forst — RIP Michael Rother' })]}
      />
    )
    const comment = screen.getByText('recorded in Forst — RIP Michael Rother')
    expect(comment).toBeInTheDocument()
    // The comment lives in its own row, not the track row
    const commentRow = comment.closest('tr')
    expect(commentRow).not.toBeNull()
    expect(commentRow).not.toHaveTextContent('Mother Sky')
  })

  it('does not render a comment sub-row when there is no dj_comment', () => {
    render(<PlaylistTable plays={[makePlay()]} />)
    // 1 header row + 1 track row only
    expect(screen.getAllByRole('row')).toHaveLength(2)
  })

  it('renders the matched/unmatched legend without a suggest-a-match action', () => {
    render(<PlaylistTable plays={[makePlay()]} />)
    expect(screen.getByText('linked to artist page')).toBeInTheDocument()
    expect(screen.getByText('not matched yet')).toBeInTheDocument()
    expect(screen.queryByText(/suggest a match/i)).not.toBeInTheDocument()
  })
})
