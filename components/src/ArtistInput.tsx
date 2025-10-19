import { useState } from 'react'
import { useDebounce } from 'use-debounce'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { FieldInfo } from '@/components/ui/form-field'
import { getArtistLocation } from '@/lib/types/artist'
import { useArtistSearch } from '@/lib/hooks/useArtist'

interface ArtistInputProps {
    field: any
    onRemove?: () => void
    showRemoveButton?: boolean
}

export const ArtistInput = ({ field, onRemove, showRemoveButton }: ArtistInputProps) => {
    const [fieldSearchStates, setFieldSearchStates] = useState<Record<string, { open: boolean; value: string }>>({})

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

    const searchValue = getFieldSearchState(field.name).value

    const [debouncedSearchValue] = useDebounce(searchValue, 300)

    const { data: searchResults } = useArtistSearch({
        query: debouncedSearchValue,
    })

    const getFilteredArtists = () => {
        return searchResults?.artists || []
    }

    // Handle artist selection/confirmation - auto-corrects casing and closes dropdown
    const handleArtistConfirm = (fieldName: string, field: any, artistName?: string) => {
        const finalName = artistName || field.state.value.trim()

        if (finalName) {
            // Always set the field value
            field.handleChange(finalName)

            // If there's an exact match, update with proper casing
            const exactMatch = searchResults?.artists?.find(
                (artist) => artist.name.toLowerCase() === finalName.toLowerCase()
            )
            if (exactMatch && exactMatch.name !== finalName) {
                field.handleChange(exactMatch.name)
            }
        }

        // Always close dropdown and clear search
        setFieldSearchState(fieldName, { open: false, value: '' })
    }

    return (
        <div className="flex flex-col space-y-2 min-w-0">
            <label htmlFor={field.name} className="text-sm font-medium text-gray-700 dark:text-gray-300">
                Artist
            </label>
            <div className="flex items-center gap-2 min-w-0">
                <div className="relative flex-1 min-w-0">
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
                        <div className="absolute top-full left-0 w-full z-50 mt-1 rounded-md border bg-popover p-0 text-popover-foreground shadow-md outline-none">
                            <div className="max-h-[300px] overflow-y-auto">
                                {(() => {
                                    const filteredArtists = getFilteredArtists()
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
                                                        <div className="flex items-center justify-between w-full min-w-0 gap-2">
                                                            <span className="truncate">{artist.name}</span>
                                                            <span className="flex-shrink-0 text-xs tracking-widest text-muted-foreground">
                                                                {getArtistLocation(artist)}
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
                                    const filteredArtists = getFilteredArtists()
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
                                                <span className="truncate">"{field.state.value}"</span>
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
