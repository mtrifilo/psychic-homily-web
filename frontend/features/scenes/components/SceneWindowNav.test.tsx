import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'

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

import { SceneWindowNav } from './SceneWindowNav'

/** An element's text with its `sr-only` children removed. */
function visibleText(el: Element): string {
  const clone = el.cloneNode(true) as HTMLElement
  clone.querySelectorAll('.sr-only').forEach(node => node.remove())
  return (clone.textContent ?? '').trim()
}

/**
 * The one nav, in each state a route puts it in.
 *
 * The views' own suites assert what each route PASSES; this file asserts what
 * the component does with it, so the shared contract has one home rather than a
 * copy per surface — which is the failure this component exists to end.
 */
describe('SceneWindowNav', () => {
  // VISIBLE text only: each link carries a screen-reader-only direction word
  // that would otherwise show up in every expected label here, and what these
  // assertions are about is what the row draws.
  const labels = (nav: HTMLElement): string[] =>
    [...nav.children].map(el => visibleText(el))

  it('draws every window as a link and marks none current on the root', () => {
    render(<SceneWindowNav slug="phoenix-az" />)

    const nav = screen.getByRole('navigation', { name: 'Show windows' })
    expect(within(nav).getAllByRole('link').map(a => a.getAttribute('href'))).toEqual([
      '/scenes/phoenix-az/tonight',
      '/scenes/phoenix-az/this-weekend',
      '/scenes/phoenix-az/week',
      '/scenes/phoenix-az/next-4-weeks',
    ])
    expect(nav.querySelector('[aria-current]')).toBeNull()
  })

  // The active window is not a link: it is where the reader already is, and a
  // link to the page you are on is a dead affordance.
  it.each([
    ['tonight', 'Tonight'],
    ['this-weekend', 'This weekend'],
    ['this-week', 'This week'],
    ['next-4-weeks', 'Next 4 weeks'],
  ] as const)('marks %s current, leaving the other three as links', (key, label) => {
    render(<SceneWindowNav slug="phoenix-az" current={key} />)

    const nav = screen.getByRole('navigation', { name: 'Show windows' })
    expect(nav.querySelector('[aria-current="page"]')).toHaveTextContent(label)
    expect(within(nav).queryByRole('link', { name: label })).toBeNull()
    expect(within(nav).getAllByRole('link')).toHaveLength(3)
  })

  it('renders no adjacent row when the route has no neighbours to offer', () => {
    render(<SceneWindowNav slug="phoenix-az" current="this-weekend" />)
    expect(screen.queryByRole('navigation', { name: 'Adjacent days' })).toBeNull()
    expect(screen.queryByRole('navigation', { name: 'Adjacent weeks' })).toBeNull()
  })

  // The arrow is for the eye and `rel` is for a crawler; neither reaches a
  // screen reader as a word, so the accessible name carries the direction.
  it('names the direction for a screen reader on each link', () => {
    render(
      <SceneWindowNav
        slug="phoenix-az"
        steps={{
          label: 'Adjacent days',
          prev: { label: 'Sep 13', href: '/scenes/phoenix-az/2026-09-13' },
          next: { label: 'Sep 15', href: '/scenes/phoenix-az/2026-09-15' },
        }}
      />
    )

    const nav = screen.getByRole('navigation', { name: 'Adjacent days' })
    expect(within(nav).getByRole('link', { name: /^Previous:/ })).toHaveAttribute(
      'href',
      '/scenes/phoenix-az/2026-09-13'
    )
    expect(within(nav).getByRole('link', { name: /^Next:/ })).toHaveAttribute(
      'href',
      '/scenes/phoenix-az/2026-09-15'
    )
  })

  it('carries an arrow on each direction it can link', () => {
    render(
      <SceneWindowNav
        slug="phoenix-az"
        steps={{
          label: 'Adjacent days',
          prev: { label: 'Sep 13', href: '/scenes/phoenix-az/2026-09-13' },
          next: { label: 'Sep 15', href: '/scenes/phoenix-az/2026-09-15' },
        }}
      />
    )

    const nav = screen.getByRole('navigation', { name: 'Adjacent days' })
    expect(labels(nav)).toEqual(['← Sep 13', 'Sep 15 →'])
    expect(within(nav).getByRole('link', { name: /← Sep 13/ })).toHaveAttribute(
      'rel',
      'prev'
    )
    expect(within(nav).getByRole('link', { name: /Sep 15 →/ })).toHaveAttribute(
      'rel',
      'next'
    )
  })

  // The servable edge. A link to a page this site 404s is a worse answer than
  // the muted statement, and the arrow goes with the link: an arrow pointing at
  // nothing draws the eye toward a destination that does not exist.
  it('mutes a direction with nothing behind it, without an arrow or a link', () => {
    render(
      <SceneWindowNav
        slug="phoenix-az"
        steps={{
          label: 'Adjacent days',
          prev: { label: 'Oct 11', href: '/scenes/phoenix-az/2026-10-11' },
        }}
      />
    )

    const nav = screen.getByRole('navigation', { name: 'Adjacent days' })
    expect(labels(nav)).toEqual(['← Oct 11', 'End of listings'])
    expect(within(nav).getAllByRole('link')).toHaveLength(1)
    expect(within(nav).queryByRole('link', { name: /End of listings/ })).toBeNull()
  })

  // The row still renders BOTH directions when neither is servable, so the page
  // says where the listings end rather than dropping the row silently.
  it('names both edges when neither direction is servable', () => {
    render(<SceneWindowNav slug="phoenix-az" steps={{ label: 'Adjacent days' }} />)

    const nav = screen.getByRole('navigation', { name: 'Adjacent days' })
    expect(labels(nav)).toEqual(['Start of listings', 'End of listings'])
    expect(within(nav).queryAllByRole('link')).toEqual([])
  })
})

/**
 * The point of the ticket, as an assertion.
 *
 * Six surfaces drew this strip from three definitions, and the copies had
 * already drifted apart. A grep is the only check that fails when a seventh
 * copy appears rather than when an existing one changes.
 */
describe('the window nav has exactly one definition', () => {
  const componentsDir = import.meta.dirname

  // Swept rather than listed: a listed guard passes for the fifth surface that
  // grows a strip of its own, which is the regression it exists to catch.
  const sources = readdirSync(componentsDir)
    .filter(name => name.endsWith('.tsx') && !name.endsWith('.test.tsx'))
    .filter(name => name !== 'SceneWindowNav.tsx')

  it.each(sources)('%s declares no window strip of its own', file => {
    const source = readFileSync(join(componentsDir, file), 'utf8')

    expect(source).not.toContain('function SceneWindowNav')
    expect(source).not.toContain('aria-label="Show windows"')
  })

  it.each([
    'SceneCalendar.tsx',
    'SceneWeekView.tsx',
    'SceneDayView.tsx',
    'SceneWindowView.tsx',
  ])('%s renders the shared component', file => {
    expect(readFileSync(join(componentsDir, file), 'utf8')).toContain(
      "from './SceneWindowNav'"
    )
  })
})
