/**
 * Crawler policy — decided 2026-07-29.
 *
 * AI crawlers are split into two groups and treated differently. Agents that
 * answer with a citation and a link back are allowed; agents that only feed
 * model weights are not.
 *
 * Our read, mid-2026 (a judgement, not a sourced fact): the lag between crawl
 * and released model is long enough that a show listing has expired before it
 * could ever surface, so training access buys us only slow, uncited entity
 * recognition. Retrieval is what produces a cited link. The curation is the
 * product, and it should not be absorbed into model weights uncited.
 *
 * Where an operator splits training and retrieval into separate tokens, we
 * decline training. Where ONE token governs both — Google-Extended and
 * Meta-ExternalAgent — blocking would forfeit the retrieval half too, so
 * retrieval wins and we allow it. Verify this per-token before moving anything
 * between the two lists below.
 *
 * Unknown agents fall through to `User-Agent: *`, which allows. Failing open is
 * deliberate — a new retrieval fetcher we have not heard of should reach us
 * rather than be blocked by an over-broad default. The corollary is that this
 * declines training at the three operators named below, not across the board:
 * bulk aggregators are addressed in the note on TRAINING_CRAWLERS.
 *
 * Scope: this governs psychichomily.com only. api.psychichomily.com serves the
 * same catalog, has no robots.txt of its own, and is advertised to crawlers by
 * /llms.txt — so this file is not a complete answer on its own.
 *
 * Every token below was verified against the operator's own documentation
 * (linked inline) rather than from memory; tokens and their semantics change.
 */
import { MetadataRoute } from 'next'

/**
 * The robots.txt disallow set — paths not worth crawling.
 *
 * This is NOT the complete private-route registry. Per-route
 * `robots: { index: false }` metadata is a separate, non-overlapping control,
 * and the two lists have drifted; reconciling them is its own change.
 *
 * Note these are prefix matches WITH a trailing slash, so `/admin/` covers
 * `/admin/users` but not the bare `/admin`. Kept verbatim from the previous
 * single-rule config so this change stays behaviour-preserving; widening them
 * is a separate, deliberate change.
 */
const NON_PUBLIC_PATHS = [
  '/admin/',
  '/profile/',
  '/auth/',
  '/submissions/',
  '/shows/submit/',
  '/library/',
  '/verify-email/',
]

/**
 * The access every allowed group gets.
 *
 * Shared as one object because a crawler obeys only the most specific group
 * naming it and ignores `User-Agent: *` entirely — so a named group inherits
 * NOTHING from the wildcard. Spreading this keeps the groups in lockstep for
 * every field, not just `disallow`; hand-copying them would mean the next
 * tightening of the wildcard silently skipped the agents we bothered to name.
 */
const PUBLIC_ACCESS = { allow: '/', disallow: NON_PUBLIC_PATHS } as const

/**
 * Agents that answer with a citation and a link back to us.
 *
 * Deliberately mixed: some are standing index crawlers (`OAI-SearchBot`,
 * `Claude-SearchBot`, `PerplexityBot`, `Meta-WebIndexer`, `Applebot`), others
 * fetch a single page at a user's request (`ChatGPT-User`, `Claude-User`,
 * `Perplexity-User`, `Meta-ExternalFetcher`). Both shapes produce a cited link,
 * which is the thing we are buying. Classify by the operator's documentation,
 * never by the shape of the token name.
 *
 * Allowed explicitly rather than left to the wildcard, so the intent survives a
 * future tightening of `User-Agent: *`.
 */
const RETRIEVAL_AGENTS = [
  // https://developers.openai.com/api/docs/bots
  'OAI-SearchBot',
  'ChatGPT-User',
  // https://support.claude.com/en/articles/8896518-does-anthropic-crawl-data-from-the-web-and-how-can-site-owners-block-the-crawler
  'Claude-SearchBot',
  'Claude-User',
  // https://docs.perplexity.ai/docs/resources/perplexity-crawlers
  // Perplexity documents that NEITHER token feeds foundation-model training.
  'PerplexityBot',
  'Perplexity-User',
  // https://support.apple.com/en-us/119829 — Siri, Spotlight and Safari search
  'Applebot',
  // https://developers.facebook.com/documentation/sharing/webmasters/web-crawlers/
  'Meta-WebIndexer',
  'Meta-ExternalFetcher',
  // DUAL-PURPOSE, allowed on the retrieval-wins rule. Meta documents one token
  // for both "training foundation AI models" and "improving products by
  // indexing content directly". Meta-WebIndexer already covers Meta AI search,
  // so what this concedes is the training half in exchange for that indexing.
  'Meta-ExternalAgent',
  // https://developers.google.com/crawling/docs/crawlers-fetchers/google-common-crawlers
  // DUAL-PURPOSE, allowed on the retrieval-wins rule. Google-Extended gates
  // Gemini training AND "Grounding in Gemini Apps" / "Grounding with Google
  // Search on Vertex AI" — grounding being retrieval-with-citation. Blocking it
  // would forfeit Gemini citations, so it stays allowed. Separately, it never
  // affected Google Search: Google documents that it "does not impact a site's
  // inclusion in Google Search nor is it used as a ranking signal".
  'Google-Extended',
]

/**
 * Agents that only ingest page content into model weights. No citation, no
 * referral traffic, so nothing is lost by declining them.
 *
 * Applebot-Extended is a training-only control that does not itself crawl:
 * Apple documents that pages disallowing it "can still be included in search
 * results", so Applebot above is unaffected.
 *
 * Considered and deliberately NOT blocked, each for its own reason — these are
 * open questions, not oversights, and because unknown agents fail open they are
 * all currently allowed:
 *
 * - CCBot (Common Crawl): documents itself as an open research archive, and its
 *   corpus also serves search and academic users. It is nonetheless the widest
 *   open path into the same models we decline above, so blocking GPTBot and
 *   ClaudeBot does not fully deliver the intent at the top of this file.
 * - Bytespider (ByteDance), Amazonbot, Diffbot, Timpibot, omgili, cohere-ai:
 *   no citation and no referral, so on the stated rationale they have no claim
 *   to access. They are unblocked only because expanding the policy beyond the
 *   operators above was not part of this decision.
 */
const TRAINING_CRAWLERS = [
  // https://developers.openai.com/api/docs/bots
  'GPTBot',
  // https://support.claude.com/en/articles/8896518-does-anthropic-crawl-data-from-the-web-and-how-can-site-owners-block-the-crawler
  'ClaudeBot',
  // https://support.apple.com/en-us/119829
  'Applebot-Extended',
]

export default function robots(): MetadataRoute.Robots {
  return {
    rules: [
      { userAgent: '*', ...PUBLIC_ACCESS },
      { userAgent: RETRIEVAL_AGENTS, ...PUBLIC_ACCESS },
      { userAgent: TRAINING_CRAWLERS, disallow: '/' },
    ],
    // With generateSitemaps() (PSY-1622), child documents live at
    // /sitemap/{id}.xml. Next does not emit an index for that shape, so
    // app/sitemap-index/route.ts lists them and robots points here.
    sitemap: 'https://psychichomily.com/sitemap-index',
  }
}
