import { Checkbox } from '../ui/checkbox'
import { Button } from '../ui/button'
import { Label } from '../ui/label'
import { ListSkeleton } from '../shared/LoadingSkeleton'
import { EmptyState } from '../shared/EmptyState'
import { FileSearch } from 'lucide-react'

interface ExportListProps<T> {
  items: T[]
  selectedIds: Set<string>
  getId: (item: T) => string
  loading?: boolean
  onToggle: (id: string) => void
  onSelectAll: () => void
  onClear: () => void
  renderItem: (item: T) => React.ReactNode
  emptyMessage?: string
}

export function ExportList<T>({
  items,
  selectedIds,
  getId,
  loading,
  onToggle,
  onSelectAll,
  onClear,
  renderItem,
  emptyMessage = 'No items loaded',
}: ExportListProps<T>) {
  if (loading) {
    return <ListSkeleton count={5} className="p-4" />
  }

  if (items.length === 0) {
    return (
      <EmptyState
        icon={FileSearch}
        title={emptyMessage}
        description="Click the Load button to fetch data"
        className="py-8"
      />
    )
  }

  return (
    <div className="bg-card rounded-lg border">
      <div className="px-4 py-2 bg-muted/50 border-b flex items-center justify-between">
        <span className="text-sm font-medium text-foreground">
          {selectedIds.size} selected
        </span>
        <div className="flex gap-2">
          <Button variant="link" size="sm" onClick={onSelectAll} className="px-0">
            Select All
          </Button>
          <span className="text-muted-foreground">|</span>
          <Button
            variant="link"
            size="sm"
            onClick={onClear}
            className="px-0 text-muted-foreground"
          >
            Clear
          </Button>
        </div>
      </div>
      <div className="max-h-96 overflow-y-auto divide-y divide-border">
        {items.map(item => {
          const id = getId(item)
          return (
            <label
              key={id}
              className="flex items-start gap-3 px-4 py-3 hover:bg-muted/30 cursor-pointer"
            >
              <Checkbox
                checked={selectedIds.has(id)}
                onCheckedChange={() => onToggle(id)}
                className="mt-0.5"
              />
              <div className="flex-1 min-w-0">{renderItem(item)}</div>
            </label>
          )
        })}
      </div>
    </div>
  )
}
