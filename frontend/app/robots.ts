/**
 * Crawler policy — decided 2026-07-29.
 *
 * AI crawlers are split into two groups and treated differently. Retrieval
 * fetchers are allowed; training crawlers are not.
 *
 * Training access buys only slow, uncited entity recognition: there is a 6-18
 * month lag between crawl and released model, by which time a show listing has
 * long since expired. Retrieval is what produces a cited link back. The two are
 * independent controls at every operator listed below, so declining training
 * costs us no retrieval and no search ranking. The curation is the product, and
 * it should not be absorbed into model weights uncited.
 *
 * Unknown agents fall through to `User-Agent: *`, which allows. Failing open is
 * deliberate — a new retrieval fetcher we have not heard of should reach us
 * rather than be blocked by an over-broad default.
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
 * Restated by every named group below, because a crawler obeys only the most
 * specific group naming it and ignores `User-Agent: *` entirely. Omitting it
 * from a named group would invite crawling of /admin/ by precisely the agents
 * we bothered to name.
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
 * Read the live page at query time and answer with a citation and a link back.
 *
 * Allowed explicitly rather than left to the wildcard, so the intent survives a
 * future tightening of `User-Agent: *`. Tokens ending in `-User` or `-Fetcher`
 * are user-initiated fetches rather than standing crawls.
 */
const RETRIEVAL_FETCHERS = [
  // https://developers.openai.com/api/docs/bots
  'OAI-SearchBot',
  'ChatGPT-User',
  // https://support.claude.com/en/articles/8896518-does-anthropic-crawl-data-from-the-web-and-how-can-site-owners-block-the-crawler
  'Claude-SearchBot',
  'Claude-User',
  // https://docs.perplexity.ai/guides/bots
  // Perplexity documents that NEITHER token feeds foundation-model training.
  'PerplexityBot',
  'Perplexity-User',
  // https://support.apple.com/en-us/119829 — Siri, Spotlight and Safari search
  'Applebot',
  // https://developers.facebook.com/docs/sharing/webmasters/web-crawlers/
  'Meta-WebIndexer',
  'Meta-ExternalFetcher',
  // Meta-ExternalAgent is DUAL-PURPOSE: one token covers both training and
  // indexing, so the training half cannot be blocked without losing retrieval.
  // Retrieval wins.
  'Meta-ExternalAgent',
]

/**
 * Ingest page content into model weights. No citation, no referral traffic.
 *
 * Google-Extended and Applebot-Extended are training-only controls that do not
 * crawl for search. Google documents that Google-Extended "does not impact a
 * site's inclusion in Google Search nor is it used as a ranking signal", and
 * Apple documents that pages disallowing Applebot-Extended "can still be
 * included in search results". Googlebot and Applebot are unaffected.
 */
const TRAINING_CRAWLERS = [
  // https://developers.openai.com/api/docs/bots
  'GPTBot',
  // https://support.claude.com/en/articles/8896518-does-anthropic-crawl-data-from-the-web-and-how-can-site-owners-block-the-crawler
  'ClaudeBot',
  // https://developers.google.com/search/docs/crawling-indexing/google-common-crawlers
  'Google-Extended',
  // https://support.apple.com/en-us/119829
  'Applebot-Extended',
]

export default function robots(): MetadataRoute.Robots {
  return {
    rules: [
      {
        userAgent: '*',
        allow: '/',
        disallow: NON_PUBLIC_PATHS,
      },
      {
        userAgent: RETRIEVAL_FETCHERS,
        allow: '/',
        disallow: NON_PUBLIC_PATHS,
      },
      {
        userAgent: TRAINING_CRAWLERS,
        disallow: '/',
      },
    ],
    sitemap: 'https://psychichomily.com/sitemap.xml',
  }
}
