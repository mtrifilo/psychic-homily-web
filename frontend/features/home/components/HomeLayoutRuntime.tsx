'use client'

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { useRouter } from 'next/navigation'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useReducedMotion } from '@/lib/hooks/common/useReducedMotion'
import { animateSlotHeight, useFlipReorder } from '../homeLayoutMotion'
import { useHomeLayout } from '../hooks/useHomeLayout'
import {
  resolveHomeLinkSlots,
  resolvePasskeyOfferSlot,
  type HomeLayoutDocument,
  type HomeSectionId,
} from '../sections'
import { SignedInHomeDiscoverLinks } from './HomeDiscoverLinks'
import { HomeLinkSlotsProvider } from './HomeLinkSlots'
import { HomePageEndRow } from './HomePageEndRow'
import { PasskeyOfferCard } from './PasskeyOfferCard'
import type { HomeVisibilityChange } from './HomeSectionList'

/** A section mid-show or mid-hide, and which way it is going. */
type SlotTransition = 'collapse' | 'expand'

/** A staged transition, and whether the section was visible before the first
 *  gesture of the chain that staged it (a reversal mid-flight keeps the
 *  value the interrupted transition recorded). */
interface StagedTransition {
  direction: SlotTransition
  wasVisible: boolean
}

/**
 * The signed-in home's client shell: the sections in this viewer's order, then
 * the page-end row that carries "Customize home →".
 *
 * `sections` arrives already rendered by the SERVER component above, one entry
 * per registry id. Every section is handed over whether or not it is visible,
 * because showing one has to paint immediately; only the ones this component
 * renders actually mount, so a hidden section issues no request.
 *
 * First paint carries the viewer's own order: `initialLayout` is the document
 * the server read on this request, and the profile query it defers to is
 * hydrated from the same read, so hydration finds the order it rendered.
 */
export function HomeLayoutRuntime({
  initialLayout,
  sections,
}: {
  initialLayout?: HomeLayoutDocument | null
  sections: Record<HomeSectionId, ReactNode>
}) {
  const { authStatus } = useAuthContext()
  const router = useRouter()
  const { sections: layout } = useHomeLayout(initialLayout)
  const headingRef = useRef<HTMLHeadingElement>(null)
  // Counts the passkey offer's exits. Focus moves in an effect, after the
  // commit that removed the card: a modal the card opened still traps focus
  // until that commit, and would pull a focus moved any earlier back inside.
  // Without scrolling: the card sits below the viewer's shows, and the
  // heading is visually hidden, so scrolling to it would only jump the page.
  const [offerExits, setOfferExits] = useState(0)
  useEffect(() => {
    if (offerExits > 0) headingRef.current?.focus({ preventScroll: true })
  }, [offerExits])

  // The server picked this variant from the viewer's cookie; signing out
  // without navigating leaves it mounted. This guard lives HERE, on the shell
  // that is always present, rather than inside a section a viewer can hide:
  // parked in the saved-shows module it disappeared with that section, and a
  // viewer who had hidden it kept the signed-in page, the Customize control,
  // and the previous account's layout after signing out.
  useEffect(() => {
    if (authStatus === 'anonymous') router.refresh()
  }, [authStatus, router])

  // Staged by the gesture that caused them, never derived from the layout: a
  // change arriving from another device should apply instantly, not play an
  // animation nobody asked for. A MAP, not one id, so toggling a second
  // section does not yank the first out mid-animation.
  const [transitions, setTransitions] = useState<
    ReadonlyMap<HomeSectionId, StagedTransition>
  >(() => new Map())

  // Direction-aware: a cancel fires this too, and a re-show staged DURING a
  // collapse must not have its own transition cleared by the collapse it
  // interrupted.
  const settleTransition = useCallback(
    (id: HomeSectionId, direction: SlotTransition) => {
      setTransitions(current => {
        if (current.get(id)?.direction !== direction) return current
        const next = new Map(current)
        next.delete(id)
        return next
      })
    },
    []
  )

  // A section stays mounted for the whole of EITHER transition. Keeping it
  // only for a collapse meant re-showing a section mid-collapse dropped it
  // from the list for a frame, destroying and rebuilding its whole subtree
  // (the graph canvas included) instead of reversing the animation.
  const rendered = layout.filter(
    section => section.visible || transitions.has(section.id)
  )
  const renderedIds = rendered.map(section => section.id)
  const { register, capture } = useFlipReorder(renderedIds)

  const handleBeforeChange = useCallback(
    (change: HomeVisibilityChange) => {
      if (!change) {
        // A reorder or a reset slides the sections between their old and new
        // positions. A show or hide does not: the slot's own height transition
        // is what moves everything below it.
        capture()
        return
      }
      setTransitions(current => {
        const next = new Map(current)
        const interrupted = current.get(change.id)
        // Deleted first so the map's insertion order stays staging order: its
        // last key is the most recently staged section (see `lastStaged`).
        next.delete(change.id)
        next.set(change.id, {
          direction: change.visible ? 'expand' : 'collapse',
          wasVisible: interrupted ? interrupted.wasVisible : !change.visible,
        })
        return next
      })
    },
    [capture]
  )

  // Read from `rendered`, not `layout`: a section still collapsing is on
  // screen, and calling the page empty while it animates would unmount it
  // before it finished and strand it in the transition map forever.
  const allHidden = rendered.length === 0
  const lastStaged = [...transitions.keys()].at(-1) ?? null
  const { cityLink, discoverLinks } = resolveHomeLinkSlots(
    layout,
    renderedIds,
    lastStaged
  )
  // Memoized on the two slots so a render that moves neither (the popover
  // opening, a transition settling) does not re-render every section that
  // reads them.
  const linkSlots = useMemo(
    () => ({ cityLink, discoverLinks }),
    [cityLink, discoverLinks]
  )
  // Placed by each section's visibility BEFORE the gestures still in flight
  // on it: the offer moves only once the section settles, a reversal
  // mid-animation included. Moving it in the commit that starts a height
  // animation would shift everything below by the card's height on the first
  // frame, the Customize link and its open popover included. A move at the
  // settle only reorders the column, whose height it does not change, so
  // nothing below the last section moves.
  const offerAfter = resolvePasskeyOfferSlot(
    rendered.map(section => {
      const staged = transitions.get(section.id)
      return staged ? { ...section, visible: staged.wasVisible } : section
    })
  )
  const offerIndex = offerAfter
    ? rendered.findIndex(section => section.id === offerAfter)
    : rendered.length - 1
  // The offer is followed by a section unless it sits at the end of the
  // column, and takes its gap from whichever side has no slot spacer.
  const isOfferLast = allHidden || offerIndex === rendered.length - 1

  // ONE keyed list for the sections, the offer and the empty-state line, so
  // the offer is the same element wherever it lands: it moves between
  // positions without remounting, and keeps its state (a passkey just added,
  // an error on screen) when the viewer hides or reorders the section above.
  const column: ReactNode[] = []
  const offer = (
    <div key="passkey-offer" ref={register('passkey-offer')}>
      {/* Focus goes to the page heading when the card leaves, so a keyboard
          viewer continues from the top of the home, not from the document. */}
      <PasskeyOfferCard
        className={isOfferLast ? 'mt-14' : 'mb-14'}
        onLeave={() => setOfferExits(count => count + 1)}
      />
    </div>
  )
  if (allHidden) {
    column.push(
      <div key="empty" className="mt-2 flex flex-col gap-2">
        <p className="text-sm text-muted-foreground">
          You have hidden every section.
        </p>
        {discoverLinks === 'empty' && <SignedInHomeDiscoverLinks />}
      </div>
    )
  }
  rendered.forEach((section, index) => {
    column.push(
      <HomeSectionSlot
        key={section.id}
        id={section.id}
        registerRef={register(section.id)}
        transition={transitions.get(section.id)?.direction}
        isLast={index === rendered.length - 1}
        onSettled={settleTransition}
      >
        {sections[section.id]}
      </HomeSectionSlot>
    )
    if (index === offerIndex) column.push(offer)
  })
  if (allHidden) column.push(offer)

  // Say nothing rather than address a viewer who is no longer there. The
  // effect above has already asked the server for the anonymous page.
  if (authStatus === 'anonymous') return null

  return (
    <HomeLinkSlotsProvider value={linkSlots}>
      <div className="flex w-full flex-col">
        {/* The page's only h1, deliberately outside the sections: every one of
            them is hideable, so a heading that lived in one would take the
            document's top-level heading with it. */}
        <h1 ref={headingRef} tabIndex={-1} className="sr-only">
          Home
        </h1>

        {/* The gap between sections rides INSIDE each slot rather than on this
            container: on the container it survives the height animation and
            vanishes at unmount, snapping everything below up by its full
            height. It also cannot be padding on the slot itself, because
            padding floors a border-box element's rendered height, so the slot
            would stop 56px short of collapsing and drop that at unmount
            instead. */}
        <div className="mt-4 flex w-full flex-col">{column}</div>

        {/* Outside the column and never conditional: the popover it anchors
            must survive the page emptying while it is open. */}
        <HomePageEndRow
          initialLayout={initialLayout}
          onBeforeChange={handleBeforeChange}
          className={allHidden ? 'mt-6' : 'mt-12'}
        />
      </div>
    </HomeLinkSlotsProvider>
  )
}

/**
 * One section's place in the column, and the only element that animates on a
 * show or hide.
 *
 * The wrapper exists so the height transition has something to animate that is
 * not the section's own layout: sections set their own flex column and gaps,
 * and overriding those from outside would be a different bug on each one.
 */
function HomeSectionSlot({
  id,
  registerRef,
  transition,
  isLast,
  onSettled,
  children,
}: {
  id: HomeSectionId
  registerRef: (node: HTMLElement | null) => void
  transition: SlotTransition | undefined
  isLast: boolean
  onSettled: (id: HomeSectionId, direction: SlotTransition) => void
  children: ReactNode
}) {
  const node = useRef<HTMLDivElement | null>(null)
  const reducedMotion = useReducedMotion()

  const setNode = (element: HTMLDivElement | null) => {
    node.current = element
    registerRef(element)
  }

  useLayoutEffect(() => {
    if (!transition || !node.current) return
    const animation = animateSlotHeight(
      node.current,
      transition,
      reducedMotion,
      () => onSettled(id, transition)
    )
    if (!animation) {
      onSettled(id, transition)
      return
    }
    // A collapse holds the slot at zero height after it finishes. If the write
    // failed and the section is staying after all, this slot is still mounted
    // and would be pinned invisible; cancelling releases it, and the cancel
    // handler settles the transition either way.
    return () => animation.cancel()
  }, [id, onSettled, reducedMotion, transition])

  return (
    <div
      ref={setNode}
      // Clipped only while a height is being animated: a section's own content
      // (popovers, the graph's hover chrome) may legitimately overflow at rest.
      style={transition ? { overflow: 'hidden' } : undefined}
      // A collapsing section is leaving; keep it out of the a11y tree and out
      // of the tab order for the frames it is still painted.
      aria-hidden={transition === 'collapse' || undefined}
      className="w-full"
    >
      {/* The spacer is a CHILD so the slot's height animation encloses it.
          A section that renders nothing (an omitted module, a self-hidden
          band) leaves it empty, and an empty spacer takes no space, so the
          next section moves up into the slot. */}
      <div className={isLast ? 'empty:hidden' : 'pb-14 empty:hidden'}>{children}</div>
    </div>
  )
}
