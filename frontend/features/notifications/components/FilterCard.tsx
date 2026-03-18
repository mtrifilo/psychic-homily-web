'use client'

import { useState } from 'react'
import { Bell, BellOff, Pencil, Trash2, Loader2, MoreHorizontal } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useUpdateFilter, useDeleteFilter } from '../hooks'
import type { NotificationFilter } from '../types'
import { getFilterSummary, formatTimeAgo } from '../types'

interface FilterCardProps {
  filter: NotificationFilter
  onEdit: (filter: NotificationFilter) => void
}

export function FilterCard({ filter, onEdit }: FilterCardProps) {
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false)

  const updateFilter = useUpdateFilter()
  const deleteFilter = useDeleteFilter()

  const isMutating = updateFilter.isPending || deleteFilter.isPending

  const handleToggleActive = () => {
    updateFilter.mutate({
      id: filter.id,
      is_active: !filter.is_active,
    })
  }

  const handleDelete = () => {
    deleteFilter.mutate(filter.id, {
      onSuccess: () => setShowDeleteConfirm(false),
    })
  }

  return (
    <div className="rounded-lg border border-border/50 bg-card p-4">
      <div className="flex items-start gap-3">
        {/* Active toggle */}
        <div className="pt-0.5">
          <Switch
            checked={filter.is_active}
            onCheckedChange={handleToggleActive}
            disabled={isMutating}
            aria-label={filter.is_active ? 'Pause filter' : 'Activate filter'}
          />
        </div>

        {/* Filter details */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            {filter.is_active ? (
              <Bell className="h-4 w-4 text-primary shrink-0" />
            ) : (
              <BellOff className="h-4 w-4 text-muted-foreground shrink-0" />
            )}
            <h3 className="font-medium text-sm truncate">
              {filter.name}
            </h3>
          </div>

          <p className="text-xs text-muted-foreground mt-1 line-clamp-2">
            {getFilterSummary(filter)}
          </p>

          <div className="flex items-center gap-3 mt-2 text-xs text-muted-foreground">
            <span>
              {filter.match_count} {filter.match_count === 1 ? 'match' : 'matches'}
            </span>
            {filter.last_matched_at && (
              <span>Last: {formatTimeAgo(filter.last_matched_at)}</span>
            )}
          </div>
        </div>

        {/* Actions */}
        <div className="flex items-center gap-1 shrink-0">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onEdit(filter)}
            className="h-8 w-8 p-0"
            title="Edit filter"
          >
            <Pencil className="h-3.5 w-3.5" />
          </Button>

          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className="h-8 w-8 p-0"
              >
                <MoreHorizontal className="h-3.5 w-3.5" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => onEdit(filter)}>
                <Pencil className="h-3.5 w-3.5 mr-2" />
                Edit
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => setShowDeleteConfirm(true)}
                className="text-destructive focus:text-destructive"
              >
                <Trash2 className="h-3.5 w-3.5 mr-2" />
                Delete
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {/* Delete confirmation */}
      {showDeleteConfirm && (
        <div className="mt-3 pt-3 border-t border-border/50 flex items-center justify-between">
          <p className="text-xs text-muted-foreground">
            Delete this filter? This cannot be undone.
          </p>
          <div className="flex items-center gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setShowDeleteConfirm(false)}
              disabled={deleteFilter.isPending}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              size="sm"
              onClick={handleDelete}
              disabled={deleteFilter.isPending}
            >
              {deleteFilter.isPending ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin mr-1" />
              ) : null}
              Delete
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
