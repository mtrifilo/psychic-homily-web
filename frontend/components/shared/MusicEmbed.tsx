'use client'

import { useQuery } from '@tanstack/react-query'
import * as Sentry from '@sentry/nextjs'
import { ExternalLink, Loader2, Music } from 'lucide-react'
import { useTheme } from 'next-themes'
import { type SpotifyEmbedKind } from '@/lib/spotify'
import {
  BANDCAMP_THEME_COLORS,
  bandcampEmbedSrc,
  isAllowedBandcampUrl,
  isBandcampReleaseUrl,
  type BandcampEmbedResponse,
  type EmbedTheme,
} from '@/lib/bandcamp'
import { useHydrated } from '@/lib/hooks/common/useHydrated'
import { playableMusicSources } from '@/lib/playableMusicSources'
import { queryKeys } from '@/lib/queryClient'

/**
 * `default` is the headed player block (or, with `compact`, the unheaded one).
 * `slim` is the one-line player for a stack of them: Bandcamp's small player,
 * a short Spotify card, no heading, no outer margin, square corners, and both
 * players coloured for the page theme.
 */
export type MusicEmbedSize = 'default' | 'slim'

interface MusicEmbedProps {
  bandcampAlbumUrl?: string | null
  bandcampProfileUrl?: string | null
  spotifyUrl?: string | null
  artistName: string
  /** Drops the heading and shortens the Spotify card. Ignored by `slim`. */
  compact?: boolean
  size?: MusicEmbedSize
}

type BandcampEmbed = Pick<BandcampEmbedResponse, 'kind' | 'id'> & {
  kind: 'album' | 'track'
  id: string
}

// Resolve a Bandcamp album/track URL to its embeddable { kind, id } descriptor
// via the `/api/bandcamp/album-id` Next.js route handler (a relative path, NOT
// the Go backend `apiRequest` stack).
//
// Return-vs-throw is deliberate, because TanStack Query caches a returned value
// as a durable success (global staleTime 15min) but does NOT cache a thrown
// error. The route scrapes a third-party site, so a transient upstream outage
// surfaces as a 5xx. If we returned null for that, the query would cache the
// null success for 15min and every same-URL instance + remount would render the
// plain fallback link until staleTime expires — even after Bandcamp recovers
// seconds later (PSY-1102 adversarial review). So:
//   - transient (5xx, 429, 408, network throw) → THROW → query errors → no
//     durable cache, a later mount retries → embed self-heals once the outage
//     clears.
//   - other 4xx (incl. the route's 404 "no embed found") → return null → a
//     genuine "this URL has no embeddable player" answer that IS safe to cache.
//   - 2xx with no usable id/kind → return null (same: a real negative answer).
//
// 429 and 408 are on the transient side for the same reason 5xx is, and they
// are not hypothetical: the route scrapes bandcamp.com once per distinct URL,
// so a page that mounts a whole bill's worth of embeds at once is exactly what
// draws a rate limit. Reading that as "no player exists" would strand every
// card on a plain link for the full staleTime, long after the limit cleared.
const TRANSIENT_RESOLVE_STATUSES = new Set([408, 429])

/**
 * A resolve that failed in a way worth retrying.
 *
 * Carries the status so the Sentry report can tell a third party rate-limiting
 * us apart from something actually broken. Being throttled is operational
 * weather, and it arrives once per distinct URL on a page, so reporting it at
 * error level would page loudest exactly when a bill is longest.
 */
class TransientResolveError extends Error {
  readonly rateLimited: boolean

  constructor(status: number) {
    super(`Bandcamp embed resolve failed: ${status}`)
    this.name = 'TransientResolveError'
    this.rateLimited = TRANSIENT_RESOLVE_STATUSES.has(status)
  }
}

async function resolveBandcampEmbed(albumUrl: string): Promise<BandcampEmbed | null> {
  const response = await fetch(
    `/api/bandcamp/album-id?url=${encodeURIComponent(albumUrl)}`
  )
  if (!response.ok) {
    if (
      response.status >= 500 ||
      TRANSIENT_RESOLVE_STATUSES.has(response.status)
    ) {
      throw new TransientResolveError(response.status)
    }
    return null
  }

  const data = (await response.json()) as BandcampEmbedResponse
  if (data.id && (data.kind === 'album' || data.kind === 'track')) {
    return { kind: data.kind, id: data.id }
  }
  return null
}

/**
 * How wide the Bandcamp player is allowed to get.
 *
 * Exported because it is a constraint on the player, not a preference of this
 * file: the player's internal layout is fixed, so a container that lets it run
 * wider ends up framing it with dead space. A caller sizing a card around one
 * of these should read the number from here rather than restate it, or a change
 * on this side leaves a silent gutter on the other.
 */
export const BANDCAMP_EMBED_MAX_WIDTH_PX = 700

/**
 * How tall the Bandcamp player renders, and therefore how much room the
 * loading placeholder has to hold open for it.
 *
 * Not exported: no caller needs the number, they need the placeholder to have
 * already used it.
 */
const BANDCAMP_EMBED_HEIGHT_PX = 120

/**
 * The height of Bandcamp's `size=small` player, which is fixed by Bandcamp:
 * the slim size's iframe and its loading placeholder both stand at it.
 */
export const BANDCAMP_SLIM_EMBED_HEIGHT_PX = 42

/** The shortest Spotify card that still shows the cover and the transport. */
const SPOTIFY_SLIM_EMBED_HEIGHT_PX = 80

/**
 * The page theme a slim player is coloured for.
 *
 * `dark` on the server and through the hydration render, whatever the reader's
 * theme: next-themes reads the stored theme in the browser's FIRST render, so
 * reading it straight into a src would make the hydration render disagree with
 * the server HTML. The reader's real theme arrives the commit after.
 */
function useEmbedTheme(): EmbedTheme {
  const hydrated = useHydrated()
  const { resolvedTheme } = useTheme()
  return hydrated && resolvedTheme === 'light' ? 'light' : 'dark'
}

/**
 * Spotify's embed src. `theme=0` is Spotify's dark card; with no `theme`
 * Spotify picks its own colour from the cover art, which is its light card.
 */
function spotifyEmbedSrc(
  kind: SpotifyEmbedKind,
  id: string,
  theme: EmbedTheme
): string {
  const themeParam = theme === 'dark' ? '&theme=0' : ''
  return `https://open.spotify.com/embed/${kind}/${id}?utm_source=generator${themeParam}`
}

type EmbedState =
  | { type: 'loading' }
  | { type: 'bandcamp'; embedKind: 'album' | 'track'; embedId: string }
  | { type: 'spotify'; spotifyKind: SpotifyEmbedKind; spotifyId: string }
  | { type: 'fallback'; url: string; label: string }
  | { type: 'none' }

export function MusicEmbed({
  bandcampAlbumUrl,
  bandcampProfileUrl,
  spotifyUrl,
  artistName,
  compact = false,
  size = 'default',
}: MusicEmbedProps) {
  const theme = useEmbedTheme()
  const slim = size === 'slim'
  const showHeading = !slim && !compact
  const sectionClass = slim ? undefined : compact ? 'mb-2' : 'mb-8'

  // The album URL only as far as it can go: `/api/bandcamp/album-id` refuses
  // anything failing this same host anchor with a 400 before it fetches, so a
  // value that cannot clear it is not "an album URL we have not resolved yet",
  // it is one the resolver will never accept. Treating it as absent for the
  // WHOLE ladder (the query and the branches below) is what keeps a junk row
  // from paying a round trip, holding the loading placeholder open, and
  // then collapsing to nothing.
  //
  // It also makes hasRenderableMusic (lib/musicAvailability) an exact necessary
  // condition rather than an approximate one: that predicate asks this same
  // question, so whatever it calls unrenderable reaches `none` here.
  const resolvableAlbumUrl =
    bandcampAlbumUrl && isAllowedBandcampUrl(bandcampAlbumUrl) ? bandcampAlbumUrl : null

  // The bandcamp resolve is the only async dependency. Keying on the album URL
  // dedups the `/api/bandcamp/album-id` request across the many MusicEmbed
  // instances a list page mounts and caches the result across nav/remount.
  // Disabled when there's no resolvable album URL so Spotify-only /
  // fallback-only embeds resolve synchronously without a wasted request. The
  // empty-string fallback key is never fetched (the query is disabled then).
  const bandcampQuery = useQuery({
    queryKey: queryKeys.bandcamp.embed(resolvableAlbumUrl ?? ''),
    queryFn: async () => {
      try {
        return await resolveBandcampEmbed(resolvableAlbumUrl as string)
      } catch (error) {
        Sentry.captureException(error, {
          // Being rate-limited is expected weather, not a defect, and it
          // arrives once per distinct URL on the page: a long bill would
          // otherwise report loudest at exactly the moment the noise is least
          // actionable. Anything else here is still an error.
          level:
            error instanceof TransientResolveError && error.rateLimited
              ? 'warning'
              : 'error',
          tags: { service: 'music-embed' },
          extra: { bandcampAlbumUrl },
        })
        // Re-throw so the query is marked errored; the embed-state derivation
        // treats an errored bandcamp resolve the same as "no embed" and falls
        // through to Spotify / fallback links.
        throw error
      }
    },
    enabled: Boolean(resolvableAlbumUrl),
    // The embed is a best-effort enhancement: a failed resolve should fall
    // through to the Spotify / fallback link immediately. Without this, a
    // network-level fetch rejection would inherit the global 3x retry-with-
    // backoff and keep the embed on a spinner for several seconds before
    // falling through — the pre-TanStack code fell through on the first error.
    retry: false,
  })

  const embed = deriveEmbedState({
    bandcampAlbumUrl: resolvableAlbumUrl,
    bandcampProfileUrl,
    spotifyUrl,
    artistName,
    bandcampIsPending: bandcampQuery.isPending,
    bandcampEmbed: bandcampQuery.data ?? null,
  })

  if (embed.type === 'none') {
    return null
  }

  if (embed.type === 'loading') {
    return (
      <section className={sectionClass}>
        {showHeading && (
          <h2 className="text-lg font-display font-semibold mb-4 flex items-center gap-2">
            <Music className="h-5 w-5" />
            Music
          </h2>
        )}
        {/* The placeholder stands at the player's own height, so the resolve
            swaps in place instead of growing the block by ~64px. This state is
            only ever reached with a Bandcamp URL in hand, so the height it has
            to reserve is that player's. It matters most where several of these
            sit in a column and their queries settle at independent moments:
            without it, each one lands as its own jump and everything below the
            stack walks down the page. */}
        <div
          className={
            slim
              ? 'flex items-center justify-center bg-muted/30'
              : `flex items-center justify-center ${compact ? 'py-4' : 'py-8'} bg-muted/30 rounded-md`
          }
          style={{
            minHeight: slim
              ? BANDCAMP_SLIM_EMBED_HEIGHT_PX
              : BANDCAMP_EMBED_HEIGHT_PX,
          }}
        >
          <Loader2
            className={`${slim ? 'h-4 w-4' : 'h-6 w-6'} animate-spin text-muted-foreground`}
          />
        </div>
      </section>
    )
  }

  return (
    <section className={sectionClass}>
      {showHeading && (
        <h2 className="text-lg font-display font-semibold mb-4 flex items-center gap-2">
          <Music className="h-5 w-5" />
          Music
        </h2>
      )}
      {embed.type === 'fallback' ? (
        <a
          href={embed.url}
          target="_blank"
          rel="noopener noreferrer"
          className="flex items-center gap-2 text-primary hover:underline text-sm"
        >
          {embed.label}
          <ExternalLink className="h-4 w-4" />
        </a>
      ) : embed.type === 'bandcamp' ? (
        <BandcampFrame
          kind={embed.embedKind}
          id={embed.embedId}
          artistName={artistName}
          slim={slim}
          theme={theme}
        />
      ) : (
        <SpotifyFrame
          kind={embed.spotifyKind}
          id={embed.spotifyId}
          artistName={artistName}
          slim={slim}
          compact={compact}
          theme={theme}
        />
      )}
    </section>
  )
}

// Slim players carry no host-side rounding: `.music-embed-container` rounds
// both itself and its iframe, so the slim wrapper does not take that class.
function playerWrapperClass(slim: boolean): string {
  return slim ? 'w-full overflow-hidden' : 'music-embed-container'
}

// Each iframe is keyed on its src. The theme is baked into the src, so a theme
// change remounts the player with the other colours instead of leaving the old
// ones painted.
function BandcampFrame({
  kind,
  id,
  artistName,
  slim,
  theme,
}: {
  kind: 'album' | 'track'
  id: string
  artistName: string
  slim: boolean
  theme: EmbedTheme
}) {
  const src = slim
    ? bandcampEmbedSrc({
        kind,
        id,
        size: 'small',
        ...BANDCAMP_THEME_COLORS[theme],
        transparent: true,
      })
    : bandcampEmbedSrc({ kind, id })
  return (
    <div className={playerWrapperClass(slim)}>
      <iframe
        key={src}
        title={`${artistName} on Bandcamp`}
        style={{
          border: 0,
          width: '100%',
          maxWidth: BANDCAMP_EMBED_MAX_WIDTH_PX,
          height: slim ? BANDCAMP_SLIM_EMBED_HEIGHT_PX : BANDCAMP_EMBED_HEIGHT_PX,
        }}
        src={src}
        // Matches the Spotify branch below, which has always had it. It
        // costs nothing on the one-embed pages this component was built for
        // and matters on the scene roster (PSY-1784), which is the first
        // surface to put ten of these on one page.
        loading="lazy"
        seamless
      />
    </div>
  )
}

// The default size keeps Spotify's dark card and the host-side radius in both
// themes; only the slim size follows the page theme.
function SpotifyFrame({
  kind,
  id,
  artistName,
  slim,
  compact,
  theme,
}: {
  kind: SpotifyEmbedKind
  id: string
  artistName: string
  slim: boolean
  compact: boolean
  theme: EmbedTheme
}) {
  const src = spotifyEmbedSrc(kind, id, slim ? theme : 'dark')
  return (
    <div className={playerWrapperClass(slim)}>
      <iframe
        key={src}
        title={`${artistName} on Spotify`}
        style={
          slim
            ? { width: '100%', height: `${SPOTIFY_SLIM_EMBED_HEIGHT_PX}px` }
            : { borderRadius: '12px', width: '100%', height: compact ? '152px' : '352px' }
        }
        src={src}
        frameBorder="0"
        allow="autoplay; clipboard-write; encrypted-media; fullscreen; picture-in-picture"
        loading="lazy"
      />
    </div>
  )
}

// Map the resolved inputs onto the rendered embed: the shared player ladder
// (`playableMusicSources`) first, then this component's own outbound-link
// fallbacks, which no other surface offers. Kept pure (no hooks, no fetch) so
// the priority logic is testable and obvious at a single level of abstraction.
function deriveEmbedState({
  bandcampAlbumUrl,
  bandcampProfileUrl,
  spotifyUrl,
  artistName,
  bandcampIsPending,
  bandcampEmbed,
}: {
  bandcampAlbumUrl?: string | null
  bandcampProfileUrl?: string | null
  spotifyUrl?: string | null
  artistName: string
  // Raw `useQuery().isPending`. A DISABLED query (no album URL) also reports
  // `isPending: true`, so this is only a real "still resolving" signal when
  // there is an album URL — the guard below enforces that here, in one place,
  // rather than relying on every caller to remember it.
  bandcampIsPending: boolean
  bandcampEmbed: BandcampEmbed | null
}): EmbedState {
  // A PLAYER, from the one ladder every surface shares: Bandcamp album/track
  // first, then the artist's Spotify. The 'platform' scope is what lets an
  // on-platform page that is not a release still be offered to the resolver,
  // which is the only thing that can tell whether it carries a player.
  //
  // Spotify's rung needs no async step, so reaching it is the answer. Bandcamp's
  // does, and a resolve that came back empty leaves the rung with nothing to
  // render, so the walk continues to whatever is below it.
  for (const source of playableMusicSources({
    bandcampUrl: bandcampAlbumUrl,
    spotifyUrl,
    bandcampScope: 'platform',
  })) {
    if (source.service === 'spotify') {
      return { type: 'spotify', spotifyKind: source.kind, spotifyId: source.id }
    }
    if (bandcampIsPending) {
      return { type: 'loading' }
    }
    if (bandcampEmbed) {
      return { type: 'bandcamp', embedKind: bandcampEmbed.kind, embedId: bandcampEmbed.id }
    }
  }

  // Outbound links, once no player is available.
  //
  // Both URLs are proven Bandcamp before either becomes an href, and this is the
  // only place that check is COMPLETE: nearly every surface that mounts this
  // component hands it a raw contributor-writable column (ShowListenModule is
  // the exception: its href is already proven by listenCardsForBill), so a gate
  // at the call sites is a gate the next caller silently skips. The iframe
  // branches above need no such gate: their src is built from a resolved
  // numeric id, never from the stored string.
  //
  // The album URL has already cleared the host anchor before it reaches this
  // function (see resolvableAlbumUrl at the call site); what is left to prove is
  // that it names ONE release, which is what a "Listen to X on Bandcamp" link
  // promises. A profile URL is a bare artist root, so its host anchor is the
  // whole rule.
  //
  // Fail CLOSED, to 'none'. Falling through to the profile when the album URL is
  // rejected is deliberate: the artist may still have a good profile link, and
  // hiding that too would punish the reader for a bad row.
  if (bandcampAlbumUrl && isBandcampReleaseUrl(bandcampAlbumUrl)) {
    return {
      type: 'fallback',
      url: bandcampAlbumUrl,
      label: `Listen to ${artistName} on Bandcamp`,
    }
  }

  if (bandcampProfileUrl && isAllowedBandcampUrl(bandcampProfileUrl)) {
    return {
      type: 'fallback',
      url: bandcampProfileUrl,
      label: `Listen to ${artistName} on Bandcamp`,
    }
  }

  return { type: 'none' }
}
