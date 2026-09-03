'use client'

import { useEffect } from 'react'
import { reportPlantedTicketTag } from './plantedTagTelemetry'
import type { TicketTagEntityType } from './plantedTagTelemetry'
import type { PlantedTicketTag } from './ticketVendors'

/**
 * Reports a planted affiliate tag once the page carrying it is on a reader's
 * screen. NOT once the link is: `ticketOffer` withholds the anchor for a
 * planted tag, so this fires precisely when there is no link to see.
 *
 * An EFFECT, not a call during render, for the ordinary reason: reporting is a
 * side effect, and render runs twice under StrictMode and again on every
 * re-render. It also puts the report on the client, so the signal counts pages
 * that were really served rather than every prerender and cache warm.
 *
 * The dedupe that makes this safe is in `plantedTagTelemetry` and is keyed by
 * row, not by component instance: remounting this hook, or rendering two links
 * to the same row, still reports once.
 *
 * Deps are the primitive fields rather than the tag object, which is rebuilt
 * on every render by `ticketLink` and would otherwise re-fire the effect
 * forever.
 */
export function usePlantedTicketTagReport(
  entityType: TicketTagEntityType,
  entityId: number | string,
  tag: PlantedTicketTag | null | undefined
): void {
  const param = tag?.param
  const host = tag?.host
  const matchesConfiguredPartner = tag?.matchesConfiguredPartner ?? false

  useEffect(() => {
    if (!param || !host) return
    reportPlantedTicketTag({
      entityType,
      entityId,
      tag: { param, host, matchesConfiguredPartner },
    })
  }, [entityType, entityId, param, host, matchesConfiguredPartner])
}
