/**
 * Geometry for keeping a selected scene dot in view above the scene preview's
 * bottom sheet in the Atlas sheet layout: the sheet's height, the camera
 * padding that height becomes, and whether a dot clears the sheet. A camera
 * padded by the sheet's height draws the map's centre (and, at globe zoom,
 * the globe itself) in the band of the pane the sheet leaves visible.
 * GlobeCanvas's preview camera effect states and applies the rule.
 */

import { bottomSheetHeightPx, type BottomSheetDetent } from '@/components/ui/bottom-sheet'
import { dotBox, inside } from './basemap/globePlaces'
import { ATLAS_SHEET_TOP_INSET_PX } from './cityView'

/**
 * The height, in CSS px, the scene preview sheet covers at the bottom of an
 * Atlas pane `paneHeightPx` tall when it stands at `detent`: the sheet's own
 * detent rule with the Atlas's top inset. It is the camera's bottom padding
 * while the preview is open.
 */
export function scenePreviewSheetPx(detent: BottomSheetDetent, paneHeightPx: number): number {
  return bottomSheetHeightPx(detent, paneHeightPx, ATLAS_SHEET_TOP_INSET_PX)
}

/** The camera padding for a bottom sheet `sheetPx` tall. */
export function scenePreviewCameraPadding(sheetPx: number): {
  top: number
  right: number
  bottom: number
  left: number
} {
  return { top: 0, right: 0, bottom: sheetPx, left: 0 }
}

/**
 * Whether a dot drawn at `point` with radius `radiusPx` lies wholly inside the
 * part of a `width` x `height` canvas that a bottom sheet `sheetPx` tall leaves
 * uncovered. `point` is null for a dot that is not drawn on the visible side
 * of the globe or inside the canvas.
 */
export function dotClearOfSheet({
  point,
  radiusPx,
  width,
  height,
  sheetPx,
}: {
  point: { x: number; y: number } | null
  radiusPx: number
  width: number
  height: number
  sheetPx: number
}): boolean {
  return (
    point !== null &&
    inside(dotBox(point.x, point.y, radiusPx), {
      left: 0,
      top: 0,
      right: width,
      bottom: height - sheetPx,
    })
  )
}
