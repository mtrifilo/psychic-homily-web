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
 * How long a chosen section stays marked while the scroll that reveals it is
 * still moving. Scroll events during that scroll pass over other sections, and
 * would otherwise mark each of them in turn.
 */
const SELECTION_HOLD_MS = 1000

/** Input that means the viewer is scrolling on their own, ending a hold. */
const USER_SCROLL_EVENTS = ['wheel', 'touchmove'] as const

/** Keys that scroll the page when focus is not in a control. */
const SCROLL_KEYS = new Set([
  'ArrowDown',
  'ArrowUp',
  'End',
  'Home',
  'PageDown',
  'PageUp',
  ' ',
])

/** Elements that consume those keys themselves instead of scrolling. */
const KEY_CONSUMING_TARGETS =
  'input, textarea, select, button, [contenteditable], [role]'

function isScrollKey(event: KeyboardEvent): boolean {
  if (!SCROLL_KEYS.has(event.key)) return false
  const target = event.target
  return !(target instanceof Element && target.matches(KEY_CONSUMING_TARGETS))
}

/**
 * The line a section's top must pass to count as the one being read: a
 * section's scroll margin, which is where a jump parks it, so a section just
 * landed on is the section marked. Read from the first section; every tracked
 * section carries the same margin.
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

function topIsOnScreen(section: HTMLElement): boolean {
  const top = section.getBoundingClientRect().top
  return top >= 0 && top < window.innerHeight
}

/**
 * The anchor a scroll position marks.
 *
 * Away from the bottom of the page: the last section whose top has passed the
 * reading line. At the bottom of a scrolled page the sections below the line
 * can never reach it, so the section the viewer chose stays marked, and
 * otherwise the last section is. `chosen` is on screen or null.
 */
function anchorAtScrollPosition(
  sections: readonly HTMLElement[],
  chosen: HTMLElement | null
): string {
  const doc = document.documentElement
  const atBottom =
    window.scrollY > 0 &&
    window.innerHeight + window.scrollY >= doc.scrollHeight - 2
  if (atBottom) return (chosen ?? sections[sections.length - 1]).id
  const readingLine = readingLinePx(sections[0])
  let current = sections[0].id
  for (const section of sections) {
    if (section.getBoundingClientRect().top <= readingLine) current = section.id
  }
  return current
}

/**
 * The section of a one-page, fragment-anchored layout that the viewer is
 * reading, for a rail to mark.
 *
 * `select` marks the section a fragment belongs to outright and holds it
 * while the reveal scroll moves; the hold ends on a timer or on the viewer's own scroll
 * input (wheel, touch, or a scrolling key outside a control), and the position
 * is measured again when it does. The chosen section
 * is remembered, for the bottom of the page where it cannot reach the reading
 * line, until the viewer scrolls on their own or its top leaves the screen.
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
  const chosen = useRef<string | null>(null)
  const measureRef = useRef<() => void>(() => {})

  const release = useCallback(() => {
    if (!holding.current) return
    holding.current = false
    measureRef.current()
  }, [])
  const { schedule: scheduleRelease, cancel: cancelRelease } = useDismissTimer(
    release,
    SELECTION_HOLD_MS
  )

  const select = useCallback(
    (fragment: string) => {
      const root = rootRef.current
      if (!root) return
      const anchor = owningAnchor(root, fragment, anchors)
      if (anchor === null) return
      setActiveAnchor(anchor)
      chosen.current = anchor
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
      let chosenSection =
        sections.find(section => section.id === chosen.current) ?? null
      if (chosenSection && !topIsOnScreen(chosenSection)) {
        chosen.current = null
        chosenSection = null
      }
      setActiveAnchor(anchorAtScrollPosition(sections, chosenSection))
    }
    measureRef.current = measure

    const onScroll = () => {
      if (frame === 0) frame = requestAnimationFrame(measure)
    }
    const onUserScroll = () => {
      chosen.current = null
      cancelRelease()
      release()
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (isScrollKey(event)) onUserScroll()
    }

    // A hold the previous run of this effect started outlives its cleanup
    // (a hidden route shown again, a development double run), and needs its
    // release timer back.
    if (holding.current) scheduleRelease()
    window.addEventListener('scroll', onScroll, { passive: true })
    for (const type of USER_SCROLL_EVENTS) {
      window.addEventListener(type, onUserScroll, { passive: true })
    }
    window.addEventListener('keydown', onKeyDown)
    return () => {
      if (frame !== 0) cancelAnimationFrame(frame)
      cancelRelease()
      measureRef.current = () => {}
      window.removeEventListener('scroll', onScroll)
      for (const type of USER_SCROLL_EVENTS) {
        window.removeEventListener(type, onUserScroll)
      }
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [anchors, rootRef, scheduleRelease, cancelRelease, release])

  return { activeAnchor, select }
}
