/**
 * ============================================================================
 * TODO(owner): POLICY COPY GOES HERE — DO NOT LET AN AGENT FILL THIS IN.
 * ============================================================================
 *
 * `AI_POLICY_COPY` below is the ONLY place the /ai-policy prose lives. page.tsx
 * renders whatever is in it and nothing else; there is no copy embedded in the
 * JSX, the metadata, or the footer.
 *
 * The page asserts that Psychic Homily contains no AI-generated writing, so
 * AI-drafted prose on this page would be self-refuting. Every `null` slot is
 * deliberately unwritten and must be filled by a human.
 *
 * WHILE ANY SLOT IS NULL the page holds itself back:
 *   - `metadata.robots` is `index: false, follow: false` (page.tsx)
 *   - the route is omitted from the sitemap (app/sitemap.ts)
 *   - a loud banner renders above the content, and each empty section renders
 *     `[POLICY COPY PENDING — see PSY-1583]` instead of prose
 * All three flip automatically the moment the last slot is filled — there is no
 * separate flag to remember. `isAiPolicyCopyPending()` is the single source of
 * that truth.
 *
 * TO PUBLISH: replace every `null` in `AI_POLICY_COPY` with real text. Nothing
 * else needs touching.
 *
 * The headings are scaffolding, not copy — rename or reorder them freely. They
 * exist so the three disclosures the page owes its readers each have a slot:
 *   1. no AI-generated music, artwork, or written content on the site
 *   2. what AI IS used for — reading and structuring publicly-posted venue
 *      calendars into show listings (extraction, not generation)
 *   3. that entity data is human-verified
 *
 * How specific the disclosure in (2) should be — naming the model and pipeline
 * versus describing the category of use — is an open owner decision recorded on
 * PSY-1583. It is a copy decision, so it is answered here, not in code.
 */

/** The canonical path. Imported by the footer and the sitemap — do not inline. */
export const AI_POLICY_PATH = '/ai-policy'

/** The page's `<h1>`. Structural, but rename it if the copy wants another name. */
export const AI_POLICY_TITLE = 'AI Policy'

/** Text rendered in place of any slot that is still `null`. */
export const COPY_PENDING_PLACEHOLDER = '[POLICY COPY PENDING — see PSY-1583]'

/**
 * A block of prose. One string per paragraph. `null` means "not written yet"
 * and is what holds the page back from being indexed.
 */
export type CopySlot = readonly string[] | null

export interface AiPolicySection {
  /** Stable anchor id, so a section can be linked and quoted on its own. */
  readonly id: string
  readonly heading: string
  readonly body: CopySlot
}

export interface AiPolicyCopy {
  /** The `<meta name="description">` / social-preview line. */
  readonly description: string | null
  /** Lead paragraphs, above the sections. */
  readonly intro: CopySlot
  /**
   * Optional. Rendered under the title when set (matching /terms and /privacy).
   * Deliberately NOT part of the pending check — a policy with copy but no date
   * is publishable; a policy with a date but no copy is not.
   */
  readonly lastUpdated: string | null
  readonly sections: readonly AiPolicySection[]
}

export const AI_POLICY_COPY: AiPolicyCopy = {
  description: null,
  intro: null,
  lastUpdated: null,
  sections: [
    {
      id: 'no-ai-generated-content',
      heading: 'No AI-generated music, artwork, or writing',
      body: null,
    },
    {
      id: 'what-ai-is-used-for',
      heading: 'What AI is used for',
      body: null,
    },
    {
      id: 'human-verification',
      heading: 'Human verification',
      body: null,
    },
  ],
}

/**
 * A slot counts as unwritten if it is absent, empty, or only whitespace.
 *
 * Exported so page.tsx decides "show the placeholder here" with the exact same
 * rule that decides "hold the page back". Two spellings of blankness would let
 * `body: []` render an empty section under a heading while the page still
 * called itself published.
 */
export function isCopySlotBlank(slot: CopySlot | string | null): boolean {
  if (slot === null) return true
  if (typeof slot === 'string') return slot.trim().length === 0
  // An empty array, or one whose every paragraph is blank, would render a
  // heading with nothing under it — indistinguishable to a reader from a
  // policy that says nothing, so it is not "written".
  return slot.length === 0 || slot.every(p => p.trim().length === 0)
}

/**
 * True while any required copy slot is still unwritten.
 *
 * Takes the copy as an argument rather than reading the module constant so the
 * published and pending regimes are both reachable from a test — the pending
 * regime is the one that must never reach production, and a check that can only
 * be exercised in one state is not a check.
 *
 * Fails closed on purpose: a section added with `body: null` holds the whole
 * page back until someone writes it, rather than shipping a page with one
 * silently empty heading.
 */
export function isAiPolicyCopyPending(copy: AiPolicyCopy): boolean {
  if (isCopySlotBlank(copy.description)) return true
  if (isCopySlotBlank(copy.intro)) return true
  return copy.sections.some(section => isCopySlotBlank(section.body))
}
