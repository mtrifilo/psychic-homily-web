'use client'

import { useState } from 'react'
import { useForm } from '@tanstack/react-form'
import { z } from 'zod'
import {
  Edit2,
  AlertCircle,
  CheckCircle2,
  Loader2,
} from 'lucide-react'
import { useVenueUpdate } from '../hooks/useVenueEdit'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useDismissTimer } from '@/lib/hooks/common'
import type { VenueWithShowCount, Venue } from '../types'
import {
  detectVenueChanges,
  venueMayOmitState,
  type VenueEditFormValues,
} from './venue-edit-utils'
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
import { FieldInfo } from '@/components/forms/FormField'

// How long the success flash shows before the dialog closes itself.
const SUCCESS_CLOSE_DELAY_MS = 1500

/**
 * The form's rules, with the state requirement supplied by the caller.
 *
 * A blank state is an exemption for the one venue the form opened on, decided
 * by `venueMayOmitState`, which owns why. Every other venue keeps the plain
 * requirement, and so does a state the user cleared.
 *
 * The exempt variant relaxes the requirement to "blank, or a state", NOT to
 * "anything". The two-character floor is a US postal-abbreviation check, and
 * dropping it outright would accept a one-character state that the exemption
 * then revokes itself over: `venueMayOmitState` answers false for any non-blank
 * state, so the next save on that venue would fail the floor against a value
 * this form had just written, and the server refuses to take it back
 * (`PUT /venues/{id}` answers 422 to `state: ""`).
 */
const makeVenueEditSchema = (stateIsOptional: boolean) =>
  z.object({
    name: z.string().min(1, 'Venue name is required'),
    address: z.string(),
    city: z.string().min(1, 'City is required'),
    state: stateIsOptional
      ? z
          .string()
          .refine(value => value === '' || value.length >= 2, {
            message: 'State is required',
          })
      : z.string().min(2, 'State is required'),
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

// Both variants, built once. The factory's whole input domain is one boolean,
// and a zod schema is a stateless parser, so per-mount construction would only
// re-allocate: VenueCard mounts one of these per row, above the admin guard.
const VENUE_EDIT_SCHEMA_STATE_REQUIRED = makeVenueEditSchema(false)
const VENUE_EDIT_SCHEMA_STATE_OPTIONAL = makeVenueEditSchema(true)

type FormValues = VenueEditFormValues

interface VenueEditFormProps {
  venue: VenueWithShowCount | Venue
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: () => void
}

// Admin-only direct-edit form. Non-admin edits go through the unified
// suggest-edit flow (EntityEditDrawer / useSuggestEdit).
//
// Callers MUST pass `key={venue.id}` so React unmounts + remounts with
// fresh state when the venue switches. The form deliberately does NOT
// useEffect to reset prop-derived state — see React's "You Might Not
// Need an Effect" guide for the rationale.
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

  // Hold the success flash, then close. Timer must not outlive unmount,
  // see useDismissTimer (PSY-1664).
  const { schedule: scheduleSuccessClose } = useDismissTimer(() => {
    resetDialogState()
    onOpenChange(false)
    onSuccess?.()
  }, SUCCESS_CLOSE_DELAY_MS)

  const isAdmin = user?.is_admin ?? false

  // Whether this venue's state field is optional.
  //
  // Captured once, at mount, for the same reason TanStack Form reads
  // `defaultValues` once: it has to describe the values the form is holding.
  // `key={venue.id}` pins the venue's IDENTITY across renders, not its columns,
  // and this prop rides a refetching query, so a recomputed rule could revoke
  // the exemption mid-edit and fail a save the form had already called legal.
  const [stateIsOptional] = useState(() => venueMayOmitState(venue))
  const venueEditSchema = stateIsOptional
    ? VENUE_EDIT_SCHEMA_STATE_OPTIONAL
    : VENUE_EDIT_SCHEMA_STATE_REQUIRED

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
            scheduleSuccessClose()
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
          <Alert className="mb-4 border-success-foreground bg-success">
            <CheckCircle2 className="h-4 w-4 text-success-foreground" />
            <AlertDescription className="text-success-foreground">
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
                    aria-invalid={field.state.meta.errors.length > 0}
                  />
                  <FieldInfo field={field} />
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
                      aria-invalid={field.state.meta.errors.length > 0}
                    />
                    <FieldInfo field={field} />
                  </div>
                )}
              </form.Field>

              <form.Field name="state">
                {field => (
                  <div className="space-y-2">
                    <Label htmlFor="state">
                      State{stateIsOptional ? '' : ' *'}
                    </Label>
                    <Input
                      id="state"
                      value={field.state.value}
                      onChange={e => field.handleChange(e.target.value)}
                      onBlur={field.handleBlur}
                      placeholder="e.g. IL"
                      aria-invalid={field.state.meta.errors.length > 0}
                    />
                    <FieldInfo field={field} />
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
