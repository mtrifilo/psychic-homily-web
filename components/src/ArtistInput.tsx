import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { FieldInfo } from '@/components/ui/form-field'

interface Artist {
    id: number
    name: string
    location: string
}

interface ArtistInputProps {
    field: any
    onRemove?: () => void
    showRemoveButton?: boolean
}

// Mock artist data - replace with API call later
const mockArtists: Artist[] = [
    { id: 1, name: 'Radiohead', location: 'Oxford, UK' },
    { id: 2, name: 'The Beatles', location: 'Liverpool, UK' },
    { id: 3, name: 'Kendrick Lamar', location: 'Compton, CA' },
    { id: 4, name: 'Billie Eilish', location: 'Los Angeles, CA' },
    { id: 5, name: 'Arctic Monkeys', location: 'Sheffield, UK' },
    { id: 6, name: 'Taylor Swift', location: 'Nashville, TN' },
    { id: 7, name: 'The Strokes', location: 'New York, NY' },
    { id: 8, name: 'Glixen', location: 'Phoenix, AZ' },
    { id: 9, name: 'Desert Sounds', location: 'Tucson, AZ' },
]

export const ArtistInput = ({ field, onRemove, showRemoveButton }: ArtistInputProps) => {
    // Field-specific search state
    const [fieldSearchStates, setFieldSearchStates] = useState<Record<string, { open: boolean; value: string }>>({})

    // Helper functions for field-specific search state
    const getFieldSearchState = (fieldName: string) => {
        return fieldSearchStates[fieldName] || { open: false, value: '' }
    }

    const setFieldSearchState = (fieldName: string, updates: Partial<{ open: boolean; value: string }>) => {
        setFieldSearchStates((prev) => {
            const newState = {
                ...prev,
                [fieldName]: { ...getFieldSearchState(fieldName), ...updates },
            }
            return newState
        })
    }

    // Filter artists based on field-specific search
    const getFilteredArtists = (fieldName: string) => {
        const searchValue = getFieldSearchState(fieldName).value
        if (!searchValue) return []
        return mockArtists.filter((artist) => artist.name.toLowerCase().includes(searchValue.toLowerCase()))
    }

    // Handle artist selection/confirmation - auto-corrects casing and closes dropdown
    const handleArtistConfirm = (fieldName: string, field: any, artistName?: string) => {
        const finalName = artistName || field.state.value.trim()

        if (finalName) {
            // Always set the field value
            field.handleChange(finalName)

            // If there's an exact match, update with proper casing
            const exactMatch = mockArtists.find((artist) => artist.name.toLowerCase() === finalName.toLowerCase())
            if (exactMatch && exactMatch.name !== finalName) {
                field.handleChange(exactMatch.name)
            }
        }

        // Always close dropdown and clear search
        setFieldSearchState(fieldName, { open: false, value: '' })
    }

    return (
        <div className="flex flex-col space-y-2">
            <label htmlFor={field.name} className="text-sm font-medium text-gray-700 dark:text-gray-300">
                Artist
            </label>
            <div className="flex items-center gap-2">
                <div className="relative flex-1">
                    <Input
                        type="text"
                        className="w-full"
                        id={field.name}
                        name={field.name}
                        value={field.state.value}
                        onBlur={() => {
                            field.handleBlur()
                            // Only handle artist confirm if there's actually a value to process
                            if (field.state.value.trim()) {
                                handleArtistConfirm(field.name, field)
                            } else {
                                // Just close dropdown if field is empty
                                setFieldSearchState(field.name, {
                                    open: false,
                                    value: '',
                                })
                            }
                        }}
                        onChange={(e) => {
                            const value = e.target.value
                            field.handleChange(value)

                            // Update search state for this field
                            setFieldSearchState(field.name, {
                                value: value,
                                open: value.length > 0,
                            })
                        }}
                        onKeyDown={(e) => {
                            if (e.key === 'Enter') {
                                e.preventDefault()
                                if (field.state.value.trim()) {
                                    handleArtistConfirm(field.name, field)
                                } else {
                                    setFieldSearchState(field.name, {
                                        open: false,
                                        value: '',
                                    })
                                }
                            }
                            if (e.key === 'Escape') {
                                setFieldSearchState(field.name, {
                                    open: false,
                                })
                            }
                        }}
                        placeholder="Enter an artist name"
                        autoComplete="off"
                    />
                    {getFieldSearchState(field.name).open && (
                        <div className="absolute top-full left-0 right-0 z-50 mt-1 rounded-md border bg-popover p-0 text-popover-foreground shadow-md outline-none">
                            <div className="max-h-[300px] overflow-y-auto">
                                {(() => {
                                    const filteredArtists = getFilteredArtists(field.name)
                                    return (
                                        filteredArtists.length > 0 && (
                                            <div className="overflow-hidden p-1 text-foreground">
                                                <div className="px-2 py-1.5 text-xs font-medium text-muted-foreground">
                                                    Existing Artists
                                                </div>
                                                {filteredArtists.map((artist) => (
                                                    <button
                                                        type="button"
                                                        key={artist.id}
                                                        className="relative flex cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none hover:bg-accent hover:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50 w-full text-left"
                                                        onMouseDown={(e) => {
                                                            e.preventDefault() // Prevent input from losing focus
                                                            handleArtistConfirm(field.name, field, artist.name)
                                                        }}
                                                    >
                                                        <div className="flex items-center justify-between w-full">
                                                            <span>{artist.name}</span>
                                                            <span className="ml-auto text-xs tracking-widest text-muted-foreground">
                                                                {artist.location}
                                                            </span>
                                                        </div>
                                                    </button>
                                                ))}
                                            </div>
                                        )
                                    )
                                })()}

                                {(() => {
                                    const fieldState = getFieldSearchState(field.name)
                                    const filteredArtists = getFilteredArtists(field.name)
                                    return fieldState.value &&
                                        !filteredArtists.some(
                                            (artist) => artist.name.toLowerCase() === fieldState.value.toLowerCase()
                                        ) ? (
                                        <>
                                            <div className="pl-3 px-2 py-1.5 text-xs font-medium text-muted-foreground">
                                                Add New Artist
                                            </div>
                                            <button
                                                type="button"
                                                className="relative flex cursor-default select-none items-center rounded-sm px-2 py-1.5 text-sm outline-none hover:bg-accent hover:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50 w-full text-left"
                                                onClick={() => {
                                                    // Keep the current input value as the artist name
                                                    handleArtistConfirm(field.name, field)
                                                }}
                                            >
                                                "{field.state.value}"
                                            </button>
                                        </>
                                    ) : null
                                })()}
                            </div>
                        </div>
                    )}
                </div>
                {showRemoveButton && onRemove && (
                    <Button type="button" variant="outline" size="sm" onClick={onRemove}>
                        X
                    </Button>
                )}
            </div>
            <FieldInfo field={field} />
        </div>
    )
}
