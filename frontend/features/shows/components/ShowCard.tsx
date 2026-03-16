'use client'

import { useState, useMemo } from 'react'
import Link from 'next/link'
import {
  Pencil,
  X,
  Trash2,
  ChevronDown,
  ChevronUp,
  MapPin,
  ExternalLink,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import {
  formatShowTime,
  formatPrice,
} from '@/lib/utils/formatters'
import { formatShowDateBadge } from '@/lib/utils/showDateBadge'
import { Button } from '@/components/ui/button'
import { ShowForm } from '@/components/forms'
import { SaveButton, SocialLinks, MusicEmbed } from '@/components/shared'
import { AttendanceButton } from './AttendanceButton'
import { DeleteShowDialog } from './DeleteShowDialog'
import { ExportShowButton } from './ExportShowButton'
import { ShowStatusBadge } from './ShowStatusBadge'
import { SHOW_LIST_FEATURE_POLICY } from './showListFeaturePolicy'
import { useAuthContext } from '@/lib/context/AuthContext'
import type { ShowResponse, ArtistResponse, AttendanceCounts } from '../types'

/**
 * Check if an artist has any music available (Bandcamp embed, Spotify, or Bandcamp profile)
 */
function artistHasMusic(artist: ArtistResponse): boolean {
  return !!(
    artist.bandcamp_embed_url ||
    artist.socials?.spotify ||
    artist.socials?.bandcamp
  )
}

/**
 * Check if any artist in the list has music
 */
function showHasArtistMusic(artists: ArtistResponse[]): boolean {
  return artists.some(artistHasMusic)
}

/**
 * Split artists into headliners and support acts based on set_type.
 * Falls back to is_headliner flag for backward compatibility.
 * If no headliner flags are set, treat the first artist as the headliner.
 */
function splitBill(artists: ArtistResponse[]): {
  headliners: ArtistResponse[]
  support: ArtistResponse[]
} {
  const headliners = artists.filter(a => a.set_type === 'headliner' || a.is_headliner === true)
  const support = artists.filter(a => a.set_type !== 'headliner' && a.is_headliner !== true)

  // If no explicit headliners, treat first artist as headliner
  if (headliners.length === 0 && artists.length > 0) {
    return {
      headliners: [artists[0]],
      support: artists.slice(1),
    }
  }

  return { headliners, support }
}

function ArtistLink({ artist, className }: { artist: ArtistResponse; className?: string }) {
  if (artist.slug) {
    return (
      <Link
        href={`/artists/${artist.slug}`}
        className={`hover:text-primary underline underline-offset-4 decoration-border hover:decoration-primary/50 transition-colors ${className || ''}`}
      >
        {artist.name}
      </Link>
    )
  }
  return <span className={className}>{artist.name}</span>
}

function HeadlinerLine({ artists }: { artists: ArtistResponse[] }) {
  if (artists.length === 0) {
    return <span className="text-muted-foreground">TBA</span>
  }

  return (
    <>
      {artists.map((artist, index) => (
        <span key={artist.id}>
          {index > 0 && (
            <span className="text-muted-foreground/60 font-normal">
              &nbsp;&bull;&nbsp;
            </span>
          )}
          <ArtistLink artist={artist} />
        </span>
      ))}
    </>
  )
}

function SetTypeLabel({ setType }: { setType: string }) {
  if (setType === 'special_guest') {
    return <span className="text-xs text-muted-foreground/70 italic"> (special guest)</span>
  }
  return null
}

function SupportLine({ artists }: { artists: ArtistResponse[] }) {
  if (artists.length === 0) return null

  return (
    <div className="text-sm text-muted-foreground mt-0.5">
      <span className="italic">w/</span>{' '}
      {artists.map((artist, index) => (
        <span key={artist.id}>
          {index > 0 && (
            <span className="text-muted-foreground/50">, </span>
          )}
          <ArtistLink artist={artist} className="text-muted-foreground hover:text-primary" />
          <SetTypeLabel setType={artist.set_type} />
        </span>
      ))}
    </div>
  )
}

/**
 * Inline support text for compact mode: "w/ Artist1, Artist2"
 */
function InlineSupportText({ artists }: { artists: ArtistResponse[] }) {
  if (artists.length === 0) return null

  const names = artists.map(a => a.name).join(', ')
  return (
    <span className="text-muted-foreground font-normal">
      {' '}w/ {names}
    </span>
  )
}

export type ShowCardDensity = 'compact' | 'comfortable' | 'expanded'

export interface ShowCardProps {
  show: ShowResponse
  isAdmin: boolean
  userId?: string
  isSaved?: boolean
  density?: ShowCardDensity
  attendanceData?: AttendanceCounts
}

export function ShowCard({ show, isAdmin, userId, isSaved, density = 'comfortable', attendanceData }: ShowCardProps) {
  const { user } = useAuthContext()
  const [isEditing, setIsEditing] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [isExpanded, setIsExpanded] = useState(density === 'expanded')
  const venue = show.venues[0]
  const artists = show.artists

  const { headliners, support } = useMemo(() => splitBill(artists), [artists])

  // Check if any artist has music to show the expand button
  const hasArtistMusic = showHasArtistMusic(artists)

  // Check if user can delete: admin or show owner
  const resolvedUserId = userId || user?.id
  const canDelete =
    isAdmin ||
    (resolvedUserId && show.submitted_by && String(show.submitted_by) === resolvedUserId)

  const dateBadge = useMemo(
    () => formatShowDateBadge(show.event_date, show.state),
    [show.event_date, show.state]
  )

  const handleEditSuccess = () => {
    setIsEditing(false)
  }

  const handleEditCancel = () => {
    setIsEditing(false)
  }

  const detailsHref = `/shows/${show.slug || show.id}`

  // ----- Compact density: single-line row -----
  if (density === 'compact') {
    const headlinerText = headliners.length > 0
      ? headliners.map(a => a.name).join(' / ')
      : 'TBA'

    return (
      <article
        className={cn(
          'flex items-center gap-3 px-3 py-1.5 hover:bg-muted/50 rounded-md transition-colors',
          show.is_cancelled && 'opacity-60'
        )}
      >
        {/* Inline date */}
        <Link
          href={detailsHref}
          className="text-xs text-muted-foreground shrink-0 w-20 tabular-nums hover:text-primary transition-colors"
        >
          {dateBadge.dayOfWeek} {dateBadge.monthDay}
        </Link>

        {/* Artist line: headliner w/ support */}
        <Link
          href={detailsHref}
          className="font-medium text-sm truncate flex-1 hover:text-primary transition-colors"
        >
          {headlinerText}
          <InlineSupportText artists={support} />
          {(show.is_cancelled || show.is_sold_out) && (
            <ShowStatusBadge show={show} className="ml-1.5 inline-flex gap-1" />
          )}
        </Link>

        {/* Venue */}
        {venue && (
          venue.slug ? (
            <Link
              href={`/venues/${venue.slug}`}
              className="text-xs text-muted-foreground shrink-0 hover:text-primary transition-colors hidden sm:inline"
            >
              {venue.name}
            </Link>
          ) : (
            <span className="text-xs text-muted-foreground shrink-0 hidden sm:inline">
              {venue.name}
            </span>
          )
        )}

        {/* Time */}
        <span className="text-xs text-muted-foreground shrink-0 hidden sm:inline tabular-nums">
          {formatShowTime(show.event_date, show.state)}
        </span>

        {/* Price */}
        {show.price != null && (
          <span className="text-xs text-muted-foreground shrink-0 hidden md:inline tabular-nums">
            {formatPrice(show.price)}
          </span>
        )}
      </article>
    )
  }

  // ----- Expanded density: richer, more spacious card -----
  if (density === 'expanded') {
    return (
      <article
        className={cn(
          'border border-border/50 rounded-lg bg-card hover:shadow-sm transition-all duration-100',
          'px-5 py-5 sm:px-6 sm:py-6',
          show.is_cancelled && 'opacity-60'
        )}
      >
        <div className="flex gap-4 sm:gap-5">
          {/* Larger date badge */}
          <Link
            href={detailsHref}
            className="shrink-0 flex flex-col items-center justify-center rounded-md bg-muted/50 hover:bg-muted transition-colors w-16 sm:w-20 py-3"
          >
            <span className="text-xs sm:text-sm font-bold tracking-widest uppercase text-muted-foreground leading-none">
              {dateBadge.dayOfWeek}
            </span>
            <span className="text-sm sm:text-base font-semibold text-primary leading-tight mt-1">
              {dateBadge.monthDay}
            </span>
          </Link>

          {/* Main content area */}
          <div className="flex-1 min-w-0">
            <div className="flex items-start justify-between gap-3">
              {/* Bill hierarchy */}
              <div className="flex-1 min-w-0">
                <h2 className="font-bold leading-tight tracking-tight truncate text-lg sm:text-xl">
                  <HeadlinerLine artists={headliners} />
                  <ShowStatusBadge show={show} className="ml-2 inline-flex gap-1" />
                </h2>

                <SupportLine artists={support} />

                {/* Venue line */}
                <div className="text-sm text-muted-foreground mt-1.5">
                  {venue && (
                    <>
                      {venue.slug ? (
                        <Link
                          href={`/venues/${venue.slug}`}
                          className="text-primary/80 hover:text-primary font-medium transition-colors"
                        >
                          {venue.name}
                        </Link>
                      ) : (
                        <span className="text-primary/80 font-medium">
                          {venue.name}
                        </span>
                      )}
                      {(show.city || show.state) && (
                        <span className="text-muted-foreground/80">
                          {' '}&middot; {[show.city, show.state].filter(Boolean).join(', ')}
                        </span>
                      )}
                    </>
                  )}
                </div>
              </div>

              {/* Right column: time, price, actions */}
              <div className="shrink-0 flex flex-col items-end gap-1.5">
                <div className="text-right">
                  <div className="text-sm font-medium">
                    {formatShowTime(show.event_date, show.state)}
                  </div>
                  <div className="text-xs text-muted-foreground">
                    {show.price != null && (
                      <span>{formatPrice(show.price)}</span>
                    )}
                    {show.price != null && show.age_requirement && (
                      <span> &middot; </span>
                    )}
                    {show.age_requirement && (
                      <span>{show.age_requirement}</span>
                    )}
                  </div>
                </div>

                {/* Action buttons row */}
                <div className="flex items-center gap-0.5">
                  <AttendanceButton showId={show.id} compact attendanceData={attendanceData} />

                  {SHOW_LIST_FEATURE_POLICY.discovery.showExpandMusic &&
                    hasArtistMusic && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setIsExpanded(!isExpanded)}
                        className="h-7 w-7 p-0"
                        title={
                          isExpanded ? 'Hide artist music' : 'Discover artist music'
                        }
                      >
                        {isExpanded ? (
                          <ChevronUp className="h-4 w-4" />
                        ) : (
                          <ChevronDown className="h-4 w-4" />
                        )}
                      </Button>
                    )}

                  {SHOW_LIST_FEATURE_POLICY.discovery.showSaveButton && (
                    <SaveButton
                      showId={show.id}
                      variant="ghost"
                      size="sm"
                      isSaved={isSaved}
                    />
                  )}

                  {SHOW_LIST_FEATURE_POLICY.discovery.showDetailsLink && (
                    <Link
                      href={detailsHref}
                      className="inline-flex items-center justify-center h-7 w-7 rounded-md text-muted-foreground hover:text-primary hover:bg-muted transition-colors"
                      title="View details"
                    >
                      <ExternalLink className="h-3.5 w-3.5" />
                    </Link>
                  )}

                  {SHOW_LIST_FEATURE_POLICY.discovery.showAdminActions &&
                    isAdmin && (
                      <Button
                        variant={isEditing ? 'secondary' : 'ghost'}
                        size="sm"
                        onClick={() => setIsEditing(!isEditing)}
                        className="h-7 w-7 p-0"
                        title={isEditing ? 'Cancel editing' : 'Edit show'}
                      >
                        {isEditing ? (
                          <X className="h-4 w-4" />
                        ) : (
                          <Pencil className="h-3.5 w-3.5" />
                        )}
                      </Button>
                    )}

                  {SHOW_LIST_FEATURE_POLICY.discovery.showAdminActions &&
                    isAdmin && (
                      <ExportShowButton
                        showId={show.id}
                        showTitle={show.title}
                        variant="ghost"
                        size="sm"
                        className="h-7 w-7 p-0"
                        iconOnly
                      />
                    )}

                  {SHOW_LIST_FEATURE_POLICY.discovery.showOwnerActions &&
                    canDelete && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setIsDeleteDialogOpen(true)}
                        className="h-7 w-7 p-0 text-muted-foreground hover:text-destructive"
                        title="Delete show"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    )}
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* Always-visible artist music section in expanded mode (when artists have music) */}
        {isExpanded && hasArtistMusic && (
          <div className="mt-5 pt-5 border-t border-border/50">
            <div className="space-y-6">
              {artists.filter(artistHasMusic).map(artist => (
                <div key={artist.id} className="space-y-2">
                  <div className="flex items-start justify-between gap-2">
                    <div>
                      {artist.slug ? (
                        <Link
                          href={`/artists/${artist.slug}`}
                          className="font-medium hover:text-primary transition-colors"
                        >
                          {artist.name}
                        </Link>
                      ) : (
                        <span className="font-medium">{artist.name}</span>
                      )}
                      {(artist.city || artist.state) && (
                        <div className="flex items-center gap-1 text-xs text-muted-foreground mt-0.5">
                          <MapPin className="h-3 w-3" />
                          <span>
                            {[artist.city, artist.state]
                              .filter(Boolean)
                              .join(', ')}
                          </span>
                        </div>
                      )}
                    </div>
                    <SocialLinks social={artist.socials} className="shrink-0" />
                  </div>
                  <MusicEmbed
                    bandcampAlbumUrl={artist.bandcamp_embed_url}
                    bandcampProfileUrl={artist.socials?.bandcamp}
                    spotifyUrl={artist.socials?.spotify}
                    artistName={artist.name}
                    compact
                  />
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Inline Edit Form */}
        {isEditing && (
          <div className="mt-5 pt-5 border-t border-border/50">
            <ShowForm
              mode="edit"
              initialData={show}
              onSuccess={handleEditSuccess}
              onCancel={handleEditCancel}
            />
          </div>
        )}

        {/* Delete Confirmation Dialog */}
        <DeleteShowDialog
          show={show}
          open={isDeleteDialogOpen}
          onOpenChange={setIsDeleteDialogOpen}
        />
      </article>
    )
  }

  // ----- Comfortable density (default): current card layout -----
  return (
    <article
      className={cn(
        'border border-border/50 rounded-lg bg-card hover:shadow-sm transition-all duration-100',
        'px-3 py-3 sm:px-4 sm:py-4',
        show.is_cancelled && 'opacity-60'
      )}
    >
      <div className="flex gap-3 sm:gap-4">
        {/* Date badge - stacked day/date */}
        <Link
          href={detailsHref}
          className="shrink-0 flex flex-col items-center justify-center rounded-md bg-muted/50 hover:bg-muted transition-colors w-14 sm:w-16 py-2"
        >
          <span className="text-[10px] sm:text-xs font-bold tracking-widest uppercase text-muted-foreground leading-none">
            {dateBadge.dayOfWeek}
          </span>
          <span className="text-xs sm:text-sm font-semibold text-primary leading-tight mt-0.5">
            {dateBadge.monthDay}
          </span>
        </Link>

        {/* Main content area */}
        <div className="flex-1 min-w-0">
          <div className="flex items-start justify-between gap-2">
            {/* Bill hierarchy */}
            <div className="flex-1 min-w-0">
              <h2 className="font-bold leading-tight tracking-tight truncate text-base sm:text-lg">
                <HeadlinerLine artists={headliners} />
                {/* Status badges inline with headliner */}
                <ShowStatusBadge show={show} className="ml-2 inline-flex gap-1" />
              </h2>

              <SupportLine artists={support} />

              {/* Venue line */}
              <div className="text-sm text-muted-foreground mt-1">
                {venue && (
                  <>
                    {venue.slug ? (
                      <Link
                        href={`/venues/${venue.slug}`}
                        className="text-primary/80 hover:text-primary font-medium transition-colors"
                      >
                        {venue.name}
                      </Link>
                    ) : (
                      <span className="text-primary/80 font-medium">
                        {venue.name}
                      </span>
                    )}
                    {(show.city || show.state) && (
                      <span className="text-muted-foreground/80">
                        {' '}&middot; {[show.city, show.state].filter(Boolean).join(', ')}
                      </span>
                    )}
                  </>
                )}
              </div>
            </div>

            {/* Right column: time, price, actions */}
            <div className="shrink-0 flex flex-col items-end gap-1">
              <div className="text-right">
                <div className="text-sm font-medium">
                  {formatShowTime(show.event_date, show.state)}
                </div>
                <div className="text-xs text-muted-foreground">
                  {show.price != null && (
                    <span>{formatPrice(show.price)}</span>
                  )}
                  {show.price != null && show.age_requirement && (
                    <span> &middot; </span>
                  )}
                  {show.age_requirement && (
                    <span>{show.age_requirement}</span>
                  )}
                </div>
              </div>

              {/* Action buttons row */}
              <div className="flex items-center gap-0.5">
                {/* Attendance (Going/Interested) */}
                <AttendanceButton showId={show.id} compact attendanceData={attendanceData} />

                {/* Expand Button - only show if artists have music */}
                {SHOW_LIST_FEATURE_POLICY.discovery.showExpandMusic &&
                  hasArtistMusic && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setIsExpanded(!isExpanded)}
                      className="h-7 w-7 p-0"
                      title={
                        isExpanded ? 'Hide artist music' : 'Discover artist music'
                      }
                    >
                      {isExpanded ? (
                        <ChevronUp className="h-4 w-4" />
                      ) : (
                        <ChevronDown className="h-4 w-4" />
                      )}
                    </Button>
                  )}

                {/* Save Button */}
                {SHOW_LIST_FEATURE_POLICY.discovery.showSaveButton && (
                  <SaveButton
                    showId={show.id}
                    variant="ghost"
                    size="sm"
                    isSaved={isSaved}
                  />
                )}

                {/* Details link */}
                {SHOW_LIST_FEATURE_POLICY.discovery.showDetailsLink && (
                  <Link
                    href={detailsHref}
                    className="inline-flex items-center justify-center h-7 w-7 rounded-md text-muted-foreground hover:text-primary hover:bg-muted transition-colors"
                    title="View details"
                  >
                    <ExternalLink className="h-3.5 w-3.5" />
                  </Link>
                )}

                {/* Admin Edit Button */}
                {SHOW_LIST_FEATURE_POLICY.discovery.showAdminActions &&
                  isAdmin && (
                    <Button
                      variant={isEditing ? 'secondary' : 'ghost'}
                      size="sm"
                      onClick={() => setIsEditing(!isEditing)}
                      className="h-7 w-7 p-0"
                      title={isEditing ? 'Cancel editing' : 'Edit show'}
                    >
                      {isEditing ? (
                        <X className="h-4 w-4" />
                      ) : (
                        <Pencil className="h-3.5 w-3.5" />
                      )}
                    </Button>
                  )}

                {/* Export Button (admin, dev only) */}
                {SHOW_LIST_FEATURE_POLICY.discovery.showAdminActions &&
                  isAdmin && (
                    <ExportShowButton
                      showId={show.id}
                      showTitle={show.title}
                      variant="ghost"
                      size="sm"
                      className="h-7 w-7 p-0"
                      iconOnly
                    />
                  )}

                {/* Delete Button (admin or owner) */}
                {SHOW_LIST_FEATURE_POLICY.discovery.showOwnerActions &&
                  canDelete && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setIsDeleteDialogOpen(true)}
                      className="h-7 w-7 p-0 text-muted-foreground hover:text-destructive"
                      title="Delete show"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  )}
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Expanded Artist Music Section */}
      {isExpanded && hasArtistMusic && (
        <div className="mt-4 pt-4 border-t border-border/50">
          <div className="space-y-6">
            {artists.filter(artistHasMusic).map(artist => (
              <div key={artist.id} className="space-y-2">
                <div className="flex items-start justify-between gap-2">
                  <div>
                    {artist.slug ? (
                      <Link
                        href={`/artists/${artist.slug}`}
                        className="font-medium hover:text-primary transition-colors"
                      >
                        {artist.name}
                      </Link>
                    ) : (
                      <span className="font-medium">{artist.name}</span>
                    )}
                    {(artist.city || artist.state) && (
                      <div className="flex items-center gap-1 text-xs text-muted-foreground mt-0.5">
                        <MapPin className="h-3 w-3" />
                        <span>
                          {[artist.city, artist.state]
                            .filter(Boolean)
                            .join(', ')}
                        </span>
                      </div>
                    )}
                  </div>
                  <SocialLinks social={artist.socials} className="shrink-0" />
                </div>
                <MusicEmbed
                  bandcampAlbumUrl={artist.bandcamp_embed_url}
                  bandcampProfileUrl={artist.socials?.bandcamp}
                  spotifyUrl={artist.socials?.spotify}
                  artistName={artist.name}
                  compact
                />
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Inline Edit Form */}
      {isEditing && (
        <div className="mt-4 pt-4 border-t border-border/50">
          <ShowForm
            mode="edit"
            initialData={show}
            onSuccess={handleEditSuccess}
            onCancel={handleEditCancel}
          />
        </div>
      )}

      {/* Delete Confirmation Dialog */}
      <DeleteShowDialog
        show={show}
        open={isDeleteDialogOpen}
        onOpenChange={setIsDeleteDialogOpen}
      />
    </article>
  )
}
