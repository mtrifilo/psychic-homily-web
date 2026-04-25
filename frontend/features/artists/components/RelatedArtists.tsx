'use client'

import { useState, useCallback, useRef, useEffect } from 'react'
import Link from 'next/link'
import {
  Loader2,
  ThumbsUp,
  ThumbsDown,
  Network,
  X,
  Plus,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { useIsAuthenticated } from '@/features/auth'
import { useArtistGraph, useArtistRelationshipVote, useCreateArtistRelationship } from '../hooks/useArtistGraph'
import { useArtistSearch } from '../hooks/useArtistSearch'
import { ArtistGraphVisualization } from './ArtistGraph'
import type { ArtistGraphLink } from '../types'

const RELATIONSHIP_BADGES: Record<string, { label: string; className: string }> = {
  similar: { label: 'Similar', className: 'bg-zinc-700/50 text-zinc-300 border-zinc-600' },
  shared_bills: { label: 'Shared Bills', className: 'bg-blue-900/30 text-blue-300 border-blue-700/50' },
  shared_label: { label: 'Shared Label', className: 'bg-purple-900/30 text-purple-300 border-purple-700/50' },
  side_project: { label: 'Side Project', className: 'bg-green-900/30 text-green-300 border-green-700/50' },
  member_of: { label: 'Member Of', className: 'bg-amber-900/30 text-amber-300 border-amber-700/50' },
  radio_cooccurrence: { label: 'Radio Co-occurrence', className: 'bg-teal-900/30 text-teal-300 border-teal-700/50' },
}

const ALL_TYPES = ['similar', 'shared_bills', 'shared_label', 'side_project', 'member_of', 'radio_cooccurrence']

interface RelatedArtistsProps {
  artistId: number
  artistSlug: string
}

export function RelatedArtists({ artistId, artistSlug }: RelatedArtistsProps) {
  const { data, isLoading } = useArtistGraph({ artistId, enabled: artistId > 0 })
  const { isAuthenticated } = useIsAuthenticated()
  const [showGraph, setShowGraph] = useState(false)
  const [activeTypes, setActiveTypes] = useState<Set<string>>(new Set(ALL_TYPES))
  const [showSuggest, setShowSuggest] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)
  // Defer the graph render until ResizeObserver reports a real width.
  // Initialising to a hard-coded value caused the canvas to render at
  // the wrong size on first paint; null + a measured update is the fix.
  const [containerWidth, setContainerWidth] = useState<number | null>(null)

  // Measure container width for graph
  useEffect(() => {
    if (!containerRef.current) return
    const observer = new ResizeObserver(entries => {
      for (const entry of entries) {
        setContainerWidth(entry.contentRect.width)
      }
    })
    observer.observe(containerRef.current)
    return () => observer.disconnect()
  }, [])

  if (isLoading) return null

  const hasRelationships = data && (data.nodes.length > 0 || data.links.length > 0)

  // Empty state: show header + message + suggest button for authenticated users
  if (!hasRelationships) {
    return (
      <div ref={containerRef} className="mt-8 px-4 md:px-0">
        <h2 className="text-lg font-semibold mb-4">Related Artists</h2>
        <p className="text-sm text-muted-foreground">
          No similar artists yet. Be the first to suggest one!
        </p>
        {isAuthenticated && (
          <div className="mt-4">
            {showSuggest ? (
              <SuggestSimilarArtist
                centerArtistId={artistId}
                centerArtistSlug={artistSlug}
                onClose={() => setShowSuggest(false)}
              />
            ) : (
              <Button
                variant="outline"
                size="sm"
                onClick={() => setShowSuggest(true)}
                className="text-muted-foreground"
              >
                <Plus className="h-4 w-4 mr-1.5" />
                Suggest similar artist
              </Button>
            )}
          </div>
        )}
      </div>
    )
  }

  const toggleType = (type: string) => {
    setActiveTypes(prev => {
      const next = new Set(prev)
      if (next.has(type)) {
        next.delete(type)
      } else {
        next.add(type)
      }
      return next
    })
  }

  // Group links by related artist for the list view
  const artistLinks = new Map<number, { links: ArtistGraphLink[]; node: typeof data.nodes[0] }>()
  for (const node of data.nodes) {
    artistLinks.set(node.id, { links: [], node })
  }
  for (const link of data.links) {
    const otherId =
      link.source_id === data.center.id ? link.target_id :
      link.target_id === data.center.id ? link.source_id : null
    if (otherId && artistLinks.has(otherId)) {
      artistLinks.get(otherId)!.links.push(link)
    }
  }

  // Sort by combined score
  const sortedArtists = Array.from(artistLinks.values())
    .filter(a => a.links.length > 0)
    .sort((a, b) => {
      const aScore = Math.max(...a.links.map(l => l.score))
      const bScore = Math.max(...b.links.map(l => l.score))
      return bScore - aScore
    })

  const hasEnoughForGraph = data.nodes.length >= 3
  // Mobile gating: below the Tailwind `sm` breakpoint (640px) the graph
  // is unusable on a phone, so we hide the View Map button entirely and
  // let the list view be the only surface — no "best viewed on desktop"
  // nag. `containerWidth === null` (pre-measurement) also gates off so
  // we never flash the button before we know the viewport width.
  const graphAvailable =
    hasEnoughForGraph && containerWidth !== null && containerWidth >= 640

  return (
    <div ref={containerRef} className="mt-8 px-4 md:px-0">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold">Related Artists</h2>
        <div className="flex items-center gap-2">
          {graphAvailable && (
            <Button
              variant={showGraph ? 'default' : 'outline'}
              size="sm"
              onClick={() => setShowGraph(!showGraph)}
            >
              <Network className="h-4 w-4 mr-1.5" />
              {showGraph ? 'Hide Map' : 'View Map'}
            </Button>
          )}
        </div>
      </div>

      {/* Graph View */}
      {showGraph && graphAvailable && (
        <div className="mb-6">
          {/* Type Filter Toggles */}
          <div className="flex flex-wrap gap-1.5 mb-3">
            {ALL_TYPES.map(type => {
              const badge = RELATIONSHIP_BADGES[type]
              const isActive = activeTypes.has(type)
              // Only show toggle if this type exists in the data
              const typeExists = data.links.some(l => l.type === type)
              if (!typeExists) return null
              return (
                <button
                  key={type}
                  onClick={() => toggleType(type)}
                  className={`inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full border transition-opacity ${
                    badge.className
                  } ${isActive ? 'opacity-100' : 'opacity-40'}`}
                >
                  <span className={`w-1.5 h-1.5 rounded-full ${isActive ? 'bg-current' : 'bg-transparent border border-current'}`} />
                  {badge.label}
                </button>
              )
            })}
          </div>

          <ArtistGraphVisualization
            data={data}
            activeTypes={activeTypes}
            // Safe non-null: graphAvailable requires containerWidth !== null
            containerWidth={containerWidth!}
          />
        </div>
      )}

      {/* List View */}
      <div className="space-y-2">
        {sortedArtists.map(({ node, links }) => (
          <RelatedArtistRow
            key={node.id}
            node={node}
            links={links}
            centerArtistId={artistId}
            centerArtistSlug={artistSlug}
            isAuthenticated={isAuthenticated}
            userVotes={data.user_votes}
          />
        ))}
      </div>

      {/* Suggest Similar Artist */}
      {isAuthenticated && (
        <div className="mt-4">
          {showSuggest ? (
            <SuggestSimilarArtist
              centerArtistId={artistId}
              centerArtistSlug={artistSlug}
              onClose={() => setShowSuggest(false)}
            />
          ) : (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowSuggest(true)}
              className="text-muted-foreground"
            >
              <Plus className="h-4 w-4 mr-1.5" />
              Suggest similar artist
            </Button>
          )}
        </div>
      )}
    </div>
  )
}

// --- Related Artist Row ---

interface RelatedArtistRowProps {
  node: { id: number; name: string; slug: string; city?: string; state?: string; upcoming_show_count: number }
  links: ArtistGraphLink[]
  centerArtistId: number
  centerArtistSlug: string
  isAuthenticated: boolean
  userVotes?: Record<string, string>
}

function RelatedArtistRow({
  node,
  links,
  centerArtistId,
  centerArtistSlug,
  isAuthenticated,
  userVotes,
}: RelatedArtistRowProps) {
  const voteMutation = useArtistRelationshipVote()

  const handleVote = (link: ArtistGraphLink, isUpvote: boolean) => {
    voteMutation.mutate({
      sourceId: link.source_id,
      targetId: link.target_id,
      type: link.type,
      isUpvote,
      centerArtistId,
    })
  }

  // Primary display info
  const similarLink = links.find(l => l.type === 'similar')
  const sharedBillsLink = links.find(l => l.type === 'shared_bills')
  const radioLink = links.find(l => l.type === 'radio_cooccurrence')

  // Format score display
  const getScoreDisplay = () => {
    const parts: string[] = []
    if (similarLink) {
      const pct = Math.round(similarLink.score * 100)
      parts.push(`${pct}% similar`)
    }
    if (sharedBillsLink && sharedBillsLink.detail) {
      const count = (sharedBillsLink.detail as Record<string, unknown>).shared_count
      if (count) {
        parts.push(`${count} shared ${Number(count) === 1 ? 'show' : 'shows'}`)
      }
    }
    if (radioLink && radioLink.detail) {
      const detail = radioLink.detail as Record<string, unknown>
      const coCount = detail.co_occurrence_count
      const stationCount = detail.station_count
      if (coCount) {
        const stationPart = stationCount && Number(stationCount) > 1
          ? ` across ${stationCount} stations`
          : ''
        parts.push(`${coCount}x on radio${stationPart}`)
      }
    }
    return parts.join(' \u00b7 ')
  }

  return (
    <div className="flex items-center gap-3 py-2 px-3 rounded-md hover:bg-muted/50 transition-colors group">
      <Link
        href={`/artists/${node.slug}`}
        className="flex-1 min-w-0 flex items-center gap-2"
      >
        <span className="text-sm font-medium truncate group-hover:text-foreground">
          {node.name}
        </span>
      </Link>

      {/* Relationship badges */}
      <div className="hidden sm:flex items-center gap-1 flex-shrink-0">
        {links.map(link => {
          const badge = RELATIONSHIP_BADGES[link.type]
          if (!badge) return null
          return (
            <Badge
              key={link.type}
              variant="outline"
              className={`text-[10px] px-1.5 py-0 ${badge.className}`}
            >
              {badge.label}
            </Badge>
          )
        })}
      </div>

      {/* Score */}
      <span className="text-xs text-muted-foreground flex-shrink-0 hidden md:block">
        {getScoreDisplay()}
      </span>

      {/* Vote buttons (only for "similar" type) */}
      {isAuthenticated && similarLink && (
        <div className="flex items-center gap-0.5 flex-shrink-0">
          <VoteButton
            link={similarLink}
            direction="up"
            userVotes={userVotes}
            onVote={() => handleVote(similarLink, true)}
            isPending={voteMutation.isPending}
          />
          <VoteButton
            link={similarLink}
            direction="down"
            userVotes={userVotes}
            onVote={() => handleVote(similarLink, false)}
            isPending={voteMutation.isPending}
          />
        </div>
      )}
    </div>
  )
}

// --- Vote Button ---

interface VoteButtonProps {
  link: ArtistGraphLink
  direction: 'up' | 'down'
  userVotes?: Record<string, string>
  onVote: () => void
  isPending: boolean
}

function VoteButton({ link, direction, userVotes, onVote, isPending }: VoteButtonProps) {
  const key = `${link.source_id}-${link.target_id}-${link.type}`
  const userVote = userVotes?.[key]
  const isActive = userVote === direction
  const count = direction === 'up' ? link.votes_up : link.votes_down
  const Icon = direction === 'up' ? ThumbsUp : ThumbsDown

  return (
    <button
      onClick={onVote}
      disabled={isPending}
      className={`inline-flex items-center gap-0.5 px-1.5 py-0.5 rounded text-xs transition-colors ${
        isActive
          ? direction === 'up'
            ? 'text-green-400 bg-green-900/20'
            : 'text-red-400 bg-red-900/20'
          : 'text-muted-foreground hover:text-foreground'
      }`}
      title={direction === 'up' ? 'Upvote similarity' : 'Downvote similarity'}
    >
      <Icon className="h-3 w-3" />
      {count > 0 && <span>{count}</span>}
    </button>
  )
}

// --- Suggest Similar Artist ---

interface SuggestSimilarArtistProps {
  centerArtistId: number
  centerArtistSlug: string
  onClose: () => void
}

function SuggestSimilarArtist({ centerArtistId, centerArtistSlug, onClose }: SuggestSimilarArtistProps) {
  const [query, setQuery] = useState('')
  const [isOpen, setIsOpen] = useState(false)
  const [activeIndex, setActiveIndex] = useState(-1)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const { data: searchResults } = useArtistSearch({ query })
  const createRelationship = useCreateArtistRelationship()

  const artists = (searchResults?.artists ?? []).filter(a => a.id !== centerArtistId)

  const handleSelect = useCallback(
    (selectedId: number) => {
      setError(null)
      createRelationship.mutate(
        {
          sourceArtistId: centerArtistId,
          targetArtistId: selectedId,
          type: 'similar',
          centerArtistId,
        },
        {
          onSuccess: () => {
            setSuccess(true)
            setQuery('')
            setIsOpen(false)
            setTimeout(() => {
              setSuccess(false)
              onClose()
            }, 2000)
          },
          onError: (err: Error) => {
            const message = err.message || 'Failed to create relationship'
            if (message.includes('already exists')) {
              setError('This artist pair already has a similarity relationship.')
            } else {
              setError(message)
            }
          },
        }
      )
    },
    [centerArtistId, centerArtistSlug, createRelationship, onClose]
  )

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value
    setQuery(value)
    setIsOpen(value.length > 0)
    setActiveIndex(-1)
    setError(null)
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (!isOpen || artists.length === 0) {
      if (e.key === 'Escape') {
        onClose()
      }
      return
    }

    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        setActiveIndex(prev => (prev < artists.length - 1 ? prev + 1 : 0))
        break
      case 'ArrowUp':
        e.preventDefault()
        setActiveIndex(prev => (prev > 0 ? prev - 1 : artists.length - 1))
        break
      case 'Enter':
        e.preventDefault()
        if (activeIndex >= 0 && activeIndex < artists.length) {
          handleSelect(artists[activeIndex].id)
        }
        break
      case 'Escape':
        onClose()
        break
    }
  }

  return (
    <div className="relative">
      <div className="flex items-center gap-2">
        <div className="relative flex-1 max-w-sm">
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={handleChange}
            onKeyDown={handleKeyDown}
            onBlur={() => setTimeout(() => setIsOpen(false), 150)}
            placeholder="Search for a similar artist..."
            autoFocus
            autoComplete="off"
            className="w-full text-sm px-3 py-1.5 rounded-md border bg-background text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
          />

          {isOpen && artists.length > 0 && (
            <div className="absolute top-full left-0 w-full z-50 mt-1 rounded-md border bg-popover text-popover-foreground shadow-md">
              <div className="max-h-[200px] overflow-y-auto p-1">
                {artists.slice(0, 8).map((artist, i) => (
                  <button
                    type="button"
                    key={artist.id}
                    className={`relative flex w-full cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none ${
                      i === activeIndex
                        ? 'bg-accent text-accent-foreground'
                        : 'hover:bg-accent hover:text-accent-foreground'
                    }`}
                    onMouseDown={e => {
                      e.preventDefault()
                      handleSelect(artist.id)
                    }}
                    onMouseEnter={() => setActiveIndex(i)}
                  >
                    <span className="truncate">{artist.name}</span>
                    {(artist.city || artist.state) && (
                      <span className="ml-auto text-xs text-muted-foreground">
                        {[artist.city, artist.state].filter(Boolean).join(', ')}
                      </span>
                    )}
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>
        <Button variant="ghost" size="sm" onClick={onClose}>
          <X className="h-4 w-4" />
        </Button>
      </div>

      {createRelationship.isPending && (
        <div className="mt-2 flex items-center gap-2 text-xs text-muted-foreground">
          <Loader2 className="h-3 w-3 animate-spin" />
          Creating relationship...
        </div>
      )}

      {error && (
        <p className="mt-2 text-xs text-destructive">{error}</p>
      )}

      {success && (
        <p className="mt-2 text-xs text-green-400">Relationship created with your upvote!</p>
      )}
    </div>
  )
}
