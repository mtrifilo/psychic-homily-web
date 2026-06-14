import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render } from '@testing-library/react'
import { renderToStaticMarkup } from 'react-dom/server'
import * as Sentry from '@sentry/nextjs'
import GlobalError from './global-error'


describe('Global error boundary (app/global-error.tsx)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('reports the error to Sentry with its digest', () => {
    // Mount with testing-library so the useEffect Sentry side effect runs.
    // (React 19 won't mount the <html>/<body> wrapper inside the container
    // div, but the effect still fires — that's all this assertion needs.)
    const error = Object.assign(new Error('boom'), { digest: 'abc123' })
    render(<GlobalError error={error} />)

    expect(Sentry.captureException).toHaveBeenCalledTimes(1)
    expect(Sentry.captureException).toHaveBeenCalledWith(error)
    expect(vi.mocked(Sentry.captureException).mock.calls[0][0]).toHaveProperty(
      'digest',
      'abc123'
    )
  })

  it('wraps its fallback in an html/body document so it can replace the root layout', () => {
    // Serialize the full document tree — DOM-nesting rules that block
    // mounting <html> inside a <div> don't apply to static markup.
    const html = renderToStaticMarkup(<GlobalError error={new Error('boom')} />)

    expect(html).toMatch(/^<html>/)
    expect(html).toContain('<body>')
    expect(html).toMatch(/<\/html>$/)
  })

  it('renders the default Next.js 500 error page', () => {
    const html = renderToStaticMarkup(<GlobalError error={new Error('boom')} />)

    expect(html).toContain('500')
  })
})
