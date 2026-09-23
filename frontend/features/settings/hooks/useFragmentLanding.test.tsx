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

  // The App Router reloads the document on a popstate whose state is not its
  // own, so landing must write nothing to a history entry.
  it('leaves a null-state history entry null after landing', () => {
    window.history.replaceState(null, '', '/settings#alerts')
    render(<Page />)
    expect(landedOn()).toEqual(['alerts'])
    expect(window.history.state).toBeNull()
  })

  it('leaves a router-owned history entry untouched after landing', () => {
    const state = { __NA: true, tree: ['/settings'] }
    window.history.replaceState(state, '', '/settings#alerts')
    render(<Page />)
    expect(window.history.state).toEqual(state)
  })

  // Fragment changes after mount are the browser's own navigation.
  it('does not land on a later hashchange', () => {
    window.history.replaceState(null, '', '/settings#alerts')
    render(<Page />)
    act(() => {
      const oldURL = window.location.href
      window.history.replaceState(null, '', '/settings#privacy')
      window.dispatchEvent(
        new HashChangeEvent('hashchange', { oldURL, newURL: window.location.href })
      )
    })
    expect(landedOn()).toEqual(['alerts'])
    expect(onLand).toHaveBeenCalledTimes(1)
  })

  it('does not land on a later popstate', () => {
    render(<Page />)
    act(() => {
      window.history.replaceState(null, '', '/settings#privacy')
      window.dispatchEvent(new PopStateEvent('popstate', { state: null }))
    })
    expect(scrollIntoView).not.toHaveBeenCalled()
  })

  // A reload or any fresh mount is a new page instance and lands again.
  it('lands again on a fresh mount', () => {
    window.history.replaceState(null, '', '/settings#alerts')
    const first = render(<Page />)
    first.unmount()
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
