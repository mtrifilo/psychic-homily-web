import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { Activity, useRef, type ReactNode } from 'react'
import { act, render } from '@testing-library/react'
import { useFragmentLanding } from './useFragmentLanding'

const scrollIntoView = vi.fn()
const onLand = vi.fn()

function Page() {
  const rootRef = useRef<HTMLDivElement>(null)
  useFragmentLanding(rootRef, onLand)
  return (
    <div ref={rootRef}>
      <section id="alerts" tabIndex={-1} />
      <section id="privacy" tabIndex={-1} />
    </div>
  )
}

function Shell({ mode, children }: { mode: 'visible' | 'hidden'; children: ReactNode }) {
  return <Activity mode={mode}>{children}</Activity>
}

const landedOn = () =>
  scrollIntoView.mock.contexts.map(context => (context as HTMLElement).id)

describe('useFragmentLanding', () => {
  beforeEach(() => {
    scrollIntoView.mockReset()
    onLand.mockReset()
    Element.prototype.scrollIntoView = scrollIntoView
    window.history.replaceState(null, '', '/settings')
  })

  afterEach(() => {
    window.history.replaceState(null, '', '/settings')
  })

  it('lands on the fragment the page mounted with', () => {
    window.history.replaceState(null, '', '/settings#alerts')
    render(<Page />)
    expect(landedOn()).toEqual(['alerts'])
    expect(document.getElementById('alerts')).toHaveFocus()
    expect(onLand).toHaveBeenCalledWith('alerts')
  })

  it('does nothing without a fragment', () => {
    render(<Page />)
    expect(scrollIntoView).not.toHaveBeenCalled()
    expect(onLand).not.toHaveBeenCalled()
  })

  it('lands again when the fragment is edited on this page', () => {
    render(<Page />)
    act(() => {
      const oldURL = window.location.href
      window.history.replaceState(null, '', '/settings#privacy')
      window.dispatchEvent(
        new HashChangeEvent('hashchange', { oldURL, newURL: window.location.href })
      )
    })
    expect(landedOn()).toEqual(['privacy'])
    expect(onLand).toHaveBeenCalledWith('privacy')
  })

  it('ignores a hashchange that arrives from another path', () => {
    render(<Page />)
    act(() => {
      window.history.replaceState(null, '', '/settings#privacy')
      window.dispatchEvent(
        new HashChangeEvent('hashchange', {
          oldURL: `${window.location.origin}/profile?tab=settings`,
          newURL: window.location.href,
        })
      )
    })
    expect(scrollIntoView).not.toHaveBeenCalled()
  })

  it('does not land again when the page mounts afresh on the same entry', () => {
    window.history.replaceState({ routerOwned: true }, '', '/settings#alerts')
    const first = render(<Page />)
    expect(landedOn()).toEqual(['alerts'])
    // The router's own state on the entry survives the landing record.
    expect(window.history.state).toMatchObject({ routerOwned: true })
    first.unmount()

    render(<Page />)
    expect(landedOn()).toEqual(['alerts'])
    expect(onLand).toHaveBeenCalledTimes(1)
  })

  it('lands on a new history entry even with the same fragment', () => {
    window.history.replaceState(null, '', '/settings#alerts')
    const first = render(<Page />)
    first.unmount()
    window.history.pushState(null, '', '/settings#alerts')
    render(<Page />)
    expect(landedOn()).toEqual(['alerts', 'alerts'])
  })

  // Back to a kept-alive route re-runs its effects with the fragment the
  // viewer arrived with; landing again would pull them off the place the
  // browser restored.
  it('does not land again when a hidden page is shown again', () => {
    window.history.replaceState(null, '', '/settings#alerts')
    const { rerender } = render(
      <Shell mode="visible">
        <Page />
      </Shell>
    )
    expect(landedOn()).toEqual(['alerts'])

    rerender(
      <Shell mode="hidden">
        <Page />
      </Shell>
    )
    rerender(
      <Shell mode="visible">
        <Page />
      </Shell>
    )
    expect(landedOn()).toEqual(['alerts'])
    expect(onLand).toHaveBeenCalledTimes(1)
  })
})
