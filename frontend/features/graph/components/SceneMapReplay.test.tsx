import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, screen } from '@testing-library/react'

import { renderWithProviders } from '@/test/utils'

import type { SceneMap, SceneMapNode } from '../sceneMap'
import { useSceneReplay } from '../useSceneReplay'
import { SceneMapZeroState } from './SceneMapZeroState'

/**
 * The growth replay end to end through the card that owns it (PSY-1737): the
 * entry affordance's visibility rules, the footer-to-scrubber swap, seeking, and
 * both ways back to the at-rest map.
 *
 * Driven through the REAL transport rather than a stubbed controller — the
 * things most worth locking here (what the affordance is gated on, what Escape
 * does, what a drag does to the clock) are exactly the wiring a stub would fake.
 * jsdom supplies no frame loop, so frames are handed over by hand.
 */
const { canvasProps } = vi.hoisted(() => ({
  canvasProps: { current: null as Record<string, unknown> | null },
}))

vi.mock('./SceneMapCanvas', () => ({
  SceneMapCanvas: (props: Record<string, unknown>) => {
    canvasProps.current = props
    return <div data-testid="scene-map-canvas" />
  },
}))

let scheduled: Array<(now: number) => void> = []
let now = 0

function runFrames(count: number, msPerFrame = 100) {
  for (let i = 0; i < count; i += 1) {
    const callbacks = scheduled
    scheduled = []
    if (callbacks.length === 0) return
    now += msPerFrame
    act(() => {
      for (const callback of callbacks) callback(now)
    })
  }
}

beforeEach(() => {
  scheduled = []
  now = 1000
  canvasProps.current = null
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
    scheduled.push(callback)
    return scheduled.length
  })
  // Cancellation has to be OBSERVABLE, or a test cannot tell a torn-down chain
  // from a live one. The transport keeps exactly one frame in flight, so
  // dropping the queue is a faithful stand-in for cancelling by handle.
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {
    scheduled = []
  })
  vi.spyOn(performance, 'now').mockImplementation(() => now)
})

afterEach(() => {
  vi.restoreAllMocks()
})

function node(id: number, appear: number): SceneMapNode {
  return {
    id,
    kind: 'artist',
    name: `Artist ${id}`,
    slug: `artist-${id}`,
    x: 0,
    y: 0,
    community: 1,
    degree: 1,
    rank: id,
    hasUpcomingShow: false,
    hasPlayableAudio: false,
    homeCity: null,
    appear,
  }
}

function sceneMap(appears: number[]): SceneMap {
  return {
    nodes: appears.map((appear, index) => node(index + 1, appear)),
    edges: [],
    regions: [{ community: 1, label: 'Around Artist 1', memberCount: 2, hull: [], captionAnchor: null }],
    artistCount: appears.length,
    labelCount: 0,
    isolateCount: 42,
    lastMapped: new Date('2026-08-02T04:00:00Z'),
    epoch: new Date('2020-01-01T00:00:00Z'),
  }
}

const REPLAYABLE = sceneMap(
  Array.from({ length: 600 }, (_, i) => Math.floor((i / 600) ** 3 * 200_000_000)),
)
/** Every node stamped with one instant: a map with no history worth watching. */
const UNREPLAYABLE = sceneMap(new Array(600).fill(0))

/**
 * Mounts the card with a live transport, exactly as the Observatory does.
 * `withReplay: false` covers the branch where the host supplies none at all.
 */
function Harness({
  map = REPLAYABLE,
  canvasWidth = 1024,
  withReplay = true,
}: {
  map?: SceneMap
  canvasWidth?: number | null
  withReplay?: boolean
}) {
  const replay = useSceneReplay(map)
  return (
    <SceneMapZeroState
      map={map}
      canvasWidth={canvasWidth}
      onSelectArtist={() => {}}
      replay={withReplay ? replay : undefined}
    />
  )
}

function renderCard(props: Parameters<typeof Harness>[0] = {}) {
  return renderWithProviders(<Harness {...props} />)
}

const chip = () => screen.queryByRole('button', { name: /watch it grow/i })
const scrubber = () => screen.queryByRole('slider', { name: /replay position/i })
const freshness = () => screen.queryByText(/Mapped nightly/)

describe('the WATCH IT GROW affordance', () => {
  it('sits beside the freshness footer on a map with a watchable history', () => {
    renderCard()

    expect(chip()).toBeInTheDocument()
    expect(freshness()).toBeInTheDocument()
    // Same row, not a second strip of chrome.
    expect(chip()!.closest('div')).toContainElement(freshness())
  })

  it('is absent when the snapshot has no history worth watching', () => {
    renderCard({ map: UNREPLAYABLE })

    expect(chip()).not.toBeInTheDocument()
    expect(freshness()).toBeInTheDocument()
  })

  it('is absent when the host supplies no transport at all', () => {
    renderCard({ withReplay: false })

    expect(chip()).not.toBeInTheDocument()
    expect(freshness()).toBeInTheDocument()
  })

  // The locked scope edge: sub-640px keeps the teaser idiom, where there is no
  // map to watch grow in the first place.
  it('is absent below the canvas breakpoint, even with a replayable map', () => {
    renderCard({ canvasWidth: null })

    expect(screen.queryByTestId('scene-map-canvas')).not.toBeInTheDocument()
    expect(chip()).not.toBeInTheDocument()
    expect(freshness()).toBeInTheDocument()
  })

  it('carries no em dashes in its copy', () => {
    renderCard()
    expect(chip()!.textContent).not.toContain('—')
  })
})

describe('entering a run', () => {
  it('swaps the freshness strip for the scrubber, and nothing else about the card', () => {
    renderCard()
    expect(scrubber()).not.toBeInTheDocument()

    fireEvent.click(chip()!)

    expect(scrubber()).toBeInTheDocument()
    expect(freshness()).not.toBeInTheDocument()
    expect(chip()).not.toBeInTheDocument()
    // The list and the map itself are untouched.
    expect(screen.getByTestId('scene-map-canvas')).toBeInTheDocument()
    expect(screen.getByText('Browse the map as a list')).toBeInTheDocument()
  })

  it('tells the canvas a run is live, and hands it the timeline', () => {
    renderCard()
    expect(canvasProps.current!.replay).toBeTruthy()
    expect(canvasProps.current!.isReplayActive).toBe(false)

    fireEvent.click(chip()!)

    expect(canvasProps.current!.isReplayActive).toBe(true)
    const replayProp = canvasProps.current!.replay as { timeline: unknown; readFrame: unknown }
    expect(replayProp.timeline).toBeTruthy()
    expect(typeof replayProp.readFrame).toBe('function')
  })

  it('hands the canvas no replay layer below the breakpoint', () => {
    renderCard({ canvasWidth: null })
    expect(canvasProps.current).toBeNull()
  })

  it('dims the isolate band for the duration of the run', () => {
    renderCard()
    const band = screen.getByText(/not yet connected/).closest('p')!
    expect(band.className).not.toContain('opacity-45')

    fireEvent.click(chip()!)

    expect(screen.getByText(/not yet connected/).closest('p')!.className).toContain('opacity-45')
  })

  it('autoplays without any further input', () => {
    renderCard()
    fireEvent.click(chip()!)

    const before = scrubber()!.getAttribute('aria-valuenow')
    runFrames(20, 500)

    expect(scrubber()!.getAttribute('aria-valuenow')).not.toBe(before)
  })
})

describe('the transport', () => {
  it('pauses and resumes', () => {
    renderCard()
    fireEvent.click(chip()!)
    runFrames(5, 500)

    fireEvent.click(screen.getByRole('button', { name: /pause the replay/i }))
    const held = scrubber()!.getAttribute('aria-valuenow')
    runFrames(10, 500)
    expect(scrubber()!.getAttribute('aria-valuenow')).toBe(held)

    fireEvent.click(screen.getByRole('button', { name: /resume the replay/i }))
    runFrames(10, 500)
    expect(scrubber()!.getAttribute('aria-valuenow')).not.toBe(held)
  })

  it('reports the date and what is on the map, and updates as the run advances', () => {
    renderCard()
    fireEvent.click(chip()!)
    runFrames(1)

    const readout = screen.getByRole('button', { name: /pause the replay/i })
    expect(readout.textContent).toMatch(/20\d\d · [\d,]+ on the map/)

    const early = readout.textContent
    runFrames(60, 400)
    expect(readout.textContent).not.toBe(early)
  })

  it('seeks on a drag anywhere along the track, and holds the clock while dragging', () => {
    renderCard()
    fireEvent.click(chip()!)
    const track = scrubber()!
    // jsdom lays nothing out and implements no pointer capture.
    track.getBoundingClientRect = () =>
      ({ left: 0, width: 200, top: 0, height: 24, right: 200, bottom: 24, x: 0, y: 0, toJSON: () => ({}) }) as DOMRect
    track.setPointerCapture = vi.fn()
    track.releasePointerCapture = vi.fn()
    track.hasPointerCapture = vi.fn(() => true)

    fireEvent.pointerDown(track, { clientX: 150, pointerId: 1 })
    expect(track.getAttribute('aria-valuenow')).toBe('75')

    // The clock must not advance out from under the pointer mid-drag.
    runFrames(10, 500)
    expect(track.getAttribute('aria-valuenow')).toBe('75')

    fireEvent.pointerMove(track, { clientX: 40, pointerId: 1 })
    expect(track.getAttribute('aria-valuenow')).toBe('20')

    // Released: playback picks up from where the drag left it.
    fireEvent.pointerUp(track, { clientX: 40, pointerId: 1 })
    runFrames(10, 500)
    expect(Number(track.getAttribute('aria-valuenow'))).toBeGreaterThan(20)
  })

  it('seeks from the keyboard, so the run is operable without a pointer', () => {
    renderCard()
    fireEvent.click(chip()!)
    const track = scrubber()!

    fireEvent.keyDown(track, { key: 'End' })
    expect(track.getAttribute('aria-valuenow')).toBe('100')

    fireEvent.keyDown(track, { key: 'Home' })
    expect(track.getAttribute('aria-valuenow')).toBe('0')

    fireEvent.keyDown(track, { key: 'PageUp' })
    expect(Number(track.getAttribute('aria-valuenow'))).toBeGreaterThan(0)
  })

  it('describes its position for assistive technology', () => {
    renderCard()
    fireEvent.click(chip()!)
    runFrames(1)

    const track = scrubber()!
    expect(track).toHaveAttribute('aria-valuemin', '0')
    expect(track).toHaveAttribute('aria-valuemax', '100')
    expect(track.getAttribute('aria-valuetext')).toMatch(/on the map/)
  })
})

describe('leaving a run', () => {
  it('settles back into the at-rest map on reaching now', () => {
    renderCard()
    fireEvent.click(chip()!)

    // The whole 25s run, then the settle. 100ms a frame because the loop caps a
    // single frame's advance at that — a backgrounded tab hands back deltas of
    // seconds, and an uncapped one would skip most of the history in one frame.
    runFrames(320, 100)

    expect(scrubber()).not.toBeInTheDocument()
    expect(freshness()).toBeInTheDocument()
    expect(chip()).toBeInTheDocument()
    expect(canvasProps.current!.isReplayActive).toBe(false)
  })

  it('restores the at-rest map from the close chip, mid-run', () => {
    renderCard()
    fireEvent.click(chip()!)
    runFrames(5, 500)

    fireEvent.click(screen.getByRole('button', { name: /close the replay/i }))
    runFrames(10, 1000)

    expect(scrubber()).not.toBeInTheDocument()
    expect(freshness()).toBeInTheDocument()
    expect(screen.getByText(/not yet connected/).closest('p')!.className).not.toContain(
      'opacity-45',
    )
  })

  it('restores the at-rest map on Escape, mid-run', () => {
    renderCard()
    fireEvent.click(chip()!)
    runFrames(5, 500)

    fireEvent.keyDown(window, { key: 'Escape' })
    runFrames(10, 1000)

    expect(scrubber()).not.toBeInTheDocument()
    expect(freshness()).toBeInTheDocument()
  })

  it('defers Escape to anything that already handled it, so an open panel wins', () => {
    renderCard()
    fireEvent.click(chip()!)
    runFrames(5, 500)

    // What the graph inspector panels do: Radix dismisses in the capture phase
    // and preventDefaults, so the run must not also close.
    const event = new KeyboardEvent('keydown', { key: 'Escape', cancelable: true, bubbles: true })
    event.preventDefault()
    act(() => {
      window.dispatchEvent(event)
    })
    runFrames(3, 100)

    expect(scrubber()).toBeInTheDocument()
  })

  it('ignores Escape when no run is in flight', () => {
    renderCard()

    fireEvent.keyDown(window, { key: 'Escape' })

    expect(chip()).toBeInTheDocument()
    expect(freshness()).toBeInTheDocument()
  })

  it('can be run again after settling', () => {
    renderCard()
    fireEvent.click(chip()!)
    fireEvent.click(screen.getByRole('button', { name: /close the replay/i }))
    runFrames(10, 1000)

    fireEvent.click(chip()!)

    expect(scrubber()).toBeInTheDocument()
    expect(scrubber()!.getAttribute('aria-valuenow')).toBe('0')
  })
})
