import { useForm } from '@tanstack/react-form'
import { z } from 'zod'
import { Button } from '@/components/ui/button'
import { FormField } from '@/components/ui/form-field'
import { useShow } from '@/lib/hooks/useShow'
import { combineDateTimeToUTC } from '@/lib/utils/timeUtils'
import { ArtistInput } from './ArtistInput'

interface Artist {
    name: string
    is_headliner?: boolean
}

interface ArtistFieldProp {
    pushValue: (value: Artist) => void
}

interface ShowSubmission {
    title?: string
    artists: Artist[]
    venue: { name: string; id?: string; city: string; state: string; address?: string }
    date: string
    time?: string
    cost?: string
    ages?: string
    city: string
    state: string
    description?: string // Description is also optional
}

const defaultShowSubmission: ShowSubmission = {
    title: '',
    artists: [{ name: '', is_headliner: true }], // First artist is headliner by default
    venue: { name: '', city: '', state: '' },
    date: '',
    time: '20:00',
    cost: '',
    ages: '',
    city: '',
    state: '',
    description: '',
}

const artistSchema = z.object({
    name: z.string().min(1, 'Artist name is required'),
    id: z.string().optional(),
    is_headliner: z.boolean().optional(),
})

const venueSchema = z.object({
    name: z.string().min(1, 'Venue name is required'),
    id: z.string().optional(),
    city: z.string().min(1, 'Venue city is required'),
    state: z.string().min(1, 'Venue state is required'),
    address: z.string().optional(),
})

const formSchema = z.object({
    title: z.string().optional(), // Show title is now optional
    artists: z.array(artistSchema).min(1, 'At least one artist is required'),
    venue: venueSchema,
    date: z.string().min(1, 'Date is required'),
    time: z.string().optional(),
    cost: z.string().optional(),
    ages: z.string().optional(),
    city: z.string().min(1, 'City is required'),
    state: z.string().min(1, 'State is required'),
    description: z.string().optional(),
})

export const ShowForm = () => {
    const showMutation = useShow()

    const form = useForm({
        defaultValues: defaultShowSubmission,
        onSubmit: async ({ value }) => {
            // Combine date and time into a UTC timestamp
            const combinedDateTime = combineDateTimeToUTC(value.date, value.time || '20:00')

            // Transform data to match backend API structure
            const submissionData = {
                title: value.title || undefined, // Send undefined for empty titles
                event_date: combinedDateTime,
                city: value.city,
                state: value.state,
                price: value.cost ? parseFloat(value.cost) : undefined,
                age_requirement: value.ages || undefined,
                description: value.description || undefined,
                venues: [
                    {
                        name: value.venue.name,
                        city: value.city,
                        state: value.state,
                        address: value.venue.address || undefined,
                    },
                ],
                artists: value.artists.map((artist) => ({
                    name: artist.name,
                    is_headliner: artist.is_headliner ?? false, // Use the actual headliner status from form
                })),
            }

            try {
                console.log('submitting form:', submissionData)
                await showMutation.mutateAsync(submissionData)
                // Reset form on success
                form.reset()
            } catch (error) {
                console.error('Failed to submit show:', error)
                // Error handling is done by the mutation
            }
        },
        validators: {
            onSubmit: formSchema,
        },
    })

    const handleAddArtist = (artistsField: ArtistFieldProp) => {
        artistsField.pushValue({ name: '', is_headliner: false }) // New artists are not headliners
    }

    return (
        <div className="flex flex-col items-start justify-center w-md">
            <form
                className="w-full space-y-4"
                onSubmit={(e) => {
                    e.preventDefault()
                    e.stopPropagation()
                    form.handleSubmit()
                }}
            >
                <div className="w-full">
                    <form.Field
                        name="artists"
                        mode="array"
                        children={(artistsField) => (
                            <div>
                                {artistsField.state.value.map((_, i) => (
                                    <div key={i} className={i > 0 ? 'mt-4' : ''}>
                                        <form.Field
                                            name={`artists[${i}].name`}
                                            children={(field) => (
                                                <ArtistInput
                                                    field={field}
                                                    showRemoveButton={artistsField.state.value.length > 1}
                                                    onRemove={() => {
                                                        const currentArtists = artistsField.state.value
                                                        const isRemovingHeadliner = currentArtists[i]?.is_headliner

                                                        artistsField.removeValue(i)

                                                        // If we removed the headliner, make the first remaining artist the headliner
                                                        if (isRemovingHeadliner && currentArtists.length > 1) {
                                                            const remainingArtists = currentArtists.filter(
                                                                (_, index) => index !== i
                                                            )
                                                            if (remainingArtists.length > 0) {
                                                                // Update the first remaining artist to be headliner
                                                                form.setFieldValue(`artists[0].is_headliner`, true)
                                                            }
                                                        }
                                                    }}
                                                />
                                            )}
                                        />
                                    </div>
                                ))}
                                <div className="mt-6">
                                    <Button type="button" onClick={() => handleAddArtist(artistsField)}>
                                        Add another artist
                                    </Button>
                                </div>
                            </div>
                        )}
                    />
                </div>
                <form.Field name="title">
                    {(field) => <FormField field={field} label="Show Title (Optional)" />}
                </form.Field>
                <form.Field name="venue.name">{(field) => <FormField field={field} label="Venue" />}</form.Field>
                <form.Field name="date">{(field) => <FormField field={field} label="Date" type="date" />}</form.Field>
                <form.Field name="time">{(field) => <FormField field={field} label="Time" type="time" />}</form.Field>
                <form.Field name="cost">
                    {(field) => <FormField field={field} label="Cost" placeholder="e.g. $20, Free" />}
                </form.Field>
                <form.Field name="ages">
                    {(field) => <FormField field={field} label="Ages" placeholder="e.g. 21+, All Ages" />}
                </form.Field>
                <form.Field name="city">
                    {(field) => (
                        <FormField
                            field={field}
                            label="City"
                            placeholder="e.g. Phoenix"
                            onChange={(value) => {
                                field.handleChange(value)
                                // Also update venue.city
                                form.setFieldValue('venue.city', value)
                            }}
                        />
                    )}
                </form.Field>
                <form.Field name="state">
                    {(field) => (
                        <FormField
                            field={field}
                            label="State"
                            placeholder="e.g. AZ"
                            onChange={(value) => {
                                field.handleChange(value)
                                // Also update venue.state
                                form.setFieldValue('venue.state', value)
                            }}
                        />
                    )}
                </form.Field>
                <form.Field name="description">{(field) => <FormField field={field} label="Description" />}</form.Field>
                <form.Subscribe
                    selector={(state) => [state.canSubmit, state.isSubmitting]}
                    children={([canSubmit, isSubmitting]) => {
                        const isDisabled = !canSubmit || isSubmitting || showMutation.isPending
                        return (
                            <Button type="submit" disabled={isDisabled} className="mt-4">
                                {isSubmitting || showMutation.isPending ? 'Submitting...' : 'Submit'}
                            </Button>
                        )
                    }}
                />
            </form>
        </div>
    )
}
