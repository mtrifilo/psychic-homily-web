'use client'

import { useState, useEffect } from 'react'
import { useForm } from '@tanstack/react-form'
import { z } from 'zod'
import {
  Edit2,
  AlertCircle,
  CheckCircle2,
  Loader2,
} from 'lucide-react'
import { useVenueUpdate } from '@/features/venues'
import { useAuthContext } from '@/lib/context/AuthContext'
import type { VenueWithShowCount, Venue } from '@/features/venues'
import { detectVenueChanges, type VenueEditFormValues } from './venue-edit-utils'
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

// Form validation schema
const venueEditSchema = z.object({
  name: z.string().min(1, 'Venue name is required'),
  address: z.string(),
  city: z.string().min(1, 'City is required'),
  state: z.string().min(2, 'State is required'),
  zipcode: z.string(),
  instagram: z.string(),
  facebook: z.string(),
  twitter: z.string(),
  youtube: z.string(),
  spotify: z.string(),
  soundcloud: z.string(),
  bandcamp: z.string(),
  website: z.string(),
})

type FormValues = VenueEditFormValues

interface VenueEditFormProps {
  venue: VenueWithShowCount | Venue
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

// PSY-503: This form is now admin-only. Non-admin edits go through the
// unified suggest-edit flow (EntityEditDrawer / useSuggestEdit).
export function VenueEditForm({
  venue,
  open,
  onOpenChange,
  onSuccess,
}: VenueEditFormProps) {
  const { user } = useAuthContext()
  const updateMutation = useVenueUpdate()
  const [showSuccess, setShowSuccess] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const resetDialogState = () => {
    setError(null)
    setShowSuccess(false)
  }

  const isAdmin = user?.is_admin ?? false

  // Initialize form with venue data
  const initialValues: FormValues = {
    name: venue.name,
    address: venue.address || '',
    city: venue.city,
    state: venue.state,
    zipcode: venue.zipcode || '',
    instagram: venue.social?.instagram || '',
    facebook: venue.social?.facebook || '',
    twitter: venue.social?.twitter || '',
    youtube: venue.social?.youtube || '',
    spotify: venue.social?.spotify || '',
    soundcloud: venue.social?.soundcloud || '',
    bandcamp: venue.social?.bandcamp || '',
    website: venue.social?.website || '',
  }

  const form = useForm({
    defaultValues: initialValues,
    onSubmit: async ({ value }) => {
      setError(null)

      const changes = detectVenueChanges(value, venue)

      if (!changes) {
        setError('No changes detected')
        return
      }

      updateMutation.mutate(
        { venueId: venue.id, data: changes },
        {
          onSuccess: () => {
            setShowSuccess(true)
            setTimeout(() => {
              resetDialogState()
              onOpenChange(false)
              onSuccess?.()
            }, 1500)
          },
          onError: err => {
            setError(
              err instanceof Error ? err.message : 'Failed to update venue'
            )
          },
        }
      )
    },
    validators: {
      onSubmit: venueEditSchema,
    },
  })

  // Reset form when venue changes
  useEffect(() => {
    if (open) {
      form.reset()
    }
  }, [open, venue.id])

  const handleDialogOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      resetDialogState()
    }
    onOpenChange(nextOpen)
  }

  // Non-admins should not see this form. Guard here as a safety net;
  // VenueCard.canEdit should already hide the trigger for non-admins.
  if (!isAdmin) {
    return null
  }

  return (
    <Dialog open={open} onOpenChange={handleDialogOpenChange}>
      <DialogContent className="sm:max-w-lg max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Edit2 className="h-5 w-5" />
            Edit Venue
          </DialogTitle>
          <DialogDescription>
            Make changes to this venue. Changes will be applied immediately.
          </DialogDescription>
        </DialogHeader>

        {showSuccess && (
          <Alert className="mb-4 border-green-500 bg-green-50 dark:bg-green-950">
            <CheckCircle2 className="h-4 w-4 text-green-600" />
            <AlertDescription className="text-green-600">
              Venue updated successfully!
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
                  <Label htmlFor="name">Venue Name *</Label>
                  <Input
                    id="name"
                    value={field.state.value}
                    onChange={e => field.handleChange(e.target.value)}
                    onBlur={field.handleBlur}
                    placeholder="e.g. The Empty Bottle"
                  />
                </div>
              )}
            </form.Field>

            <form.Field name="address">
              {field => (
                <div className="space-y-2">
                  <Label htmlFor="address">Address</Label>
                  <Input
                    id="address"
                    value={field.state.value}
                    onChange={e => field.handleChange(e.target.value)}
                    onBlur={field.handleBlur}
                    placeholder="e.g. 1035 N Western Ave"
                  />
                </div>
              )}
            </form.Field>

            <div className="grid grid-cols-2 gap-4">
              <form.Field name="city">
                {field => (
                  <div className="space-y-2">
                    <Label htmlFor="city">City *</Label>
                    <Input
                      id="city"
                      value={field.state.value}
                      onChange={e => field.handleChange(e.target.value)}
                      onBlur={field.handleBlur}
                      placeholder="e.g. Chicago"
                    />
                  </div>
                )}
              </form.Field>

              <form.Field name="state">
                {field => (
                  <div className="space-y-2">
                    <Label htmlFor="state">State *</Label>
                    <Input
                      id="state"
                      value={field.state.value}
                      onChange={e => field.handleChange(e.target.value)}
                      onBlur={field.handleBlur}
                      placeholder="e.g. IL"
                    />
                  </div>
                )}
              </form.Field>
            </div>

            <form.Field name="zipcode">
              {field => (
                <div className="space-y-2">
                  <Label htmlFor="zipcode">Zipcode</Label>
                  <Input
                    id="zipcode"
                    value={field.state.value}
                    onChange={e => field.handleChange(e.target.value)}
                    onBlur={field.handleBlur}
                    placeholder="e.g. 60622"
                  />
                </div>
              )}
            </form.Field>
          </div>

          {/* Social Links */}
          <div className="space-y-4 pt-4 border-t">
            <h3 className="text-sm font-medium">Social Links</h3>

            <div className="grid grid-cols-2 gap-4">
              <form.Field name="website">
                {field => (
                  <div className="space-y-2">
                    <Label htmlFor="website">Website</Label>
                    <Input
                      id="website"
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
                    <Label htmlFor="instagram">Instagram</Label>
                    <Input
                      id="instagram"
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
                    <Label htmlFor="facebook">Facebook</Label>
                    <Input
                      id="facebook"
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
                    <Label htmlFor="twitter">Twitter/X</Label>
                    <Input
                      id="twitter"
                      value={field.state.value}
                      onChange={e => field.handleChange(e.target.value)}
                      onBlur={field.handleBlur}
                      placeholder="https://twitter.com/..."
                    />
                  </div>
                )}
              </form.Field>

              <form.Field name="spotify">
                {field => (
                  <div className="space-y-2">
                    <Label htmlFor="spotify">Spotify</Label>
                    <Input
                      id="spotify"
                      value={field.state.value}
                      onChange={e => field.handleChange(e.target.value)}
                      onBlur={field.handleBlur}
                      placeholder="https://open.spotify.com/..."
                    />
                  </div>
                )}
              </form.Field>

              <form.Field name="bandcamp">
                {field => (
                  <div className="space-y-2">
                    <Label htmlFor="bandcamp">Bandcamp</Label>
                    <Input
                      id="bandcamp"
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
              onClick={() => handleDialogOpenChange(false)}
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
