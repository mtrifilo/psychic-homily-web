import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { COPY_PENDING_PLACEHOLDER, type AiPolicyCopy } from './content'

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
 * `page.tsx` reads the copy at module scope (metadata has to be a static
 * export), so both regimes need a fresh module graph. Passing no argument loads
 * the real, shipped copy — which is the state that must never reach production
 * unnoticed, and therefore the state most worth pinning.
 */
async function loadPage(copy?: AiPolicyCopy): Promise<PageModule> {
  vi.resetModules()
  const actual = await vi.importActual<ContentModule>('./content')
  vi.doMock('./content', () => ({
    ...actual,
    AI_POLICY_COPY: copy ?? actual.AI_POLICY_COPY,
  }))
  return import('./page')
}

// Fixture prose only — stands in for the owner's copy in the published-regime
// tests. It is not, and must never become, the real policy text.
const WRITTEN_COPY: AiPolicyCopy = {
  description: 'FIXTURE description.',
  intro: ['FIXTURE intro paragraph.'],
  lastUpdated: 'July 31, 2026',
  sections: [
    {
      id: 'no-ai-generated-content',
      heading: 'No AI-generated music, artwork, or writing',
      body: ['FIXTURE first section.'],
    },
    {
      id: 'what-ai-is-used-for',
      heading: 'What AI is used for',
      body: ['FIXTURE second section.'],
    },
  ],
}

describe('AI policy page (/ai-policy) — copy pending', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('scaffolds a heading for each of the three required disclosures', async () => {
    const { default: Page } = await loadPage()
    render(<Page />)

    expect(
      screen.getByRole('heading', {
        name: /No AI-generated music, artwork, or writing/i,
      })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('heading', { name: /What AI is used for/i })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('heading', { name: /Human verification/i })
    ).toBeInTheDocument()
  })

  // The whole risk of this change is a placeholder shipping as if it were
  // policy. Pin that it is loud, machine-detectable, and present for every
  // unwritten slot rather than only the first.
  it('marks every unwritten slot with the obvious placeholder', async () => {
    const { default: Page } = await loadPage()
    render(<Page />)

    // Three sections plus the intro.
    expect(screen.getAllByText(COPY_PENDING_PLACEHOLDER)).toHaveLength(4)
  })

  it('renders a draft banner that disclaims the page as policy', async () => {
    const { default: Page } = await loadPage()
    render(<Page />)

    const banner = screen.getByRole('alert')
    expect(banner).toHaveTextContent(/not real policy copy/i)
    expect(banner).toHaveTextContent(/noindex/i)
  })

  it('holds the page out of search: noindex, nofollow, no description', async () => {
    const { metadata } = await loadPage()

    expect(metadata.robots).toEqual({ index: false, follow: false })
    expect(metadata.description).toBeUndefined()
  })

  it('still declares its canonical URL', async () => {
    const { metadata } = await loadPage()

    expect(metadata.alternates?.canonical).toBe(
      'https://psychichomily.com/ai-policy'
    )
  })

  // The root layout applies `template: '%s | Psychic Homily'`. /terms and
  // /privacy hardcode the suffix as well and end up doubling it.
  it('leaves the site suffix to the root layout title template', async () => {
    const { metadata } = await loadPage()

    expect(metadata.title).toBe('AI Policy')
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
    expect(screen.getByText('FIXTURE first section.')).toBeInTheDocument()
    expect(screen.getByText('FIXTURE second section.')).toBeInTheDocument()
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

    expect(container.querySelector('#what-ai-is-used-for')).not.toBeNull()
  })
})
