'use client'

import { useState, useEffect } from 'react'
import { useForm } from '@tanstack/react-form'
import { z } from 'zod'
import { Loader2, Edit2, AlertCircle, CheckCircle2 } from 'lucide-react'
import { useArtistUpdate } from '@/lib/hooks/useAdminArtists'
import type { Artist, ArtistEditRequest } from '@/lib/types/artist'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Alert, AlertDescription } from '@/components/ui/alert'

const artistEditSchema = z.object({
  name: z.string().min(1, 'Artist name is required'),
  city: z.string(),
  state: z.string(),
  instagram: z.string(),
  facebook: z.string(),
  twitter: z.string(),
  youtube: z.string(),
  spotify: z.string(),
  soundcloud: z.string(),
  bandcamp: z.string(),
  website: z.string(),
})

interface FormValues {
  name: string
  city: string
  state: string
  instagram: string
  facebook: string
  twitter: string
  youtube: string
  spotify: string
  soundcloud: string
  bandcamp: string
  website: string
}

interface ArtistEditFormProps {
  artist: Artist
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

export function ArtistEditForm({
  artist,
  open,
  onOpenChange,
  onSuccess,
}: ArtistEditFormProps) {
  const updateMutation = useArtistUpdate()
  const [showSuccess, setShowSuccess] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const initialValues: FormValues = {
    name: artist.name,
    city: artist.city || '',
    state: artist.state || '',
    instagram: artist.social?.instagram || '',
    facebook: artist.social?.facebook || '',
    twitter: artist.social?.twitter || '',
    youtube: artist.social?.youtube || '',
    spotify: artist.social?.spotify || '',
    soundcloud: artist.social?.soundcloud || '',
    bandcamp: artist.social?.bandcamp || '',
    website: artist.social?.website || '',
  }

  const form = useForm({
    defaultValues: initialValues,
    onSubmit: async ({ value }) => {
      setError(null)

      // Build request with only changed fields
      const changes: ArtistEditRequest = {}

      if (value.name !== artist.name) changes.name = value.name
      if (value.city !== (artist.city || ''))
        changes.city = value.city || undefined
      if (value.state !== (artist.state || ''))
        changes.state = value.state || undefined
      if (value.instagram !== (artist.social?.instagram || ''))
        changes.instagram = value.instagram || undefined
      if (value.facebook !== (artist.social?.facebook || ''))
        changes.facebook = value.facebook || undefined
      if (value.twitter !== (artist.social?.twitter || ''))
        changes.twitter = value.twitter || undefined
      if (value.youtube !== (artist.social?.youtube || ''))
        changes.youtube = value.youtube || undefined
      if (value.spotify !== (artist.social?.spotify || ''))
        changes.spotify = value.spotify || undefined
      if (value.soundcloud !== (artist.social?.soundcloud || ''))
        changes.soundcloud = value.soundcloud || undefined
      if (value.bandcamp !== (artist.social?.bandcamp || ''))
        changes.bandcamp = value.bandcamp || undefined
      if (value.website !== (artist.social?.website || ''))
        changes.website = value.website || undefined

      if (Object.keys(changes).length === 0) {
        setError('No changes detected')
        return
      }

      updateMutation.mutate(
        { artistId: artist.id, data: changes },
        {
          onSuccess: () => {
            setShowSuccess(true)
            setTimeout(() => {
              setShowSuccess(false)
              onOpenChange(false)
              onSuccess?.()
            }, 1500)
          },
          onError: err => {
            setError(
              err instanceof Error ? err.message : 'Failed to update artist'
            )
          },
        }
      )
    },
    validators: {
      onSubmit: artistEditSchema,
    },
  })

  // Reset form when dialog opens
  useEffect(() => {
    if (open) {
      form.reset()
      setError(null)
      setShowSuccess(false)
    }
  }, [open, artist.id])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Edit2 className="h-5 w-5" />
            Edit Artist
          </DialogTitle>
          <DialogDescription>
            Make changes to this artist. Changes will be applied immediately.
          </DialogDescription>
        </DialogHeader>

        {showSuccess && (
          <Alert className="mb-4 border-green-500 bg-green-50 dark:bg-green-950">
            <CheckCircle2 className="h-4 w-4 text-green-600" />
            <AlertDescription className="text-green-600">
              Artist updated successfully!
            </AlertDescription>
          </Alert>
        )}

        {error && (
          <Alert variant="destructive" className="mb-4">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <form
          onSubmit={e => {
            e.preventDefault()
            e.stopPropagation()
            form.handleSubmit()
          }}
          className="space-y-4"
        >
          {/* Basic Info */}
          <div className="space-y-4">
            <h3 className="text-sm font-medium">Basic Information</h3>

            <form.Field name="name">
              {field => (
                <div className="space-y-2">
                  <Label htmlFor="artist-name">Name *</Label>
                  <Input
                    id="artist-name"
                    value={field.state.value}
                    onChange={e => field.handleChange(e.target.value)}
                    onBlur={field.handleBlur}
                    placeholder="e.g. The National"
                  />
                </div>
              )}
            </form.Field>

            <div className="grid grid-cols-2 gap-4">
              <form.Field name="city">
                {field => (
                  <div className="space-y-2">
                    <Label htmlFor="artist-city">City</Label>
                    <Input
                      id="artist-city"
                      value={field.state.value}
                      onChange={e => field.handleChange(e.target.value)}
                      onBlur={field.handleBlur}
                      placeholder="e.g. Phoenix"
                    />
                  </div>
                )}
              </form.Field>

              <form.Field name="state">
                {field => (
                  <div className="space-y-2">
                    <Label htmlFor="artist-state">State</Label>
                    <Input
                      id="artist-state"
                      value={field.state.value}
                      onChange={e => field.handleChange(e.target.value)}
                      onBlur={field.handleBlur}
                      placeholder="e.g. AZ"
                    />
                  </div>
                )}
              </form.Field>
            </div>
          </div>

          {/* Social Links */}
          <div className="space-y-4 pt-4 border-t">
            <h3 className="text-sm font-medium">Social Links</h3>

            <div className="grid grid-cols-2 gap-4">
              <form.Field name="website">
                {field => (
                  <div className="space-y-2">
                    <Label htmlFor="artist-website">Website</Label>
                    <Input
                      id="artist-website"
                      value={field.state.value}
                      onChange={e => field.handleChange(e.target.value)}
                      onBlur={field.handleBlur}
                      placeholder="https://..."
                    />
                  </div>
                )}
              </form.Field>

              <form.Field name="instagram">
                {field => (
                  <div className="space-y-2">
                    <Label htmlFor="artist-instagram">Instagram</Label>
                    <Input
                      id="artist-instagram"
                      value={field.state.value}
                      onChange={e => field.handleChange(e.target.value)}
                      onBlur={field.handleBlur}
                      placeholder="https://instagram.com/..."
                    />
                  </div>
                )}
              </form.Field>

              <form.Field name="facebook">
                {field => (
                  <div className="space-y-2">
                    <Label htmlFor="artist-facebook">Facebook</Label>
                    <Input
                      id="artist-facebook"
                      value={field.state.value}
                      onChange={e => field.handleChange(e.target.value)}
                      onBlur={field.handleBlur}
                      placeholder="https://facebook.com/..."
                    />
                  </div>
                )}
              </form.Field>

              <form.Field name="twitter">
                {field => (
                  <div className="space-y-2">
                    <Label htmlFor="artist-twitter">Twitter/X</Label>
                    <Input
                      id="artist-twitter"
                      value={field.state.value}
                      onChange={e => field.handleChange(e.target.value)}
                      onBlur={field.handleBlur}
                      placeholder="https://twitter.com/..."
                    />
                  </div>
                )}
              </form.Field>

              <form.Field name="youtube">
                {field => (
                  <div className="space-y-2">
                    <Label htmlFor="artist-youtube">YouTube</Label>
                    <Input
                      id="artist-youtube"
                      value={field.state.value}
                      onChange={e => field.handleChange(e.target.value)}
                      onBlur={field.handleBlur}
                      placeholder="https://youtube.com/..."
                    />
                  </div>
                )}
              </form.Field>

              <form.Field name="spotify">
                {field => (
                  <div className="space-y-2">
                    <Label htmlFor="artist-spotify">Spotify</Label>
                    <Input
                      id="artist-spotify"
                      value={field.state.value}
                      onChange={e => field.handleChange(e.target.value)}
                      onBlur={field.handleBlur}
                      placeholder="https://open.spotify.com/..."
                    />
                  </div>
                )}
              </form.Field>

              <form.Field name="soundcloud">
                {field => (
                  <div className="space-y-2">
                    <Label htmlFor="artist-soundcloud">SoundCloud</Label>
                    <Input
                      id="artist-soundcloud"
                      value={field.state.value}
                      onChange={e => field.handleChange(e.target.value)}
                      onBlur={field.handleBlur}
                      placeholder="https://soundcloud.com/..."
                    />
                  </div>
                )}
              </form.Field>

              <form.Field name="bandcamp">
                {field => (
                  <div className="space-y-2">
                    <Label htmlFor="artist-bandcamp">Bandcamp</Label>
                    <Input
                      id="artist-bandcamp"
                      value={field.state.value}
                      onChange={e => field.handleChange(e.target.value)}
                      onBlur={field.handleBlur}
                      placeholder="https://....bandcamp.com"
                    />
                  </div>
                )}
              </form.Field>
            </div>
          </div>

          <DialogFooter className="pt-4">
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={updateMutation.isPending}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={updateMutation.isPending}>
              {updateMutation.isPending ? (
                <>
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                  Saving...
                </>
              ) : (
                'Save Changes'
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
