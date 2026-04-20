import { BadgeCheck } from 'lucide-react'
import { cn } from '@/lib/utils'

type TagOfficialIndicatorSize = 'sm' | 'md'

interface TagOfficialIndicatorProps {
  /**
   * Tag name for the tooltip. When provided, the title reads
   * "{name} (Official)" so users can distinguish official marks
   * across multiple tags in a row. Omitting it falls back to
   * the generic "Official tag" label.
   */
  tagName?: string
  size?: TagOfficialIndicatorSize
  className?: string
}

export function TagOfficialIndicator({
  tagName,
  size = 'sm',
  className,
}: TagOfficialIndicatorProps) {
  const iconSize =
    size === 'md' ? 'h-4 w-4' : 'h-3.5 w-3.5'
  const title = tagName ? `${tagName} (Official)` : 'Official tag'

  return (
    <span
      title={title}
      aria-label="Official tag"
      role="img"
      className={cn(
        'inline-flex items-center gap-1 text-primary shrink-0',
        className
      )}
    >
      <BadgeCheck className={cn(iconSize, 'shrink-0')} aria-hidden="true" />
      {size === 'md' && (
        <span className="text-[10px] font-medium uppercase tracking-wider">
          Official
        </span>
      )}
    </span>
  )
}
