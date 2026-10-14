import { createContext, useContext } from 'react'
import type { ComponentProps, ReactNode } from 'react'

/**
 * A `next/link` stand-in. It renders a plain anchor with the props it was
 * given (a `ref` included), plus `data-prefetch`, so a test can check whether
 * a link may prefetch: the `prefetch` prop as a string, or `'default'` when
 * the caller passed none.
 *
 * `useLinkStatus` reports a link pending when its href is the one a test
 * passed to `setPendingLinkHref`, and idle otherwise (the stub never
 * navigates). The pending href is module state; a test that sets it resets it
 * with `setPendingLinkHref(null)`.
 *
 * Usage:
 *   vi.mock('next/link', () => import('@/test/mocks/nextLink'))
 */
export default function NextLinkStub({
  href,
  prefetch,
  children,
  ...rest
}: Omit<ComponentProps<'a'>, 'href'> & {
  href: string
  prefetch?: boolean | null
  children?: ReactNode
}) {
  return (
    <LinkStatus.Provider value={href === pendingHref}>
      <a
        href={href}
        {...rest}
        data-prefetch={prefetch === undefined ? 'default' : String(prefetch)}
      >
        {children}
      </a>
    </LinkStatus.Provider>
  )
}

let pendingHref: string | null = null

const LinkStatus = createContext(false)

export function setPendingLinkHref(href: string | null): void {
  pendingHref = href
}

export function useLinkStatus(): { pending: boolean } {
  return { pending: useContext(LinkStatus) }
}
