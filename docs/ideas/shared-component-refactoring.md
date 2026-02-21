# Shared Component Refactoring Opportunities

This document outlines UI patterns that are duplicated across the codebase and would benefit from extraction into shared components under `/frontend/components/shared/` or `/frontend/components/filters/`.

## Completed

### LoadingSpinner
- **Location**: `/components/shared/LoadingSpinner.tsx`
- **Status**: Done
- Extracted common loading spinner pattern with size variants (`sm`, `md`, `lg`)

### CityFilters / FilterChip
- **Location**: `/components/filters/`
- **Status**: Done
- Generic filter components with `CityWithCount` interface
- Used by both VenueList and ShowList

---

## High Priority

### 1. ShowCard Component

**Impact**: ~400 lines of duplication across 2 files

**Current State**:
- `show-list.tsx` has `ShowCard` (~200 lines)
- `home-show-list.tsx` has `ShowCard` (~175 lines)
- Both are nearly identical with minor styling differences

**Differences**:
| Aspect | home-show-list | show-list |
|--------|----------------|-----------|
| Padding | `py-4 -mx-2 px-2` | `py-5 -mx-3 px-3` |
| Border radius | `rounded-md` | `rounded-lg` |
| Heading levels | `h2`/`h3` | `h1`/`h2`/`h3` |
| Font size | `text-base` | `text-lg` |
| userId source | Context lookup | Passed as prop |
| Export button | Not included | Included (admin only) |

**Proposed Solution**:
```tsx
// /components/shared/ShowCard.tsx
interface ShowCardProps {
  show: ShowResponse
  isAdmin: boolean
  userId?: string
  variant?: 'compact' | 'default'  // compact for homepage
  showExportButton?: boolean
}
```

**Files to Update**:
- `show-list.tsx` - Import shared ShowCard
- `home-show-list.tsx` - Import shared ShowCard with `variant="compact"`

---

### 2. EmptyState Component

**Impact**: 10+ instances across the codebase

**Current Pattern**:
```tsx
<div className="text-center py-12 text-muted-foreground">
  <p>No {items} at this time.</p>
  {hasFilter && (
    <button onClick={onClear} className="mt-4 text-primary hover:underline">
      View all {items}
    </button>
  )}
</div>
```

**Found In**:
- `VenueList.tsx:66-79`
- `show-list.tsx:354-367`
- `home-show-list.tsx:295-300`
- `FavoriteVenuesTab.tsx:360+`
- `VenueCard.tsx:255`
- `ArtistShowsList.tsx:156`
- `VenueShowsList.tsx:144`

**Proposed Solution**:
```tsx
// /components/shared/EmptyState.tsx
interface EmptyStateProps {
  message: string
  description?: string
  action?: {
    label: string
    onClick: () => void
  }
  className?: string
}

export function EmptyState({ message, description, action, className }: EmptyStateProps) {
  return (
    <div className={cn("text-center py-12 text-muted-foreground", className)}>
      <p>{message}</p>
      {description && <p className="mt-1 text-sm">{description}</p>}
      {action && (
        <button
          onClick={action.onClick}
          className="mt-4 text-primary hover:underline"
        >
          {action.label}
        </button>
      )}
    </div>
  )
}
```

**Usage**:
```tsx
<EmptyState
  message={selectedCity ? `No venues found in ${selectedCity}.` : 'No venues available at this time.'}
  action={selectedCity ? { label: 'View all venues', onClick: () => handleFilterChange(null, null) } : undefined}
/>
```

---

### 3. ErrorState Component

**Impact**: 12+ instances across the codebase

**Current Pattern**:
```tsx
<div className="text-center py-12 text-destructive">
  <p>Failed to load {resource}. Please try again later.</p>
</div>
```

**Found In**:
- `VenueList.tsx:38-43`
- `show-list.tsx:324-329`
- `ArtistShowsList.tsx:148`
- `VenueCard.tsx:231`
- `VenueDetail.tsx:86`
- `ArtistDetail.tsx:218`
- `ShowDetail.tsx:78`
- `VenueShowsList.tsx:136`
- `FavoriteVenuesTab.tsx:270, 309`
- `settings/delete-account-dialog.tsx:127`
- `settings/passkey-management.tsx:40, 43`
- `settings/oauth-accounts.tsx:151`

**Proposed Solution**:
```tsx
// /components/shared/ErrorState.tsx
interface ErrorStateProps {
  message?: string  // defaults to "Something went wrong. Please try again later."
  title?: string    // optional heading
  onRetry?: () => void
  className?: string
}

export function ErrorState({
  message = "Something went wrong. Please try again later.",
  title,
  onRetry,
  className
}: ErrorStateProps) {
  return (
    <div className={cn("text-center py-12", className)}>
      {title && <h3 className="text-lg font-semibold text-destructive mb-2">{title}</h3>}
      <p className="text-destructive">{message}</p>
      {onRetry && (
        <button
          onClick={onRetry}
          className="mt-4 text-primary hover:underline"
        >
          Try again
        </button>
      )}
    </div>
  )
}
```

---

## Medium Priority

### 4. SectionHeader Component

**Impact**: 3 instances on homepage, pattern used elsewhere

**Current Pattern** (from `app/page.tsx`):
```tsx
<div className="flex justify-between items-center mb-5">
  <h2 className="text-2xl font-bold tracking-tight">{title}</h2>
  <Link
    href={href}
    className="text-sm text-muted-foreground hover:text-primary transition-colors hover:underline underline-offset-4"
  >
    View all →
  </Link>
</div>
```

**Proposed Solution**:
```tsx
// /components/shared/SectionHeader.tsx
interface SectionHeaderProps {
  title: string
  viewAllHref?: string
  viewAllLabel?: string  // defaults to "View all →"
  className?: string
}
```

---

### 5. DetailPageLayout Component

**Impact**: 3 detail pages with similar structure

**Current Pattern** (Artist, Venue, Show detail pages):
- Loading state with spinner
- Error state with message
- Not found state
- Main content with consistent max-width

**Files**:
- `ArtistDetail.tsx`
- `VenueDetail.tsx`
- `ShowDetail.tsx`

**Proposed Solution**:
```tsx
// /components/shared/DetailPageLayout.tsx
interface DetailPageLayoutProps {
  isLoading: boolean
  error: Error | null
  notFound: boolean
  notFoundMessage?: string
  children: React.ReactNode
}
```

---

## Low Priority

### 6. Card Wrapper Component

Many components use similar card styling:
```tsx
<article className="bg-card/50 border border-border/50 rounded-xl p-6 hover:border-border transition-colors">
```

Could extract if pattern becomes more prevalent.

### 7. TimeFilter Component

Both `ArtistShowsList` and `VenueShowsList` have identical time filter UI (Upcoming/Past toggle). Could extract if more pages need this pattern.

---

## Implementation Order

Recommended order based on impact and complexity:

1. **EmptyState** - Low complexity, high reuse
2. **ErrorState** - Low complexity, high reuse
3. **ShowCard** - High complexity, eliminates most duplication
4. **SectionHeader** - Low complexity, moderate reuse
5. **DetailPageLayout** - Medium complexity, 3 pages benefit

---

## Patterns

### Smooth Loading States Pattern

**Status**: Implemented

When filters change, we want to avoid jarring loading flashes. Instead of replacing content with a spinner, we keep old data visible while new data loads.

**The Pattern**:
1. **keepPreviousData** - TanStack Query option that keeps old data visible while fetching
2. **useTransition** - React hook that marks navigation as non-urgent
3. **isFetching** - Check if we're fetching in background (vs initial load)

**Implementation**:

```typescript
// In your data hook (e.g., useShows.ts)
import { useQuery, keepPreviousData } from '@tanstack/react-query'

export const useUpcomingShows = (options) => {
  return useQuery({
    queryKey: [...],
    queryFn: ...,
    staleTime: 5 * 60 * 1000,
    placeholderData: keepPreviousData,  // Keep old data while fetching
  })
}
```

```typescript
// In your component
import { useTransition } from 'react'

export function ShowList() {
  const [isPending, startTransition] = useTransition()
  const { data, isLoading, isFetching } = useUpcomingShows({ ... })

  const handleFilterChange = (city, state) => {
    startTransition(() => {
      router.push(...)
    })
  }

  // Only show full spinner on FIRST load (no data yet)
  if (isLoading && !data) {
    return <LoadingSpinner />
  }

  const isUpdating = isFetching || isPending

  return (
    <section>
      <CityFilters isLoading={isUpdating} ... />
      {/* Dim content while fetching, don't hide it */}
      <div className={isUpdating ? 'opacity-60 transition-opacity duration-150' : ''}>
        {/* Content */}
      </div>
    </section>
  )
}
```

**User Experience**:
- Click filter → old data stays visible (slightly dimmed at 60% opacity)
- New data smoothly replaces old data
- No full spinner, no layout shift

**Files Using This Pattern**:
- `/components/show-list.tsx`
- `/components/VenueList.tsx`
- `/lib/hooks/useShows.ts`
- `/lib/hooks/useVenues.ts`

---

## Notes

- All shared components should be added to `/components/shared/index.ts` barrel export
- Consider adding Storybook stories for shared components if project adopts Storybook
- Update CLAUDE.md if new patterns emerge from this refactoring
