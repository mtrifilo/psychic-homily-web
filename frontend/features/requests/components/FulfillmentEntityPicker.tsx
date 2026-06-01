'use client'

/**
 * FulfillmentEntityPicker — PSY-917
 *
 * Mandatory entity picker for the "Propose a fulfillment" flow. Proposing a
 * fulfillment REQUIRES naming a concrete entity (decision locked in PSY-917):
 * every proposal points at a real graph entity so the requester's review
 * panel always has a "View proposed {entity}" link. There is no skip/optional
 * path.
 *
 * Scope: results are filtered to the request's own `entity_type` — you can't
 * propose a venue to fulfill an artist request (the backend rejects the
 * mismatch with a 400; PSY-748). Scoping the picker up front keeps the user
 * from picking something that'll only fail on submit.
 *
 * Reuses the shared `useEntitySearch` hook (same search surface as the
 * collections Add-Items picker and the Cmd+K palette) rather than rolling a
 * parallel search. The hook returns results grouped by type; we read only the
 * group matching `entityType`.
 *
 * The component is selection-only. It calls `onSubmit(entityId)` with the
 * chosen id; the parent owns the mutation, its pending/error state, and the
 * surrounding dialog. The backend's type-mismatch / not-found validation
 * error is passed back in via `submitError` so it renders inline beneath the
 * confirm button.
 */

import { useMemo, useState } from 'react'
import { Search, Check, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { InlineErrorBanner } from '@/components/shared'
import {
  useEntitySearch,
  ENTITY_SEARCH_UNAVAILABLE_MESSAGE,
  type EntitySearchResult,
  type EntitySearchResults,
} from '@/lib/hooks/common/useEntitySearch'
import { getEntityTypeLabel } from '../types'

/**
 * Maps a request entity_type to the matching result group on
 * EntitySearchResults. `tag` is intentionally absent — tags aren't a
 * requestable entity type (REQUEST_ENTITY_TYPES in ../types).
 */
const ENTITY_TYPE_TO_GROUP: Record<
  string,
  keyof Omit<EntitySearchResults, 'tags'>
> = {
  artist: 'artists',
  venue: 'venues',
  show: 'shows',
  release: 'releases',
  label: 'labels',
  festival: 'festivals',
}

export interface FulfillmentEntityPickerProps {
  /** The request's entity_type — scopes search results to this type only. */
  entityType: string
  /** Disable the confirm button + inputs while the parent mutation is in flight. */
  isSubmitting?: boolean
  /**
   * Backend validation / submit error to surface inline (e.g. the PSY-748
   * type-mismatch 400). Null when there's nothing to show.
   */
  submitError?: string | null
  /** Fired with the chosen entity's numeric id when the user confirms. */
  onSubmit: (entityId: number) => void
  /** Fired when the user backs out without proposing. */
  onCancel: () => void
}

export function FulfillmentEntityPicker({
  entityType,
  isSubmitting = false,
  submitError = null,
  onSubmit,
  onCancel,
}: FulfillmentEntityPickerProps) {
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState<EntitySearchResult | null>(null)

  const {
    data: searchResults,
    isSearching,
    searchError,
  } = useEntitySearch({ query, enabled: true })

  // Only surface results of the request's own entity type. A request for an
  // unmapped type (shouldn't happen — CreateRequest gates the enum) yields no
  // rows rather than crashing.
  const rows: EntitySearchResult[] = useMemo(() => {
    const group = ENTITY_TYPE_TO_GROUP[entityType]
    if (!group) return []
    return searchResults[group]
  }, [searchResults, entityType])

  const typeLabel = getEntityTypeLabel(entityType).toLowerCase()
  const trimmedQuery = query.trim()

  return (
    <div className="space-y-3" data-testid="fulfillment-entity-picker">
      <div className="relative">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          placeholder={`Search ${typeLabel}s…`}
          value={query}
          onChange={(e) => {
            setQuery(e.target.value)
            // A new query invalidates the prior selection — the user is
            // looking for something else.
            setSelected(null)
          }}
          className="pl-9"
          autoFocus
          disabled={isSubmitting}
          data-testid="fulfillment-entity-picker-search-input"
        />
      </div>

      {trimmedQuery.length === 0 ? (
        <p className="py-3 text-center text-sm text-muted-foreground">
          Search for the {typeLabel} that fulfills this request.
        </p>
      ) : trimmedQuery.length < 2 ? (
        <p className="py-3 text-center text-sm text-muted-foreground">
          Keep typing to search…
        </p>
      ) : isSearching ? (
        <div className="flex items-center justify-center py-4">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      ) : searchError ? (
        <InlineErrorBanner testId="fulfillment-entity-picker-search-error">
          {ENTITY_SEARCH_UNAVAILABLE_MESSAGE}
        </InlineErrorBanner>
      ) : rows.length === 0 ? (
        <p className="py-3 text-center text-sm text-muted-foreground">
          No {typeLabel}s found for &quot;{trimmedQuery}&quot;.
        </p>
      ) : (
        <div
          className="max-h-64 space-y-1 overflow-y-auto"
          data-testid="fulfillment-entity-picker-results"
        >
          {rows.map((row) => {
            const isSelected =
              selected?.entityType === row.entityType && selected?.id === row.id
            return (
              <button
                key={`${row.entityType}-${row.id}`}
                type="button"
                onClick={() => setSelected(row)}
                disabled={isSubmitting}
                aria-pressed={isSelected}
                className={cnPickerRow(isSelected)}
                data-testid="fulfillment-entity-picker-result-row"
              >
                <div className="min-w-0 flex-1 text-left">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-medium">
                      {row.name}
                    </span>
                    <Badge
                      variant="secondary"
                      className="shrink-0 px-1.5 py-0 text-[10px]"
                    >
                      {getEntityTypeLabel(row.entityType)}
                    </Badge>
                  </div>
                  {row.subtitle && (
                    <p className="truncate text-xs text-muted-foreground">
                      {row.subtitle}
                    </p>
                  )}
                </div>
                {isSelected && (
                  <Check className="h-4 w-4 shrink-0 text-primary" />
                )}
              </button>
            )
          })}
        </div>
      )}

      {selected && (
        <p
          className="text-sm text-muted-foreground"
          data-testid="fulfillment-entity-picker-selection"
        >
          Proposing:{' '}
          <span className="font-medium text-foreground">{selected.name}</span>
        </p>
      )}

      {submitError && (
        <p
          className="text-sm text-destructive"
          data-testid="fulfillment-entity-picker-submit-error"
        >
          {submitError}
        </p>
      )}

      <div className="flex items-center gap-2">
        <Button
          size="sm"
          onClick={() => selected && onSubmit(selected.id)}
          disabled={!selected || isSubmitting}
          data-testid="fulfillment-entity-picker-confirm"
        >
          {isSubmitting && <Loader2 className="mr-1.5 h-4 w-4 animate-spin" />}
          Propose this {typeLabel}
        </Button>
        <Button
          variant="outline"
          size="sm"
          onClick={onCancel}
          disabled={isSubmitting}
        >
          Cancel
        </Button>
      </div>
    </div>
  )
}

/**
 * Row styling kept in a tiny helper so the long conditional className doesn't
 * bury the JSX. Selected rows get a primary ring + tint; the rest get the
 * standard hover affordance.
 */
function cnPickerRow(isSelected: boolean): string {
  const base =
    'flex w-full items-center gap-3 rounded-md p-2 transition-colors disabled:opacity-60'
  return isSelected
    ? `${base} bg-primary/10 ring-1 ring-primary`
    : `${base} hover:bg-muted/50`
}
