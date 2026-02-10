'use client'

import { useState, useEffect, useRef } from 'react'
import { useRouter } from 'next/navigation'
import { useForm } from '@tanstack/react-form'
import { z } from 'zod'
import {
  AlertCircle,
  Loader2,
  Plus,
  Music,
  Calendar,
  MapPin,
  CheckCircle2,
  X,
  ShieldCheck,
  Info,
  EyeOff,
} from 'lucide-react'
import { useShowSubmit, type ShowSubmission } from '@/lib/hooks/useShowSubmit'
import {
  useShowUpdate,
  type ShowUpdate,
  type ShowUpdateResponse,
} from '@/lib/hooks/useShowUpdate'
import {
  combineDateTimeToUTC,
  parseISOToDateAndTime,
  getTimezoneForState,
} from '@/lib/utils/timeUtils'
import type { Venue } from '@/lib/types/venue'
import type { ShowResponse, VenueResponse, OrphanedArtist } from '@/lib/types/show'
import type { ExtractedShowData } from '@/lib/types/extraction'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import { FormField, ArtistInput, VenueInput } from '@/components/forms'
import { OrphanedArtistsDialog } from '@/components/forms/OrphanedArtistsDialog'
import { useAuthContext } from '@/lib/context/AuthContext'

// Form validation schema
const showFormSchema = z.object({
  title: z.string(),
  artists: z
    .array(
      z.object({
        name: z.string().min(1, 'Artist name is required'),
        is_headliner: z.boolean(),
      })
    )
    .min(1, 'At least one artist is required'),
  venue: z.object({
    name: z.string().min(1, 'Venue name is required'),
    city: z.string().min(1, 'City is required'),
    state: z.string().min(1, 'State is required'),
    address: z.string(),
  }),
  date: z.string().min(1, 'Date is required'),
  time: z.string(),
  cost: z.string(),
  ages: z.string(),
  description: z.string(),
})

interface FormArtist {
  name: string
  is_headliner: boolean
}

interface FormValues {
  title: string
  artists: FormArtist[]
  venue: {
    id?: number
    name: string
    city: string
    state: string
    address: string
  }
  date: string
  time: string
  cost: string
  ages: string
  description: string
}

const defaultValues: FormValues = {
  title: '',
  artists: [{ name: '', is_headliner: true }],
  venue: { id: undefined, name: '', city: '', state: '', address: '' },
  date: '',
  time: '20:00',
  cost: '',
  ages: '',
  description: '',
}

/**
 * Convert ShowResponse data to form values for editing
 */
function showToFormValues(show: ShowResponse): FormValues {
  const venue = show.venues[0]
  const venueTz = venue?.state ? getTimezoneForState(venue.state) : undefined
  const { date, time } = parseISOToDateAndTime(show.event_date, venueTz)

  return {
    title: show.title || '',
    artists: show.artists.map(artist => ({
      name: artist.name,
      is_headliner: artist.is_headliner ?? false,
    })),
    venue: {
      name: venue?.name || '',
      city: venue?.city || show.city || '',
      state: venue?.state || show.state || '',
      address: venue?.address || '',
    },
    date,
    time,
    cost: show.price != null ? `$${show.price}` : '',
    ages: show.age_requirement || '',
    description: show.description || '',
  }
}

/** Pre-filled venue data for locking venue selection */
interface PrefilledVenue {
  id: number
  slug: string
  name: string
  city: string
  state: string
  address?: string | null
  verified?: boolean
}

interface ShowFormProps {
  /** Mode for the form - 'create' for new shows, 'edit' for updating existing */
  mode: 'create' | 'edit'
  /** Initial show data for pre-filling the form (required for edit mode) */
  initialData?: ShowResponse
  /** Pre-filled venue (locks venue selection) */
  prefilledVenue?: PrefilledVenue
  /** Callback after successful submission */
  onSuccess?: () => void
  /** Callback when cancel button is clicked (only shown in edit mode) */
  onCancel?: () => void
  /** Whether to redirect to shows page after create (defaults to true for create mode) */
  redirectOnCreate?: boolean
  /** AI-extracted show data to populate form (used with AIFormFiller) */
  initialExtraction?: ExtractedShowData
}

export function ShowForm({
  mode,
  initialData,
  prefilledVenue,
  onSuccess,
  onCancel,
  redirectOnCreate = true,
  initialExtraction,
}: ShowFormProps) {
  const router = useRouter()
  const { user } = useAuthContext()
  const submitMutation = useShowSubmit()
  const updateMutation = useShowUpdate()
  const [showSuccess, setShowSuccess] = useState(false)
  const [isPrivateShow, setIsPrivateShow] = useState(false)
  const [orphanedArtists, setOrphanedArtists] = useState<OrphanedArtist[]>([])
  const [showOrphanDialog, setShowOrphanDialog] = useState(false)

  // Track selected venue for editability checks
  // Initialize from prefilledVenue, initialData (for edit mode), or null for create mode
  const [selectedVenue, setSelectedVenue] = useState<VenueResponse | null>(
    () =>
      prefilledVenue
        ? {
            id: prefilledVenue.id,
            slug: prefilledVenue.slug,
            name: prefilledVenue.name,
            city: prefilledVenue.city,
            state: prefilledVenue.state,
            address: prefilledVenue.address ?? null,
            verified: prefilledVenue.verified ?? false,
          }
        : (initialData?.venues[0] ?? null)
  )

  const isEditMode = mode === 'edit'
  const isAdmin = user?.is_admin ?? false
  const mutation = isEditMode ? updateMutation : submitMutation

  // Venue location fields are editable if:
  // 1. Prefilled venue is NOT used (it locks venue selection), AND
  // 2. User is admin (always editable), OR
  // 3. No venue selected (new venue scenario), OR
  // 4. Selected venue is unverified
  const isVenueLocationEditable =
    !prefilledVenue && (isAdmin || !selectedVenue || !selectedVenue.verified)

  // Compute initial values based on mode
  const initialFormValues = (() => {
    if (isEditMode && initialData) {
      return showToFormValues(initialData)
    }
    if (prefilledVenue) {
      return {
        ...defaultValues,
        venue: {
          id: prefilledVenue.id,
          name: prefilledVenue.name,
          city: prefilledVenue.city,
          state: prefilledVenue.state,
          address: prefilledVenue.address || '',
        },
      }
    }
    return defaultValues
  })()

  // Track venue name for showing/hiding the "new venue" warning
  const [venueName, setVenueName] = useState('')

  const form = useForm({
    defaultValues: initialFormValues,
    onSubmit: async ({ value }) => {
      // Combine date and time into UTC timestamp using the venue's timezone
      const venueTimezone = value.venue.state ? getTimezoneForState(value.venue.state) : undefined
      const eventDate = combineDateTimeToUTC(value.date, value.time || '20:00', venueTimezone)

      // Parse cost to number if provided
      const price = value.cost
        ? parseFloat(value.cost.replace(/[^0-9.]/g, ''))
        : undefined

      if (isEditMode && initialData) {
        // Build update payload including venues and artists
        const updates: ShowUpdate = {
          title: value.title || undefined,
          event_date: eventDate,
          city: value.venue.city,
          state: value.venue.state,
          price: isNaN(price as number) ? undefined : price,
          age_requirement: value.ages || undefined,
          description: value.description || undefined,
          venues: [
            {
              id: value.venue.id,
              name: value.venue.name,
              city: value.venue.city,
              state: value.venue.state,
              address: value.venue.address || undefined,
            },
          ],
          artists: value.artists.map(artist => ({
            name: artist.name,
            is_headliner: artist.is_headliner,
          })),
        }

        updateMutation.mutate(
          { showId: initialData.id, updates },
          {
            onSuccess: (data: ShowUpdateResponse) => {
              setShowSuccess(true)
              if (
                data.orphaned_artists &&
                data.orphaned_artists.length > 0
              ) {
                setOrphanedArtists(data.orphaned_artists)
                setShowOrphanDialog(true)
              } else {
                setTimeout(() => {
                  onSuccess?.()
                }, 1500)
              }
            },
          }
        )
      } else {
        // Build create submission payload
        const submission: ShowSubmission = {
          title: value.title || undefined,
          event_date: eventDate,
          city: value.venue.city,
          state: value.venue.state,
          price: isNaN(price as number) ? undefined : price,
          age_requirement: value.ages || undefined,
          description: value.description || undefined,
          venues: [
            {
              id: value.venue.id,
              name: value.venue.name,
              city: value.venue.city,
              state: value.venue.state,
              address: value.venue.address || undefined,
            },
          ],
          artists: value.artists.map(artist => ({
            name: artist.name,
            is_headliner: artist.is_headliner,
          })),
          // Only include is_private for new/unverified venue submissions
          is_private: isPrivateShow || undefined,
        }

        submitMutation.mutate(submission, {
          onSuccess: data => {
            const isPrivate = data.status === 'private'
            form.reset()
            setIsPrivateShow(false)

            if (isPrivate) {
              // Private shows redirect with dialog trigger
              router.push('/collection?submitted=private')
            } else if (redirectOnCreate) {
              // Approved submissions show brief success then redirect
              setShowSuccess(true)
              setTimeout(() => {
                router.push('/collection')
              }, 2000)
            } else {
              setShowSuccess(true)
              setTimeout(() => {
                onSuccess?.()
              }, 1500)
            }
          },
        })
      }
    },
    validators: {
      onSubmit: showFormSchema,
    },
  })

  // Populate form from AI extraction
  // Using a ref to track the last applied extraction to avoid duplicate applications
  const lastAppliedExtraction = useRef<ExtractedShowData | null>(null)

  useEffect(() => {
    if (!initialExtraction) return
    // Skip if we've already applied this exact extraction
    if (lastAppliedExtraction.current === initialExtraction) return
    lastAppliedExtraction.current = initialExtraction

    // Use requestAnimationFrame to batch state updates and avoid cascading renders
    requestAnimationFrame(() => {
      // Set artists
      if (initialExtraction.artists.length > 0) {
        form.setFieldValue(
          'artists',
          initialExtraction.artists.map(a => ({
            name: a.matched_name || a.name,
            is_headliner: a.is_headliner,
          }))
        )
      }

      // Set venue
      if (initialExtraction.venue) {
        const v = initialExtraction.venue
        form.setFieldValue('venue', {
          id: v.matched_id,
          name: v.matched_name || v.name,
          city: v.city || '',
          state: v.state || '',
          address: '',
        })
        // Update venue name for new venue warning
        setVenueName(v.matched_name || v.name)
        // Update selected venue if matched
        if (v.matched_id && v.matched_name && v.matched_slug) {
          setSelectedVenue({
            id: v.matched_id,
            slug: v.matched_slug,
            name: v.matched_name,
            address: null,
            city: v.city || '',
            state: v.state || '',
            verified: true, // Assume matched venues are verified
          })
        } else {
          setSelectedVenue(null)
        }
      }

      // Set date
      if (initialExtraction.date) {
        form.setFieldValue('date', initialExtraction.date)
      }

      // Set time
      if (initialExtraction.time) {
        form.setFieldValue('time', initialExtraction.time)
      }

      // Set cost
      if (initialExtraction.cost) {
        form.setFieldValue('cost', initialExtraction.cost)
      }

      // Set ages
      if (initialExtraction.ages) {
        form.setFieldValue('ages', initialExtraction.ages)
      }

      // Set description
      if (initialExtraction.description) {
        form.setFieldValue('description', initialExtraction.description)
      }
    })
  }, [initialExtraction, form])

  // Handle venue selection to auto-fill city/state and track selected venue
  const handleVenueSelect = (venue: Venue | null) => {
    if (venue) {
      // Store the full venue object for editability checks
      setSelectedVenue({
        id: venue.id,
        slug: venue.slug,
        name: venue.name,
        address: venue.address,
        city: venue.city,
        state: venue.state,
        verified: venue.verified,
      })
      form.setFieldValue('venue.id', venue.id)
      form.setFieldValue('venue.city', venue.city)
      form.setFieldValue('venue.state', venue.state)
      if (venue.address) {
        form.setFieldValue('venue.address', venue.address)
      }
      // Reset private show option when selecting a verified venue
      if (venue.verified) {
        setIsPrivateShow(false)
      }
    } else {
      // Cleared venue selection (user is typing a new venue)
      setSelectedVenue(null)
      form.setFieldValue('venue.id', undefined)
    }
  }

  const handleAddArtist = () => {
    const currentArtists = form.getFieldValue('artists')
    form.setFieldValue('artists', [
      ...currentArtists,
      { name: '', is_headliner: false },
    ])
  }

  const handleRemoveArtist = (index: number) => {
    const currentArtists = form.getFieldValue('artists')
    if (currentArtists.length <= 1) return

    const wasHeadliner = currentArtists[index]?.is_headliner
    const newArtists = currentArtists.filter(
      (_: FormArtist, i: number) => i !== index
    )

    // If we removed the headliner, make the first remaining artist the headliner
    if (wasHeadliner && newArtists.length > 0) {
      newArtists[0].is_headliner = true
    }

    form.setFieldValue('artists', newArtists)
  }

  // Render success state (only shown for approved/edit submissions, pending/private redirect immediately)
  if (showSuccess) {
    return (
      <div className="flex flex-col items-center justify-center py-8 text-center">
        <div className="rounded-full bg-primary/10 p-3 mb-3">
          <CheckCircle2 className="h-6 w-6 text-primary" />
        </div>
        <h2 className="text-lg font-semibold mb-1">
          {isEditMode ? 'Show Updated!' : 'Show Submitted!'}
        </h2>
        <p className="text-sm text-muted-foreground mb-3">
          {isEditMode
            ? 'Your changes have been saved.'
            : 'Your show has been added.'}
        </p>
        <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <form
      onSubmit={e => {
        e.preventDefault()
        e.stopPropagation()
        form.handleSubmit()
      }}
      className="space-y-6"
    >
      {/* Form Header */}
      <div className="space-y-1.5 pb-2">
        <h3 className="text-lg font-semibold">Show Details</h3>
        <p className="text-sm text-muted-foreground">
          Fill out the information below to add a show. Artists and venues will
          be matched or created automatically.
        </p>
      </div>

      {/* Artists Section */}
      <div className="space-y-4">
        <div className="flex items-center gap-2">
          <Music className="h-4 w-4 text-muted-foreground" />
          <h3 className="font-medium">Artists</h3>
        </div>

        <form.Field name="artists" mode="array">
          {artistsField => (
            <div className="space-y-4">
              {artistsField.state.value.map((_: FormArtist, index: number) => (
                <form.Field key={index} name={`artists[${index}].name`}>
                  {field => (
                    <ArtistInput
                      field={field}
                      index={index}
                      showRemoveButton={artistsField.state.value.length > 1}
                      onRemove={() => handleRemoveArtist(index)}
                    />
                  )}
                </form.Field>
              ))}
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={handleAddArtist}
              >
                <Plus className="h-4 w-4 mr-2" />
                Add another artist
              </Button>
            </div>
          )}
        </form.Field>
      </div>

      {/* Show Title (Optional) */}
      <form.Field name="title">
        {field => (
          <FormField
            field={field}
            label="Show Title (Optional)"
            placeholder="Leave blank to auto-generate from artists"
          />
        )}
      </form.Field>

      {/* Venue Section */}
      <div className="space-y-4">
        <div className="flex items-center gap-2">
          <MapPin className="h-4 w-4 text-muted-foreground" />
          <h3 className="font-medium">Venue & Location</h3>
        </div>

        {prefilledVenue ? (
          <div className="space-y-2">
            <Label>Venue</Label>
            <div className="flex items-center gap-2 px-3 py-2 rounded-md bg-muted border">
              <MapPin className="h-4 w-4 text-muted-foreground" />
              <span className="font-medium">{prefilledVenue.name}</span>
              <span className="text-muted-foreground">
                — {prefilledVenue.city}, {prefilledVenue.state}
              </span>
            </div>
          </div>
        ) : (
          <form.Field name="venue.name">
            {field => (
              <VenueInput
                field={field}
                onVenueSelect={handleVenueSelect}
                onVenueNameChange={setVenueName}
              />
            )}
          </form.Field>
        )}

        <div className="grid grid-cols-2 gap-4">
          <form.Field name="venue.city">
            {field => (
              <FormField
                field={field}
                label="City"
                placeholder="Phoenix"
                disabled={!isVenueLocationEditable}
              />
            )}
          </form.Field>

          <form.Field name="venue.state">
            {field => (
              <FormField
                field={field}
                label="State"
                placeholder="AZ"
                disabled={!isVenueLocationEditable}
              />
            )}
          </form.Field>
        </div>

        <form.Field name="venue.address">
          {field => (
            <FormField
              field={field}
              label="Address (Optional)"
              placeholder="123 Main St"
              disabled={!isVenueLocationEditable}
            />
          )}
        </form.Field>

        {/* Verified venue indicator (for non-admins with verified venue selected, but not for prefilled venues) */}
        {selectedVenue?.verified && !isAdmin && !prefilledVenue && (
          <div className="flex items-start gap-2 rounded-md bg-emerald-500/10 border border-emerald-500/20 p-3">
            <ShieldCheck className="h-4 w-4 text-emerald-500 mt-0.5 flex-shrink-0" />
            <div className="text-sm">
              <p className="font-medium text-emerald-600 dark:text-emerald-400">
                Verified Venue
              </p>
              <p className="text-muted-foreground text-xs mt-0.5">
                This venue has been verified by our team. Location details are
                locked to ensure accuracy. Contact an admin if changes are
                needed.
              </p>
            </div>
          </div>
        )}

        {/* New venue info (for non-admins entering a new venue) */}
        {!selectedVenue && !isAdmin && venueName && (
          <div className="space-y-3">
            <div className="flex items-start gap-2 rounded-md bg-blue-500/10 border border-blue-500/20 p-3">
              <Info className="h-4 w-4 text-blue-500 mt-0.5 flex-shrink-0" />
              <div className="text-sm">
                <p className="font-medium text-blue-600 dark:text-blue-400">
                  New Venue
                </p>
                <p className="text-muted-foreground text-xs mt-0.5">
                  Your show will be published with city-only location until the
                  venue is verified. This protects venue privacy for
                  new or unconfirmed locations.
                </p>
              </div>
            </div>

            {/* Private show option - only shown for new venues in create mode */}
            {!isEditMode && (
              <div className="flex items-start gap-3 rounded-md bg-slate-500/10 border border-slate-500/20 p-3">
                <Checkbox
                  id="is-private-show"
                  checked={isPrivateShow}
                  onCheckedChange={checked => setIsPrivateShow(!!checked)}
                  className="mt-0.5"
                />
                <label
                  htmlFor="is-private-show"
                  className="text-sm cursor-pointer"
                >
                  <span className="font-medium flex items-center gap-1.5">
                    <EyeOff className="h-3.5 w-3.5" />
                    Do not publish - this show is just for my list
                  </span>
                  <p className="text-muted-foreground text-xs mt-0.5">
                    Private shows will only appear in your personal list.
                  </p>
                </label>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Date & Time Section */}
      <div className="space-y-4">
        <div className="flex items-center gap-2">
          <Calendar className="h-4 w-4 text-muted-foreground" />
          <h3 className="font-medium">Date & Time</h3>
        </div>

        <div className="grid grid-cols-2 gap-4">
          <form.Field name="date">
            {field => <FormField field={field} label="Date" type="date" />}
          </form.Field>

          <form.Field name="time">
            {field => <FormField field={field} label="Time" type="time" />}
          </form.Field>
        </div>
      </div>

      {/* Additional Details */}
      <div className="space-y-4">
        <h3 className="font-medium">Additional Details</h3>

        <div className="grid grid-cols-2 gap-4">
          <form.Field name="cost">
            {field => (
              <FormField
                field={field}
                label="Cost (Optional)"
                placeholder="$20, Free"
              />
            )}
          </form.Field>

          <form.Field name="ages">
            {field => (
              <FormField
                field={field}
                label="Ages (Optional)"
                placeholder="21+, All Ages"
              />
            )}
          </form.Field>
        </div>

        <form.Field name="description">
          {field => (
            <FormField
              field={field}
              label="Description (Optional)"
              type="textarea"
              placeholder="Additional show details..."
            />
          )}
        </form.Field>
      </div>

      {mutation.error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{mutation.error.message}</AlertDescription>
        </Alert>
      )}

      {/* Action Buttons */}
      <div className={isEditMode ? 'flex gap-3' : ''}>
        {isEditMode && onCancel && (
          <Button
            type="button"
            variant="outline"
            className="flex-1"
            onClick={onCancel}
          >
            <X className="h-4 w-4 mr-2" />
            Cancel
          </Button>
        )}
        <form.Subscribe
          selector={state => [state.canSubmit, state.isSubmitting]}
        >
          {([canSubmit, isSubmitting]) => (
            <Button
              type="submit"
              className={isEditMode ? 'flex-1' : 'w-full'}
              disabled={!canSubmit || isSubmitting || mutation.isPending}
            >
              {isSubmitting || mutation.isPending ? (
                <>
                  <Loader2 className="h-4 w-4 animate-spin mr-2" />
                  {isEditMode ? 'Saving...' : 'Submitting...'}
                </>
              ) : isEditMode ? (
                'Save Changes'
              ) : (
                'Submit Show'
              )}
            </Button>
          )}
        </form.Subscribe>
      </div>

      {isEditMode && (
        <OrphanedArtistsDialog
          open={showOrphanDialog}
          onOpenChange={setShowOrphanDialog}
          artists={orphanedArtists}
          onComplete={() => {
            onSuccess?.()
          }}
        />
      )}
    </form>
  )
}
