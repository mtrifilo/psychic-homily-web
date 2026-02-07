import { Label } from '../ui/label'
import { Badge } from '../ui/badge'
import { cn } from '../../lib/utils'

interface EnvironmentSelectorProps {
  value: 'stage' | 'production'
  onChange: (value: 'stage' | 'production') => void
  hasStageToken: boolean
  hasProductionToken: boolean
}

export function EnvironmentSelector({
  value,
  onChange,
  hasStageToken,
  hasProductionToken,
}: EnvironmentSelectorProps) {
  return (
    <div className="space-y-3">
      <div>
        <Label className="text-sm font-medium">Target Environment</Label>
        <p className="text-xs text-muted-foreground mt-1">
          Where to import scraped events
        </p>
      </div>

      <div className="space-y-2">
        <EnvironmentOption
          id="env-stage"
          label="Stage"
          description="Test imports in staging environment"
          selected={value === 'stage'}
          hasToken={hasStageToken}
          onClick={() => onChange('stage')}
        />
        <EnvironmentOption
          id="env-production"
          label="Production"
          description="Import directly to live site"
          selected={value === 'production'}
          hasToken={hasProductionToken}
          isProduction
          onClick={() => onChange('production')}
        />
      </div>
    </div>
  )
}

interface EnvironmentOptionProps {
  id: string
  label: string
  description: string
  selected: boolean
  hasToken: boolean
  isProduction?: boolean
  onClick: () => void
}

function EnvironmentOption({
  id,
  label,
  description,
  selected,
  hasToken,
  isProduction,
  onClick,
}: EnvironmentOptionProps) {
  return (
    <label
      htmlFor={id}
      className={cn(
        'flex items-center gap-3 p-3 rounded-lg border cursor-pointer transition-colors',
        selected ? 'border-primary bg-primary/5' : 'border-border hover:bg-muted/50'
      )}
    >
      <input
        type="radio"
        id={id}
        name="environment"
        checked={selected}
        onChange={onClick}
        className="sr-only"
      />
      <div
        className={cn(
          'w-4 h-4 rounded-full border-2 flex items-center justify-center shrink-0',
          selected ? 'border-primary' : 'border-muted-foreground/30'
        )}
      >
        {selected && <div className="w-2 h-2 rounded-full bg-primary" />}
      </div>
      <div className="flex-1">
        <span className={cn('text-sm font-medium', selected && 'text-primary')}>
          {label}
        </span>
        <p className={cn('text-xs', isProduction ? 'text-destructive' : 'text-muted-foreground')}>
          {description}
        </p>
      </div>
      <Badge
        variant={hasToken ? 'default' : 'secondary'}
        className={cn(
          hasToken
            ? 'bg-green-100 text-green-700 hover:bg-green-100 dark:bg-green-950/50 dark:text-green-400'
            : 'bg-amber-100 text-amber-700 hover:bg-amber-100 dark:bg-amber-950/50 dark:text-amber-400'
        )}
      >
        {hasToken ? 'Token configured' : 'No token'}
      </Badge>
    </label>
  )
}
