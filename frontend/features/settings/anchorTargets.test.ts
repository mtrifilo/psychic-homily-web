import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { findAnchorTarget, jumpToAnchor } from './anchorTargets'

describe('anchorTargets', () => {
  let decoy: HTMLElement
  let root: HTMLElement
  let target: HTMLElement
  const scrollIntoView = vi.fn()

  beforeEach(() => {
    scrollIntoView.mockReset()
    Element.prototype.scrollIntoView = scrollIntoView
    window.history.replaceState(null, '', '/settings')
    // A hidden tree ahead of the root in document order, with the same id.
    decoy = document.createElement('div')
    decoy.hidden = true
    decoy.innerHTML = '<div id="alerts" tabindex="-1"></div>'
    root = document.createElement('div')
    root.innerHTML = '<section id="alerts" tabindex="-1"></section>'
    document.body.append(decoy, root)
    target = root.querySelector('section') as HTMLElement
  })

  afterEach(() => {
    document.body.innerHTML = ''
    window.history.replaceState(null, '', '/settings')
  })

  it('finds the target inside the root, not the first match in the document', () => {
    expect(document.getElementById('alerts')).not.toBe(target)
    expect(findAnchorTarget(root, 'alerts')).toBe(target)
  })

  it('returns null for an anchor the root does not hold', () => {
    expect(findAnchorTarget(root, 'feeds')).toBeNull()
  })

  it('jumps to, focuses, and records the root target', () => {
    jumpToAnchor(root, 'alerts')

    expect(scrollIntoView).toHaveBeenCalledTimes(1)
    expect(scrollIntoView.mock.contexts[0]).toBe(target)
    expect(target).toHaveFocus()
    expect(window.location.hash).toBe('#alerts')
  })

  // The App Router restores only history entries written through its patched
  // pushState; a native fragment navigation leaves Back from the next page
  // stranded on that page.
  it('records the fragment through history.pushState, not a native navigation', () => {
    const pushState = vi.spyOn(window.history, 'pushState')
    const onHashChange = vi.fn()
    window.addEventListener('hashchange', onHashChange)
    try {
      jumpToAnchor(root, 'alerts')
      expect(pushState).toHaveBeenCalledTimes(1)
      expect(pushState).toHaveBeenCalledWith(null, '', '#alerts')

      jumpToAnchor(root, 'alerts')
      expect(pushState).toHaveBeenCalledTimes(1)
      expect(scrollIntoView).toHaveBeenCalledTimes(2)
    } finally {
      pushState.mockRestore()
      window.removeEventListener('hashchange', onHashChange)
    }
    expect(onHashChange).not.toHaveBeenCalled()
  })

  it('does nothing without a root or a target', () => {
    jumpToAnchor(null, 'alerts')
    jumpToAnchor(root, 'feeds')
    expect(scrollIntoView).not.toHaveBeenCalled()
    expect(window.location.hash).toBe('')
  })
})
