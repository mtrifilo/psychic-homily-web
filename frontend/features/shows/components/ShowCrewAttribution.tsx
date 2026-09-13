'use client'

import { useId, useMemo } from 'react'
import Link from 'next/link'
import { cn } from '@/lib/utils'
import { entityHref } from '@/lib/entity-slug'
// Deep-imported, not through the `@/features/tags` barrel — the note in
// features/shows/components/index.ts explains what a barrel costs here.
import { useEntityTags } from '@/features/tags/hooks'
import {
  compareEntityTagsByConfidence,
  isCrewTagCategory,
  CREW_CHIP_CLASS,
  CREW_CHIP_LINK_CLASS,
} from '@/features/tags/types'

/**
 * The row's type register per Figma `1666:2`: mono micro-caps on the muted
 * tone, label and chips at one size. 11px is the site's crew-chip density.
 */
const CREW_ROW_TEXT_CLASS = 'font-mono text-[11px] uppercase tracking-[0.04em]'

/**
 * The shared crew chip at this page's density; everything but the size is the
 * crew tag's own treatment.
 */
const SHOW_CREW_CHIP_CLASS = cn(CREW_CHIP_CLASS, 'text-[11px]')
const SHOW_CREW_CHIP_LINK_CLASS = cn(CREW_CHIP_LINK_CLASS, 'text-[11px]')

/**
 * The bookers this listing is credited to: the show's crew-category tags,
 * drawn as the first row of the tags and provenance footer per Figma `1666:2`.
 *
 * A crew is a TAG, not an entity, so a chip lands on `/tags/{slug}`; a crew
 * whose slug cannot address one is still NAMED rather than dropped, because
 * the name is what the chip is for.
 *
 * The row is absent entirely — no label, no scaffold — on a show carrying no
 * crew tag, which is most of them. It reads the same query key the tag list
 * below it reads, so the pair costs one request.
 *
 * `role="list"` is explicit on the chip list because the label depends on it:
 * a list whose markers are off loses its list semantics in WebKit, and a
 * generic element drops `aria-labelledby` with them. The label element carries
 * the copy in sentence case and is uppercased in CSS, so a screen reader says
 * the phrase rather than spelling it.
 */
export function ShowCrewAttribution({ showId }: { showId: number }) {
  const { data } = useEntityTags('show', showId)
  const labelId = useId()

  // A nameless crew is dropped rather than drawn: a chip with no accessible
  // name is an empty box in the tab order.
  //
  // Sorted, because `ListEntityTags` returns rows in no defined order — the
  // same ranking the tag list below applies to this payload, so the two rows
  // cannot disagree about which of two crews leads.
  const crews = useMemo(
    () =>
      (data?.tags ?? [])
        .filter(tag => isCrewTagCategory(tag.category) && tag.name.trim())
        .sort(compareEntityTagsByConfidence),
    [data?.tags]
  )

  if (!crews.length) return null

  return (
    <div
      className="flex flex-wrap items-center gap-x-2 gap-y-1.5"
      data-testid="show-crew-attribution"
    >
      <span id={labelId} className={cn(CREW_ROW_TEXT_CLASS, 'text-muted-foreground')}>
        Presented by
      </span>
      <ul
        role="list"
        aria-labelledby={labelId}
        className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1.5"
      >
        {crews.map(crew => {
          const href = entityHref('/tags', crew.slug)
          return (
            <li key={crew.tag_id}>
              {href ? (
                <Link href={href} className={SHOW_CREW_CHIP_LINK_CLASS}>
                  {crew.name}
                </Link>
              ) : (
                <span className={SHOW_CREW_CHIP_CLASS}>{crew.name}</span>
              )}
            </li>
          )
        })}
      </ul>
    </div>
  )
}
