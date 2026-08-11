import { describe, it, expect, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/utils'

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string
    children: React.ReactNode
  }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}))

import { EntityNameLink, RoomLink, SceneSectionHeading } from './sceneChrome'

// The nullable-slug href guard is a rule this codebase has learned the hard way
// (PSY-1754) and now has ONE implementation. These are its tests.
describe('EntityNameLink', () => {
  it('links to the entity page when there is a slug', () => {
    renderWithProviders(
      <EntityNameLink name="Gatecreeper" slug="gatecreeper" basePath="/artists" />
    )
    expect(screen.getByRole('link', { name: 'Gatecreeper' })).toHaveAttribute(
      'href',
      '/artists/gatecreeper'
    )
  })

  // `/artists/` and `/venues/` with an empty slug resolve to the INDEX page
  // rather than 404ing, so an unguarded href silently sends the reader to a
  // directory that never mentions what they clicked.
  it.each([
    ['an empty string', ''],
    ['whitespace', '   '],
    ['null', null],
    ['undefined', undefined],
  ])('names but does not link an entity whose slug is %s', (_label, slug) => {
    renderWithProviders(
      <EntityNameLink name="Gatecreeper" slug={slug} basePath="/artists" />
    )
    expect(screen.getByText('Gatecreeper')).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  it('trims a padded slug rather than minting a second URL for one entity', () => {
    renderWithProviders(
      <EntityNameLink name="Valley Bar" slug=" valley-bar " basePath="/venues" />
    )
    expect(screen.getByRole('link', { name: 'Valley Bar' })).toHaveAttribute(
      'href',
      '/venues/valley-bar'
    )
  })
})

describe('RoomLink', () => {
  // The day and week pages' rooms footer rides on the same guard. Its own
  // styling is deliberately different, which is the whole reason the guard is
  // parameterised rather than duplicated.
  it('keeps its own underline treatment while delegating the guard', () => {
    renderWithProviders(<RoomLink venue={{ name: 'Valley Bar', slug: 'valley-bar' }} />)
    const link = screen.getByRole('link', { name: 'Valley Bar' })
    expect(link).toHaveAttribute('href', '/venues/valley-bar')
    expect(link.className).toContain('underline-offset-4')
  })

  it('names a slugless room unlinked', () => {
    renderWithProviders(<RoomLink venue={{ name: 'Turn! Turn! Turn!' }} />)
    expect(screen.getByText('Turn! Turn! Turn!')).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })
})

describe('SceneSectionHeading', () => {
  it('reads as one sentence with the qualifier inside the heading', () => {
    renderWithProviders(<SceneSectionHeading title="Rooms / 12 tracked" note="alphabetical" />)
    expect(
      screen.getByRole('heading', { name: 'Rooms / 12 tracked · alphabetical' })
    ).toBeInTheDocument()
  })

  // A `0` note has to survive: these headings count things, and `note={0}`
  // going missing is the falsy-render bug this asserts against.
  it('renders a zero note rather than dropping it', () => {
    renderWithProviders(<SceneSectionHeading title="Bands / based in London" note={0} />)
    expect(
      screen.getByRole('heading', { name: 'Bands / based in London · 0' })
    ).toBeInTheDocument()
  })

  it('renders no middot when there is no note', () => {
    renderWithProviders(<SceneSectionHeading title="Rooms / none tracked yet" />)
    expect(
      screen.getByRole('heading', { name: 'Rooms / none tracked yet' })
    ).toBeInTheDocument()
  })

  it('places the action beside the heading', () => {
    renderWithProviders(
      <SceneSectionHeading title="Bands / based in Phoenix" note={17} action={<button>Show all</button>} />
    )
    expect(screen.getByRole('button', { name: 'Show all' })).toBeInTheDocument()
  })
})
