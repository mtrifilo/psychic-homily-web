'use client'

import { useEffect, useState } from 'react'
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
} from 'lucide-react'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useShowSubmit, type ShowSubmission } from '@/lib/hooks/useShowSubmit'
import { combineDateTimeToUTC } from '@/lib/utils/timeUtils'
import type { Venue } from '@/lib/types/venue'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { FormField, ArtistInput, VenueInput } from '@/components/forms'

// Form validation schema
const showFormSchema = z.object({
  title: z.string().optional(),
  artists: z
    .array(
      z.object({
        name: z.string().min(1, 'Artist name is required'),
        is_headliner: z.boolean().optional(),
      })
    )
    .min(1, 'At least one artist is required'),
  venue: z.object({
    name: z.string().min(1, 'Venue name is required'),
    city: z.string().min(1, 'City is required'),
    state: z.string().min(1, 'State is required'),
    address: z.string().optional(),
  }),
  date: z.string().min(1, 'Date is required'),
  time: z.string().optional(),
  cost: z.string().optional(),
  ages: z.string().optional(),
  description: z.string().optional(),
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

function ShowForm() {
  const router = useRouter()
  const showMutation = useShowSubmit()
  const [showSuccess, setShowSuccess] = useState(false)

  const form = useForm({
    defaultValues,
    onSubmit: async ({ value }) => {
      // Combine date and time into UTC timestamp
      const eventDate = combineDateTimeToUTC(value.date, value.time || '20:00')

      // Parse cost to number if provided
      const price = value.cost
        ? parseFloat(value.cost.replace(/[^0-9.]/g, ''))
        : undefined

      // Build submission payload
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

      showMutation.mutate(submission, {
        onSuccess: () => {
          setShowSuccess(true)
          form.reset()
          // Redirect after a short delay to show success message
          setTimeout(() => {
            router.push('/shows')
          }, 2000)
        },
      })
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
    return (
      <div className="flex flex-col items-center justify-center py-12 text-center">
        <div className="rounded-full bg-primary/10 p-4 mb-4">
          <CheckCircle2 className="h-8 w-8 text-primary" />
        </div>
        <h2 className="text-xl font-semibold mb-2">Show Submitted!</h2>
        <p className="text-muted-foreground mb-4">
          Your show has been added. Redirecting to shows...
        </p>
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
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
      {showMutation.error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{showMutation.error.message}</AlertDescription>
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
              {artistsField.state.value.map(
                (_: FormArtist, index: number) => (
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
                )
              )}
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

      {/* Submit Button */}
      <form.Subscribe selector={state => [state.canSubmit, state.isSubmitting]}>
        {([canSubmit, isSubmitting]) => (
          <Button
            type="submit"
            className="w-full"
            disabled={!canSubmit || isSubmitting || showMutation.isPending}
          >
            {isSubmitting || showMutation.isPending ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Submitting...
              </>
            ) : (
              'Submit Show'
            )}
          </Button>
        )}
      </form.Subscribe>
    </form>
  )
}

export default function SubmissionsPage() {
  const router = useRouter()
  const { isAuthenticated, isLoading, user } = useAuthContext()

  // Redirect unauthenticated users to login
  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      router.push('/auth')
    }
  }, [isAuthenticated, isLoading, router])

  // Show loading state while checking auth
  if (isLoading) {
    return (
      <div className="flex min-h-[calc(100vh-64px)] items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  // Don't render if not authenticated (will redirect)
  if (!isAuthenticated) {
    return null
  }

  return (
    <div className="min-h-[calc(100vh-64px)] bg-background px-4 py-8">
      <div className="mx-auto max-w-lg">
        {/* Header */}
        <div className="mb-8 text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
            <Music className="h-6 w-6 text-primary" />
          </div>
          <h1 className="text-2xl font-bold tracking-tight">Submit a Show</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Add an upcoming show to the Arizona music calendar
          </p>
          {user && (
            <p className="mt-1 text-xs text-muted-foreground">
              Submitting as {user.email}
            </p>
          )}
        </div>

        {/* Form Card */}
        <Card className="border-border/50 bg-card/50 backdrop-blur-sm">
          <CardHeader className="pb-4">
            <CardTitle className="text-lg">Show Details</CardTitle>
            <CardDescription>
              Fill out the information below to add a show. Artists and venues
              will be matched or created automatically.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <ShowForm />
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

