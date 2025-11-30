export function combineDateTimeToUTC(dateString: string, timeString: string): string {
    // Create a date object from the date string (assumes local timezone)
    const date = new Date(dateString)

    // Parse the time string (HH:MM format)
    const [hours, minutes] = timeString.split(':').map(Number)

    // Set the time on the date object
    date.setHours(hours, minutes, 0, 0)

    // Convert to UTC and return as ISO string
    return date.toISOString()
}
