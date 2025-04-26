import * as React from 'react'
import { useDeferredValue, memo } from 'react'
import {
    Combobox as HeadlessCombobox,
    ComboboxInput,
    ComboboxButton,
    ComboboxOptions,
    ComboboxOption,
} from '@headlessui/react'
import { Check, ChevronsUpDown, Plus } from 'lucide-react'
import { cn } from '../../lib/utils'

const ITEM_HEIGHT = 36 // Height of each option in pixels
const VISIBLE_ITEMS = 8 // Number of items to show at once
const BUFFER_ITEMS = 16 // Number of extra items to render above and below

export interface ComboboxOption {
    value: string
    label: string
}

export interface ComboboxProps {
    options: readonly ComboboxOption[]
    value: string
    onValueChange: (value: string) => void
    placeholder?: string
    className?: string
    allowNew?: boolean
}

function ComboboxTrigger({
    query,
    setQuery,
    setOpen,
    handleKeyDown,
    selectedOption,
    placeholder,
}: Readonly<{
    query: string
    setQuery: (value: string) => void
    setOpen: (value: boolean) => void
    handleKeyDown: (event: React.KeyboardEvent<HTMLInputElement>) => void
    selectedOption: ComboboxOption | null
    placeholder: string
}>) {
    return (
        <div className="relative w-full cursor-default overflow-hidden rounded-lg bg-white text-left border border-gray-200 focus-within:border-gray-400">
            <ComboboxInput
                className="w-full border-none py-2 pl-3 pr-10 text-sm leading-5 text-gray-900 focus:ring-0"
                onChange={(event: React.ChangeEvent<HTMLInputElement>) => {
                    setQuery(event.target.value)
                }}
                onKeyDown={handleKeyDown}
                value={query}
                placeholder={selectedOption?.label || placeholder}
            />
            <ComboboxButton
                onMouseDown={() => setOpen(true)}
                className="absolute inset-y-0 right-0 flex items-center pr-2"
            >
                <ChevronsUpDown className="h-4 w-4 text-gray-400" aria-hidden="true" />
            </ComboboxButton>
        </div>
    )
}

function VirtualizedOptions({
    containerRef,
    totalHeight,
    handleScroll,
    visibleOptions,
    offsetY,
    isStale,
    query,
}: Readonly<{
    containerRef: React.RefObject<HTMLDivElement | null>
    totalHeight: number
    handleScroll: (event: React.UIEvent<HTMLDivElement>) => void
    visibleOptions: ComboboxOption[]
    offsetY: number
    isStale: boolean
    query: string
}>) {
    return (
        <div
            ref={containerRef}
            style={{
                maxHeight: VISIBLE_ITEMS * ITEM_HEIGHT,
                overflowY: 'auto',
            }}
            onScroll={handleScroll}
            className="h-full"
        >
            <div
                style={{
                    height: totalHeight,
                    position: 'relative',
                    opacity: isStale ? 0.7 : 1,
                    transition: 'opacity 150ms ease',
                    paddingBottom: '4px',
                }}
            >
                <div style={{ position: 'absolute', top: 0, left: 0, right: 0, transform: `translateY(${offsetY}px)` }}>
                    {visibleOptions.map((option) => (
                        <ComboboxOption
                            key={option.value}
                            className={({ active }) =>
                                cn(
                                    'relative cursor-default select-none py-2 pl-10 pr-4',
                                    active ? 'bg-gray-100' : 'text-gray-900'
                                )
                            }
                            value={option}
                        >
                            {({ selected, active }) => (
                                <>
                                    <span className={cn('block truncate', selected ? 'font-medium' : 'font-normal')}>
                                        {option.label}
                                    </span>
                                    <OptionIcon isNew={option.value === query} isSelected={selected} active={active} />
                                </>
                            )}
                        </ComboboxOption>
                    ))}
                </div>
            </div>
        </div>
    )
}

function OptionIcon({ isNew, isSelected, active }: Readonly<{ isNew: boolean; isSelected: boolean; active: boolean }>) {
    if (!isNew && !isSelected) return null
    const Icon = isNew ? Plus : Check
    return (
        <span
            className={cn(
                'absolute inset-y-0 left-0 flex items-center pl-3',
                active ? 'text-gray-600' : 'text-gray-600'
            )}
        >
            <Icon className="h-4 w-4" aria-hidden="true" />
        </span>
    )
}

export function Combobox({
    options,
    value,
    onValueChange,
    placeholder = 'Select option...',
    className,
    allowNew = false,
}: Readonly<ComboboxProps>) {
    const [query, setQuery] = React.useState('')
    const [isOpen, setIsOpen] = React.useState(false)
    const containerRef = React.useRef<HTMLDivElement>(null)
    const [scrollPosition, setScrollPosition] = React.useState(0)

    const selectedOption = React.useMemo(
        () => options.find((option) => option.value === value) ?? null,
        [options, value]
    )

    const deferredQuery = useDeferredValue(query)
    const deferredOptions = useDeferredValue(options)
    const isStale = query !== deferredQuery

    const filteredOptions = React.useMemo(() => {
        const filtered = deferredOptions
            .filter((option) => option.label.toLowerCase().includes(deferredQuery.toLowerCase()))
            .sort((a, b) => a.label.localeCompare(b.label))
        return filtered
    }, [deferredOptions, deferredQuery])

    const displayedOptions = React.useMemo(() => {
        if (!allowNew || !query || filteredOptions.some((opt) => opt.label.toLowerCase() === query.toLowerCase())) {
            return filteredOptions
        }
        return [...filteredOptions, { value: query, label: `Add new: "${query}"` }]
    }, [filteredOptions, query, allowNew])

    const totalHeight = displayedOptions.length * ITEM_HEIGHT
    const visibleItems = Math.min(VISIBLE_ITEMS, displayedOptions.length)
    const startIndex = Math.max(0, Math.floor(scrollPosition / ITEM_HEIGHT) - BUFFER_ITEMS)
    const endIndex = Math.min(startIndex + visibleItems + 2 * BUFFER_ITEMS, displayedOptions.length)
    const visibleOptions = displayedOptions.slice(startIndex, endIndex)
    const offsetY = startIndex * ITEM_HEIGHT

    const handleScroll = React.useCallback((event: React.UIEvent<HTMLDivElement>) => {
        setScrollPosition(event.currentTarget.scrollTop)
    }, [])

    const handleSelect = React.useCallback(
        (option: ComboboxOption | null) => {
            if (option) {
                onValueChange(option.value)
                setQuery('')
                setIsOpen(false)
            }
        },
        [onValueChange]
    )

    return (
        <HeadlessCombobox
            value={selectedOption}
            onChange={handleSelect}
            nullable
            by={(a, b) => (a?.value ?? '') === (b?.value ?? '')}
        >
            {({ activeOption }) => (
                <div className={cn('relative w-full', className)}>
                    <ComboboxTrigger
                        query={query}
                        setQuery={(value) => {
                            setQuery(value)
                            setIsOpen(true)
                        }}
                        setOpen={setIsOpen}
                        handleKeyDown={(event: React.KeyboardEvent<HTMLInputElement>) => {
                            if (event.key === 'Enter') {
                                event.preventDefault()
                                if (activeOption) {
                                    handleSelect(activeOption)
                                } else if (
                                    allowNew &&
                                    query &&
                                    !options.some((opt) => opt.label.toLowerCase() === query.toLowerCase())
                                ) {
                                    onValueChange(query)
                                    setQuery('')
                                    setIsOpen(false)
                                }
                            } else if (event.key === 'Escape') {
                                setQuery('')
                                setIsOpen(false)
                            }
                        }}
                        selectedOption={selectedOption}
                        placeholder={placeholder}
                    />

                    {isOpen && (
                        <ComboboxOptions className="absolute z-10 mt-1 w-full overflow-hidden rounded-md bg-white py-1 shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none sm:text-sm">
                            <VirtualizedOptions
                                containerRef={containerRef}
                                totalHeight={totalHeight}
                                handleScroll={handleScroll}
                                visibleOptions={visibleOptions}
                                offsetY={offsetY}
                                isStale={isStale}
                                query={query}
                            />
                        </ComboboxOptions>
                    )}
                </div>
            )}
        </HeadlessCombobox>
    )
}
