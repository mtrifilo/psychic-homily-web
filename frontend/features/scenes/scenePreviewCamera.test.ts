import { describe, it, expect } from 'vitest'
import {
  dotClearOfSheet,
  scenePreviewCameraPadding,
  scenePreviewSheetPx,
} from './scenePreviewCamera'

describe('scenePreviewSheetPx', () => {
  // The Atlas pane on a 390x844 phone, below the top bar and above the tab
  // bar, measured in the browser.
  it('gives the phone pane Peek 120, Half 45% of the pane and Full the pane less its top strip', () => {
    expect(scenePreviewSheetPx('peek', 731)).toBe(120)
    expect(scenePreviewSheetPx('half', 731)).toBeCloseTo(328.95, 2)
    expect(scenePreviewSheetPx('full', 731)).toBe(731 - 112)
  })

  it('caps Half at 400 and Full at 660 on a tall pane', () => {
    expect(scenePreviewSheetPx('half', 1000)).toBe(400)
    expect(scenePreviewSheetPx('full', 1000)).toBe(660)
  })

  it('gives Half the height of Full on a pane too short for a Half step above Peek', () => {
    // 45% of 360 is 162, under Peek's 120 plus the 48px minimum step.
    expect(scenePreviewSheetPx('half', 360)).toBe(scenePreviewSheetPx('full', 360))
    expect(scenePreviewSheetPx('full', 360)).toBe(360 - 112)
  })

  it('never covers the top strip, even at Peek on a very short pane', () => {
    expect(scenePreviewSheetPx('peek', 200)).toBe(88)
    expect(scenePreviewSheetPx('full', 100)).toBe(0)
  })
})

describe('scenePreviewCameraPadding', () => {
  it('pads only the bottom edge, by the sheet height', () => {
    expect(scenePreviewCameraPadding(328.95)).toEqual({
      top: 0,
      right: 0,
      bottom: 328.95,
      left: 0,
    })
    expect(scenePreviewCameraPadding(0)).toEqual({ top: 0, right: 0, bottom: 0, left: 0 })
  })
})

describe('dotClearOfSheet', () => {
  const pane = { width: 390, height: 731, radiusPx: 6 }

  it('is clear when the whole dot lies above the sheet edge', () => {
    // The Half sheet's edge is at 731 - 329 = 402.
    expect(dotClearOfSheet({ ...pane, point: { x: 195, y: 395 }, sheetPx: 329 })).toBe(true)
  })

  it('is not clear when any part of the dot reaches under the sheet', () => {
    expect(dotClearOfSheet({ ...pane, point: { x: 195, y: 397 }, sheetPx: 329 })).toBe(false)
    expect(dotClearOfSheet({ ...pane, point: { x: 195, y: 655 }, sheetPx: 329 })).toBe(false)
  })

  it('is not clear when the dot leaves the canvas at a side or the top', () => {
    expect(dotClearOfSheet({ ...pane, point: { x: 3, y: 200 }, sheetPx: 120 })).toBe(false)
    expect(dotClearOfSheet({ ...pane, point: { x: 388, y: 200 }, sheetPx: 120 })).toBe(false)
    expect(dotClearOfSheet({ ...pane, point: { x: 195, y: 2 }, sheetPx: 120 })).toBe(false)
  })

  it('is not clear for a dot that is not drawn', () => {
    expect(dotClearOfSheet({ ...pane, point: null, sheetPx: 0 })).toBe(false)
  })
})
