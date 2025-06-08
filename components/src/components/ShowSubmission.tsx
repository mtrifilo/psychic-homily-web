import { Button, Flex, Text, TextArea, TextField } from '@radix-ui/themes'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { bands, venues } from '../lib/show-data'
import { Combobox } from './ui/combobox'
import { showSchema, type ShowFormData } from '../lib/schemas/show-schema'

// Types
interface Band {
    id: string
    name: string
    isCustom?: boolean
}

// Sub-components
function BandChip({ band, onRemove }: Readonly<{ band: Band; onRemove: () => void }>) {
    return (
        <div
            className={`inline-flex items-center gap-1 px-1.5 py-0.5 text-sm rounded ${band.isCustom ? 'bg-blue-50' : 'bg-gray-100'}`}
        >
            <span>{band.name}</span>
            {band.isCustom && <span className="text-xs text-blue-500">(new)</span>}
            <button
                type="button"
                onClick={onRemove}
                className="ml-1 text-gray-400 hover:text-gray-700 text-lg leading-none"
            >
                ×
            </button>
        </div>
    )
}

function VenueChip({
    venueName,
    isNew,
    onRemove,
}: Readonly<{ venueName: string; isNew: boolean; onRemove: () => void }>) {
    return (
        <div className="inline-flex items-center gap-1 px-1.5 py-0.5 text-sm rounded bg-gray-100">
            <span>{venueName}</span>
            {isNew && <span className="text-xs text-blue-500">(new)</span>}
            <button
                type="button"
                onClick={onRemove}
                className="ml-1 text-gray-400 hover:text-gray-700 text-lg leading-none"
            >
                ×
            </button>
        </div>
    )
}

function BandSelector({
    selectedBands,
    onBandSelect,
    onBandRemove,
}: Readonly<{
    selectedBands: Band[]
    onBandSelect: (value: string) => void
    onBandRemove: (id: string) => void
}>) {
    const [bandInput, setBandInput] = useState('')

    // Convert bands data to options format
    const bandOptions = Object.entries(bands).map(([id, band]) => ({
        value: id,
        label: band.name,
    }))

    const handleBandSelect = (value: string) => {
        onBandSelect(value)
        setBandInput('')
    }

    return (
        <div className="flex flex-col gap-2">
            <Combobox
                options={bandOptions}
                value={bandInput}
                onValueChange={handleBandSelect}
                placeholder="Search or add new band..."
                className="w-full"
                allowNew
            />

            <div className="flex flex-wrap gap-2">
                {selectedBands.map((band) => (
                    <BandChip key={band.id} band={band} onRemove={() => onBandRemove(band.id)} />
                ))}
            </div>
        </div>
    )
}

function VenueSelector({
    selectedVenue,
    onVenueSelect,
    onVenueRemove,
}: Readonly<{
    selectedVenue: string
    onVenueSelect: (value: string) => void
    onVenueRemove: () => void
}>) {
    const [venueInput, setVenueInput] = useState('')

    // Convert venues data to options format
    const venueOptions = Object.entries(venues).map(([id, venue]) => ({
        value: id,
        label: `${venue.name} - ${venue.city}, ${venue.state}`,
    }))

    const handleVenueSelect = (value: string) => {
        onVenueSelect(value)
        setVenueInput('')
    }

    const venueName = venues[selectedVenue as keyof typeof venues]?.name || selectedVenue
    const isNewVenue = !venues[selectedVenue as keyof typeof venues]

    return (
        <div className="flex flex-col gap-2">
            <Combobox
                options={venueOptions}
                value={venueInput}
                onValueChange={handleVenueSelect}
                placeholder="Search or add new venue..."
                className="w-full"
                allowNew
            />
            {selectedVenue && <VenueChip venueName={venueName} isNew={isNewVenue} onRemove={onVenueRemove} />}
        </div>
    )
}

export function ShowSubmission() {
    const {
        register,
        handleSubmit,
        formState: { errors },
        setValue,
        watch,
    } = useForm<ShowFormData>({
        resolver: zodResolver(showSchema),
        defaultValues: {
            bands: [],
            state: 'AZ', // Default state
            notes: '',
        },
    })

    const selectedBands = watch('bands')
    const [selectedVenue, setSelectedVenue] = useState<string>('')

    const handleBandSelect = (value: string) => {
        const existingBand = bands[value as keyof typeof bands]
        const newBand = {
            id: existingBand ? value : `new-${Date.now()}`,
            name: existingBand ? existingBand.name : value.trim(),
            isCustom: !existingBand,
        }

        if (!selectedBands.find((b) => b.id === newBand.id)) {
            setValue('bands', [...selectedBands, newBand], {
                shouldValidate: true,
            })
        }
    }

    const handleBandRemove = (id: string) => {
        setValue(
            'bands',
            selectedBands.filter((b) => b.id !== id),
            { shouldValidate: true }
        )
    }

    const handleVenueSelect = (value: string) => {
        setSelectedVenue(value)
        const venue = venues[value as keyof typeof venues]
        if (venue) {
            setValue('venue', venue.name, { shouldValidate: true })
            setValue('city', venue.city, { shouldValidate: true })
            setValue('state', venue.state, { shouldValidate: true })
        } else {
            setValue('venue', value.trim(), { shouldValidate: true })
        }
    }

    const onSubmit = async (data: ShowFormData) => {
        try {
            // Validate and sanitize with Zod
            const validatedData = showSchema.parse(data)
            console.log('Sanitized form data:', validatedData)

            // TODO: Submit to your API
            // await submitShow(validatedData)
        } catch (error) {
            console.error('Validation error:', error)
        }
    }

    return (
        <Flex direction="column" className="p-4 gap-4" width="400px">
            <Text size="5" weight="bold">
                Submit a Show
            </Text>
            <form className="flex flex-col gap-4" onSubmit={handleSubmit(onSubmit)}>
                <BandSelector
                    selectedBands={selectedBands}
                    onBandSelect={handleBandSelect}
                    onBandRemove={handleBandRemove}
                />
                {errors.bands && (
                    <Text color="red" size="2">
                        {errors.bands.message}
                    </Text>
                )}

                <VenueSelector
                    selectedVenue={selectedVenue}
                    onVenueSelect={handleVenueSelect}
                    onVenueRemove={() => setSelectedVenue('')}
                />
                {errors.venue && (
                    <Text color="red" size="2">
                        {errors.venue.message}
                    </Text>
                )}

                <TextField.Root size="2" type="date" placeholder="Date" {...register('date')} />
                {errors.date && (
                    <Text color="red" size="2">
                        {errors.date.message}
                    </Text>
                )}

                <TextField.Root size="2" type="time" placeholder="Time" {...register('time')} />
                {errors.time && (
                    <Text color="red" size="2">
                        {errors.time.message}
                    </Text>
                )}

                <TextField.Root size="2" type="text" placeholder="Price" {...register('price')} />
                {errors.price && (
                    <Text color="red" size="2">
                        {errors.price.message}
                    </Text>
                )}

                <TextField.Root size="2" type="text" placeholder="City" {...register('city')} />
                {errors.city && (
                    <Text color="red" size="2">
                        {errors.city.message}
                    </Text>
                )}

                <TextField.Root size="2" type="text" placeholder="State" maxLength={2} {...register('state')} />
                {errors.state && (
                    <Text color="red" size="2">
                        {errors.state.message}
                    </Text>
                )}

                <TextArea placeholder="Notes" {...register('notes')} />
                {errors.notes && (
                    <Text color="red" size="2">
                        {errors.notes.message}
                    </Text>
                )}

                <Button type="submit" variant="classic" color="gray" highContrast>
                    Submit
                </Button>
            </form>
        </Flex>
    )
}
