import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, screen } from '@testing-library/react'

import { renderWithProviders } from '@/test/utils'

import type { SceneMap, SceneMapNode } from '../sceneMap'
import type { SceneReplayFrame } from '../useSceneReplay'
import { useSceneReplay } from '../useSceneReplay'
import { SceneMapZeroState } from './SceneMapZeroState'

/**
 * The reduced-motion treatment for the growth replay (PSY-1743).
 *
 * The decided behaviour: `prefers-reduced-motion` visitors get the SAME replay
 * surface, opened settled at now and paused, with the full seek still available
 * — nothing moves unless they drag it. Standard motion is untouched, and the
 * last case here is what proves that rather than assumes it.
 *
 * Driven through the real transport and the real card, like `SceneMapReplay`'s
 * suite, because the branch is only meaningful in terms of what the card ends up
 * showing. The media query itself is stubbed at the hook: `useReducedMotion`
 * reads `matchMedia` through `useSyncExternalStore`, and jsdom's `matchMedia`
 * stub answers `false` to everything.
 */
const { canvasProps, motion } = vi.hoisted(() => ({
  canvasProps: { current: null as Record<string, unknown> | null },
  motion: { reduced: false },
}))

vi.mock('@/features/artists/hooks/useReducedMotion', () => ({
  useReducedMotion: () => motion.reduced,
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
    // Wrapped even though a reduced-motion run's frames change no React state:
    // the standard-motion guard at the bottom of this file does play, and an
    // unwrapped frame that reached a phase change would warn rather than fail.
    act(() => {
      for (const callback of callbacks) callback(now)
    })
  }
}

beforeEach(() => {
  scheduled = []
  now = 1000
  canvasProps.current = null
  motion.reduced = true
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
    scheduled.push(callback)
    return scheduled.length
  })
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
    homeCaption: null,
    appear,
  }
}

function sceneMap(appears: number[]): SceneMap {
  return {
    nodes: appears.map((appear, index) => node(index + 1, appear)),
    edges: [],
    regions: [
      { community: 1, label: 'Around Artist 1', memberCount: 2, hull: [], captionAnchor: null },
    ],
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

function Harness() {
  const replay = useSceneReplay(REPLAYABLE)
  return (
    <SceneMapZeroState
      map={REPLAYABLE}
      canvasWidth={1024}
      onSelectArtist={() => {}}
      replay={replay}
    />
  )
}

const chip = () => screen.queryByRole('button', { name: /watch it grow/i })
const scrubber = () => screen.queryByRole('slider', { name: /replay position/i })
const freshness = () => screen.queryByText(/Mapped nightly/)
const playControl = () =>
  screen.queryByRole('button', { name: /(pause|resume) the replay/i })

/** The frame the canvas is actually drawing from, read the way the canvas reads it. */
function canvasFrame(): SceneReplayFrame {
  const replay = canvasProps.current!.replay as { readFrame: () => SceneReplayFrame }
  return replay.readFrame()
}

function startReplay() {
  renderWithProviders(<Harness />)
  fireEvent.click(chip()!)
}

describe('the growth replay under prefers-reduced-motion (PSY-1743)', () => {
  it('still offers the run: the chip is present and opens the scrubber', () => {
    startReplay()

    expect(scrubber()).toBeInTheDocument()
    expect(freshness()).not.toBeInTheDocument()
    expect(canvasProps.current!.isReplayActive).toBe(true)
  })

  it('opens settled at now instead of autoplaying from the beginning', () => {
    startReplay()

    expect(scrubber()!.getAttribute('aria-valuenow')).toBe('100')
    // The clock is handed plenty of frames and moves the playhead nowhere.
    runFrames(40, 500)
    expect(scrubber()!.getAttribute('aria-valuenow')).toBe('100')
  })

  it('clears the map furniture with no crossfade to sit through', () => {
    startReplay()

    // Frame zero, before the loop has ticked at all: already where the 400ms
    // enter fade would have ended.
    expect(canvasFrame().decorationAlpha).toBe(0)
    expect(canvasFrame().active).toBe(true)
    expect(canvasFrame().progress).toBe(1)
  })

  it('drops the play control, because there is nothing to play or pause', () => {
    startReplay()

    expect(playControl()).not.toBeInTheDocument()
    // The readout it normally carries is still on the strip.
    expect(scrubber()!.parentElement!.textContent).toMatch(/20\d\d · [\d,]+ on the map/)
    // And so is the way out.
    expect(screen.getByRole('button', { name: /close the replay/i })).toBeInTheDocument()
  })

  it('keeps the full seek — the playhead moves where the visitor puts it', () => {
    startReplay()
    const track = scrubber()!

    fireEvent.keyDown(track, { key: 'Home' })
    expect(track.getAttribute('aria-valuenow')).toBe('0')

    fireEvent.keyDown(track, { key: 'PageUp' })
    expect(Number(track.getAttribute('aria-valuenow'))).toBeGreaterThan(0)

    // A seek is the ONLY thing that moves it: the clock still does not advance
    // away from where the drag left it.
    fireEvent.keyDown(track, { key: 'End' })
    runFrames(20, 500)
    expect(track.getAttribute('aria-valuenow')).toBe('100')
  })

  it('seeks on a drag, and the run does not resume when the drag is released', () => {
    startReplay()
    const track = scrubber()!
    // jsdom lays nothing out and implements no pointer capture.
    track.getBoundingClientRect = () =>
      ({
        left: 0,
        width: 200,
        top: 0,
        height: 24,
        right: 200,
        bottom: 24,
        x: 0,
        y: 0,
        toJSON: () => ({}),
      }) as DOMRect
    track.setPointerCapture = vi.fn()
    track.releasePointerCapture = vi.fn()
    track.hasPointerCapture = vi.fn(() => true)

    fireEvent.pointerDown(track, { clientX: 80, pointerId: 1 })
    expect(track.getAttribute('aria-valuenow')).toBe('40')

    fireEvent.pointerUp(track, { clientX: 80, pointerId: 1 })
    runFrames(20, 500)
    expect(track.getAttribute('aria-valuenow')).toBe('40')
  })

  it('returns to the at-rest map with no settle sweep to watch', () => {
    startReplay()
    const track = scrubber()!
    fireEvent.keyDown(track, { key: 'Home' })

    fireEvent.click(screen.getByRole('button', { name: /close the replay/i }))

    // No frames run: the at-rest map is back already, rather than 600ms of the
    // rest of the history sweeping in.
    expect(scrubber()).not.toBeInTheDocument()
    expect(freshness()).toBeInTheDocument()
    expect(chip()).toBeInTheDocument()
    expect(canvasProps.current!.isReplayActive).toBe(false)
    expect(canvasFrame().active).toBe(false)
    expect(canvasFrame().decorationAlpha).toBe(1)
  })

  it('leaves on Escape the same way, and can be opened again afterwards', () => {
    startReplay()

    fireEvent.keyDown(window, { key: 'Escape' })
    expect(scrubber()).not.toBeInTheDocument()

    fireEvent.click(chip()!)
    expect(scrubber()!.getAttribute('aria-valuenow')).toBe('100')
  })

  // The other half of the acceptance criterion: the branch above must not have
  // moved anything for everybody else.
  it('leaves standard motion autoplaying from the beginning, with its play control', () => {
    motion.reduced = false
    startReplay()

    expect(scrubber()!.getAttribute('aria-valuenow')).toBe('0')
    expect(playControl()).toBeInTheDocument()

    runFrames(20, 500)
    expect(Number(scrubber()!.getAttribute('aria-valuenow'))).toBeGreaterThan(0)
  })
})
