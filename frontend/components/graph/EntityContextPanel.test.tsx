import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { EntityContextPanel } from './EntityContextPanel'

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string
    children: React.ReactNode
    [key: string]: unknown
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

vi.mock('@/components/shared/MusicEmbed', () => ({
  MusicEmbed: ({
    bandcampAlbumUrl,
    artistName,
  }: {
    bandcampAlbumUrl?: string | null
    artistName: string
  }) => (
    <div
      data-testid="music-embed"
      data-bandcamp={bandcampAlbumUrl ?? ''}
      data-artist={artistName}
    />
  ),
}))

describe('EntityContextPanel', () => {
  it('renders the mono type-tag, name, meta, primary, facts, and Open page', () => {
    const onClose = vi.fn()
    render(
      <EntityContextPanel
        entityType="venue"
        name="Valley Bar"
        slug="valley-bar-phoenix-az"
        meta="Phoenix, AZ"
        primary={{
          kind: 'labeled',
          label: 'Next show',
          text: 'Fri Jul 24 · Diners + Glass Heels',
        }}
        facts={[
          '14 upcoming shows',
          '5 artists in this graph have played here',
        ]}
        onClose={onClose}
      />,
    )

    expect(screen.getByText('VENUE')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Valley Bar' })).toBeInTheDocument()
    expect(screen.getByText('Phoenix, AZ')).toBeInTheDocument()
    expect(screen.getByText('Next show')).toBeInTheDocument()
    expect(
      screen.getByText('Fri Jul 24 · Diners + Glass Heels'),
    ).toBeInTheDocument()
    expect(screen.getByText('14 upcoming shows')).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: '[ Open page → ]' }),
    ).toHaveAttribute('href', '/venues/valley-bar-phoenix-az')
    expect(screen.queryByText(/Center here/)).not.toBeInTheDocument()
  })

  it('dispatches emphasis primary for label/festival roster counts', () => {
    render(
      <EntityContextPanel
        entityType="label"
        name="Related Records"
        slug="related-records"
        primary={{ kind: 'emphasis', text: '6 roster artists in this graph' }}
        onClose={vi.fn()}
      />,
    )
    expect(screen.getByText('LABEL')).toBeInTheDocument()
    expect(
      screen.getByText('6 roster artists in this graph'),
    ).toBeInTheDocument()
  })

  it('dispatches embed primary for releases via MusicEmbed', () => {
    render(
      <EntityContextPanel
        entityType="release"
        name="Three"
        slug="three"
        meta="Diners · 2026"
        primary={{
          kind: 'embed',
          bandcampAlbumUrl: 'https://diners.bandcamp.com/album/three',
          title: 'Three',
        }}
        onClose={vi.fn()}
      />,
    )
    const embed = screen.getByTestId('music-embed')
    expect(embed).toHaveAttribute(
      'data-bandcamp',
      'https://diners.bandcamp.com/album/three',
    )
  })

  it('renders BILL labeled primary for shows', () => {
    render(
      <EntityContextPanel
        entityType="show"
        name="Fri Jul 24 · Valley Bar"
        slug="fri-jul-24-valley-bar"
        primary={{
          kind: 'labeled',
          label: 'Bill',
          text: 'Diners · Glass Heels · Twin Ponies',
        }}
        onClose={vi.fn()}
      />,
    )
    expect(screen.getByText('Bill')).toBeInTheDocument()
    expect(
      screen.getByText('Diners · Glass Heels · Twin Ponies'),
    ).toBeInTheDocument()
  })

  it('closes via the X button', () => {
    const onClose = vi.fn()
    render(
      <EntityContextPanel
        entityType="festival"
        name="Desert Daze"
        slug="desert-daze"
        onClose={onClose}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: /Close details/ }))
    expect(onClose).toHaveBeenCalledOnce()
  })

  it('shows a loading skeleton while enrichment is in flight', () => {
    render(
      <EntityContextPanel
        entityType="venue"
        name="Valley Bar"
        slug="valley-bar"
        isLoading
        onClose={vi.fn()}
      />,
    )
    expect(screen.getByLabelText('Loading details')).toBeInTheDocument()
  })
})
