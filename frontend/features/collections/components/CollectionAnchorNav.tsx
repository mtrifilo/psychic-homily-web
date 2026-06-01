'use client'

/**
 * CollectionAnchorNav — sticky jump nav for the collection detail page
 * (PSY-892 D1, implemented in PSY-898).
 *
 * Collections are uniquely long-form curation surfaces — items + tags +
 * discussion stack vertically and can exceed 3+ viewport heights. The nav
 * pins below the site TopBar and tracks the section currently in view via
 * IntersectionObserver. No other entity detail page carries this chrome;
 * collections earn it (see the PSY-892 decisions comment).
 *
 * Section order matches D6: Items → Tags → Discussion. Styling follows the
 * DS TabTrigger underline-active pattern (border-b accent, muted inactive).
 *
 * Not exported from the feature barrel on purpose — only
 * `CollectionDetail.tsx` consumes it (see PSY-951 de-barrel note in
 * components/index.ts).
 */

import { useEffect, useState } from 'react'
import { cn } from '@/lib/utils'

/**
 * Total height (px) of the sticky chrome that sits above anchored content:
 * the site TopBar (`--topbar-height` = 3.5rem = 56px) + this nav (h-10 =
 * 40px + 1px border) + breathing room. Single source of truth for both the
 * IntersectionObserver's top offset and the section scroll-margins below —
 * change them together.
 */
const STICKY_CHROME_OFFSET_PX = 104

/**
 * Tailwind class for sections the nav jumps to. Pairs `id="<section>"` with a
 * scroll margin that keeps the jumped-to heading clear of the sticky chrome.
 * Kept as a full literal (not interpolated) so Tailwind's scanner picks it up;
 * the value mirrors {@link STICKY_CHROME_OFFSET_PX}.
 */
export const ANCHOR_SECTION_SCROLL_MT = 'scroll-mt-[104px]'

export interface AnchorSection {
  /** DOM id of the section element this nav entry jumps to. */
  id: string
  label: string
}

export function CollectionAnchorNav({
  sections,
}: {
  /**
   * Stable array of sections (define it as a module-level constant in the
   * parent) — the IntersectionObserver effect keys on this reference.
   */
  sections: AnchorSection[]
}) {
  const [activeId, setActiveId] = useState<string>(sections[0]?.id ?? '')

  useEffect(() => {
    // Track which section currently occupies the reading band. The top
    // rootMargin offsets the sticky chrome (TopBar + this nav); the -60%
    // bottom margin keeps the band in the upper part of the viewport so the
    // section the reader is actually looking at wins when two intersect.
    const observer = new IntersectionObserver(
      entries => {
        const visible = entries
          .filter(e => e.isIntersecting)
          .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)
        if (visible.length > 0) {
          setActiveId(visible[0].target.id)
        }
      },
      { rootMargin: `-${STICKY_CHROME_OFFSET_PX}px 0px -60% 0px` }
    )

    // The Items section lives inside a dynamic()-imported chunk
    // (CollectionItemsList) that may not have rendered yet when this effect
    // runs on a client-side navigation. Retry until every section element
    // exists before observing, so all three nav entries track correctly.
    let cancelled = false
    let retryTimer: ReturnType<typeof setTimeout> | null = null
    let attempts = 0
    const MAX_ATTEMPTS = 50 // ~5s at 100ms — sections always render well before this

    const tryObserve = () => {
      if (cancelled) return
      const els = sections.map(s => document.getElementById(s.id))
      if (els.every((el): el is HTMLElement => el !== null)) {
        for (const el of els) observer.observe(el)
      } else if (attempts < MAX_ATTEMPTS) {
        attempts += 1
        retryTimer = setTimeout(tryObserve, 100)
      }
    }
    tryObserve()

    return () => {
      cancelled = true
      if (retryTimer) clearTimeout(retryTimer)
      observer.disconnect()
    }
  }, [sections])

  const handleClick = (id: string) => {
    setActiveId(id)
    // Optional-call guard: jsdom doesn't implement scrollIntoView.
    document.getElementById(id)?.scrollIntoView?.({
      behavior: 'smooth',
      block: 'start',
    })
  }

  return (
    <nav
      className="sticky top-[var(--topbar-height)] z-40 -mx-4 mb-6 border-b border-border/50 bg-background/95 px-4 backdrop-blur-sm supports-[backdrop-filter]:bg-background/60"
      aria-label="Page sections"
      data-testid="collection-anchor-nav"
    >
      <div className="flex items-center gap-1">
        {sections.map(section => (
          <button
            key={section.id}
            type="button"
            onClick={() => handleClick(section.id)}
            aria-current={activeId === section.id ? 'true' : undefined}
            data-testid={`anchor-nav-${section.id}`}
            className={cn(
              'inline-flex h-10 items-center border-b-2 px-3 text-sm font-medium transition-colors',
              activeId === section.id
                ? 'border-primary text-foreground'
                : 'border-transparent text-muted-foreground hover:text-foreground'
            )}
          >
            {section.label}
          </button>
        ))}
      </div>
    </nav>
  )
}
