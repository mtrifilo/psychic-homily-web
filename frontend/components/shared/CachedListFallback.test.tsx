import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { renderToString } from 'react-dom/server'
import { hydrateRoot } from 'react-dom/client'
import { act, cleanup, screen } from '@testing-library/react'
import {
  QueryClient,
  QueryClientProvider,
  hashKey,
  useQuery,
} from '@tanstack/react-query'
import {
  entriesFor,
  renderOnNavigation,
  warmClient,
} from '@/test/firstScreenRender'

const nav = vi.hoisted(() => ({ params: new URLSearchParams() }))
vi.mock('next/navigation', () => ({
  useSearchParams: () => nav.params,
}))

import { CachedListFallback } from './CachedListFallback'

const ROWS_KEY = ['things', 'list', undefined] as const
const COUNTS_KEY = ['things', 'counts'] as const
const FACET_KEY = ['things', 'facet'] as const
const SEEDED = [ROWS_KEY, COUNTS_KEY] as const
const DEFEATING = ['status'] as const

const never = () => new Promise<string>(() => {})

/** A list that reads two seeded keys and one facet the server never seeds. */
function ThingList() {
  const rows = useQuery({ queryKey: ROWS_KEY, queryFn: never })
  useQuery({ queryKey: COUNTS_KEY, queryFn: never })
  useQuery({ queryKey: FACET_KEY, queryFn: never })
  return <p data-testid="list">{rows.data ? String(rows.data) : 'list loading'}</p>
}

const fallback = (
  <CachedListFallback
    seededKeys={SEEDED}
    defeatingParams={DEFEATING}
    loader={<p data-testid="loader">loader</p>}
  >
    <ThingList />
  </CachedListFallback>
)

const warm = () =>
  warmClient([
    [ROWS_KEY, 'cached rows'],
    [COUNTS_KEY, 3],
  ])

describe('CachedListFallback', () => {
  beforeEach(() => {
    nav.params = new URLSearchParams()
  })

  it('server-renders the loader, even over a warm cache', () => {
    const client = warm()
    const html = renderToString(
      <QueryClientProvider client={client}>{fallback}</QueryClientProvider>
    )

    expect(html).toContain('data-testid="loader"')
    expect(html).not.toContain('cached rows')
    expect(entriesFor(client, [FACET_KEY])).toEqual([undefined])
  })

  it('on a navigation with a warm cache, renders the list from it', () => {
    renderOnNavigation(fallback, warm())

    expect(screen.getByTestId('list')).toHaveTextContent('cached rows')
    expect(screen.queryByTestId('loader')).not.toBeInTheDocument()
  })

  // The seed replaces no existing entry, so the list may only run over seeded
  // keys that already exist. What it may create is everything else.
  it('creates no seeded entry; the only entry it creates is an unseeded one', () => {
    const client = warm()
    const before = entriesFor(client, SEEDED)
    const hashesBefore = client.getQueryCache().getAll().map(q => q.queryHash)

    renderOnNavigation(fallback, client)

    expect(entriesFor(client, SEEDED)).toEqual(before)
    const created = client
      .getQueryCache()
      .getAll()
      .map(q => q.queryHash)
      .filter(hash => !hashesBefore.includes(hash))
    expect(created).toEqual([hashKey(FACET_KEY)])
  })

  it('on a navigation with a cold cache, renders the loader and creates nothing', () => {
    const client = new QueryClient()
    renderOnNavigation(fallback, client)

    expect(screen.getByTestId('loader')).toBeInTheDocument()
    expect(screen.queryByTestId('list')).not.toBeInTheDocument()
    expect(client.getQueryCache().getAll()).toEqual([])
  })

  it('renders the loader while any seeded key is missing', () => {
    const client = warmClient([[ROWS_KEY, 'cached rows']])
    renderOnNavigation(fallback, client)

    expect(screen.getByTestId('loader')).toBeInTheDocument()
    expect(entriesFor(client, [COUNTS_KEY])).toEqual([undefined])
  })

  // Off the first screen the server seeds nothing, so the list fetches for
  // itself here exactly as it would inside the boundary.
  it('off the first screen, renders the list even on a cold cache', () => {
    nav.params = new URLSearchParams('status=active')
    renderOnNavigation(fallback, new QueryClient())

    expect(screen.getByTestId('list')).toHaveTextContent('list loading')
  })

  it('reads an empty defeating param as the first screen, as the seed gate does', () => {
    nav.params = new URLSearchParams('status=')
    renderOnNavigation(fallback, new QueryClient())

    expect(screen.getByTestId('loader')).toBeInTheDocument()
  })

  it('reads a repeated defeating param as off the first screen', () => {
    nav.params = new URLSearchParams('status=&status=active')
    renderOnNavigation(fallback, new QueryClient())

    expect(screen.getByTestId('list')).toBeInTheDocument()
  })
})

describe('CachedListFallback hydration', () => {
  let container: HTMLDivElement

  beforeEach(() => {
    nav.params = new URLSearchParams()
    container = document.createElement('div')
    document.body.appendChild(container)
  })

  afterEach(() => {
    cleanup()
    container.remove()
  })

  // A first load hydrates the server's loader. The browser cache may already
  // hold the rows; the hydration render must still match the server HTML.
  it('hydrates the server loader over a warm cache without a mismatch, then shows the list', async () => {
    container.innerHTML = renderToString(
      <QueryClientProvider client={new QueryClient()}>{fallback}</QueryClientProvider>
    )
    const recoverable = vi.fn()
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})

    let root: ReturnType<typeof hydrateRoot> | undefined
    await act(async () => {
      root = hydrateRoot(
        container,
        <QueryClientProvider client={warm()}>{fallback}</QueryClientProvider>,
        { onRecoverableError: recoverable }
      )
    })

    expect(recoverable).not.toHaveBeenCalled()
    expect(consoleError).not.toHaveBeenCalled()
    expect(container.textContent).toContain('cached rows')

    consoleError.mockRestore()
    act(() => root?.unmount())
  })
})
