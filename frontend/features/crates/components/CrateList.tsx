'use client'

import { useState } from 'react'
import { Plus } from 'lucide-react'
import { useCrates, useCreateCrate } from '../hooks'
import { CrateCard } from './CrateCard'
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
import type { Crate } from '../types'

export function CrateList() {
  const { isAuthenticated } = useAuthContext()
  const { data, isLoading, error, refetch } = useCrates()
  const [createDialogOpen, setCreateDialogOpen] = useState(false)

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
        <p>Failed to load crates. Please try again later.</p>
        <Button variant="outline" className="mt-4" onClick={() => refetch()}>
          Retry
        </Button>
      </div>
    )
  }

  const allCrates = data?.crates ?? []

  // Separate featured and non-featured
  const featured = allCrates.filter((c: Crate) => c.is_featured)
  const regular = allCrates.filter((c: Crate) => !c.is_featured)

  return (
    <section className="w-full max-w-6xl">
      {/* Actions bar */}
      {isAuthenticated && (
        <div className="flex justify-end mb-6">
          <Dialog open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
            <DialogTrigger asChild>
              <Button size="sm">
                <Plus className="h-4 w-4 mr-1.5" />
                Create Crate
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Create Crate</DialogTitle>
              </DialogHeader>
              <CreateCrateForm
                onSuccess={() => setCreateDialogOpen(false)}
              />
            </DialogContent>
          </Dialog>
        </div>
      )}

      {/* Featured crates */}
      {featured.length > 0 && (
        <div className="mb-8">
          <h2 className="text-lg font-semibold mb-4">Featured</h2>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
            {featured.map((crate: Crate) => (
              <CrateCard key={crate.id} crate={crate} />
            ))}
          </div>
        </div>
      )}

      {/* All crates */}
      <div>
        {featured.length > 0 && regular.length > 0 && (
          <h2 className="text-lg font-semibold mb-4">All Crates</h2>
        )}
        {allCrates.length === 0 ? (
          <div className="text-center py-12 text-muted-foreground">
            <p>No public crates yet.</p>
            {isAuthenticated && (
              <p className="text-sm mt-2">
                Be the first to create one!
              </p>
            )}
          </div>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
            {regular.map((crate: Crate) => (
              <CrateCard key={crate.id} crate={crate} />
            ))}
          </div>
        )}
      </div>
    </section>
  )
}

// ──────────────────────────────────────────────
// Create Crate Form (inline in dialog)
// ──────────────────────────────────────────────

function CreateCrateForm({ onSuccess }: { onSuccess: () => void }) {
  const createMutation = useCreateCrate()
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [isPublic, setIsPublic] = useState(true)
  const [collaborative, setCollaborative] = useState(false)

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!title.trim()) return

    createMutation.mutate(
      {
        title: title.trim(),
        description: description.trim() || undefined,
        is_public: isPublic,
        collaborative,
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
          htmlFor="crate-title"
          className="text-sm font-medium mb-1.5 block"
        >
          Title
        </label>
        <Input
          id="crate-title"
          value={title}
          onChange={e => setTitle(e.target.value)}
          placeholder="My Favorite Artists"
          required
          autoFocus
        />
      </div>

      <div>
        <label
          htmlFor="crate-description"
          className="text-sm font-medium mb-1.5 block"
        >
          Description (optional)
        </label>
        <Textarea
          id="crate-description"
          value={description}
          onChange={e => setDescription(e.target.value)}
          placeholder="A brief description of this crate..."
          rows={3}
        />
      </div>

      <div className="flex items-center gap-6">
        <label className="flex items-center gap-2 text-sm cursor-pointer">
          <input
            type="checkbox"
            checked={isPublic}
            onChange={e => setIsPublic(e.target.checked)}
            className="rounded border-border"
          />
          Public
        </label>

        <label className="flex items-center gap-2 text-sm cursor-pointer">
          <input
            type="checkbox"
            checked={collaborative}
            onChange={e => setCollaborative(e.target.checked)}
            className="rounded border-border"
          />
          Collaborative
        </label>
      </div>

      {createMutation.error && (
        <p className="text-sm text-destructive">
          {createMutation.error instanceof Error
            ? createMutation.error.message
            : 'Failed to create crate'}
        </p>
      )}

      <div className="flex justify-end gap-2">
        <Button
          type="submit"
          disabled={!title.trim() || createMutation.isPending}
        >
          {createMutation.isPending ? 'Creating...' : 'Create'}
        </Button>
      </div>
    </form>
  )
}
