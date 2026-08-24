'use client'

import { useState } from 'react'
import { Bell, Plus, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useNotificationFilters } from '../hooks'
import { FilterCard } from './FilterCard'
import { FilterForm } from './FilterForm'
import type { NotificationFilter } from '../types'

export function FilterList() {
  const { data, isLoading, error } = useNotificationFilters()
  const [showCreateForm, setShowCreateForm] = useState(false)
  const [editingFilter, setEditingFilter] = useState<NotificationFilter | undefined>()

  const filters = data?.filters ?? []

  if (isLoading) {
    return (
      <div className="flex justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="py-12 text-center">
        <p className="text-sm text-destructive">
          Failed to load your custom alerts. Please try again.
        </p>
      </div>
    )
  }

  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Custom alerts</h1>
          {/* Names the OTHER thing on this page. PSY-1905 merged Follow and
              Notify me on artist and venue pages, and the merged control
              deliberately leaves the filters the old button created alone
              (they are a separate lane with its own per-alert email switch).
              This page is where they live and where they can be edited,
              paused or deleted, so it has to say so: otherwise a reader who
              set one up before the merge sees a list of things they do not
              remember building, on a page whose heading claims to be about
              criteria they wrote themselves. */}
          <p className="text-sm text-muted-foreground mt-1">
            Get told when a new show matching your criteria is added. Anything
            you set up with &ldquo;Notify me&rdquo; is listed here too, with
            its own email switch.
          </p>
        </div>
        <Button onClick={() => setShowCreateForm(true)} className="gap-1.5">
          <Plus className="h-4 w-4" />
          New alert
        </Button>
      </div>

      {/* Filter list */}
      {filters.length === 0 ? (
        <div className="rounded-lg border border-dashed border-muted-foreground/25 bg-muted/30 py-12 text-center">
          <Bell className="h-10 w-10 text-muted-foreground/40 mx-auto mb-3" />
          <h3 className="text-sm font-medium mb-1">No custom alerts</h3>
          <p className="text-xs text-muted-foreground mb-4 max-w-sm mx-auto">
            Build one to be told when shows matching your interests are added:
            filter by tag, price cap, or several cities at once. Label and tag
            pages carry a &quot;Notify me&quot; button for quick setup.
          </p>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setShowCreateForm(true)}
            className="gap-1.5"
          >
            <Plus className="h-4 w-4" />
            Create your first alert
          </Button>
        </div>
      ) : (
        <div className="space-y-3">
          {filters.map(filter => (
            <FilterCard
              key={filter.id}
              filter={filter}
              onEdit={f => setEditingFilter(f)}
            />
          ))}
        </div>
      )}

      {/* Create dialog */}
      <FilterForm
        open={showCreateForm}
        onOpenChange={setShowCreateForm}
      />

      {/* Edit dialog */}
      <FilterForm
        open={!!editingFilter}
        onOpenChange={open => {
          if (!open) setEditingFilter(undefined)
        }}
        filter={editingFilter}
      />
    </div>
  )
}
