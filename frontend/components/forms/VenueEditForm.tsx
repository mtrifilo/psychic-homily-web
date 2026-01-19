'use client'

import { useState, useEffect } from 'react'
import { useForm } from '@tanstack/react-form'
import { z } from 'zod'
import {
  Loader2,
  Edit2,
  AlertCircle,
  CheckCircle2,
  Clock,
  X,
} from 'lucide-react'
import { useVenueUpdate, useMyPendingVenueEdit, useCancelPendingVenueEdit } from '@/lib/hooks/useVenueEdit'
import { useAuthContext } from '@/lib/context/AuthContext'
import type { VenueWithShowCount, VenueEditRequest, Venue } from '@/lib/types/venue'
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

interface FormValues {
  name: string
  address: string
  city: string
  state: string
  zipcode: string
  instagram: string
  facebook: string
  twitter: string
  youtube: string
  spotify: string
  soundcloud: string
  bandcamp: string
  website: string
}

interface VenueEditFormProps {
  venue: VenueWithShowCount | Venue
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

export function VenueEditForm({
  venue,
  open,
  onOpenChange,
  onSuccess,
}: VenueEditFormProps) {
  const { user } = useAuthContext()
  const updateMutation = useVenueUpdate()
  const cancelMutation = useCancelPendingVenueEdit()
  const [showSuccess, setShowSuccess] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const isAdmin = user?.is_admin ?? false

  // Fetch user's pending edit if they're not an admin
  const { data: pendingEditData, isLoading: isPendingEditLoading } =
    useMyPendingVenueEdit(venue.id, open && !isAdmin)

  const hasPendingEdit = pendingEditData?.pending_edit != null

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

      // Build request with only changed fields
      const changes: VenueEditRequest = {}

      if (value.name !== venue.name) changes.name = value.name
      if (value.address !== (venue.address || ''))
        changes.address = value.address || undefined
      if (value.city !== venue.city) changes.city = value.city
      if (value.state !== venue.state) changes.state = value.state
      if (value.zipcode !== (venue.zipcode || ''))
        changes.zipcode = value.zipcode || undefined
      if (value.instagram !== (venue.social?.instagram || ''))
        changes.instagram = value.instagram || undefined
      if (value.facebook !== (venue.social?.facebook || ''))
        changes.facebook = value.facebook || undefined
      if (value.twitter !== (venue.social?.twitter || ''))
        changes.twitter = value.twitter || undefined
      if (value.youtube !== (venue.social?.youtube || ''))
        changes.youtube = value.youtube || undefined
      if (value.spotify !== (venue.social?.spotify || ''))
        changes.spotify = value.spotify || undefined
      if (value.soundcloud !== (venue.social?.soundcloud || ''))
        changes.soundcloud = value.soundcloud || undefined
      if (value.bandcamp !== (venue.social?.bandcamp || ''))
        changes.bandcamp = value.bandcamp || undefined
      if (value.website !== (venue.social?.website || ''))
        changes.website = value.website || undefined

      // Check if any changes were made
      if (Object.keys(changes).length === 0) {
        setError('No changes detected')
        return
      }

      updateMutation.mutate(
        { venueId: venue.id, data: changes },
        {
          onSuccess: response => {
            setShowSuccess(true)
            setTimeout(() => {
              setShowSuccess(false)
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
      setError(null)
      setShowSuccess(false)
    }
  }, [open, venue.id])

  const handleCancelPendingEdit = () => {
    cancelMutation.mutate(venue.id, {
      onSuccess: () => {
        // The query will be invalidated automatically
      },
    })
  }

  // Show pending edit status for non-admins
  if (!isAdmin && hasPendingEdit && pendingEditData?.pending_edit) {
    const pendingEdit = pendingEditData.pending_edit
    return (
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Clock className="h-5 w-5 text-amber-500" />
              Pending Edit
            </DialogTitle>
            <DialogDescription>
              You have a pending edit for this venue awaiting admin review.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4 py-4">
            <Alert>
              <AlertCircle className="h-4 w-4" />
              <AlertDescription>
                Your changes have been submitted and are awaiting admin
                approval. The venue will be updated once approved.
              </AlertDescription>
            </Alert>

            <div className="rounded-lg border border-border bg-muted/50 p-4">
              <p className="text-sm font-medium mb-2">Proposed Changes:</p>
              <ul className="space-y-1 text-sm text-muted-foreground">
                {pendingEdit.name && (
                  <li>
                    <span className="font-medium">Name:</span> {pendingEdit.name}
                  </li>
                )}
                {pendingEdit.address && (
                  <li>
                    <span className="font-medium">Address:</span>{' '}
                    {pendingEdit.address}
                  </li>
                )}
                {pendingEdit.city && (
                  <li>
                    <span className="font-medium">City:</span> {pendingEdit.city}
                  </li>
                )}
                {pendingEdit.state && (
                  <li>
                    <span className="font-medium">State:</span>{' '}
                    {pendingEdit.state}
                  </li>
                )}
                {pendingEdit.website && (
                  <li>
                    <span className="font-medium">Website:</span>{' '}
                    {pendingEdit.website}
                  </li>
                )}
              </ul>
              <p className="text-xs text-muted-foreground mt-2">
                Submitted:{' '}
                {new Date(pendingEdit.created_at).toLocaleDateString()}
              </p>
            </div>
          </div>

          <DialogFooter>
            <Button
              variant="outline"
              onClick={handleCancelPendingEdit}
              disabled={cancelMutation.isPending}
            >
              {cancelMutation.isPending ? (
                <>
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                  Canceling...
                </>
              ) : (
                <>
                  <X className="h-4 w-4 mr-2" />
                  Cancel Edit
                </>
              )}
            </Button>
            <Button onClick={() => onOpenChange(false)}>Close</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    )
  }

  // Loading state
  if (!isAdmin && isPendingEditLoading) {
    return (
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="sm:max-w-lg">
          <div className="flex items-center justify-center py-8">
            <Loader2 className="h-6 w-6 animate-spin" />
          </div>
        </DialogContent>
      </Dialog>
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Edit2 className="h-5 w-5" />
            Edit Venue
          </DialogTitle>
          <DialogDescription>
            {isAdmin
              ? 'Make changes to this venue. Changes will be applied immediately.'
              : 'Submit changes for admin review. Your changes will be visible after approval.'}
          </DialogDescription>
        </DialogHeader>

        {!isAdmin && (
          <Alert className="mb-4">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>
              Changes will be submitted for admin review before being applied.
            </AlertDescription>
          </Alert>
        )}

        {showSuccess && (
          <Alert className="mb-4 border-green-500 bg-green-50 dark:bg-green-950">
            <CheckCircle2 className="h-4 w-4 text-green-600" />
            <AlertDescription className="text-green-600">
              {isAdmin
                ? 'Venue updated successfully!'
                : 'Your changes have been submitted for review!'}
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
              onClick={() => onOpenChange(false)}
              disabled={updateMutation.isPending}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={updateMutation.isPending}>
              {updateMutation.isPending ? (
                <>
                  <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                  {isAdmin ? 'Saving...' : 'Submitting...'}
                </>
              ) : isAdmin ? (
                'Save Changes'
              ) : (
                'Submit for Review'
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
