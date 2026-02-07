import { Check } from 'lucide-react'
import { cn } from '../../lib/utils'
import type { VenueConfig } from '../../lib/types'

interface VenueCardProps {
  venue: VenueConfig
  selected: boolean
  onToggle: () => void
}

export function VenueCard({ venue, selected, onToggle }: VenueCardProps) {
  return (
    <div
      onClick={onToggle}
      className={cn(
        'p-4 rounded-lg border-2 cursor-pointer transition-all',
        selected
          ? 'border-primary bg-primary/5'
          : 'border-border bg-card hover:border-muted-foreground/30'
      )}
    >
      <div className="flex items-start justify-between">
        <div>
          <h3 className="font-medium text-foreground">{venue.name}</h3>
          <p className="text-xs text-muted-foreground mt-1">
            {venue.city}, {venue.state} • {venue.providerType}
          </p>
        </div>
        <div
          className={cn(
            'w-5 h-5 rounded border-2 flex items-center justify-center shrink-0',
            selected
              ? 'bg-primary border-primary text-primary-foreground'
              : 'border-muted-foreground/30'
          )}
        >
          {selected && <Check className="w-3 h-3" />}
        </div>
      </div>
    </div>
  )
}
