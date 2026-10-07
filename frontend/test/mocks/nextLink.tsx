import type { ComponentProps, ReactNode } from 'react'

/**
 * A `next/link` stand-in for tests that check whether a link may prefetch. It
 * renders a plain anchor with the props it was given, plus `data-prefetch`:
 * the `prefetch` prop as a string, or `'default'` when the caller passed none.
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
    <a
      href={href}
      {...rest}
      data-prefetch={prefetch === undefined ? 'default' : String(prefetch)}
    >
      {children}
    </a>
  )
}
