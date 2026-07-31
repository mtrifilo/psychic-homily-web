/**
 * Campaign tagging for links WE post.
 *
 * With near-zero users, "which post actually worked" is the whole question, and
 * a referrer cannot answer it: every Bluesky link arrives as `bsky.app` whether
 * it was a weekly city page, a single show, or an artist. PostHog already
 * ingests `utm_*` automatically, as both event and person properties — what was
 * missing is a convention, and something that emits it so it does not depend on
 * remembering the spelling each week.
 *
 * ## The convention
 *
 *     ?utm_source=<channel>&utm_campaign=<artifact>[&utm_content=<which post>]
 *
 * Two required fields and one optional third. `utm_medium` is deliberately
 * unused — it is redundant with `utm_source` here, and a field that carries no
 * information is the one that gets spelled differently week to week.
 *
 * `utm_content` is the standard "which of several links" slot, and it is what
 * makes the second half of the question answerable: a Monday and a Thursday
 * post about the SAME week would otherwise collapse into one indistinguishable
 * row, as would r/chicago and r/indieheads. Omit it when there is only one post.
 *
 *     https://psychichomily.com/scenes/chicago-il/2026-W31
 *       ?utm_source=bluesky&utm_campaign=week-chicago-il-2026-w31&utm_content=thu
 *
 * **This is effectively permanent** — historical events cannot be retagged, so
 * changing the vocabulary later splits every comparison across the change. Add
 * a source to `CAMPAIGN_SOURCES` rather than inventing one at a call site, and
 * build campaign ids with the helpers below rather than by hand.
 *
 * ## Campaign data has exactly one reader
 *
 * Vercel Analytics cross-checks totals and referrer host, but NOT `utm_*` —
 * those dimensions return HTTP 402 on this plan. So campaign attribution comes
 * only from consent-gated PostHog, which undercounts, and undercounts tagged
 * links *specifically* (see the runbook). Read campaign numbers as a floor.
 *
 * ## What must NOT be tagged
 *
 * Links a *user* shares (PSY-1580) must go out as the canonical URL, untagged.
 * Tagging them would attribute organic word-of-mouth to whichever post the
 * sharer happened to arrive from, corrupting the one clean signal available —
 * and campaign tags survive copy-paste, so a single tagged share can poison a
 * whole chain. Only links the owner posts get tags.
 *
 * ## Known blind spots, so the numbers get read correctly
 *
 * - **PostHog undercounts, unevenly.** `canUseAnalytics` is false while consent
 *   is `null`, so visitors who merely IGNORE the banner produce no PostHog data
 *   at all — not just those who decline. Vercel Analytics is un-gated and is
 *   the cross-check (it excludes self-flagged internal traffic, PSY-1546).
 * - **Group chats are invisible.** iMessage / WhatsApp / Signal send no
 *   referrer, so the highest-intent channel lands in "direct". A working share
 *   loop will look like unattributed direct traffic, not like nothing.
 */

/**
 * Where a link was posted.
 *
 * A closed set on purpose: `bluesky` and `Bluesky` and `bsky` would be three
 * different rows in every breakdown, forever.
 */
export const CAMPAIGN_SOURCES = [
  'bluesky',
  'reddit',
  'instagram',
  'substack',
  'discord',
  'mastodon',
  // Present in the pre-push baseline (facebook.com and m.facebook.com both
  // referred traffic in the 28 days before any posting), so it needs a spelling
  // before someone invents one.
  'facebook',
] as const

export type CampaignSource = (typeof CAMPAIGN_SOURCES)[number]

/** `CampaignSource` is compile-time only; this is the runtime half. */
function isCampaignSource(value: string): value is CampaignSource {
  return (CAMPAIGN_SOURCES as readonly string[]).includes(value)
}

/** A scene slug as the backend emits it: lowercase, hyphen-separated. */
const SLUG = /^[a-z0-9]+(?:-[a-z0-9]+)*$/
/** Zero-padded ISO week key. `2026-W1` is NOT valid — the route 404s on it. */
const ISO_WEEK = /^\d{4}-W\d{2}$/i

/**
 * Campaign ids are TYPE-PREFIXED.
 *
 * Every id says what kind of thing was posted, so campaign rows stay groupable
 * by artifact type and nobody has to invent a spelling the first time a show or
 * an artist gets posted. Decided up front precisely because the alternative is
 * an ad-hoc decision made at a call site under posting pressure — and campaign
 * ids, like sources, cannot be retagged.
 *
 *     week-chicago-il-2026-w31
 *     show-ovlov-empty-bottle-2026-07-27
 *     artist-ovlov
 *
 * Each part is normalized separately: trimming the JOINED string would leave a
 * trailing space sitting in the middle of the result, producing a second
 * permanent row for the same artifact.
 */
function campaignId(type: 'week' | 'show' | 'artist', ...parts: string[]): string {
  return [type, ...parts.map(p => p.trim())].join('-').toLowerCase()
}

/**
 * Campaign id for a weekly city page.
 *
 * Uses the scene SLUG rather than the bare city, so `portland-or` and
 * `portland-me` stay distinguishable — the same ambiguity the share card itself
 * had to solve. Lowercased because analytics breakdowns are case-sensitive and
 * `2026-W31` vs `2026-w31` would split one week into two rows.
 */
export function weekCampaignId(sceneSlug: string, isoWeek: string): string {
  return campaignId('week', sceneSlug, isoWeek)
}

/** Campaign id for a single show. */
export function showCampaignId(showSlug: string): string {
  return campaignId('show', showSlug)
}

/** Campaign id for an artist page. */
export function artistCampaignId(artistSlug: string): string {
  return campaignId('artist', artistSlug)
}

/**
 * Add campaign tags to a URL we are about to post.
 *
 * Existing `utm_source` / `utm_campaign` are overwritten rather than
 * duplicated, so re-tagging is idempotent. Other query parameters survive
 * semantically but the query IS re-serialized (`?share` becomes `?share=`),
 * which is harmless for our own links and worth knowing for anything else.
 *
 * Everything is validated loudly rather than cleaned quietly. The scheme is
 * permanent and analytics cannot be retagged, so a thrown error at the moment
 * of posting is cheap and a junk row is forever. That applies to the SOURCE as
 * much as the campaign: `CampaignSource` is a compile-time union, and a cast, a
 * CLI argument or a JSON config would otherwise put `Bluesky` and `bluesky` in
 * two separate rows of every breakdown.
 */
export function buildCampaignUrl(
  url: string,
  source: CampaignSource,
  campaign: string,
  /** Which post/placement, when one artifact is posted more than once. */
  content?: string
): string {
  if (!isCampaignSource(source)) {
    throw new Error(
      `buildCampaignUrl: unknown source ${JSON.stringify(source)} — add it to CAMPAIGN_SOURCES rather than inventing a spelling`
    )
  }

  const normalizedCampaign = campaign.trim().toLowerCase()
  if (!normalizedCampaign) {
    throw new Error('buildCampaignUrl: campaign must not be empty')
  }

  // `new URL()` alone is not validation: it happily accepts `htp://…`,
  // `mailto:` and `javascript:`, while REJECTING the root-relative form a
  // caller is most likely to reach for. These are only ever our own public
  // links, so require the one shape that can actually be posted.
  let parsed: URL
  try {
    parsed = new URL(url)
  } catch {
    throw new Error(
      `buildCampaignUrl: expected an absolute https URL, got ${JSON.stringify(url)}`
    )
  }
  if (parsed.protocol !== 'https:') {
    throw new Error(`buildCampaignUrl: expected https, got ${parsed.protocol}`)
  }

  parsed.searchParams.set('utm_source', source)
  parsed.searchParams.set('utm_campaign', normalizedCampaign)

  // Absent means "one post, nothing to distinguish". Present-but-blank is a
  // junk row, so it fails loudly like the others — and a stale tag from a
  // previously-tagged URL must be cleared rather than silently inherited.
  if (content === undefined) {
    parsed.searchParams.delete('utm_content')
  } else {
    const normalizedContent = content.trim().toLowerCase()
    if (!normalizedContent) {
      throw new Error('buildCampaignUrl: content must be omitted, not empty')
    }
    parsed.searchParams.set('utm_content', normalizedContent)
  }

  return parsed.toString()
}

/**
 * The tagged URL for a weekly city page — the common case, in one call.
 *
 * Points at the ARCHIVED permalink rather than the rolling `/week` URL, matching
 * what the page declares canonical: a post is about one specific week, and the
 * rolling URL would show a different week to anyone who followed the link later.
 */
export function weeklyPageCampaignUrl(
  siteOrigin: string,
  sceneSlug: string,
  isoWeek: string,
  source: CampaignSource,
  /** e.g. `mon` / `thu` / `r-chicago` — omit when the week is posted once. */
  content?: string
): string {
  // Both go into the PATH, so a bad value produces a publicly posted 404 that
  // still logs a confident-looking campaign row — wrong-but-plausible, and
  // undiscoverable until someone reads a dashboard weeks later. Reject rather
  // than sanitize: a slug with a space is a bug, not input to tidy up.
  //
  // The week must be ZERO-PADDED. `2026-W1` is the natural output of
  // `${year}-W${week}` without `padStart`, and the route 404s on it while
  // still yielding a distinct, unmergeable campaign id.
  if (!SLUG.test(sceneSlug)) {
    throw new Error(
      `weeklyPageCampaignUrl: ${JSON.stringify(sceneSlug)} is not a scene slug`
    )
  }
  if (!ISO_WEEK.test(isoWeek)) {
    throw new Error(
      `weeklyPageCampaignUrl: ${JSON.stringify(isoWeek)} is not a zero-padded ISO week key`
    )
  }

  // Uppercased to match the form the page treats as canonical. The route
  // matches case-insensitively, so a lowercase week renders 200 at a
  // non-canonical URL and splits the analytics "top pages" row in two.
  const week = isoWeek.toUpperCase()
  const origin = siteOrigin.replace(/\/+$/, '')
  return buildCampaignUrl(
    `${origin}/scenes/${sceneSlug}/${week}`,
    source,
    weekCampaignId(sceneSlug, week),
    content
  )
}
