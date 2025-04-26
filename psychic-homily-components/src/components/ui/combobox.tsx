import * as React from 'react'
import { useDeferredValue } from 'react'
import {
    Combobox as HeadlessCombobox,
    ComboboxInput,
    ComboboxButton,
    ComboboxOptions,
    ComboboxOption,
} from '@headlessui/react'
import { Check, ChevronsUpDown, Plus } from 'lucide-react'
import { cn } from '../../lib/utils'

const ITEM_HEIGHT = 36
const VISIBLE_ITEMS = 8
const BUFFER_ITEMS = 16

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

/* helper for the ✔ / ＋ icon */
function OptionIcon({ isNew, isSelected }: Readonly<{ isNew: boolean; isSelected: boolean; active: boolean }>) {
    if (!isNew && !isSelected) return null
    const Icon = isNew ? Plus : Check
    return (
        <span className="absolute inset-y-0 left-0 flex items-center pl-3 text-gray-600">
            <Icon className="h-4 w-4" aria-hidden="true" />
        </span>
    )
}

/* virtualised list */
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
    handleScroll: (e: React.UIEvent<HTMLDivElement>) => void
    visibleOptions: ComboboxOption[]
    offsetY: number
    isStale: boolean
    query: string
}>) {
    return (
        <div
            ref={containerRef}
            style={{ maxHeight: VISIBLE_ITEMS * ITEM_HEIGHT, overflowY: 'auto' }}
            onScroll={handleScroll}
            className="h-full"
        >
            <div
                style={{
                    height: totalHeight,
                    position: 'relative',
                    opacity: isStale ? 0.7 : 1,
                    transition: 'opacity 150ms ease',
                    paddingBottom: 4,
                }}
            >
                <div
                    style={{
                        position: 'absolute',
                        top: 0,
                        left: 0,
                        right: 0,
                        transform: `translateY(${offsetY}px)`,
                    }}
                >
                    {visibleOptions.map((option) => (
                        <ComboboxOption
                            key={option.value}
                            value={option}
                            className={({ active }) =>
                                cn(
                                    'relative cursor-default select-none py-2 pl-10 pr-4',
                                    active ? 'bg-gray-100' : 'text-gray-900'
                                )
                            }
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

/* ---------- main component ---------- */

export function Combobox({
    options,
    value,
    onValueChange,
    placeholder = 'Select option…',
    className,
    allowNew = false,
}: Readonly<ComboboxProps>) {
    const [query, setQuery] = React.useState('')
    const [scroll, setScroll] = React.useState(0)
    const containerRef = React.useRef<HTMLDivElement>(null)

    /* selected value */
    const selectedOption = React.useMemo(() => options.find((o) => o.value === value) ?? null, [options, value])

    /* filtering with React 18's deferred value */
    const dQuery = useDeferredValue(query)
    const dOptions = useDeferredValue(options)
    const listIsStale = query !== dQuery

    const filtered = React.useMemo(() => {
        return dOptions
            .filter((o) => o.label.toLowerCase().includes(dQuery.toLowerCase()))
            .sort((a, b) => a.label.localeCompare(b.label))
    }, [dOptions, dQuery])

    const displayed = React.useMemo(() => {
        if (!allowNew || !query || filtered.some((o) => o.label.toLowerCase() === query.toLowerCase())) {
            return filtered
        }
        return [...filtered, { value: query, label: `Add new: "${query}"` }]
    }, [filtered, query, allowNew])

    /* virtual-scroll window */
    const totalHeight = displayed.length * ITEM_HEIGHT
    const visible = Math.min(VISIBLE_ITEMS, displayed.length)
    const startIdx = Math.max(0, Math.floor(scroll / ITEM_HEIGHT) - BUFFER_ITEMS)
    const endIdx = Math.min(startIdx + visible + 2 * BUFFER_ITEMS, displayed.length)
    const slice = displayed.slice(startIdx, endIdx)
    const offsetY = startIdx * ITEM_HEIGHT

    /* scroll handler */
    const handleScroll = React.useCallback(
        (e: React.UIEvent<HTMLDivElement>) => setScroll(e.currentTarget.scrollTop),
        []
    )

    /* selection handler */
    const handleSelect = React.useCallback(
        (opt: ComboboxOption | null) => {
            if (opt) {
                onValueChange(opt.value)
                setQuery('')
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
            {({ open, activeOption }) => {
                const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
                    if (e.key === 'Enter' && !activeOption) {
                        if (allowNew && query && !options.some((o) => o.label.toLowerCase() === query.toLowerCase())) {
                            e.preventDefault()
                            onValueChange(query)
                            setQuery('')
                        }
                    }
                    if (e.key === 'Escape') setQuery('')
                }

                return (
                    <div className={cn('relative w-full', className)}>
                        {/* trigger */}
                        <div className="relative flex w-full items-center rounded-lg bg-white text-left">
                            <ComboboxInput
                                value={query}
                                onChange={(e) => setQuery(e.target.value)}
                                onKeyDown={handleKeyDown}
                                placeholder={selectedOption?.label || placeholder}
                                className="flex h-9 w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-500 focus:outline-none focus:ring-2 focus:ring-slate-400 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
                            />

                            <ComboboxButton className="flex items-center px-2 py-2 bg-gray-900 rounded-lg ml-2">
                                <ChevronsUpDown className="h-4 w-4 text-white" aria-hidden="true" />
                            </ComboboxButton>
                        </div>

                        {/* options panel */}
                        {open && (
                            <ComboboxOptions className="absolute z-10 mt-1 w-full overflow-hidden rounded-md bg-white py-1 shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none sm:text-sm">
                                <VirtualizedOptions
                                    containerRef={containerRef}
                                    totalHeight={totalHeight}
                                    handleScroll={handleScroll}
                                    visibleOptions={slice}
                                    offsetY={offsetY}
                                    isStale={listIsStale}
                                    query={query}
                                />
                            </ComboboxOptions>
                        )}
                    </div>
                )
            }}
        </HeadlessCombobox>
    )
}
