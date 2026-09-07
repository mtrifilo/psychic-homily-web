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
  entityHref,
  EntityNameLink,
  EntityNameList,
  RoomList,
  SCENE_ACCENT_LINK_CLASS,
  SceneSectionHeading,
} from './sceneChrome'

// The guard itself, tested directly, because callers whose link body is not a
// bare name reach for it instead of for EntityNameLink.
describe('entityHref', () => {
  it('builds a one-segment href from a slug', () => {
    expect(entityHref('/collections', 'phoenix-diy')).toBe(
      '/collections/phoenix-diy'
    )
  })

  it('encodes a slug that would otherwise splice a second segment', () => {
    expect(entityHref('/collections', 'a/b')).toBe('/collections/a%2Fb')
  })

  // Null means "do not link", never "link to the index": `/collections/` with
  // an empty slug resolves to the browse page rather than 404ing (PSY-1754).
  it('returns null for a missing, empty or whitespace slug', () => {
    expect(entityHref('/collections', undefined)).toBeNull()
    expect(entityHref('/collections', null)).toBeNull()
    expect(entityHref('/collections', '')).toBeNull()
    expect(entityHref('/collections', '   ')).toBeNull()
  })

  // Dot segments are the one shape encoding leaves untouched, and they reach
  // the same wrong destination the empty slug does: `/collections/..` walks
  // back up to `/collections`.
  it('returns null for a dot-segment slug', () => {
    expect(entityHref('/collections', '.')).toBeNull()
    expect(entityHref('/collections', '..')).toBeNull()
    expect(entityHref('/collections', ' .. ')).toBeNull()
  })

  // A slug that merely CONTAINS dots is a real slug and still links.
  it('links a slug that contains dots', () => {
    expect(entityHref('/artists', 'r.e.m')).toBe('/artists/r.e.m')
  })
})

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

// The dot-separated names line has one implementation, and these are its tests.
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

  // Items without an id key on slug-or-name.
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
  // about the data and never a reason to drop one of them. React's duplicate-key
  // complaint is expected here rather than a failure, so it is captured to keep
  // it out of the suite's output; the test below asserts its absence where the
  // key IS unique.
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

  // The `id` branch is what makes the previous case collision-free, and the
  // only observable difference is React's duplicate-key complaint, so that is
  // what this asserts the absence of.
  it('keys on the id, so identical names and slugs do not collide', () => {
    const complained = vi.spyOn(console, 'error').mockImplementation(() => {})
    try {
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
      expect(complained).not.toHaveBeenCalled()
    } finally {
      complained.mockRestore()
    }
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

// Two components render this constant, and neither asserts a class of its own.
// Without this, dropping the focus ring or the accent tone changes both
// rendered surfaces and fails nothing.
describe('SCENE_ACCENT_LINK_CLASS', () => {
  it('carries a visible focus indicator', () => {
    expect(SCENE_ACCENT_LINK_CLASS).toContain('focus-visible:outline-2')
    expect(SCENE_ACCENT_LINK_CLASS).toContain('focus-visible:outline-ring')
  })

  it('is the accent tone in the section headings type', () => {
    expect(SCENE_ACCENT_LINK_CLASS).toContain('text-primary')
    expect(SCENE_ACCENT_LINK_CLASS).toContain(
      'font-mono text-[11px] uppercase tracking-widest'
    )
  })

  // Hover must not recolour: against the primary base that reads as receding.
  it('signals hover by underlining', () => {
    expect(SCENE_ACCENT_LINK_CLASS).toContain('hover:underline')
    expect(SCENE_ACCENT_LINK_CLASS).not.toContain('hover:text-')
  })
})
