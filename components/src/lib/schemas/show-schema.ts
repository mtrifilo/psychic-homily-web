import { z } from 'zod'
import DOMPurify from 'isomorphic-dompurify'

// Regex patterns
const TIME_PATTERN = /^([01]?[0-9]|2[0-3]):[0-5][0-9]$/
const PRICE_PATTERN = /^\d+(\.\d{2})?$/
const STATE_PATTERN = /^[A-Z]{2}$/

// Sanitize text using DOMPurify
const sanitizeString = (str: string) => {
    // Use DOMPurify's strict mode with additional configuration
    const clean = DOMPurify.sanitize(str, {
        ALLOWED_TAGS: [], // No HTML tags allowed
        ALLOWED_ATTR: [], // No attributes allowed
        KEEP_CONTENT: true, // Keep the text content
        SANITIZE_DOM: true, // Sanitize DOM
    }).trim()

    // Normalize whitespace after sanitization
    return clean.replace(/\s+/g, ' ').trim()
}

export const showSchema = z.object({
    bands: z
        .array(
            z.object({
                id: z.string().min(1),
                name: z
                    .string()
                    .min(1, 'Band name is required')
                    .max(100, 'Band name is too long')
                    .transform(sanitizeString),
                isCustom: z.boolean().optional(),
            })
        )
        .min(1, 'At least one band is required'),

    venue: z.string().min(1, 'Venue is required').max(100, 'Venue name is too long').transform(sanitizeString),

    date: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, 'Invalid date format. Use YYYY-MM-DD'),

    time: z
        .string()
        .regex(TIME_PATTERN, 'Invalid time format. Use HH:MM (24-hour format)')
        .transform((str) => str.trim()),

    price: z
        .string()
        .regex(PRICE_PATTERN, 'Invalid price format. Use numbers only (e.g., 15 or 15.00)')
        .transform((str) => str.replace(/[^\d.]/g, '')), // Remove non-numeric chars except decimal

    city: z.string().min(1, 'City is required').max(100, 'City name is too long').transform(sanitizeString),

    state: z
        .string()
        .regex(STATE_PATTERN, 'State must be a 2-letter code (e.g., AZ)')
        .transform((str) => str.toUpperCase()),

    notes: z.string().max(1000, 'Notes are too long').transform(sanitizeString).optional(),
})

export type ShowFormData = z.infer<typeof showSchema>
