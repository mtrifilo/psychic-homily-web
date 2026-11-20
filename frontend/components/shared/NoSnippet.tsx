import type { ReactNode } from 'react'

/**
 * Interface copy that search engines must not quote as a page's snippet.
 *
 * Google honours `data-nosnippet` on `span`, `div` and `section` only, and may
 * read it before or after rendering, so the attribute is on the element from
 * the moment it is first added to the page and is never toggled. A `span`
 * keeps the copy's layout exactly as it was without one.
 */
export function NoSnippet({ children }: { children: ReactNode }) {
  return <span data-nosnippet="">{children}</span>
}
