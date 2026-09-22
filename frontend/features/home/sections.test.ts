import { describe, it, expect } from 'vitest'
import {
  HOME_SECTIONS,
  isDefaultHomeLayout,
  moveHomeSection,
  resolveCityLinkSlot,
  resolveHomeLayout,
  setHomeSectionVisibility,
  toHomeLayoutDocument,
  type HomeLayoutSectionEntry,
} from './sections'

const DEFAULT_ORDER = HOME_SECTIONS.map(section => section.id)

function document(sections: HomeLayoutSectionEntry[], version = 1) {
  return { version, sections }
}

function order(sections: { id: string }[]) {
  return sections.map(section => section.id)
}

describe('resolveHomeLayout', () => {
  it('falls back to the shipped order when nothing is stored', () => {
    for (const stored of [null, undefined, document([]), { version: 1, sections: null }]) {
      expect(order(resolveHomeLayout(stored))).toEqual(DEFAULT_ORDER)
    }
  })

  it('renders a stored order verbatim', () => {
    const stored = document([
      { id: 'radio_shows', visible: true },
      { id: 'saved_shows', visible: true },
      { id: 'nearby_shows', visible: true },
      { id: 'community_stats', visible: true },
      { id: 'city_graph', visible: true },
    ])

    expect(order(resolveHomeLayout(stored))).toEqual([
      'radio_shows',
      'saved_shows',
      'nearby_shows',
      'community_stats',
      'city_graph',
    ])
  })

  it('keeps a hidden section in its slot so re-showing restores position', () => {
    const stored = document([
      { id: 'saved_shows', visible: true },
      { id: 'nearby_shows', visible: false },
      { id: 'community_stats', visible: true },
      { id: 'city_graph', visible: true },
      { id: 'radio_shows', visible: true },
    ])

    const resolved = resolveHomeLayout(stored)
    expect(order(resolved)).toEqual(DEFAULT_ORDER)
    expect(resolved[1]).toMatchObject({ id: 'nearby_shows', visible: false })
  })

  it('drops ids the registry no longer knows', () => {
    const stored = {
      version: 1,
      sections: [
        { id: 'retired_section', visible: true },
        ...DEFAULT_ORDER.map(id => ({ id, visible: true })),
      ],
    } as unknown as Parameters<typeof resolveHomeLayout>[0]

    expect(order(resolveHomeLayout(stored))).toEqual(DEFAULT_ORDER)
  })

  it('restores an omitted section at its DEFAULT index, not at the end', () => {
    const stored = document(
      DEFAULT_ORDER.filter(id => id !== 'community_stats').map(id => ({
        id,
        visible: true,
      }))
    )

    const resolved = resolveHomeLayout(stored)
    expect(order(resolved)).toEqual(DEFAULT_ORDER)
    expect(resolved[2]).toMatchObject({ id: 'community_stats', visible: true })
  })

  it('keeps the first occurrence of a duplicated id', () => {
    const stored = document([
      { id: 'radio_shows', visible: false },
      { id: 'radio_shows', visible: true },
      ...DEFAULT_ORDER.filter(id => id !== 'radio_shows').map(id => ({
        id,
        visible: true,
      })),
    ])

    const resolved = resolveHomeLayout(stored)
    expect(resolved.filter(section => section.id === 'radio_shows')).toHaveLength(1)
    expect(resolved[0]).toMatchObject({ id: 'radio_shows', visible: false })
  })

  it('ignores a document version it does not understand', () => {
    const stored = document([{ id: 'radio_shows', visible: false }], 2)

    expect(order(resolveHomeLayout(stored))).toEqual(DEFAULT_ORDER)
    expect(resolveHomeLayout(stored).every(section => section.visible)).toBe(true)
  })
})

describe('moveHomeSection', () => {
  it('swaps with the neighbour one step at a time', () => {
    const sections = resolveHomeLayout(null)
    const moved = moveHomeSection(sections, 'community_stats', 'up')

    expect(order(moved ?? [])).toEqual([
      'saved_shows',
      'community_stats',
      'nearby_shows',
      'city_graph',
      'radio_shows',
    ])
  })

  it('answers null at either end, and for an id it does not hold', () => {
    const sections = resolveHomeLayout(null)

    expect(moveHomeSection(sections, 'saved_shows', 'up')).toBeNull()
    expect(moveHomeSection(sections, 'radio_shows', 'down')).toBeNull()
    expect(moveHomeSection([], 'radio_shows', 'up')).toBeNull()
  })

  it('moves a hidden section without changing anything else', () => {
    const hidden = setHomeSectionVisibility(
      resolveHomeLayout(null),
      'city_graph',
      false
    )
    const moved = moveHomeSection(hidden, 'city_graph', 'up')

    expect(order(moved ?? [])).toEqual([
      'saved_shows',
      'nearby_shows',
      'city_graph',
      'community_stats',
      'radio_shows',
    ])
    expect(moved?.find(s => s.id === 'city_graph')?.visible).toBe(false)
  })
})

describe('setHomeSectionVisibility', () => {
  it('never reorders', () => {
    const sections = resolveHomeLayout(null)
    const hidden = setHomeSectionVisibility(sections, 'nearby_shows', false)

    expect(order(hidden)).toEqual(DEFAULT_ORDER)
    expect(hidden[1].visible).toBe(false)
  })
})

describe('toHomeLayoutDocument', () => {
  it('writes version 1 and id/visible pairs only', () => {
    const sections = setHomeSectionVisibility(
      resolveHomeLayout(null),
      'radio_shows',
      false
    )

    expect(toHomeLayoutDocument(sections)).toEqual({
      version: 1,
      sections: [
        { id: 'saved_shows', visible: true },
        { id: 'nearby_shows', visible: true },
        { id: 'community_stats', visible: true },
        { id: 'city_graph', visible: true },
        { id: 'radio_shows', visible: false },
      ],
    })
  })

  it('round-trips through resolveHomeLayout', () => {
    const custom = moveHomeSection(
      setHomeSectionVisibility(resolveHomeLayout(null), 'city_graph', false),
      'radio_shows',
      'up'
    )
    expect(custom).not.toBeNull()

    expect(resolveHomeLayout(toHomeLayoutDocument(custom!))).toEqual(custom)
  })
})

describe('isDefaultHomeLayout', () => {
  it('is true for the shipped layout and false after any change', () => {
    const sections = resolveHomeLayout(null)

    expect(isDefaultHomeLayout(sections)).toBe(true)
    expect(
      isDefaultHomeLayout(moveHomeSection(sections, 'radio_shows', 'up') ?? [])
    ).toBe(false)
    expect(
      isDefaultHomeLayout(
        setHomeSectionVisibility(sections, 'radio_shows', false)
      )
    ).toBe(false)
  })
})

describe('resolveCityLinkSlot', () => {
  const sections = resolveHomeLayout(null)

  it('leaves the link on the nearby section while it renders', () => {
    expect(resolveCityLinkSlot(sections)).toBe('nearby')
  })

  it('hands it to the saved-shows footer when nearby is hidden', () => {
    expect(
      resolveCityLinkSlot(
        setHomeSectionVisibility(sections, 'nearby_shows', false)
      )
    ).toBe('saved')
  })

  it('hands it to the toolbar when both carriers are hidden', () => {
    const both = setHomeSectionVisibility(
      setHomeSectionVisibility(sections, 'nearby_shows', false),
      'saved_shows',
      false
    )

    expect(resolveCityLinkSlot(both)).toBe('toolbar')
  })
})
