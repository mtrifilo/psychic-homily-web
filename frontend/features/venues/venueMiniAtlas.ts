/**
 * The `/venues` mini Atlas's view model: which rows pin, where, and how big.
 *
 * Pure (no React, no MapLibre) so the rules unit-test without WebGL, and a
 * leaf import so the page that decides whether to render the pane does not
 * pull the map library in to ask.
 */

import { venuePinPosition } from '@/features/scenes/venuePinPosition'
import type { VenueWithShowCount } from './types'

/** The pane's size, measured from the approved frame (Venues page 1721:36). */
export const MINI_ATLAS_WIDTH_PX = 368
export const MINI_ATLAS_HEIGHT_PX = 400

/** Padding held between the fitted pins and the pane's edges, in CSS px. */
export const MINI_ATLAS_FIT_PADDING_PX = 28

/**
 * How far in the fit may go. A city whose rooms sit on one block would
 * otherwise land at street-level zoom, where the pane shows four buildings and
 * says nothing about where the room is in its city.
 */
export const MINI_ATLAS_MAX_FIT_ZOOM = 14

/** One room as the map draws it. */
export interface MiniAtlasPin {
  id: number
  name: string
  lng: number
  lat: number
  upcomingShowCount: number
}

/** A south-west / north-east corner pair, in [lng, lat]. */
export type MiniAtlasBounds = [[number, number], [number, number]]

/**
 * The rows that can be drawn, in row order.
 *
 * Position comes from `venuePinPosition` and nowhere else: street coordinates
 * when the API served them, the city centroid otherwise. The two are NOT drawn
 * differently — which coordinate source a pin came from is a privacy
 * consequence, not something a reader is being told about the room.
 *
 * A row with no coordinates at all is dropped. It still lists in the table:
 * the pane maps what it can, and the table is the complete list.
 */
export function miniAtlasPins(
  venues: readonly VenueWithShowCount[],
): MiniAtlasPin[] {
  const pins: MiniAtlasPin[] = []
  for (const venue of venues) {
    const position = venuePinPosition(venue)
    if (!position) continue
    pins.push({
      id: venue.id,
      name: venue.name,
      lng: position.lng,
      lat: position.lat,
      upcomingShowCount: venue.upcoming_show_count,
    })
  }
  return pins
}

/** The box that holds every pin, or null when there is nothing to hold. */
export function miniAtlasBounds(
  pins: readonly MiniAtlasPin[],
): MiniAtlasBounds | null {
  if (pins.length === 0) return null
  let west = pins[0].lng
  let east = pins[0].lng
  let south = pins[0].lat
  let north = pins[0].lat
  for (const pin of pins) {
    if (pin.lng < west) west = pin.lng
    if (pin.lng > east) east = pin.lng
    if (pin.lat < south) south = pin.lat
    if (pin.lat > north) north = pin.lat
  }
  return [
    [west, south],
    [east, north],
  ]
}

/**
 * What the pane tells a screen reader.
 *
 * The canvas is hidden from assistive tech and every room on it is a row in
 * the table beside it, so this says how much of the list is mapped rather than
 * trying to narrate a map. `mapped` can trail `total` when a room has no
 * coordinates, and saying so is the honest version of an incomplete map.
 */
export function miniAtlasSummary(
  mapped: number,
  total: number,
  cityLabel: string,
): string {
  const rooms = mapped === 1 ? 'room' : 'rooms'
  const scope = mapped === total ? '' : ` of the ${total} listed`
  return `Map of ${mapped} ${rooms}${scope} in ${cityLabel}. Every room on it is a row in the table beside it.`
}
