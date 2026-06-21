import type { Metadata } from 'next'

/**
 * Public (un-gated) view of the stage-atmosphere mocks (spotlight + smoke +
 * god-rays) for easy comparison during exploration. noindex. Admin-gated copy
 * stays at /admin/hero-lab/smoke; delete this route to re-gate.
 */
export const metadata: Metadata = {
  title: 'Hero Lab — atmosphere mocks',
  robots: { index: false, follow: false },
}

export { default } from '../../admin/hero-lab/smoke/page'
