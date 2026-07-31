import React from 'react'
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import {
  AI_POLICY_COPY,
  COPY_PENDING_PLACEHOLDER,
  isAiPolicyCopyPending,
  type AiPolicyCopy,
} from './content'

vi.mock('next/link', () => ({
  default: ({
    href,
    children,
    ...props
  }: {
    href: string
    children: React.ReactNode
  }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}))

type ContentModule = typeof import('./content')
type PageModule = typeof import('./page')

/**
 * Load page.tsx against a given copy object.
 *
 * `page.tsx` reads the copy at module scope — metadata has to be a static
 * export — so each regime needs a fresh module graph.
 *
 * Both regimes are driven by FIXTURES, never by the shipped copy, so that
 * writing the real policy text is a one-file change: nothing in here has to be
 * edited when `AI_POLICY_COPY` stops being empty. The one assertion that does
 * read the shipped copy checks a relationship that holds in either state.
 */
async function loadPage(copy: AiPolicyCopy): Promise<PageModule> {
  vi.resetModules()
  const actual = await vi.importActual<ContentModule>('./content')
  vi.doMock('./content', () => ({ ...actual, AI_POLICY_COPY: copy }))
  return import('./page')
}

/** Mirrors the scaffold: every slot unwritten. */
const PENDING_COPY: AiPolicyCopy = {
  description: null,
  intro: null,
  lastUpdated: null,
  sections: [
    { id: 'first', heading: 'First disclosure', body: null },
    { id: 'second', heading: 'Second disclosure', body: null },
  ],
}

// Fixture prose only. It stands in for the owner's copy in the published-regime
// tests and must never become the real policy text.
const WRITTEN_COPY: AiPolicyCopy = {
  description: 'FIXTURE description.',
  intro: ['FIXTURE intro paragraph.'],
  lastUpdated: 'July 31, 2026',
  sections: [
    { id: 'first', heading: 'First disclosure', body: ['FIXTURE first.'] },
    {
      id: 'second',
      heading: 'Second disclosure',
      body: ['FIXTURE second, a.', 'FIXTURE second, b.'],
    },
  ],
}

describe('AI policy page (/ai-policy) — copy pending', () => {
  // The whole risk of this change is a placeholder shipping as if it were
  // policy. Pin that the marker is loud, machine-detectable, and present for
  // every unwritten slot rather than only the first.
  it('marks every unwritten slot with the obvious placeholder', async () => {
    const { default: Page } = await loadPage(PENDING_COPY)
    render(<Page />)

    // Both sections plus the intro.
    expect(screen.getAllByText(COPY_PENDING_PLACEHOLDER)).toHaveLength(3)
  })

  it('renders a draft banner that disclaims the page as policy', async () => {
    const { default: Page } = await loadPage(PENDING_COPY)
    render(<Page />)

    const banner = screen.getByRole('alert')
    expect(banner).toHaveTextContent(/not real policy copy/i)
    expect(banner).toHaveTextContent(/noindex/i)
  })

  it('holds the page out of search: noindex, nofollow, no description', async () => {
    const { metadata } = await loadPage(PENDING_COPY)

    expect(metadata.robots).toEqual({ index: false, follow: false })
    expect(metadata.description).toBeUndefined()
  })

  it('omits the last-updated line while there is nothing to date', async () => {
    const { default: Page } = await loadPage(PENDING_COPY)
    render(<Page />)

    expect(screen.queryByText(/Last Updated/)).not.toBeInTheDocument()
  })
})

describe('AI policy page (/ai-policy) — copy written', () => {
  it('drops the banner and every placeholder once the copy lands', async () => {
    const { default: Page } = await loadPage(WRITTEN_COPY)
    render(<Page />)

    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.queryByText(COPY_PENDING_PLACEHOLDER)).not.toBeInTheDocument()
    expect(screen.queryByText(/POLICY COPY PENDING/i)).not.toBeInTheDocument()
  })

  it('renders the owner copy as paragraphs, with the last-updated line', async () => {
    const { default: Page } = await loadPage(WRITTEN_COPY)
    render(<Page />)

    expect(screen.getByText('FIXTURE intro paragraph.')).toBeInTheDocument()
    expect(screen.getByText('FIXTURE first.')).toBeInTheDocument()
    expect(screen.getByText('FIXTURE second, a.')).toBeInTheDocument()
    expect(screen.getByText('FIXTURE second, b.')).toBeInTheDocument()
    expect(screen.getByText(/Last Updated: July 31, 2026/)).toBeInTheDocument()
  })

  it('becomes indexable and gains a description', async () => {
    const { metadata } = await loadPage(WRITTEN_COPY)

    expect(metadata.robots).toBeUndefined()
    expect(metadata.description).toBe('FIXTURE description.')
  })

  it('gives each section a stable anchor so it can be linked and quoted', async () => {
    const { default: Page } = await loadPage(WRITTEN_COPY)
    const { container } = render(<Page />)

    expect(container.querySelector('#first')).not.toBeNull()
    expect(container.querySelector('#second')).not.toBeNull()
  })
})

describe('AI policy page (/ai-policy) — the shipped copy', () => {
  it('renders every section heading it declares', async () => {
    const { default: Page } = await loadPage(AI_POLICY_COPY)
    render(<Page />)

    for (const section of AI_POLICY_COPY.sections) {
      expect(
        screen.getByRole('heading', { name: section.heading })
      ).toBeInTheDocument()
    }
  })

  // The invariant, not the current state: whatever the shipped copy says, the
  // crawl directive and the visible banner must agree with each other. Holds
  // before and after the owner writes the prose, so publishing needs no edit
  // here.
  it('keeps the crawl directive and the draft banner in agreement', async () => {
    const pending = isAiPolicyCopyPending(AI_POLICY_COPY)
    const { default: Page, metadata } = await loadPage(AI_POLICY_COPY)
    render(<Page />)

    expect(metadata.robots === undefined).toBe(!pending)
    expect(screen.queryByRole('alert') === null).toBe(!pending)
  })

  it('still declares its canonical URL', async () => {
    const { metadata } = await loadPage(AI_POLICY_COPY)

    expect(metadata.alternates?.canonical).toBe(
      'https://psychichomily.com/ai-policy'
    )
  })

  // The root layout applies `template: '%s | Psychic Homily'`. /terms and
  // /privacy hardcode the suffix as well and end up doubling it.
  it('leaves the site suffix to the root layout title template', async () => {
    const { metadata } = await loadPage(AI_POLICY_COPY)

    expect(metadata.title).toBe('AI Policy')
  })
})
