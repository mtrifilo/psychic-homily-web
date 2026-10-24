'use client'

import { createContext, useContext } from 'react'
import type {
  HomeCityLinkSlot,
  HomeDiscoverLinksSlot,
  HomeLinkSlots,
} from '../sections'

/**
 * Which surface owns each relocating link row this render.
 *
 * Both rows belong to the nearby section. Hiding that section would otherwise
 * take them with it, so ownership falls back to the saved-shows footer and
 * then to the toolbar row (and, for the Discover row, to the empty-state
 * line). Every surface that can carry a row reads the same value, so "exactly
 * one renders it" is enforced rather than coordinated.
 *
 * Defaults to the nearby section, which is where both rows live on an
 * untouched layout and on any surface that mounts these sections without the
 * layout runtime.
 */
const HomeLinkSlotsContext = createContext<HomeLinkSlots>({
  cityLink: 'nearby',
  discoverLinks: 'nearby',
})

export const HomeLinkSlotsProvider = HomeLinkSlotsContext.Provider

export function useHomeCityLinkSlot(): HomeCityLinkSlot {
  return useContext(HomeLinkSlotsContext).cityLink
}

export function useHomeDiscoverLinksSlot(): HomeDiscoverLinksSlot {
  return useContext(HomeLinkSlotsContext).discoverLinks
}
