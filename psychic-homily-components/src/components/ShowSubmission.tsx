import { Button, Flex, Text, TextArea, TextField } from '@radix-ui/themes'
import { useState } from 'react'
import { bands, venues } from '../lib/show-data'
import { Combobox } from './ui/combobox'

// Types
interface Band {
    id: string
    name: string
    isCustom?: boolean
}

// Sub-components
function BandChip({ band, onRemove }: Readonly<{ band: Band; onRemove: () => void }>) {
    return (
        <div className={`flex items-center gap-1 px-2 py-1 rounded ${band.isCustom ? 'bg-blue-50' : 'bg-gray-100'}`}>
            <span>{band.name}</span>
            {band.isCustom && <span className="text-xs text-blue-500">(new)</span>}
            <button type="button" onClick={onRemove} className="ml-2 text-gray-500 hover:text-gray-700">
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
        <div className="flex items-center gap-1 px-2 py-1 rounded bg-gray-100">
            <span>{venueName}</span>
            {isNew && <span className="text-xs text-blue-500">(new)</span>}
            <button type="button" onClick={onRemove} className="ml-2 text-gray-500 hover:text-gray-700">
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
    // State
    const [selectedBands, setSelectedBands] = useState<Band[]>([])
    const [selectedVenue, setSelectedVenue] = useState<string>('')

    // Event handlers
    const handleBandSelect = (value: string) => {
        const existingBand = bands[value as keyof typeof bands]
        if (existingBand) {
            if (!selectedBands.find((b) => b.id === value)) {
                setSelectedBands([
                    ...selectedBands,
                    {
                        id: value,
                        name: existingBand.name,
                    },
                ])
            }
        } else {
            const newBandId = `new-${Date.now()}`
            setSelectedBands([
                ...selectedBands,
                {
                    id: newBandId,
                    name: value,
                    isCustom: true,
                },
            ])
        }
    }

    const handleBandRemove = (id: string) => {
        setSelectedBands(selectedBands.filter((b) => b.id !== id))
    }

    const handleVenueSelect = (value: string) => {
        setSelectedVenue(value)
    }

    return (
        <Flex direction="column" className="p-4 gap-4" width="400px">
            <Text size="5" weight="bold">
                Submit a Show
            </Text>
            <form className="flex flex-col gap-4">
                {/* Bands Selection */}
                <BandSelector
                    selectedBands={selectedBands}
                    onBandSelect={handleBandSelect}
                    onBandRemove={handleBandRemove}
                />

                {/* Venue Selection */}
                <VenueSelector
                    selectedVenue={selectedVenue}
                    onVenueSelect={handleVenueSelect}
                    onVenueRemove={() => setSelectedVenue('')}
                />

                {/* Form Fields */}
                <TextField.Root placeholder="Date" type="date" />
                <TextField.Root placeholder="Time" />
                <TextField.Root placeholder="Price" />
                <TextField.Root placeholder="City" />
                <TextField.Root placeholder="State" />
                <TextField.Root placeholder="Show Country" readOnly defaultValue="United States" />
                <TextArea placeholder="Notes" />
                <Button type="submit" variant="classic" color="gray" highContrast>
                    Submit
                </Button>
            </form>
        </Flex>
    )
}
