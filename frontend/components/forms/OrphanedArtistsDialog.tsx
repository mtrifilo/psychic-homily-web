'use client'

import { useState } from 'react'
import { Loader2, Trash2, UserX } from 'lucide-react'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import type { OrphanedArtist } from '@/lib/types/show'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'

interface OrphanedArtistsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  artists: OrphanedArtist[]
  onComplete?: () => void
}

export function OrphanedArtistsDialog({
  open,
  onOpenChange,
  artists,
  onComplete,
}: OrphanedArtistsDialogProps) {
  const [isDeleting, setIsDeleting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleDelete = async () => {
    setIsDeleting(true)
    setError(null)

    try {
      await Promise.all(
        artists.map(artist =>
          apiRequest(API_ENDPOINTS.ARTISTS.DELETE(artist.id), {
            method: 'DELETE',
          })
        )
      )
      onOpenChange(false)
      onComplete?.()
    } catch {
      setError('Failed to delete some artists. They may have been associated with other shows.')
    } finally {
      setIsDeleting(false)
    }
  }

  const handleKeep = () => {
    onOpenChange(false)
    onComplete?.()
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <UserX className="h-5 w-5 text-muted-foreground" />
            Orphaned Artists Found
          </DialogTitle>
          <DialogDescription>
            The following artists are no longer associated with any shows.
            Would you like to delete them?
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-2 py-4">
          <ul className="space-y-2">
            {artists.map(artist => (
              <li
                key={artist.id}
                className="flex items-center gap-2 rounded-md border border-border/50 bg-muted/30 px-3 py-2 text-sm"
              >
                <span className="font-medium">{artist.name}</span>
              </li>
            ))}
          </ul>

          {error && (
            <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
              {error}
            </div>
          )}
        </div>

        <DialogFooter className="gap-2 sm:gap-0">
          <Button
            variant="outline"
            onClick={handleKeep}
            disabled={isDeleting}
          >
            Keep All
          </Button>
          <Button
            variant="destructive"
            onClick={handleDelete}
            disabled={isDeleting}
          >
            {isDeleting ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Deleting...
              </>
            ) : (
              <>
                <Trash2 className="mr-2 h-4 w-4" />
                Delete {artists.length === 1 ? 'Artist' : 'Artists'}
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
