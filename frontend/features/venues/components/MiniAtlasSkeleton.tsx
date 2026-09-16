import { Skeleton } from '@/components/ui/skeleton'

/**
 * The mini Atlas's placeholder, filling the pane's reserved box.
 *
 * Its own module because two things show it at two different moments: the
 * `next/dynamic` boundary while the MapLibre chunk is in flight, and the map
 * itself until its style has painted. One shape, so the handover between them
 * is invisible.
 */
export function MiniAtlasSkeleton() {
  return <Skeleton className="absolute inset-0 rounded-md" />
}
