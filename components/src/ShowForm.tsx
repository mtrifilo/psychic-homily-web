import { useForm } from '@tanstack/react-form'
import { z } from 'zod'
import { Button } from './components/ui/button'
import { FormField, FieldInfo } from '@/components/ui/form-field'
import { Input } from './components/ui/input'

interface Artist {
    name: string
}

interface ArtistFieldProp {
    pushValue: (value: Artist) => void
}

interface ShowSubmission {
    artists: Artist[]
    venue: { name: string; id?: string }
    date: string
    time?: string
    cost?: string
    ages?: string
    city: string
    state: string
    description: string
}

const defaultShowSubmission: ShowSubmission = {
    artists: [{ name: '' }],
    venue: { name: '' },
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
})

const venueSchema = z.object({
    name: z.string().min(1, 'Venue name is required'),
    id: z.string().optional(),
})

const formSchema = z.object({
    artists: z.array(artistSchema).min(1, 'At least one artist is required'),
    venue: venueSchema,
    date: z.string().min(1, 'Date is required'),
    time: z.string().optional(),
    cost: z.string().optional(),
    ages: z.string().optional(),
    city: z.string().min(1, 'City is required'),
    state: z.string().min(1, 'State is required'),
    description: z.string(),
})

export const ShowForm = () => {
    const form = useForm({
        defaultValues: defaultShowSubmission,
        onSubmit: async ({ value }) => {
            console.log('submitted:', value)
        },
        validators: {
            onSubmit: formSchema,
        },
    })

    const handleAddArtist = (artistsField: ArtistFieldProp) => {
        artistsField.pushValue({ name: '' })
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
                                            children={(field) => {
                                                return (
                                                    <div className="flex flex-col space-y-2">
                                                        <label
                                                            htmlFor={field.name}
                                                            className="text-sm font-medium text-gray-700 dark:text-gray-300"
                                                        >
                                                            Artist
                                                        </label>
                                                        <div className="flex items-center gap-2">
                                                            <Input
                                                                type="text"
                                                                className="flex-1"
                                                                id={field.name}
                                                                name={field.name}
                                                                value={field.state.value}
                                                                onBlur={field.handleBlur}
                                                                onChange={(e) => {
                                                                    field.handleChange(e.target.value)
                                                                }}
                                                                onKeyDown={(e) => {
                                                                    if (e.key === 'Enter') {
                                                                        e.preventDefault()
                                                                        handleAddArtist(artistsField)
                                                                    }
                                                                }}
                                                            />
                                                            {artistsField.state.value.length > 1 && (
                                                                <Button
                                                                    type="button"
                                                                    variant="outline"
                                                                    size="sm"
                                                                    onClick={() => {
                                                                        artistsField.removeValue(i)
                                                                    }}
                                                                >
                                                                    X
                                                                </Button>
                                                            )}
                                                        </div>
                                                        <FieldInfo field={field} />
                                                    </div>
                                                )
                                            }}
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
                <form.Field name="venue.name">
                    {(field) => <FormField field={field} label="Venue" onEnterPress={form.handleSubmit} />}
                </form.Field>
                <form.Field name="date">
                    {(field) => <FormField field={field} label="Date" type="date" onEnterPress={form.handleSubmit} />}
                </form.Field>
                <form.Field name="time">
                    {(field) => <FormField field={field} label="Time" type="time" onEnterPress={form.handleSubmit} />}
                </form.Field>
                <form.Field name="cost">
                    {(field) => (
                        <FormField
                            field={field}
                            label="Cost"
                            placeholder="e.g. $20, Free"
                            onEnterPress={form.handleSubmit}
                        />
                    )}
                </form.Field>
                <form.Field name="ages">
                    {(field) => (
                        <FormField
                            field={field}
                            label="Ages"
                            placeholder="e.g. 21+, All Ages"
                            onEnterPress={form.handleSubmit}
                        />
                    )}
                </form.Field>
                <form.Field name="city">
                    {(field) => (
                        <FormField
                            field={field}
                            label="City"
                            placeholder="e.g. Phoenix"
                            onEnterPress={form.handleSubmit}
                        />
                    )}
                </form.Field>
                <form.Field name="state">
                    {(field) => (
                        <FormField field={field} label="State" placeholder="e.g. AZ" onEnterPress={form.handleSubmit} />
                    )}
                </form.Field>
                <form.Field name="description">
                    {(field) => <FormField field={field} label="Description" onEnterPress={form.handleSubmit} />}
                </form.Field>
                <form.Subscribe
                    selector={(state) => [state.canSubmit, state.isSubmitting]}
                    children={([canSubmit, isSubmitting]) => (
                        <Button type="submit" disabled={!canSubmit || isSubmitting} className="mt-4">
                            {isSubmitting ? 'Submitting...' : 'Submit'}
                        </Button>
                    )}
                />
            </form>
        </div>
    )
}
