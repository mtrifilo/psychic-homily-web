'use client'

import { useState } from 'react'
import { Plus } from 'lucide-react'
import { useRequests, useCreateRequest } from '../hooks'
import { RequestCard } from './RequestCard'
import {
  REQUEST_ENTITY_TYPES,
  REQUEST_SORT_OPTIONS,
  getEntityTypeLabel,
  getStatusLabel,
} from '../types'
import type { Request } from '../types'
import { LoadingSpinner } from '@/components/shared'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { useAuthContext } from '@/lib/context/AuthContext'

const PAGE_SIZE = 20

const FILTERABLE_STATUSES = ['pending', 'in_progress', 'fulfilled'] as const

export function RequestList() {
  const { isAuthenticated } = useAuthContext()
  const [entityType, setEntityType] = useState('')
  const [status, setStatus] = useState('')
  const [sortBy, setSortBy] = useState('votes')
  const [offset, setOffset] = useState(0)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)

  const { data, isLoading, error, refetch } = useRequests({
    entity_type: entityType || undefined,
    status: status || undefined,
    sort_by: sortBy,
    limit: PAGE_SIZE,
    offset,
  })

  const requests = data?.requests ?? []
  const total = data?.total ?? 0
  const hasMore = offset + PAGE_SIZE < total

  const handleFilterChange = () => {
    setOffset(0)
  }

  if (isLoading && !data) {
    return (
      <div className="flex justify-center items-center py-12">
        <LoadingSpinner />
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center py-12 text-destructive">
        <p>Failed to load requests. Please try again later.</p>
        <Button variant="outline" className="mt-4" onClick={() => refetch()}>
          Retry
        </Button>
      </div>
    )
  }

  return (
    <section className="w-full max-w-6xl">
      {/* Filter bar + actions */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center gap-3 mb-6">
        <div className="flex items-center gap-2 flex-wrap flex-1">
          {/* Entity type filter */}
          <select
            value={entityType}
            onChange={e => {
              setEntityType(e.target.value)
              handleFilterChange()
            }}
            className="rounded-md border border-border bg-background px-3 py-1.5 text-sm"
            aria-label="Filter by entity type"
          >
            <option value="">All Types</option>
            {REQUEST_ENTITY_TYPES.map(type => (
              <option key={type} value={type}>
                {getEntityTypeLabel(type)}
              </option>
            ))}
          </select>

          {/* Status filter */}
          <select
            value={status}
            onChange={e => {
              setStatus(e.target.value)
              handleFilterChange()
            }}
            className="rounded-md border border-border bg-background px-3 py-1.5 text-sm"
            aria-label="Filter by status"
          >
            <option value="">All Statuses</option>
            {FILTERABLE_STATUSES.map(s => (
              <option key={s} value={s}>
                {getStatusLabel(s)}
              </option>
            ))}
          </select>

          {/* Sort */}
          <select
            value={sortBy}
            onChange={e => {
              setSortBy(e.target.value)
              handleFilterChange()
            }}
            className="rounded-md border border-border bg-background px-3 py-1.5 text-sm"
            aria-label="Sort by"
          >
            {REQUEST_SORT_OPTIONS.map(opt => (
              <option key={opt} value={opt}>
                {opt === 'votes'
                  ? 'Most Votes'
                  : opt === 'newest'
                    ? 'Newest'
                    : 'Oldest'}
              </option>
            ))}
          </select>
        </div>

        {isAuthenticated && (
          <Dialog open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
            <DialogTrigger asChild>
              <Button size="sm">
                <Plus className="h-4 w-4 mr-1.5" />
                New Request
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>New Request</DialogTitle>
              </DialogHeader>
              <CreateRequestForm
                onSuccess={() => setCreateDialogOpen(false)}
              />
            </DialogContent>
          </Dialog>
        )}
      </div>

      {/* Results count */}
      {total > 0 && (
        <p className="text-sm text-muted-foreground mb-4">
          {total} {total === 1 ? 'request' : 'requests'} found
        </p>
      )}

      {/* Request cards */}
      {requests.length === 0 ? (
        <div className="text-center py-12 text-muted-foreground">
          <p>No requests found.</p>
          {isAuthenticated && (
            <p className="text-sm mt-2">
              Be the first to create a request!
            </p>
          )}
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
          {requests.map((request: Request) => (
            <RequestCard key={request.id} request={request} />
          ))}
        </div>
      )}

      {/* Pagination */}
      {(offset > 0 || hasMore) && (
        <div className="flex items-center justify-center gap-3 mt-8">
          <Button
            variant="outline"
            size="sm"
            disabled={offset === 0}
            onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
          >
            Previous
          </Button>
          <span className="text-sm text-muted-foreground">
            Page {Math.floor(offset / PAGE_SIZE) + 1} of{' '}
            {Math.ceil(total / PAGE_SIZE)}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={!hasMore}
            onClick={() => setOffset(offset + PAGE_SIZE)}
          >
            Next
          </Button>
        </div>
      )}
    </section>
  )
}

// ──────────────────────────────────────────────
// Create Request Form (inline in dialog)
// ──────────────────────────────────────────────

function CreateRequestForm({ onSuccess }: { onSuccess: () => void }) {
  const createMutation = useCreateRequest()
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [entityType, setEntityType] = useState('artist')

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!title.trim()) return

    createMutation.mutate(
      {
        title: title.trim(),
        description: description.trim() || undefined,
        entity_type: entityType,
      },
      {
        onSuccess: () => {
          setTitle('')
          setDescription('')
          onSuccess()
        },
      }
    )
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div>
        <label
          htmlFor="request-title"
          className="text-sm font-medium mb-1.5 block"
        >
          Title
        </label>
        <Input
          id="request-title"
          value={title}
          onChange={e => setTitle(e.target.value)}
          placeholder="e.g., Add Slowdive discography"
          required
          autoFocus
        />
      </div>

      <div>
        <label
          htmlFor="request-entity-type"
          className="text-sm font-medium mb-1.5 block"
        >
          Entity Type
        </label>
        <select
          id="request-entity-type"
          value={entityType}
          onChange={e => setEntityType(e.target.value)}
          className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
        >
          {REQUEST_ENTITY_TYPES.map(type => (
            <option key={type} value={type}>
              {getEntityTypeLabel(type)}
            </option>
          ))}
        </select>
      </div>

      <div>
        <label
          htmlFor="request-description"
          className="text-sm font-medium mb-1.5 block"
        >
          Description (optional)
        </label>
        <Textarea
          id="request-description"
          value={description}
          onChange={e => setDescription(e.target.value)}
          placeholder="Describe what you'd like added and any relevant details..."
          rows={3}
        />
      </div>

      {createMutation.error && (
        <p className="text-sm text-destructive">
          {createMutation.error instanceof Error
            ? createMutation.error.message
            : 'Failed to create request'}
        </p>
      )}

      <div className="flex justify-end gap-2">
        <Button
          type="submit"
          disabled={!title.trim() || createMutation.isPending}
        >
          {createMutation.isPending ? 'Creating...' : 'Create Request'}
        </Button>
      </div>
    </form>
  )
}
