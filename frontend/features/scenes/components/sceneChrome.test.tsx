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

import {
  EntityNameLink,
  EntityNameList,
  RoomList,
  SceneSectionHeading,
} from './sceneChrome'

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

  // A stored slug is data, and a `/` in it would splice a second path segment
  // onto a route the caller chose.
  it('keeps a slug to one path segment', () => {
    renderWithProviders(
      <EntityNameLink name="Sneaky" slug="../admin/users" basePath="/artists" />
    )
    expect(screen.getByRole('link', { name: 'Sneaky' })).toHaveAttribute(
      'href',
      '/artists/..%2Fadmin%2Fusers'
    )
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

// Two surfaces print dot-separated entity names and one component draws both,
// which is the arrangement these tests exist to hold.
describe('EntityNameList', () => {
  it('joins the names with middots and links each one', () => {
    const { container } = renderWithProviders(
      <EntityNameList
        items={[
          { id: 1, name: 'Gatecreeper', slug: 'gatecreeper' },
          { id: 2, name: 'Diners', slug: 'diners' },
          { id: 3, name: 'Playboy Manbaby', slug: 'playboy-manbaby' },
        ]}
        basePath="/artists"
      />
    )

    expect(container.textContent).toBe('Gatecreeper · Diners · Playboy Manbaby')
    expect(screen.getByRole('link', { name: 'Diners' })).toHaveAttribute(
      'href',
      '/artists/diners'
    )
  })

  // The separator lives OUTSIDE the anchor. Asserting the joined text would
  // pass just as well with the middot inside a link, which is the edit this
  // guards against.
  it('keeps the separator out of every link', () => {
    renderWithProviders(
      <EntityNameList
        items={[
          { id: 1, name: 'Gatecreeper', slug: 'gatecreeper' },
          { id: 2, name: 'Diners', slug: 'diners' },
        ]}
        basePath="/artists"
      />
    )
    for (const link of screen.getAllByRole('link')) {
      expect(link.textContent).not.toContain('·')
    }
  })

  it('names a slugless entity without linking it', () => {
    renderWithProviders(
      <EntityNameList
        items={[
          { id: 1, name: 'Gatecreeper', slug: '' },
          { id: 2, name: 'Diners', slug: 'diners' },
        ]}
        basePath="/artists"
      />
    )
    expect(screen.getByText('Gatecreeper')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Gatecreeper' })).not.toBeInTheDocument()
  })

  // The id-less caller is RoomList, whose items key on slug-or-name.
  it('renders every row when the caller has no ids and no slugs', () => {
    const { container } = renderWithProviders(
      <EntityNameList
        items={[{ name: 'The Rebel Lounge' }, { name: 'Valley Bar' }]}
        basePath="/venues"
      />
    )
    expect(container.textContent).toBe('The Rebel Lounge · Valley Bar')
  })

  // The key collision the doc block names: no id, no slug, same name twice.
  // Both rows still have to reach the DOM, because a name repeating is a fact
  // about the data and never a reason to drop one of them. React warns on the
  // duplicate key, which is the expected output here rather than a failure, so
  // the warning is captured instead of printed.
  it('renders both rows when two id-less, slugless items share a name', () => {
    const warned = vi.spyOn(console, 'error').mockImplementation(() => {})
    try {
      const { container } = renderWithProviders(
        <EntityNameList
          items={[{ name: 'The Lounge' }, { name: 'The Lounge' }]}
          basePath="/venues"
        />
      )
      expect(container.textContent).toBe('The Lounge · The Lounge')
    } finally {
      warned.mockRestore()
    }
  })

  // Distinct ids keep distinct keys even when name and slug are identical,
  // which is the case the `id` field exists for.
  it('keys on the id when the caller has one', () => {
    const { container } = renderWithProviders(
      <EntityNameList
        items={[
          { id: 1, name: 'Repeat', slug: '' },
          { id: 2, name: 'Repeat', slug: '' },
        ]}
        basePath="/artists"
      />
    )
    expect(container.textContent).toBe('Repeat · Repeat')
  })
})

describe('RoomList', () => {
  it('prints the rooms as one middot-separated line, each linked when it can be', () => {
    const { container } = renderWithProviders(
      <RoomList
        venues={[
          { name: 'Valley Bar', slug: 'valley-bar' },
          { name: 'Turn! Turn! Turn!' },
        ]}
      />
    )

    expect(container.textContent).toBe('Valley Bar · Turn! Turn! Turn!')
    expect(screen.getByRole('link', { name: 'Valley Bar' })).toHaveAttribute(
      'href',
      '/venues/valley-bar'
    )
    expect(screen.queryByRole('link', { name: 'Turn! Turn! Turn!' })).not.toBeInTheDocument()
  })

  it('keeps the rooms footer underline treatment', () => {
    renderWithProviders(<RoomList venues={[{ name: 'Valley Bar', slug: 'valley-bar' }]} />)
    expect(screen.getByRole('link', { name: 'Valley Bar' }).className).toContain(
      'underline-offset-4'
    )
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
