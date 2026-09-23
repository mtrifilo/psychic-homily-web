'use client'

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type RefObject,
} from 'react'
import { useDismissTimer } from '@/lib/hooks/common/useDismissTimer'
import { findAnchorTarget } from '../anchorTargets'

/**
 * How long a chosen section stays marked while the jump it caused settles.
 * A section near the bottom of the page can never scroll up to the reading
 * line, so without the hold the scroll that follows a click would mark the
 * section above the one the viewer chose.
 */
const SELECTION_HOLD_MS = 1000

/**
 * The line a section's top must pass to count as the one being read: the
 * section's own scroll margin, which is where a fragment jump parks it. A
 * section just landed on is therefore the section marked.
 */
function readingLinePx(section: HTMLElement): number {
  return parseFloat(getComputedStyle(section).scrollMarginTop) || 0
}

/**
 * The tracked anchor a fragment belongs to: the fragment itself when it is
 * one, otherwise the tracked anchor whose element contains the fragment's
 * element, so a link to a row inside a section marks that section.
 */
function owningAnchor(
  root: HTMLElement,
  fragment: string,
  anchors: readonly string[]
): string | null {
  if (fragment === '') return null
  if (anchors.includes(fragment)) return fragment
  let node = findAnchorTarget(root, fragment)?.parentElement ?? null
  while (node && node !== root) {
    if (node.id && anchors.includes(node.id)) return node.id
    node = node.parentElement
  }
  return null
}

/**
 * The section of a one-page, fragment-anchored layout that the viewer is
 * reading, for a rail to mark.
 *
 * The marked section is the last one whose top has passed the reading line,
 * except at the very bottom of a scrolled page, where it is the last section:
 * a short final section never reaches the line. A fragment that names a
 * section (a cold load or a jump) marks that section outright and holds it
 * until the jump has settled.
 *
 * Sections are looked up inside `rootRef` only: another route's tree can stay
 * mounted, hidden, in the same document and carry the same ids.
 *
 * `anchors` must be a stable reference; the listeners key on it.
 */
export function useActiveSection(
  anchors: readonly string[],
  rootRef: RefObject<HTMLElement | null>
) {
  const [activeAnchor, setActiveAnchor] = useState<string>(anchors[0] ?? '')
  const holding = useRef(false)
  const { schedule: scheduleRelease, cancel: cancelRelease } = useDismissTimer(
    () => {
      holding.current = false
    },
    SELECTION_HOLD_MS
  )

  const select = useCallback(
    (fragment: string) => {
      const root = rootRef.current
      if (!root) return
      const anchor = owningAnchor(root, fragment, anchors)
      if (anchor === null) return
      setActiveAnchor(anchor)
      holding.current = true
      scheduleRelease()
    },
    [anchors, rootRef, scheduleRelease]
  )

  useEffect(() => {
    let frame = 0

    const measure = () => {
      frame = 0
      const root = rootRef.current
      if (holding.current || !root) return
      const sections = anchors
        .map(anchor => findAnchorTarget(root, anchor))
        .filter((section): section is HTMLElement => section !== null)
      if (sections.length === 0) return
      const doc = document.documentElement
      const atBottom =
        window.innerHeight + window.scrollY >= doc.scrollHeight - 2
      if (atBottom && window.scrollY > 0) {
        setActiveAnchor(sections[sections.length - 1].id)
        return
      }
      const readingLine = readingLinePx(sections[0])
      let current = sections[0].id
      for (const section of sections) {
        if (section.getBoundingClientRect().top <= readingLine) {
          current = section.id
        }
      }
      setActiveAnchor(current)
    }

    const onScroll = () => {
      if (frame === 0) frame = requestAnimationFrame(measure)
    }
    const onHashChange = () => select(window.location.hash.replace(/^#/, ''))

    onHashChange()
    window.addEventListener('scroll', onScroll, { passive: true })
    window.addEventListener('hashchange', onHashChange)
    return () => {
      if (frame !== 0) cancelAnimationFrame(frame)
      cancelRelease()
      holding.current = false
      window.removeEventListener('scroll', onScroll)
      window.removeEventListener('hashchange', onHashChange)
    }
  }, [anchors, rootRef, select, cancelRelease])

  return { activeAnchor, select }
}
