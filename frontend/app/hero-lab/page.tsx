import type { Metadata } from 'next'

/**
 * Public (un-gated) view of the A–E hero gallery for easy comparison during the
 * exploration phase (PSY-1138/1161). noindex — internal dev surface, not for
 * search. The admin-gated copy stays at /admin/hero-lab; delete this route to
 * re-gate. Renders the same component.
 */
export const metadata: Metadata = {
  title: 'Hero Lab',
  robots: { index: false, follow: false },
}

export { default } from '../admin/hero-lab/page'
