'use client'

import { useState } from 'react'
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
  Clock,
  X,
} from 'lucide-react'
import { useShowSubmit, type ShowSubmission } from '@/lib/hooks/useShowSubmit'
import { useShowUpdate, type ShowUpdate } from '@/lib/hooks/useShowUpdate'
import {
  combineDateTimeToUTC,
  parseISOToDateAndTime,
} from '@/lib/utils/timeUtils'
import type { Venue } from '@/lib/types/venue'
import type { ShowResponse } from '@/lib/types/show'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { FormField, ArtistInput, VenueInput } from '@/components/forms'

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
  venue: { name: '', city: '', state: '', address: '' },
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
  const { date, time } = parseISOToDateAndTime(show.event_date)
  const venue = show.venues[0]

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

interface ShowFormProps {
  /** Mode for the form - 'create' for new shows, 'edit' for updating existing */
  mode: 'create' | 'edit'
  /** Initial show data for pre-filling the form (required for edit mode) */
  initialData?: ShowResponse
  /** Callback after successful submission */
  onSuccess?: () => void
  /** Callback when cancel button is clicked (only shown in edit mode) */
  onCancel?: () => void
  /** Whether to redirect to shows page after create (defaults to true for create mode) */
  redirectOnCreate?: boolean
}

export function ShowForm({
  mode,
  initialData,
  onSuccess,
  onCancel,
  redirectOnCreate = true,
}: ShowFormProps) {
  const router = useRouter()
  const submitMutation = useShowSubmit()
  const updateMutation = useShowUpdate()
  const [showSuccess, setShowSuccess] = useState(false)
  const [isPendingSubmission, setIsPendingSubmission] = useState(false)

  const isEditMode = mode === 'edit'
  const mutation = isEditMode ? updateMutation : submitMutation

  // Compute initial values based on mode
  const initialFormValues =
    isEditMode && initialData ? showToFormValues(initialData) : defaultValues

  const form = useForm({
    defaultValues: initialFormValues,
    onSubmit: async ({ value }) => {
      // Combine date and time into UTC timestamp
      const eventDate = combineDateTimeToUTC(value.date, value.time || '20:00')

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
            onSuccess: () => {
              setShowSuccess(true)
              setTimeout(() => {
                onSuccess?.()
              }, 1500)
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

        submitMutation.mutate(submission, {
          onSuccess: data => {
            const isPending = data.status === 'pending'
            setIsPendingSubmission(isPending)
            setShowSuccess(true)
            form.reset()

            // Don't redirect for pending submissions - user should see the notice
            if (isPending) {
              // Call onSuccess after showing the pending notice
              setTimeout(() => {
                onSuccess?.()
              }, 4000) // Longer delay for pending notice
            } else if (redirectOnCreate) {
              setTimeout(() => {
                router.push('/shows')
              }, 2000)
            } else {
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

  // Handle venue selection to auto-fill city/state
  const handleVenueSelect = (venue: Venue | null) => {
    if (venue) {
      form.setFieldValue('venue.city', venue.city)
      form.setFieldValue('venue.state', venue.state)
      if (venue.address) {
        form.setFieldValue('venue.address', venue.address)
      }
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

  if (showSuccess) {
    // Show pending notice for submissions with unverified venues
    if (isPendingSubmission) {
      return (
        <div className="flex flex-col items-center justify-center py-8 text-center">
          <div className="rounded-full bg-amber-500/10 p-3 mb-3">
            <Clock className="h-6 w-6 text-amber-500" />
          </div>
          <h2 className="text-lg font-semibold mb-1">Pending Review</h2>
          <p className="text-sm text-muted-foreground mb-3 max-w-xs">
            Your show includes a new venue that needs admin verification. It
            will be visible once approved.
          </p>
          <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
        </div>
      )
    }

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
      {mutation.error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{mutation.error.message}</AlertDescription>
        </Alert>
      )}

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

        <form.Field name="venue.name">
          {field => (
            <VenueInput field={field} onVenueSelect={handleVenueSelect} />
          )}
        </form.Field>

        <div className="grid grid-cols-2 gap-4">
          <form.Field name="venue.city">
            {field => (
              <FormField field={field} label="City" placeholder="Phoenix" />
            )}
          </form.Field>

          <form.Field name="venue.state">
            {field => (
              <FormField field={field} label="State" placeholder="AZ" />
            )}
          </form.Field>
        </div>

        <form.Field name="venue.address">
          {field => (
            <FormField
              field={field}
              label="Address (Optional)"
              placeholder="123 Main St"
            />
          )}
        </form.Field>
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
    </form>
  )
}
