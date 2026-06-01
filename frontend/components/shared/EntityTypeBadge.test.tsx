import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import {
  EntityTypeBadge,
  getEntityTypeBadgeClasses,
} from './EntityTypeBadge'

// The six core entity types and the chart token each is bound to. Mirrors the
// interleaved warm/cool map in EntityTypeBadge.tsx; the test fails loudly if
// the single-sourced map drifts off the DS palette.
const ENTITY_TOKEN: Array<[string, string]> = [
  ['artist', 'chart-1'],
  ['venue', 'chart-6'],
  ['show', 'chart-3'],
  ['release', 'chart-7'],
  ['label', 'chart-2'],
  ['festival', 'chart-8'],
]

describe('getEntityTypeBadgeClasses', () => {
  it.each(ENTITY_TOKEN)(
    'binds %s to the %s DS token (bg + text + border)',
    (type, token) => {
      const classes = getEntityTypeBadgeClasses(type)
      expect(classes).toContain(`bg-${token}/15`)
      expect(classes).toContain(`text-${token}`)
      expect(classes).toContain(`border-${token}/30`)
    }
  )

  it('never emits a raw off-palette Tailwind hue', () => {
    for (const [type] of ENTITY_TOKEN) {
      const classes = getEntityTypeBadgeClasses(type)
      expect(classes).not.toMatch(
        /\b(?:bg|text|border)-(?:blue|purple|green|amber|rose|cyan|pink|orange)-\d/
      )
    }
  })

  it('assigns a distinct token to each entity type', () => {
    const all = ENTITY_TOKEN.map(([type]) => getEntityTypeBadgeClasses(type))
    expect(new Set(all).size).toBe(ENTITY_TOKEN.length)
  })

  it('never reuses chart-4 (it equals --destructive in light mode)', () => {
    for (const [type] of ENTITY_TOKEN) {
      expect(getEntityTypeBadgeClasses(type)).not.toContain('chart-4')
    }
  })

  it('falls back to muted styling for an unknown type', () => {
    expect(getEntityTypeBadgeClasses('mixtape')).toBe(
      'bg-muted text-muted-foreground border-border'
    )
  })
})

describe('EntityTypeBadge', () => {
  it('renders the raw type as the default label', () => {
    render(<EntityTypeBadge type="artist" testId="b" />)
    expect(screen.getByTestId('b')).toHaveTextContent('artist')
  })

  it('applies the type token class to the rendered pill', () => {
    render(<EntityTypeBadge type="venue" testId="b" />)
    expect(screen.getByTestId('b').className).toContain('text-chart-6')
  })

  it('honors a label override', () => {
    render(<EntityTypeBadge type="show" label="Show" testId="b" />)
    expect(screen.getByTestId('b')).toHaveTextContent('Show')
  })

  it('merges an extra className', () => {
    render(<EntityTypeBadge type="label" className="ml-2" testId="b" />)
    expect(screen.getByTestId('b').className).toContain('ml-2')
  })
})
