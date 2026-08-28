/**
 * Quoting a field note on a surface that is NOT the note's own card
 * (PSY-1590) — today the Atlas venue panel's teaser, tomorrow whatever else
 * wants one.
 *
 * This lives in `features/comments` rather than beside its first caller
 * because everything here is field-note DISPLAY POLICY, not panel chrome: what
 * may be quoted out of context, and how a note's Markdown body becomes one
 * line of prose. Its counterpart — FieldNoteCard's spoiler gate — is in this
 * same feature, and a second copy of that rule living in a city-map module is
 * exactly how the two drift until a note the author asked to hide gets quoted
 * somewhere.
 */

import type { Comment, VenueFieldNote } from './types'

/**
 * Whether a note's author flagged it as containing setlist spoilers.
 *
 * One reader for the whole app: `FieldNoteCard` renders such a note behind a
 * click-to-reveal gate, and any surface that cannot offer that gate must not
 * quote the note at all. Both readers must agree on the predicate forever, so
 * there is exactly one.
 */
export function isSetlistSpoiler(note: Pick<Comment, 'structured_data'>): boolean {
  return note.structured_data?.setlist_spoiler === true
}

/**
 * Flatten a field note's Markdown SOURCE into one line of readable prose.
 *
 * Teasers quote `body`, not the backend's rendered `body_html`: dropping
 * sanitized HTML into a surface like the Atlas — which has no route-level
 * error boundary — to show three clamped lines is not a trade worth making.
 * But `body` is raw author input, so without this a note written as
 * `**Loudest set**` would render its asterisks.
 *
 * Deliberately NOT a Markdown parser. The rules below are conservative on
 * purpose, because the real hazard is the opposite of the obvious one: this
 * function's input is mostly ORDINARY PROSE, not Markdown, and an eager
 * emphasis rule DELETES characters from someone's verbatim words rather than
 * leaving a stray marker behind. `lo_fi_house` must not become `lofihouse`,
 * and a URL like `https://ex.com/a_b_c` must survive intact. Hence the
 * word-boundary anchors on the single-character markers: CommonMark does not
 * treat intraword `_` as emphasis either, so honouring it was simply wrong.
 */
export function fieldNoteTeaserText(body: string): string {
  return (
    body
      // Tags first. `body` is raw author input — the backend sanitizes
      // `body_html`, never `body` — so literal HTML would otherwise render its
      // angle brackets. Not an XSS fix (React escapes the text child either
      // way); it stops a passive panel from displaying `<b>loudest</b>`.
      .replace(/<[^>]*>/g, ' ')
      // Images before links: `![alt](src)` is a link pattern with a prefix, so
      // the link rule would otherwise eat it and leave a bare `!`.
      .replace(/!\[([^\]]*)\]\([^)]*\)/g, '$1')
      .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
      // Paired double markers are unambiguous — no boundary needed.
      .replace(/\*\*([^*]+)\*\*/g, '$1')
      .replace(/__([^_]+)__/g, '$1')
      .replace(/~~([^~]+)~~/g, '$1')
      // Single markers only when they open and close at a word boundary, so
      // intraword underscores and free-standing bullet asterisks survive.
      .replace(/(^|[\s([{])\*(?!\s)([^*]*[^\s*])\*(?=[\s)\]}.,!?;:]|$)/g, '$1$2')
      .replace(/(^|[\s([{])_(?!\s)([^_]*[^\s_])_(?=[\s)\]}.,!?;:]|$)/g, '$1$2')
      .replace(/`([^`]+)`/g, '$1')
      // Line-leading block markers: headings, quotes, bullets, ordered items.
      .replace(/^[ \t]*#{1,6}[ \t]+/gm, '')
      .replace(/^[ \t]*>[ \t]?/gm, '')
      .replace(/^[ \t]*[-*+][ \t]+/gm, '')
      .replace(/^[ \t]*\d+\.[ \t]+/gm, '')
      // A teaser is a single clamped run, so paragraph breaks become spaces
      // rather than vanishing and running two sentences together.
      .replace(/\s+/g, ' ')
      .trim()
  )
}

/** A note chosen for quoting, paired with the prose actually rendered. */
export interface FieldNoteTeaserPick {
  note: VenueFieldNote
  /** Non-empty by construction — the pick rejects notes that flatten away. */
  text: string
}

/**
 * Choose which note a teaser quotes, and hand back the prose with it.
 *
 * The list arrives best-first from the backend, so this is a filter for
 * "quotable HERE", never a re-ranking. Returning the derived text alongside
 * the note is deliberate: the caller then holds no second copy of the
 * "non-empty body" invariant and does not recompute the flattening.
 *
 * Two skips:
 *
 *   - **Setlist spoilers.** Belt and braces. The venue rollup already excludes
 *     them server-side (the body should never reach a surface that cannot gate
 *     it), but a teaser fed from any other endpoint must not depend on that.
 *   - **Notes whose body flattens to nothing**, e.g. a body that was only
 *     markup.
 *
 * Note what is deliberately NOT a skip: an empty `show_title`. Most shows
 * carry no title of their own, and the caller names them from the bill via
 * `showDisplayTitle`. Dropping untitled shows would discard most of the
 * content this feature exists to surface.
 *
 * Returns null when nothing on the page is quotable; the caller then renders
 * no section at all.
 */
export function pickFieldNoteForTeaser(
  notes: readonly VenueFieldNote[] | null | undefined,
): FieldNoteTeaserPick | null {
  if (!notes) return null
  for (const note of notes) {
    if (isSetlistSpoiler(note)) continue
    const text = fieldNoteTeaserText(note.body ?? '')
    if (!text) continue
    return { note, text }
  }
  return null
}
